package scenario

import (
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration — длительность в формате Go: 30s, 15m, 1h30m.
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) String() string { return time.Duration(d).String() }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return typeErr(n, "ожидалась длительность (пример: 15m, 1h30m)")
	}
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return typeErr(n, "некорректная длительность %q (пример: 15m, 1h30m)", n.Value)
	}
	if v < 0 {
		return typeErr(n, "длительность не может быть отрицательной: %q", n.Value)
	}
	*d = Duration(v)
	return nil
}

// RequireKind — сколько ответов уровня нужно для зачёта.
type RequireKind int

const (
	RequireAll RequireKind = iota // по умолчанию
	RequireAny
	RequireN
)

type Require struct {
	Kind RequireKind
	N    int // для RequireN
}

// Needed — сколько ответов нужно при total ответах на уровне.
func (r Require) Needed(total int) int {
	switch r.Kind {
	case RequireAny:
		return 1
	case RequireN:
		return r.N
	default:
		return total
	}
}

func (r *Require) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return typeErr(n, "require: ожидалось all, any или число")
	}
	switch n.Value {
	case "all":
		*r = Require{Kind: RequireAll}
		return nil
	case "any":
		*r = Require{Kind: RequireAny}
		return nil
	}
	v, err := strconv.Atoi(n.Value)
	if err != nil {
		return typeErr(n, "require: ожидалось all, any или число, получено %q", n.Value)
	}
	*r = Require{Kind: RequireN, N: v}
	return nil
}

// typeErr возвращает *yaml.TypeError: такие ошибки декодер копит,
// а не обрывается на первой.
func typeErr(n *yaml.Node, format string, args ...any) error {
	return &yaml.TypeError{Errors: []string{
		fmt.Sprintf("строка %d: %s", n.Line, fmt.Sprintf(format, args...)),
	}}
}
