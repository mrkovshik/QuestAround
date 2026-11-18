package core

import (
	"crypto/sha256"
	"strings"
)

// transferAlphabet — без похожих символов (0/O, 1/I/L), чтобы код можно было продиктовать.
const transferAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// TransferCode — код переноса игрока в другой транспорт (VK-бот): QA-XXXX-XXXX.
//
// Код выводится из хэша токена игрока, поэтому приложение считает его само,
// без сервера (когда сайт недоступен), а сервер проверяет по журналу.
// Тот же алгоритм — в web/src/lib/transfer.ts.
func TransferCode(tokenHash string) string {
	if tokenHash == "" {
		return ""
	}
	h := sha256.Sum256([]byte("questaround-vk:" + tokenHash))
	var b strings.Builder
	b.WriteString("QA-")
	for i := range 8 {
		if i == 4 {
			b.WriteByte('-')
		}
		b.WriteByte(transferAlphabet[int(h[i])%len(transferAlphabet)])
	}
	return b.String()
}

// PlayerByTransferCode — активный игрок с таким кодом переноса.
func (g *Game) PlayerByTransferCode(code string) (*Player, bool) {
	code = strings.ToUpper(strings.TrimSpace(code))
	for _, p := range g.Players {
		if !p.Removed && TransferCode(p.TokenHash) == code {
			return p, true
		}
	}
	return nil, false
}
