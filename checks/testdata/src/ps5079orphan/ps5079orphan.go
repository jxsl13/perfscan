package ps5079orphan

import "strings"

func orphan(s string) string {
	return strings.Trim(s, "") // want `1 adjacent strings boundary operation\(s\) use an empty prefix`
}
