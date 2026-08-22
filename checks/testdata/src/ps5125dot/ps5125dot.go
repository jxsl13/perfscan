package ps5125dot

import . "strings"

func replace(value string) string {
	if Contains(value, ":") {
		value = Replace(value, ":", "-", 1)
	}
	return value
}
