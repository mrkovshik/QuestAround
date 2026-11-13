package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeleteRestorePurge(t *testing.T) {
	ts := newTestServer(t)
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, e2eYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID, "title": "Тестовый прогон"}).json(t, &run)
	admin := "/admin/v1/runs/" + run.ID
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)
	ts.must(200, "POST", admin+"/start", adminToken, nil)
	ts.must(201, "POST", "/api/v1/uploads", a.Token, photoForm(t, nil, pngBytes(t)))
	if _, err := os.Stat(filepath.Join(ts.mediaDir, run.ID)); err != nil {
		t.Fatalf("фото не сохранилось: %v", err)
	}

	// идущую игру удалить нельзя
	if c := ts.must(409, "DELETE", admin, adminToken, nil).errCode(t); c != "run_in_progress" {
		t.Fatal(c)
	}
	ts.must(200, "POST", admin+"/finish", adminToken, nil)
	ts.must(204, "DELETE", admin, adminToken, nil)

	var runs []runDTO
	ts.must(200, "GET", "/admin/v1/runs", adminToken, nil).json(t, &runs)
	if len(runs) != 0 {
		t.Fatalf("удалённая игра в списке: %+v", runs)
	}
	ts.must(401, "GET", "/api/v1/state", a.Token, nil)
	ts.must(404, "GET", "/api/v1/games/"+run.PublicCode, "", nil)
	ts.must(404, "GET", admin+"/dashboard", adminToken, nil)
	// стереть навсегда можно только из корзины — эта уже там; проверим на живой ниже

	var trash []trashDTO
	ts.must(200, "GET", "/admin/v1/trash", adminToken, nil).json(t, &trash)
	if len(trash) != 1 || trash[0].Title != "Тестовый прогон" || trash[0].Teams != 1 || !trash[0].Started ||
		trash[0].PurgeAt.Sub(trash[0].DeletedAt) != 30*24*time.Hour {
		t.Fatalf("корзина: %+v", trash)
	}

	// восстановление: всё возвращается, игрок снова в игре
	var back runDTO
	ts.must(200, "POST", admin+"/restore", adminToken, nil).json(t, &back)
	if back.Status != "finished" || back.Teams != 1 {
		t.Fatalf("восстановленная: %+v", back)
	}
	ts.must(200, "GET", "/api/v1/state", a.Token, nil)
	if c := ts.must(409, "DELETE", "/admin/v1/trash/"+run.ID, adminToken, nil).errCode(t); c != "not_in_trash" {
		t.Fatal(c)
	}

	// автоочистка: срок не вышел — игра лежит; вышел — стёрта вместе с фото
	ts.must(204, "DELETE", admin, adminToken, nil)
	ts.api.PurgeExpired(context.Background())
	ts.must(200, "GET", "/admin/v1/trash", adminToken, nil).json(t, &trash)
	if len(trash) != 1 {
		t.Fatalf("стёрлась раньше срока: %+v", trash)
	}
	ts.api.cfg.TrashRetention = time.Nanosecond
	ts.api.PurgeExpired(context.Background())
	ts.must(200, "GET", "/admin/v1/trash", adminToken, nil).json(t, &trash)
	if len(trash) != 0 {
		t.Fatalf("не стёрлась: %+v", trash)
	}
	if _, err := os.Stat(filepath.Join(ts.mediaDir, run.ID)); !os.IsNotExist(err) {
		t.Errorf("фото остались: %v", err)
	}
	ts.must(404, "POST", admin+"/restore", adminToken, nil)

	// ручное «стереть навсегда»
	var run2 runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run2)
	ts.must(204, "DELETE", "/admin/v1/runs/"+run2.ID, adminToken, nil) // не начатую — можно сразу
	ts.must(204, "DELETE", "/admin/v1/trash/"+run2.ID, adminToken, nil)
	ts.must(404, "GET", "/admin/v1/runs/"+run2.ID, adminToken, nil)
}
