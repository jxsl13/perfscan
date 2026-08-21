package ps5080alias

import st "strings"

func lastImport(input string) string {
	return st.ReplaceAll(st.Replace(input, "x", "y", 0), "z", "z") // want `2 adjacent strings Replace/ReplaceAll call\(s\) are content no-ops`
}
