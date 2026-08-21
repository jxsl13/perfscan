package ps5108dot

import . "strings"

func repeat(text string) string {
	return Repeat(Repeat(text, 2), 3)
}
