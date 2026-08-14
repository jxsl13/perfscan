package ps5012

import str "strings"

// An aliased strings import: the rewrite reuses the file's own
// qualifier verbatim. No import surgery is ever needed — the rewritten
// call keeps the same package referenced.
func aliased(s, old, new string) string {
	return str.Replace(s, old, new, str.Count(s, old)) // want `strings\.Replace\(s, old, new, strings\.Count\(s, old\)\) walks s twice`
}
