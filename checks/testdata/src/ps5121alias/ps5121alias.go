package ps5121alias

import (
	b "bytes"
	s "strings"
)

func tail(value string) string {
	if s.Contains(value, "|") { // want `strings.Contains proves.*index 1`
		return s.SplitN(value, "|", 2)[1]
	}
	return value
}

func head(value []byte) []byte {
	if b.Contains(value, []byte{'|'}) { // want `bytes.Contains proves.*index 0`
		return b.SplitN(value, []byte("|"), 2)[0]
	}
	return value
}
