package ps5128dot

import . "strings"

func split(value string) []string {
	if Contains(value, ":") {
		return Split(value, ":")
	}
	return []string{value}
}
