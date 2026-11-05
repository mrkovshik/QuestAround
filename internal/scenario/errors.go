package scenario

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Error — одна проблема в сценарии.
type Error struct {
	Line int    // 0 — строка неизвестна
	Path string // например levels[2].answers[0].pattern
	Msg  string
}

func (e Error) Error() string {
	var b strings.Builder
	if e.Line > 0 {
		fmt.Fprintf(&b, "строка %d: ", e.Line)
	}
	if e.Path != "" {
		fmt.Fprintf(&b, "%s: ", e.Path)
	}
	b.WriteString(e.Msg)
	return b.String()
}

// Errors — все найденные проблемы; валидатор не останавливается на первой.
type Errors []Error

func (es Errors) Error() string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "\n")
}

// locator находит номер строки по пути в YAML-дереве.
type locator struct{ root *yaml.Node }

// line возвращает строку самого глубокого найденного элемента пути.
// Элементы пути: string — ключ мапы, int — индекс последовательности.
func (l locator) line(path ...any) int {
	n := l.root
	if n == nil {
		return 0
	}
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	line := n.Line
	for _, p := range path {
		var next *yaml.Node
		switch k := p.(type) {
		case string:
			if n.Kind == yaml.MappingNode {
				for i := 0; i+1 < len(n.Content); i += 2 {
					if n.Content[i].Value == k {
						line = n.Content[i].Line
						next = n.Content[i+1]
						break
					}
				}
			}
		case int:
			if n.Kind == yaml.SequenceNode && k >= 0 && k < len(n.Content) {
				next = n.Content[k]
				line = next.Line
			}
		}
		if next == nil {
			return line
		}
		n = next
	}
	return line
}

func renderPath(path []any) string {
	var b strings.Builder
	for _, p := range path {
		switch k := p.(type) {
		case string:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(k)
		case int:
			fmt.Fprintf(&b, "[%d]", k)
		}
	}
	return b.String()
}
