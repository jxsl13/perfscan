package ps5124dot

import . "strings"

func count(value string) int {
	if Contains(value, ":") {
		return Count(value, ":")
	}
	return 0
}
