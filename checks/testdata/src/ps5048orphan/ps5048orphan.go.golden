package ps5048orphan

import "strconv"

// strconv.Itoa here are the file's only strconv references: fixing would orphan
// the import, so the report stays advisory.
func onlyRef(a, b int) bool {
	return strconv.Itoa(a) == strconv.Itoa(b) // want `strconv\.Itoa\(a\) == strconv\.Itoa\(b\) formats two ints to throwaway strings just to compare them; a == b compares the ints directly`
}
