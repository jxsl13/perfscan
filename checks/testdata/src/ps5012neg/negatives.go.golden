package ps5012neg

import "strings"

// A method set spelling the same names on a non-strings receiver: the
// callee is not the standard library's package-level function.
type fake struct{}

func (fake) Replace(s, old, new string, n int) string { return s }
func (fake) Count(s, sep string) int                  { return 0 }

// None of these is the counted-replace-all pattern: no diagnostics at
// all.
func negatives(s, t, old, other, new string, n int, fk fake) {
	// The cap is a Count over a DIFFERENT haystack: a genuine partial
	// replace, not the replace-all idiom.
	_ = strings.Replace(s, old, new, strings.Count(t, old))

	// ... or over a different needle.
	_ = strings.Replace(s, old, new, strings.Count(s, other))

	// ... or with the Count arguments swapped.
	_ = strings.Replace(s, old, new, strings.Count(old, s))

	// An arithmetic wrapper around the Count is not the exact cap.
	_ = strings.Replace(s, old, new, strings.Count(s, old)+1)
	_ = strings.Replace(s, old, new, strings.Count(s, old)-1)

	// A plain n, a literal, or -1 (PS-clean spellings) never match.
	_ = strings.Replace(s, old, new, n)
	_ = strings.Replace(s, old, new, 2)
	_ = strings.Replace(s, old, new, -1)
	_ = strings.ReplaceAll(s, old, new)

	// The same names on a non-strings method set.
	_ = fk.Replace(s, old, new, fk.Count(s, old))
	_ = strings.Replace(s, old, new, fk.Count(s, old))

	// strings.Count feeding anything but the n of the SAME Replace.
	_ = strings.Count(s, old)
}
