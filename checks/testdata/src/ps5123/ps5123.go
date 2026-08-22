package ps5123

import (
	"bytes"
	"slices"
	"strings"
)

const (
	colonA = ":"
	colonB = ":"
)

func stringReturn(value string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value, colonA) {
		return strings.Index(value, colonB)
	}
	return -1
}

func stringElse(value, needle string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value, needle) {
		return strings.Index(value, needle)
	} else {
		return -1
	}
}

func byteReturn(value, needle []byte) int {
	// want +1 `bytes.Contains searches before bytes.Index repeats the same lookup`
	if bytes.Contains(value, needle) {
		return bytes.Index(value, needle)
	}
	return -1
}

func stringAny(value, chars string) int {
	// want +1 `strings.ContainsAny searches before strings.IndexAny repeats the same lookup`
	if strings.ContainsAny(value, chars) {
		return strings.IndexAny(value, chars)
	}
	return -1
}

func byteAny(value []byte) int {
	// want +1 `bytes.ContainsAny searches before bytes.IndexAny repeats the same lookup`
	if bytes.ContainsAny(value, ":;") {
		return bytes.IndexAny(value, ":;")
	}
	return -1
}

func stringRune(value string, needle rune) int {
	// want +1 `strings.ContainsRune searches before strings.IndexRune repeats the same lookup`
	if strings.ContainsRune(value, needle) {
		return strings.IndexRune(value, needle)
	}
	return -1
}

func byteRune(value []byte) int {
	// want +1 `bytes.ContainsRune searches before bytes.IndexRune repeats the same lookup`
	if bytes.ContainsRune(value, 'λ') {
		return bytes.IndexRune(value, '\u03bb')
	}
	return -1
}

func sliceIndex(value []int, target int) int {
	// want +1 `slices.Contains searches before slices.Index repeats the same lookup`
	if slices.Contains(value, target) {
		return slices.Index(value, target)
	}
	return -1
}

func initialized(value string) int {
	// want +2 `strings.Contains searches before strings.Index repeats the same lookup`
	index := -1
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	}
	return index
}

func existing(value string, index int) int {
	// want +2 `strings.Contains searches before strings.Index repeats the same lookup`
	index = -1
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	}
	return index
}

func assignmentElse(value string, index int) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	} else {
		index = -1
	}
	return index
}

func parenthesized(value string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains((value), (":")) {
		return (strings.Index((value), (":")))
	}
	return (-1)
}

// --- negatives ---

func differentInput(value, other string) int {
	if strings.Contains(value, ":") {
		return strings.Index(other, ":")
	}
	return -1
}

func differentNeedle(value string) int {
	if strings.Contains(value, ":") {
		return strings.Index(value, ";")
	}
	return -1
}

func constructedBytes(value []byte) int {
	if bytes.Contains(value, []byte(":")) {
		return bytes.Index(value, []byte(":"))
	}
	return -1
}

func effectfulNeedle(value string, needle func() string) int {
	if strings.Contains(value, needle()) {
		return strings.Index(value, needle())
	}
	return -1
}

func initializedCondition(value string) int {
	if enabled := true; enabled && strings.Contains(value, ":") {
		return strings.Index(value, ":")
	}
	return -1
}

func negated(value string) int {
	if !strings.Contains(value, ":") {
		return strings.Index(value, ":")
	}
	return -1
}

func additionalWork(value string) int {
	if strings.Contains(value, ":") {
		_ = len(value)
		return strings.Index(value, ":")
	}
	return -1
}

func wrongFallback(value string) int {
	if strings.Contains(value, ":") {
		return strings.Index(value, ":")
	}
	return -2
}

func delayedFallback(value string) int {
	if strings.Contains(value, ":") {
		return strings.Index(value, ":")
	}
	_ = value
	return -1
}

func assignmentWithoutInitializer(value string, index int) int {
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	}
	return index
}

func separatedInitializer(value string) int {
	index := -1
	_ = value
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	}
	return index
}

func wrongElse(value string, index int) int {
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	} else {
		index = 0
	}
	return index
}

func shortDeclaration(value string) int {
	if strings.Contains(value, ":") {
		index := strings.Index(value, ":")
		return index
	}
	return -1
}

func functionValues(value string) int {
	contains, index := strings.Contains, strings.Index
	if contains(value, ":") {
		return index(value, ":")
	}
	return -1
}

type helper string

func (value helper) Contains(needle string) bool { return needle != "" }
func (value helper) Index(needle string) int     { return len(value) + len(needle) }

func methods(value helper) int {
	if value.Contains(":") {
		return value.Index(":")
	}
	return -1
}

var _ = []any{
	stringReturn, stringElse, byteReturn, stringAny, byteAny, stringRune,
	byteRune, sliceIndex, initialized, existing, assignmentElse, parenthesized,
	differentInput, differentNeedle, constructedBytes, effectfulNeedle,
	initializedCondition, negated, additionalWork, wrongFallback,
	delayedFallback, assignmentWithoutInitializer, separatedInitializer,
	wrongElse, shortDeclaration, functionValues, methods,
}
