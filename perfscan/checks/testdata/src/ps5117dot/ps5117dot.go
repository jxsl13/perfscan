package ps5117dot

import . "strings"

func canonical(payload string) string {
	return Join(Fields(Join(Fields(payload), " ")), " ")
}
