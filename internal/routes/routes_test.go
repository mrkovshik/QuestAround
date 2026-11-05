package routes

import (
	"reflect"
	"testing"
)

func lv(ids ...string) []Level {
	out := make([]Level, len(ids))
	for i, id := range ids {
		out[i] = Level{ID: id}
	}
	return out
}

// pilotLevels: общий старт и финиш, цепочка c1→c2 в середине.
func pilotLevels() []Level {
	return []Level{
		{ID: "start", Pin: PinFirst},
		{ID: "a"},
		{ID: "c1", Chain: "c"},
		{ID: "c2", Chain: "c"},
		{ID: "b"},
		{ID: "d"},
		{ID: "final", Pin: PinLast},
	}
}

func TestSame(t *testing.T) {
	got, err := Build(pilotLevels(), Options{Mode: ModeSame, Teams: 3})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"start", "a", "c1", "c2", "b", "d", "final"}
	for i, r := range got {
		if !reflect.DeepEqual(r, want) {
			t.Errorf("команда %d: %v, ожидали %v", i, r, want)
		}
	}
}

func TestRotateSpreadsStarts(t *testing.T) {
	// 4 подвижных блока (a, [c1 c2], b, d), 4 команды → у каждой свой первый блок.
	got, err := Build(pilotLevels(), Options{Mode: ModeRotate, Teams: 4})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"start", "a", "c1", "c2", "b", "d", "final"},
		{"start", "c1", "c2", "b", "d", "a", "final"},
		{"start", "b", "d", "a", "c1", "c2", "final"},
		{"start", "d", "a", "c1", "c2", "b", "final"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestRotateFewerTeamsMaxSpacing(t *testing.T) {
	// 2 команды на 4 уровня → стартуют с противоположных точек круга.
	got, err := Build(lv("a", "b", "c", "d"), Options{Mode: ModeRotate, Teams: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got[0][0] != "a" || got[1][0] != "c" {
		t.Errorf("стартовые уровни %s, %s; ожидали a, c", got[0][0], got[1][0])
	}
}

func TestRotateMoreTeamsThanLevels(t *testing.T) {
	got, err := Build(lv("a", "b", "c"), Options{Mode: ModeRotate, Teams: 6})
	if err != nil {
		t.Fatal(err)
	}
	var firsts []string
	for _, r := range got {
		firsts = append(firsts, r[0])
	}
	want := []string{"a", "a", "b", "b", "c", "c"}
	if !reflect.DeepEqual(firsts, want) {
		t.Errorf("первые уровни %v, ожидали %v", firsts, want)
	}
}

func TestRandomDeterministic(t *testing.T) {
	opt := Options{Mode: ModeRandom, Teams: 5, Seed: 42}
	a, err := Build(pilotLevels(), opt)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Build(pilotLevels(), opt)
	if !reflect.DeepEqual(a, b) {
		t.Error("одинаковый seed должен давать одинаковые маршруты")
	}
	c, _ := Build(pilotLevels(), Options{Mode: ModeRandom, Teams: 5, Seed: 43})
	if reflect.DeepEqual(a, c) {
		t.Error("разные seed дали одинаковые маршруты (маловероятно)")
	}
}

func TestExplicitRoundRobin(t *testing.T) {
	levels := lv("a", "b", "c")
	r1 := []string{"a", "b", "c"}
	r2 := []string{"c", "a", "b"}
	got, err := Build(levels, Options{Mode: ModeExplicit, Teams: 3, Explicit: [][]string{r1, r2}})
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{r1, r2, r1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	// Результат — копия: правка не должна задевать исходный маршрут.
	got[0][0] = "zzz"
	if r1[0] != "a" {
		t.Error("Build вернул ссылку на исходный срез")
	}
}

func TestExplicitInvalid(t *testing.T) {
	_, err := Build(pilotLevels(), Options{
		Mode:     ModeExplicit,
		Teams:    1,
		Explicit: [][]string{{"a", "start", "c1", "c2", "b", "d", "final"}},
	})
	if err == nil {
		t.Fatal("ожидали ошибку: start закреплён первым")
	}
}

func TestErrors(t *testing.T) {
	if _, err := Build(lv("a"), Options{Mode: ModeSame, Teams: 0}); err == nil {
		t.Error("teams=0 должно быть ошибкой")
	}
	if _, err := Build(nil, Options{Mode: ModeSame, Teams: 1}); err == nil {
		t.Error("пустой список уровней должен быть ошибкой")
	}
	if _, err := Build(lv("a"), Options{Mode: "zigzag", Teams: 1}); err == nil {
		t.Error("неизвестный режим должен быть ошибкой")
	}
}

func TestOnlyPinned(t *testing.T) {
	levels := []Level{{ID: "s", Pin: PinFirst}, {ID: "f", Pin: PinLast}}
	for _, m := range []Mode{ModeSame, ModeRotate, ModeRandom} {
		got, err := Build(levels, Options{Mode: m, Teams: 2})
		if err != nil {
			t.Fatalf("%s: %v", m, err)
		}
		for _, r := range got {
			if !reflect.DeepEqual(r, []string{"s", "f"}) {
				t.Errorf("%s: %v", m, r)
			}
		}
	}
}

// Любой построенный маршрут должен проходить Validate.
func TestAllModesProduceValidRoutes(t *testing.T) {
	levels := pilotLevels()
	for _, m := range []Mode{ModeSame, ModeRotate, ModeRandom} {
		for teams := 1; teams <= 12; teams++ {
			got, err := Build(levels, Options{Mode: m, Teams: teams, Seed: uint64(teams)})
			if err != nil {
				t.Fatalf("%s/%d: %v", m, teams, err)
			}
			for i, r := range got {
				if p := Validate(levels, r); len(p) > 0 {
					t.Errorf("%s/%d команда %d: %v (%v)", m, teams, i, p, r)
				}
			}
		}
	}
}

func TestValidate(t *testing.T) {
	levels := pilotLevels()
	cases := []struct {
		name  string
		route []string
		ok    bool
	}{
		{"корректный", []string{"start", "b", "c1", "c2", "a", "d", "final"}, true},
		{"нет уровня", []string{"start", "a", "c1", "c2", "b", "final"}, false},
		{"дубль", []string{"start", "a", "a", "c1", "c2", "b", "d", "final"}, false},
		{"чужой уровень", []string{"start", "a", "c1", "c2", "b", "x", "final"}, false},
		{"финиш не последний", []string{"start", "a", "c1", "c2", "b", "final", "d"}, false},
		{"цепочка разорвана", []string{"start", "c1", "a", "c2", "b", "d", "final"}, false},
		{"цепочка в обратном порядке", []string{"start", "c2", "c1", "a", "b", "d", "final"}, false},
	}
	for _, c := range cases {
		p := Validate(levels, c.route)
		if (len(p) == 0) != c.ok {
			t.Errorf("%s: problems=%v", c.name, p)
		}
	}
}
