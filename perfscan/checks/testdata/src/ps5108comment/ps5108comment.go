package ps5108comment

import (
	"strings"
	"unicode/utf8"
)

func commented(text string) string {
	return strings.Repeat( /* preserve rationale */ strings.Repeat(text, 2), 3) // want "strings.Repeat is nested 2 times with positive constant counts; combine them to 6"
}

func localConstant(text string) string {
	const outer = 3
	return strings.Repeat(strings.Repeat(text, 2), outer) // want "strings.Repeat is nested 2 times with positive constant counts; combine them to 6"
}

func importedConstant(text string) string {
	return strings.Repeat(strings.Repeat(text, 2), utf8.UTFMax) // want "strings.Repeat is nested 2 times with positive constant counts; combine them to 8"
}
