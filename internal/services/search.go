package services

import (
	"strings"

	"hng14-s0-gender-classify/internal/repository"
)

func ParseSearchQuery(q string) repository.ProfileFilter {
	q = strings.ToLower(q)
	filter := repository.ProfileFilter{
		Page: 1,
		Limit: 10,
	}

	words := strings.Fields(q)

	for _, word := range words {
		if isGender(word) {
			if filter.Gender == "" {
				filter.Gender = word
			}
		}
	}

	if contains(q, []string{"male", "males", "man", "men", "boy", "boys"}) {
		filter.Gender = "male"
	}
	if contains(q, []string{"female", "females", "woman", "women", "girl", "girls"}) {
		filter.Gender = "female"
	}

	if contains(q, []string{"child", "children", "kid", "kids"}) {
		filter.AgeGroup = "child"
	}
	if contains(q, []string{"teenager", "teens", "teen"}) {
		filter.AgeGroup = "teenager"
	}
	if contains(q, []string{"adult", "adults", "grown", "grownup"}) {
		filter.AgeGroup = "adult"
	}
	if contains(q, []string{"senior", "seniors", "elder", "elderly", "old"}) {
		filter.AgeGroup = "senior"
	}

	if contains(q, []string{"young", "youth", "youths"}) {
		minAge := 16
		maxAge := 24
		filter.MinAge = &minAge
		filter.MaxAge = &maxAge
	}

	if idx := indexOfAny(q, []string{"above", "over", "older than", "more than"}); idx >= 0 {
		age := extractNumberAfter(q, idx)
		if age > 0 {
			filter.MinAge = &age
		}
	}

	if idx := indexOfAny(q, []string{"below", "under", "younger than", "less than"}); idx >= 0 {
		age := extractNumberAfter(q, idx)
		if age > 0 {
			filter.MaxAge = &age
		}
	}

	if idx := strings.Index(q, "age"); idx >= 0 {
		age := extractNumberInRange(q[idx:])
		if age.Min > 0 {
			filter.MinAge = &age.Min
		}
		if age.Max > 0 {
			filter.MaxAge = &age.Max
		}
	}

	if idx := indexOfAny(q, []string{"from", "in", "at"}); idx >= 0 {
		after := q[idx+4:]
		for _, country := range countryNames {
			if strings.Contains(after, strings.ToLower(country)) {
				for code, name := range countryNames {
					if strings.ToLower(name) == strings.ToLower(country) {
						filter.CountryID = code
						break
					}
				}
				break
			}
		}
	}

	for code, name := range countryNames {
		if strings.Contains(q, strings.ToLower(name)) {
			filter.CountryID = code
			break
		}
	}

	filter.SortBy = "created_at"
	filter.Order = "desc"

	return filter
}

func isGender(word string) bool {
	return contains(word, []string{"male", "males", "female", "females", "man", "men", "woman", "women", "boy", "boys", "girl", "girls"})
}

func contains(s string, substrings []string) bool {
	s = strings.ToLower(s)
	for _, sub := range substrings {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func indexOfAny(s string, subs []string) int {
	for i := range len(s) {
		for _, sub := range subs {
			if i+len(sub) <= len(s) && strings.ToLower(s[i:i+len(sub)]) == sub {
				return i
			}
		}
	}
	return -1
}

func extractNumberAfter(s string, idx int) int {
	after := s[idx:]
	var numStr string
	for _, c := range after {
		if c >= '0' && c <= '9' {
			numStr += string(c)
		} else if len(numStr) > 0 {
			break
		}
	}
	if numStr == "" {
		return 0
	}
	return toInt(numStr)
}

type rangeVal struct {
	Min int
	Max int
}

func extractNumberInRange(s string) rangeVal {
	var result rangeVal
	words := strings.Fields(s)
	for i, word := range words {
		num := extractNumber(word)
		if num > 0 {
			if i+1 < len(words) && (words[i+1] == "to" || words[i+1] == "-" || words[i+1] == "and") {
				result.Min = num
			} else if result.Min == 0 {
				result.Min = num
				result.Max = num
			}
		}
	}
	return result
}

func extractNumber(s string) int {
	var numStr string
	for _, c := range s {
		if c >= '0' && c <= '9' {
			numStr += string(c)
		}
	}
	return toInt(numStr)
}

func toInt(s string) int {
	val := 0
	for _, c := range s {
		val = val*10 + int(c-'0')
	}
	return val
}
