package ps5075alias

import s "strings"

func normalize(v string) string {
	return s.ToUpper((s.ToUpper(v))) // want `strings\.ToUpper is applied 2 times`
}
