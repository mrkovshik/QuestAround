package core

import (
	"math"
	"regexp"

	"questline/internal/scenario"
)

// isTextAnswer — ответ проверяется по набранному тексту.
func isTextAnswer(t scenario.AnswerType) bool {
	return t == scenario.AnswerCode || t == scenario.AnswerRegex || t == scenario.AnswerChoice
}

// matchText проверяет набранный текст против ответа типа code/regex/choice.
func matchText(a scenario.Answer, input string, norm []string) bool {
	in := scenario.Normalize(input, norm)
	if in == "" {
		return false
	}
	switch a.Type {
	case scenario.AnswerCode:
		for _, v := range a.Values {
			if scenario.Normalize(v, norm) == in {
				return true
			}
		}
	case scenario.AnswerRegex:
		// Паттерн проверен валидатором; компиляция на каждую попытку дешевле,
		// чем держать кэш, пока ответов единицы в секунду.
		re, err := regexp.Compile(a.Pattern)
		return err == nil && re.MatchString(in)
	case scenario.AnswerChoice:
		return scenario.Normalize(a.Correct, norm) == in
	}
	return false
}

type geoMatch int

const (
	geoMiss geoMatch = iota
	geoHit
	geoInaccurate // точность GPS хуже порога — нельзя судить
)

func matchGeo(a scenario.Answer, lat, lng, accuracy float64) geoMatch {
	if a.Type != scenario.AnswerGeo || len(a.Point) != 2 {
		return geoMiss
	}
	if a.MaxAccuracyM > 0 && accuracy > a.MaxAccuracyM {
		return geoInaccurate
	}
	if Distance(lat, lng, a.Point[0], a.Point[1]) <= a.RadiusM {
		return geoHit
	}
	return geoMiss
}

const earthRadiusM = 6371000

// Distance — расстояние между точками в метрах (гаверсинус).
func Distance(lat1, lng1, lat2, lng2 float64) float64 {
	rad := math.Pi / 180
	dLat := (lat2 - lat1) * rad
	dLng := (lng2 - lng1) * rad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Sqrt(h))
}
