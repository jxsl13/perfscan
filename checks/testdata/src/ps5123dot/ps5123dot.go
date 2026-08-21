package ps5123dot

import . "strings"

func locate(value string) int {
	if Contains(value, ":") {
		return Index(value, ":")
	}
	return -1
}
