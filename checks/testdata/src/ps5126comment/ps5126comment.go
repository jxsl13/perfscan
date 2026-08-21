package ps5126comment

import "strings"

func guardComment(value string) int {
	// want +1 `strings.Contains searches forward before strings.LastIndex repeats the lookup backward`
	if strings.Contains(value /* retain guard rationale */, ":") {
		return strings.LastIndex(value, ":")
	}
	return -1
}

func fallbackComment(value string) int {
	// want +1 `strings.Contains searches forward before strings.LastIndex repeats the lookup backward`
	if strings.Contains(value, ":") {
		return strings.LastIndex(value, ":")
	}
	return -1 // retain fallback rationale
}

func localGuardNeedle(value string) int {
	const guardNeedle = ":"
	// want +1 `strings.Contains searches forward before strings.LastIndex repeats the lookup backward`
	if strings.Contains(value, guardNeedle) {
		return strings.LastIndex(value, ":")
	}
	return -1
}

func initializerComment(value string) int {
	// want +2 `strings.Contains searches forward before strings.LastIndex repeats the lookup backward`
	index := -1 // retain sentinel rationale
	if strings.Contains(value, ":") {
		index = strings.LastIndex(value, ":")
	}
	return index
}

func keptComment(value string) int {
	// want +1 `strings.Contains searches forward before strings.LastIndex repeats the lookup backward`
	if strings.Contains(value, ":") {
		return strings.LastIndex(value /* retain lookup rationale */, ":")
	}
	return -1
}
