package ps5122comment

import "strings"

func guardComment(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value /* retain guard rationale */, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func closingComment(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value, ":", "-")
	} // retain branch rationale
	return value
}

func localGuardNeedle(value string) string {
	const guardNeedle = ":"
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, guardNeedle) {
		value = strings.ReplaceAll(value, ":", "-")
	}
	return value
}

func fallbackComment(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, ":") {
		return strings.ReplaceAll(value, ":", "-")
	}
	return value // retain fallback rationale
}

func keptComment(value string) string {
	// want +1 `strings.Contains scans before strings.ReplaceAll repeats the match work`
	if strings.Contains(value, ":") {
		value = strings.ReplaceAll(value /* retain input rationale */, ":", "-")
	}
	return value
}
