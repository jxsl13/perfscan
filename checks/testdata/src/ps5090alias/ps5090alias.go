package ps5090alias

import (
	q "strconv"
	s "strings"
)

func aliasedPackages(text string) string {
	return q.Quote(s.Clone(text)) // want `strconv.Quote materializes an independent quoted representation but receives 1 throwaway strings.Clone layer`
}
