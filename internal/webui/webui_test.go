package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerMethods(t *testing.T) {
	h := Handler()
	for _, m := range []string{http.MethodPost, http.MethodPut} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(m, "/g/X", nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: %d", m, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/g/X/join/Y", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("GET SPA-маршрута: %d", rec.Code)
	}
}
