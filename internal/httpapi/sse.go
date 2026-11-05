package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"questline/internal/core"
	"questline/internal/runner"
)

// SSE: сервер только сообщает «состояние изменилось» (event: state_changed,
// data: seq), клиент сам перезапрашивает /state или /dashboard. Потерянное
// сообщение не страшно — следующее принесёт свежий seq; при переподключении
// сразу приходит текущий seq.

func (s *Server) playerStream(w http.ResponseWriter, r *http.Request, p *player) {
	s.stream(w, r, p.Runner, p.TeamID)
}

func (s *Server) adminStream(w http.ResponseWriter, r *http.Request) {
	rn, err := s.runner(r)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	s.stream(w, r, rn, "")
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request, rn *runner.Runner, teamID string) {
	rc := http.NewResponseController(w)
	// Стрим живёт долго — снимаем дедлайн записи сервера.
	_ = rc.SetWriteDeadline(time.Time{})

	updates, cancel := rn.Subscribe(teamID)
	defer cancel()

	var seq int64
	if err := rn.Read(r.Context(), func(g *core.Game) { seq = g.Seq() }); err != nil {
		s.writeError(w, r, err)
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // nginx: не буферизовать
	w.WriteHeader(http.StatusOK)

	send := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("retry: 3000\n\nevent: state_changed\ndata: %d\n\n", seq) {
		return
	}

	keepAlive := time.NewTicker(s.cfg.SSEKeepAlive)
	defer keepAlive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case v, ok := <-updates:
			if !ok {
				return // игра остановлена — клиент переподключится
			}
			if !send("event: state_changed\ndata: %d\n\n", v) {
				return
			}
		case <-keepAlive.C:
			if !send(": ping\n\n") {
				return
			}
		}
	}
}
