package ps5126dot

import . "strings"

func locate(value string) int {
	if Contains(value, ":") {
		return LastIndex(value, ":")
	}
	return -1
}
