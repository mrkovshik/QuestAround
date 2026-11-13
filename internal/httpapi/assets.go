package httpapi

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"questaround/internal/core"
	"questaround/internal/scenario"
)

// --- раздача файлов ---

func (s *Server) publicMedia(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	f, ctype, err := s.cfg.Media.Open(r.Context(), key)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", ctype)
	if strings.HasPrefix(key, "assets/") {
		// имя — хеш содержимого: файл по этому адресу не меняется никогда
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "private, max-age=604800")
	}
	http.ServeContent(w, r, "", time.Time{}, f)
}

// resolveMedia превращает список из сценария (имена и ссылки) в адреса для <img>.
// Имена, которых нет в библиотеке, пропускаются.
func (s *Server) resolveMedia(list []string) []string {
	if len(list) == 0 {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, m := range list {
		if scenario.IsMediaURL(m) {
			out = append(out, m)
		} else if key, ok := s.cfg.Media.AssetKey(m); ok {
			out = append(out, mediaURL(key))
		}
	}
	return out
}

func (s *Server) resolveLevels(levels []levelDTO) {
	for i := range levels {
		l := &levels[i]
		l.Media = s.resolveMedia(l.Media)
		for j := range l.Hints {
			l.Hints[j].Media = s.resolveMedia(l.Hints[j].Media)
		}
		for j := range l.Bonuses {
			l.Bonuses[j].Media = s.resolveMedia(l.Bonuses[j].Media)
		}
	}
}

// checkMediaKey — к сообщению можно приложить только файл, загруженный в эту игру.
func checkMediaKey(runID, key string) error {
	if key != "" && !strings.HasPrefix(key, runID+"/") {
		return newErr(http.StatusBadRequest, "bad_media", "файл не найден — загрузите фото ещё раз")
	}
	return nil
}

// --- загрузка фото к сообщениям ---

type uploadDTO struct {
	Key string `json:"key"`
	URL string `json:"url"`
}

// saveUpload читает первое поле-файл формы и сохраняет его в медиа игры.
func (s *Server) saveUpload(w http.ResponseWriter, r *http.Request, runID string) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoSize)
	mr, err := r.MultipartReader()
	if err != nil {
		s.writeError(w, r, newErr(http.StatusBadRequest, "bad_multipart", "ожидалась форма multipart/form-data"))
		return
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if part.FileName() == "" {
			continue
		}
		key, err := s.cfg.Media.Save(r.Context(), runID, part)
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, uploadDTO{Key: key, URL: mediaURL(key)})
		return
	}
	s.writeError(w, r, newErr(http.StatusBadRequest, "no_file", "в форме нет файла"))
}

func (s *Server) playerUpload(w http.ResponseWriter, r *http.Request, p *player) {
	s.saveUpload(w, r, p.RunID)
}

func (s *Server) hostUpload(w http.ResponseWriter, r *http.Request) {
	run, err := s.cfg.Store.GetRun(r.Context(), r.PathValue("run"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.saveUpload(w, r, run.ID)
}

// --- библиотека картинок ---

type assetDTO struct {
	Asset
	URL string `json:"url"`
}

func (s *Server) listAssets(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Media.Assets(r.Context())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	out := make([]assetDTO, 0, len(list))
	for _, a := range list {
		out = append(out, assetDTO{a, mediaURL(a.Key)})
	}
	writeJSON(w, http.StatusOK, out)
}

// uploadAssets: multipart, любое число файлов; имя в библиотеке — имя файла.
func (s *Server) uploadAssets(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 100<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		s.writeError(w, r, newErr(http.StatusBadRequest, "bad_multipart", "ожидалась форма multipart/form-data"))
		return
	}
	out := []assetDTO{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		if part.FileName() == "" {
			continue
		}
		name := filepath.Base(strings.ReplaceAll(part.FileName(), `\`, "/"))
		if !scenario.IsMediaName(name) {
			s.writeError(w, r, newErr(http.StatusBadRequest, "bad_name",
				fmt.Sprintf("«%s»: нужен файл .jpg, .png, .webp или .gif без символов / \\ ? # :", name)))
			return
		}
		a, err := s.cfg.Media.PutAsset(r.Context(), name, part)
		if err != nil {
			if errors.Is(err, errNotImage) {
				err = newErr(http.StatusUnsupportedMediaType, "not_image", fmt.Sprintf("«%s» — не картинка", name))
			}
			s.writeError(w, r, err)
			return
		}
		out = append(out, assetDTO{a, mediaURL(a.Key)})
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) deleteAsset(w http.ResponseWriter, r *http.Request) {
	if err := s.cfg.Media.DeleteAsset(r.Context(), r.PathValue("name")); err != nil {
		s.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- галерея фото команд ---

type photoDTO struct {
	Seq        int64     `json:"seq"`
	At         time.Time `json:"at"`
	TeamID     string    `json:"team_id,omitempty"`
	TeamName   string    `json:"team_name,omitempty"`
	PlayerName string    `json:"player_name,omitempty"`
	LevelID    string    `json:"level_id"`
	Title      string    `json:"title"` // «Уровень» или «Уровень · бонус»
	Status     string    `json:"status"`
	Comment    string    `json:"comment,omitempty"`
	URL        string    `json:"url"`
	Key        string    `json:"-"`
}

func photoView(g *core.Game, ph *core.Photo) photoDTO {
	d := photoDTO{Seq: ph.Seq, At: ph.At, TeamID: ph.TeamID, LevelID: ph.LevelID,
		Status: string(ph.Status), Comment: ph.Comment, URL: mediaURL(ph.MediaKey), Key: ph.MediaKey}
	if t := g.Teams[ph.TeamID]; t != nil {
		d.TeamName = t.Name
	}
	if p := g.Players[ph.PlayerID]; p != nil {
		d.PlayerName = p.Name
	}
	d.Title = ph.LevelID
	for i := range g.Scenario.Levels {
		l := &g.Scenario.Levels[i]
		if l.ID != ph.LevelID {
			continue
		}
		d.Title = l.Title
		for _, b := range l.Bonuses {
			if b.ID == ph.BonusID {
				d.Title += " · бонус «" + b.Title + "»"
			}
		}
	}
	return d
}

// teamPhotos — фото одной команды (для экрана игрока), новые сверху.
func teamPhotos(g *core.Game, teamID string) []photoDTO {
	out := []photoDTO{}
	for i := len(g.Photos) - 1; i >= 0; i-- {
		if ph := g.Photos[i]; ph.TeamID == teamID {
			d := photoView(g, ph)
			d.TeamID, d.TeamName = "", ""
			out = append(out, d)
		}
	}
	return out
}

func (s *Server) gamePhotos(r *http.Request) (string, []photoDTO, error) {
	rn, err := s.runner(r)
	if err != nil {
		return "", nil, err
	}
	var title string
	out := []photoDTO{}
	err = rn.Read(r.Context(), func(g *core.Game) {
		title = g.Scenario.Title
		for _, ph := range g.Photos {
			out = append(out, photoView(g, ph))
		}
	})
	return title, out, err
}

func (s *Server) photos(w http.ResponseWriter, r *http.Request) {
	_, list, err := s.gamePhotos(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// photosZip — архив: папка на команду, в имени файла время, уровень и статус.
// ?all=1 — вместе с отклонёнными.
func (s *Server) photosZip(w http.ResponseWriter, r *http.Request) {
	title := s.runTitle(r.Context(), r.PathValue("run"))
	_, list, err := s.gamePhotos(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	all := r.URL.Query().Get("all") == "1"
	// архив может качаться долго по мобильной сети — снимаем общий дедлайн записи
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="photos.zip"; filename*=UTF-8''%s.zip`,
		urlPathEscape(safeName(title, "фото"))))
	zw := zip.NewWriter(w)
	used := map[string]int{}
	for _, p := range list {
		if p.Status == string(core.PhotoRejected) && !all {
			continue
		}
		f, _, err := s.cfg.Media.Open(r.Context(), p.Key)
		if err != nil {
			continue // файл потерян — пропускаем, архив важнее
		}
		name := fmt.Sprintf("%s/%s %s", safeName(p.TeamName, "команда"), p.At.Local().Format("15-04-05"), safeName(p.Title, p.LevelID))
		if p.Status != string(core.PhotoAccepted) {
			name += " (" + map[string]string{"pending": "не проверено", "rejected": "отклонено"}[p.Status] + ")"
		}
		if used[name]++; used[name] > 1 {
			name += fmt.Sprintf(" %d", used[name])
		}
		hw, err := zw.CreateHeader(&zip.FileHeader{Name: name + path.Ext(p.Key), Method: zip.Store, Modified: p.At})
		if err == nil {
			_, err = io.Copy(hw, f)
		}
		f.Close()
		if err != nil {
			s.log.Warn("архив фото оборван", "err", err)
			return
		}
	}
	zw.Close()
}

// safeName — строка, пригодная для имени файла в архиве.
func safeName(s, fallback string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	if r := []rune(s); len(r) > 60 {
		s = string(r[:60])
	}
	if s == "" {
		return fallback
	}
	return s
}

func urlPathEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
