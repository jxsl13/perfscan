package ps2013

import str "strings"

// An aliased strings import matches (the constructor is resolved by import
// PATH) and the rewrite reuses the alias — a hardcoded "strings" qualifier
// would not compile here.
func aliased(s string) string {
	return str.NewReplacer("th", "TH").Replace(s) // want `strings\.NewReplacer with a single constant pair`
}
