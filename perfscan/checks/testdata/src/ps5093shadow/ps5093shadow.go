package ps5093shadow

import "strings"

func length(text string) int {
	len := func(string) int { return 99 }
	_ = len
	return strings.NewReader(text).Len() // want "strings.NewReader[(]...[)].Len constructs an ephemeral container"
}

func size(text string) any {
	type int64 int
	return strings.NewReader(text).Size() // want "strings.NewReader[(]...[)].Size constructs an ephemeral container"
}
