package ps5122

import (
	"bytes"
	"strings"
)

const (
	colonA = ":"
	colonB = ":"
)

func assign(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func named(value, old, replacement string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, old) {
		value = strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func equalConstants(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, colonA) {
		value = strings.ReplaceAll(value, colonB, ";")
	}
	return value
}

func emptyNeedle(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, "") {
		value = strings.ReplaceAll(value, "", "|")
	}
	return value
}

func parenthesized(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains((value), (":")) {
		value = (strings.ReplaceAll((value), (":"), ("-")))
	}
	return value
}

func earlyReturn(value, old, replacement string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, old) {
		return strings.ReplaceAll(value, old, replacement)
	}
	return value
}

func elseReturn(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	} else {
		return value
	}
}

func initializedOutput(value string) string {
	// want +2 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	output := value
	if strings.Contains(value, ":") {
		output = strings.ReplaceAll(value, ":", "-")
	}
	return output
}

func assignmentElseOutput(value, output string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, ":") {
		output = strings.ReplaceAll(value, ":", "-")
	} else {
		output = value
	}
	return output
}

// --- negatives ---

func differentTarget(value, output string) string {
	if strings.Contains(value, ":") {
		output = strings.ReplaceAll(value, ":", "-")
	}
	return output
}

func differentInput(value, other string) string {
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(other, ":", "-")
	}
	return value
}

func differentNeedle(value string) string {
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ";", "-")
	}
	return value
}

func effectfulReplacement(value string, replacement func() string) string {
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", replacement())
	}
	return value
}

func selectedReplacement(value string, holder struct{ replacement string }) string {
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", holder.replacement)
	}
	return value
}

func additionalWork(value string) string {
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
		value += "!"
	}
	return value
}

func initialized(value string) string {
	if enabled := true; enabled && strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func negated(value string) string {
	if !strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func compound(value string, enabled bool) string {
	if enabled && strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func assignmentElse(value string) string {
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	} else {
		value = "absent"
	}
	return value
}

func shortDeclaration(value string) string {
	if strings.Contains(value, ":") {
		output := strings.ReplaceAll(value, ":", "-")
		return output
	}
	return value
}

func delayedFallback(value string) string {
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	}
	value += ""
	return value
}

func wrongFallback(value string) string {
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	}
	return "absent"
}

func byteSlice(value []byte) []byte {
	if bytes.Contains(value, []byte(":")) {
		value = bytes.ReplaceAll(value, []byte(":"), []byte("-"))
	}
	return value
}

func replaceLimit(value string) string {
	if strings.Contains(value, ":") {
		value = strings.Replace(value, ":", "-", -1)
	}
	return value
}

func functionValues(value string) string {
	contains, replace := strings.Contains, strings.ReplaceAll
	if contains(value, ":") {
		value = replace(value, ":", "-")
	}
	return value
}

type helper string

func (value helper) Contains(old string) bool { return len(old) != 0 }
func (value helper) ReplaceAll(old, replacement string) helper {
	return value + helper(old) + helper(replacement)
}

func methods(value helper) helper {
	if value.Contains(":") {
		value = value.ReplaceAll(":", "-")
	}
	return value
}

var _ = []any{
	assign, named, equalConstants, emptyNeedle, parenthesized, earlyReturn, elseReturn,
	initializedOutput, assignmentElseOutput,
	differentTarget, differentInput, differentNeedle,
	effectfulReplacement, selectedReplacement, additionalWork, initialized,
	negated, compound, assignmentElse, shortDeclaration, delayedFallback,
	wrongFallback, byteSlice, replaceLimit, functionValues, methods,
}
