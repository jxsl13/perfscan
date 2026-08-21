package ps5121dot

import . "strings"

func tail(value string) string {
	if Contains(value, ":") {
		return SplitN(value, ":", 2)[1]
	}
	return value
}
