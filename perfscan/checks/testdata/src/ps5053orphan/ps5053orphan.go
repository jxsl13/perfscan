package ps5053orphan

import "strconv"

// These strconv.Quote calls are the file's only strconv references: fixing would
// orphan the import, so the report stays advisory.
func onlyRef(a, b string) bool {
	return strconv.Quote(a) == strconv.Quote(b) // want `strconv\.Quote\(a\) == strconv\.Quote\(b\) quotes two strings just to compare them; a == b compares the strings directly`
}
