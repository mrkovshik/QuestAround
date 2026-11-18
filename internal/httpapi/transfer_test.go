package httpapi

import (
	"strings"
	"testing"

	"questaround/internal/core"
)

// Код переноса в VK: приложение считает его из токена, ведущий видит его в дашборде.
func TestTransferCodes(t *testing.T) {
	ts := newTestServer(t)
	ts.api.cfg.VKURL = "https://vk.com/write-243408916"
	var sv scenarioVersionDTO
	ts.must(201, "POST", "/admin/v1/scenarios", adminToken, slivYAML).json(t, &sv)
	var run runDTO
	ts.must(201, "POST", "/admin/v1/runs", adminToken, map[string]string{"scenario_version_id": sv.ID}).json(t, &run)
	var a credentialsDTO
	ts.must(201, "POST", "/api/v1/games/"+run.PublicCode+"/teams", "", map[string]string{"team_name": "Альфа", "captain_name": "Аня"}).json(t, &a)

	var st playerStateDTO
	ts.must(200, "GET", "/api/v1/state", a.Token, nil).json(t, &st)
	if st.VKURL != "https://vk.com/write-243408916" {
		t.Fatalf("vk_url = %q", st.VKURL)
	}

	_, _, secret, _ := parseToken(a.Token)
	want := core.TransferCode(hashSecret(secret))
	if !strings.HasPrefix(want, "QA-") {
		t.Fatalf("код %q", want)
	}
	var d dashboardDTO
	ts.must(200, "GET", "/admin/v1/runs/"+run.ID+"/dashboard", adminToken, nil).json(t, &d)
	if got := d.Teams[0].TransferCodes[a.PlayerID]; got != want {
		t.Fatalf("код в дашборде %q, ожидали %q", got, want)
	}

	// Бот выключен — кодов в дашборде нет.
	ts.api.cfg.VKURL = ""
	d = dashboardDTO{}
	ts.must(200, "GET", "/admin/v1/runs/"+run.ID+"/dashboard", adminToken, nil).json(t, &d)
	if d.Teams[0].TransferCodes != nil {
		t.Fatalf("коды при выключенном боте: %v", d.Teams[0].TransferCodes)
	}
}
