package core

import (
	"strings"
	"unicode/utf8"
)

// Витрина: показывать ли игру в общем списке («Доступные игры» VK-бота)
// и в каком она городе. По умолчанию игра скрыта — тестовые игры никто не
// видит; по ссылке или коду она доступна всегда.

const (
	EvListingSet EventType = "listing_set"
	maxCityLen             = 40
)

type ListingSet struct {
	City   string `json:"city,omitempty"`
	Listed bool   `json:"listed,omitempty"`
}

func (ListingSet) Type() EventType { return EvListingSet }

func init() {
	payloadFactory[EvListingSet] = func() Payload { return &ListingSet{} }
}

// Listing — витрина игры.
type Listing struct {
	City   string
	Listed bool
}

// SetListing — город и видимость игры в списке (ведущий, в любой момент).
type SetListing struct {
	City   string
	Listed bool
}

func (SetListing) isCommand() {}

func (g *Game) setListing(t *tx, c SetListing) error {
	city := strings.Join(strings.Fields(c.City), " ")
	if utf8.RuneCountInString(city) > maxCityLen {
		return ErrBadCommand
	}
	t.emit("", "host", "", ListingSet{City: city, Listed: c.Listed})
	return nil
}

func (g *Game) applyListing(e Event) bool {
	p, ok := e.Payload.(ListingSet)
	if !ok {
		return false
	}
	g.Listing = Listing{City: p.City, Listed: p.Listed}
	return true
}
