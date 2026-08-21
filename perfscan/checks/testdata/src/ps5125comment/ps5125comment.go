package ps5125comment

import "strings"

func guardComment(value string) string {
	// want +1 `strings.Contains scans before strings.Replace repeats the same lookup`
	if strings.Contains(value /* retain guard rationale */, ":") {
		value = strings.Replace(value, ":", "-", 1)
	}
	return value
}

func closingComment(value string) string {
	// want +1 `strings.Contains scans before strings.Replace repeats the same lookup`
	if strings.Contains(value, ":") {
		value = strings.Replace(value, ":", "-", 1)
	} // retain branch rationale
	return value
}

func localGuardNeedle(value string) string {
	const guardNeedle = ":"
	// want +1 `strings.Contains scans before strings.Replace repeats the same lookup`
	if strings.Contains(value, guardNeedle) {
		value = strings.Replace(value, ":", "-", 1)
	}
	return value
}

func fallbackComment(value string) string {
	// want +1 `strings.Contains scans before strings.Replace repeats the same lookup`
	if strings.Contains(value, ":") {
		return strings.Replace(value, ":", "-", 1)
	}
	return value // retain fallback rationale
}

func keptComment(value string) string {
	// want +1 `strings.Contains scans before strings.Replace repeats the same lookup`
	if strings.Contains(value, ":") {
		value = strings.Replace(value /* retain input rationale */, ":", "-", 1)
	}
	return value
}
