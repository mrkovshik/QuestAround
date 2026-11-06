package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"

	"questaround/internal/core"
	"questaround/internal/runner"
)

// Токен игрока: "<runID>.<playerID>.<secret>". runID и playerID позволяют
// найти игру и игрока без отдельной таблицы, в журнале хранится только
// sha256(secret).

func newSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashSecret(secret string) string {
	h := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(h[:])
}

func makeToken(runID, playerID, secret string) string {
	return runID + "." + playerID + "." + secret
}

func parseToken(tok string) (runID, playerID, secret string, ok bool) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

const (
	idAlphabet   = "abcdefghijkmnpqrstuvwxyz23456789"
	codeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

func randString(n int, alphabet string) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

func newID() string         { return randString(12, idAlphabet) }
func newInviteCode() string { return randString(8, codeAlphabet) }

// player — авторизованный игрок в контексте запроса.
type player struct {
	RunID    string
	PlayerID string
	TeamID   string
	Runner   *runner.Runner
}

type playerHandler func(w http.ResponseWriter, r *http.Request, p *player)

type ctxKey struct{}

func (s *Server) playerAuth(h playerHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := s.authenticate(r.Context(), bearer(r))
		if err != nil {
			s.writeError(w, r, err)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, p)), p)
	})
}

func (s *Server) authenticate(ctx context.Context, tok string) (*player, error) {
	runID, playerID, secret, ok := parseToken(tok)
	if !ok {
		return nil, errUnauthorized
	}
	rn, err := s.cfg.Runners.Get(ctx, runID)
	if err != nil {
		return nil, errUnauthorized
	}
	want := hashSecret(secret)
	var (
		teamID string
		valid  bool
	)
	err = rn.Read(ctx, func(g *core.Game) {
		pl := g.Players[playerID]
		if pl == nil || pl.Removed {
			return
		}
		valid = subtle.ConstantTimeCompare([]byte(pl.TokenHash), []byte(want)) == 1
		teamID = pl.TeamID
	})
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, errUnauthorized
	}
	return &player{RunID: runID, PlayerID: playerID, TeamID: teamID, Runner: rn}, nil
}
