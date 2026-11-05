package scenario

import "strings"

// Операции нормализации ответа (defaults.answer_normalize).
const (
	NormTrim           = "trim"            // обрезать пробелы по краям
	NormLower          = "lower"           // нижний регистр
	NormYoToE          = "yo_to_e"         // ё → е
	NormCollapseSpaces = "collapse_spaces" // несколько пробелов → один
)

var knownNormalize = map[string]bool{
	NormTrim: true, NormLower: true, NormYoToE: true, NormCollapseSpaces: true,
}

var defaultNormalize = []string{NormTrim, NormLower, NormYoToE}

// Normalize применяет операции к ответу в указанном порядке.
// Неизвестные операции игнорируются (их отсекает валидатор).
func Normalize(s string, ops []string) string {
	for _, op := range ops {
		switch op {
		case NormTrim:
			s = strings.TrimSpace(s)
		case NormLower:
			s = strings.ToLower(s)
		case NormYoToE:
			s = strings.NewReplacer("ё", "е", "Ё", "Е").Replace(s)
		case NormCollapseSpaces:
			s = strings.Join(strings.Fields(s), " ")
		}
	}
	return s
}
