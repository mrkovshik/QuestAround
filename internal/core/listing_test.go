package core

import (
	"strings"
	"testing"
)

func TestListing(t *testing.T) {
	h := newHarness(t, regYAML)
	h.eq("по умолчанию скрыта", h.g.Listing.Listed, false)
	h.do(SetListing{City: "  Горячий   Ключ ", Listed: true})
	h.eq("город", h.g.Listing.City, "Горячий Ключ")
	h.eq("видна", h.g.Listing.Listed, true)
	h.fail(SetListing{City: strings.Repeat("я", 41)}, ErrBadCommand)
	h.do(RegisterTeam{TeamID: "t1", Name: "A", InviteCode: "I1", Captain: np("cap")})
	h.do(StartRun{})
	h.do(SetListing{City: "Нови-Сад"}) // можно и во время игры
	h.eq("город", h.g.Listing.City, "Нови-Сад")
	h.eq("скрыта", h.g.Listing.Listed, false)
	h.checkReplay()
}
