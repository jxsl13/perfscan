package ps5051orphan

import "strconv"

// These FormatInt calls are the file's only strconv references: fixing would
// orphan the import, so the report stays advisory.
func onlyRef(a, b int64) bool {
	return strconv.FormatInt(a, 16) == strconv.FormatInt(b, 16) // want `formats two integers to throwaway strings just to compare them; a == b`
}
