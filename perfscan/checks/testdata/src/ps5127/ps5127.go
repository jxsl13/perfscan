package ps5127

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

const invalidReplacement = "\xff"

func inPlace(value, replacement string) string {
	// want +1 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, replacement)
	}
	return value
}

func returned(value string) string {
	// want +1 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(value, "�")
	}
	return value
}

func returnedElse(value string) string {
	// want +1 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(value, invalidReplacement)
	} else {
		return value
	}
}

func initialized(value string) string {
	// want +2 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	result := value
	if !utf8.ValidString(value) {
		result = strings.ToValidUTF8(value, "")
	}
	return result
}

func existing(value, result string) string {
	// want +2 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	result = value
	if !utf8.ValidString(value) {
		result = strings.ToValidUTF8(value, "?")
	}
	return result
}

func assignmentElse(value, result string) string {
	// want +1 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	if !utf8.ValidString(value) {
		result = strings.ToValidUTF8(value, "?")
	} else {
		result = value
	}
	return result
}

func parenthesized(value string) string {
	// want +1 `utf8.ValidString scans before strings.ToValidUTF8 repeats validation and repair`
	if !(utf8.ValidString((value))) {
		return (strings.ToValidUTF8((value), ("?")))
	}
	return (value)
}

// --- negatives ---

func positiveCondition(value string) string {
	if utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "?")
	}
	return value
}

func doubleNegation(value string) string {
	if !!utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "?")
	}
	return value
}

func compound(value string, enabled bool) string {
	if enabled && !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, "?")
	}
	return value
}

func differentInput(value, other string) string {
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(other, "?")
	}
	return value
}

func effectfulReplacement(value string, replacement func() string) string {
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, replacement())
	}
	return value
}

type options struct{ replacement string }

func selectorReplacement(value string, option options) string {
	if !utf8.ValidString(value) {
		value = strings.ToValidUTF8(value, option.replacement)
	}
	return value
}

func initializedCondition(value string) string {
	if valid := utf8.ValidString(value); !valid {
		value = strings.ToValidUTF8(value, "?")
	}
	return value
}

func additionalWork(value string) string {
	if !utf8.ValidString(value) {
		_ = len(value)
		value = strings.ToValidUTF8(value, "?")
	}
	return value
}

func wrongFallback(value string) string {
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(value, "?")
	}
	return ""
}

func delayedFallback(value string) string {
	if !utf8.ValidString(value) {
		return strings.ToValidUTF8(value, "?")
	}
	_ = len(value)
	return value
}

func assignmentWithoutInitializer(value, result string) string {
	if !utf8.ValidString(value) {
		result = strings.ToValidUTF8(value, "?")
	}
	return result
}

func separatedInitializer(value string) string {
	result := value
	_ = len(value)
	if !utf8.ValidString(value) {
		result = strings.ToValidUTF8(value, "?")
	}
	return result
}

func shortDeclaration(value string) string {
	if !utf8.ValidString(value) {
		result := strings.ToValidUTF8(value, "?")
		return result
	}
	return value
}

func functionValues(value string) string {
	valid, sanitize := utf8.ValidString, strings.ToValidUTF8
	if !valid(value) {
		return sanitize(value, "?")
	}
	return value
}

func byteSanitizer(value []byte) []byte {
	if !utf8.Valid(value) {
		value = bytes.ToValidUTF8(value, []byte("?"))
	}
	return value
}

type validator string

func (validator) ValidString(string) bool { return false }

func methods(check validator, value string) string {
	if !check.ValidString(value) {
		value = strings.ToValidUTF8(value, "?")
	}
	return value
}

var _ = []any{
	inPlace, returned, returnedElse, initialized, existing, assignmentElse,
	parenthesized, positiveCondition, doubleNegation, compound, differentInput,
	effectfulReplacement, selectorReplacement, initializedCondition,
	additionalWork, wrongFallback, delayedFallback,
	assignmentWithoutInitializer, separatedInitializer, shortDeclaration,
	functionValues, byteSanitizer, methods,
}
