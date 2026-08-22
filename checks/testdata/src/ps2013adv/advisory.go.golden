package ps2013adv

import "strings"

// A comment anywhere in the punctuation the fix would replace (everything
// around the verbatim s argument) would be silently destroyed by the
// rewrite, so the fix is withheld and the report stays advisory: the
// golden is identical to this source.

func commentInPair(s string) string {
	return strings.NewReplacer("a" /* old */, "b").Replace(s) // want `strings\.NewReplacer with a single constant pair`
}

func commentBeforeArg(s string) string {
	return strings.NewReplacer("a", "b").Replace( /* keep */ s) // want `strings\.NewReplacer with a single constant pair`
}

func commentAfterArg(s string) string {
	return strings.NewReplacer("a", "b").Replace(s /* tail */) // want `strings\.NewReplacer with a single constant pair`
}
