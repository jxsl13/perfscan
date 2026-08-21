package ps5120alias

import text "strings"

func head(value string) string {
	result := text.SplitN(value, ":", 2)[0] // want `strings.SplitN\(\.\.\.\)\[0\] allocates a piece slice only to assign its head; strings.Cut returns the identical head directly with no result-slice allocation`
	return result
}
