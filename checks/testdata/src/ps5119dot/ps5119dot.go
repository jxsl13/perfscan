package ps5119dot

import . "strings"

func trim(value, prefix string) string {
	if HasPrefix(value, prefix) {
		return TrimPrefix(value, prefix)
	}
	return value
}
