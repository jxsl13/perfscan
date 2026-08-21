package ps5123comment

import "strings"

func guardComment(value string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value /* retain guard rationale */, ":") {
		return strings.Index(value, ":")
	}
	return -1
}

func fallbackComment(value string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value, ":") {
		return strings.Index(value, ":")
	}
	return -1 // retain fallback rationale
}

func localGuardNeedle(value string) int {
	const guardNeedle = ":"
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value, guardNeedle) {
		return strings.Index(value, ":")
	}
	return -1
}

func initializerComment(value string) int {
	// want +2 `strings.Contains searches before strings.Index repeats the same lookup`
	index := -1 // retain sentinel rationale
	if strings.Contains(value, ":") {
		index = strings.Index(value, ":")
	}
	return index
}

func keptComment(value string) int {
	// want +1 `strings.Contains searches before strings.Index repeats the same lookup`
	if strings.Contains(value, ":") {
		return strings.Index(value /* retain lookup rationale */, ":")
	}
	return -1
}
