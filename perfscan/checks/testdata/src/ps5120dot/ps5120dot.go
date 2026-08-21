package ps5120dot

import . "strings"

func head(value string) string {
	result := SplitN(value, ":", 2)[0]
	return result
}
