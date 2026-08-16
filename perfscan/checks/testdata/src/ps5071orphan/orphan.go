package ps5071orphan

import "strconv"

// The Itoa here is the file's only strconv reference: fixing it would
// orphan the strconv import, so the fix is withheld (advisory report).
func onlyUse(x int) bool {
	return strconv.Itoa(x) == "200" // want `strconv\.Itoa\(x\) == a decimal string constant formats the int to a throwaway string just to compare it`
}
