package ps5122dot

import . "strings"

func clean(value string) string {
	if Contains(value, ":") {
		value = ReplaceAll(value, ":", "-")
	}
	return value
}
