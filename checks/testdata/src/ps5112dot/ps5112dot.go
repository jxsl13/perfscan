package ps5112dot

import . "strings"

func inverse(s string) string { return Join(Split(s, ","), ",") }
