package ps5019neg

import (
	"bytes"
	"strings"
)

// A method set spelling the same names on a non-bytes receiver: the
// callee is not the standard library's package-level function.
type fake struct{}

func (fake) Replace(b, old, new []byte, n int) []byte { return b }
func (fake) Count(b, sep []byte) int                  { return 0 }

// None of these is the counted-replace-all pattern: no diagnostics at
// all.
func negatives(b, t, old, other, new []byte, s string, n int, fk fake) {
	// The cap is a Count over a DIFFERENT haystack: a genuine partial
	// replace, not the replace-all idiom.
	_ = bytes.Replace(b, old, new, bytes.Count(t, old))

	// ... or over a different needle.
	_ = bytes.Replace(b, old, new, bytes.Count(b, other))

	// ... or with the Count arguments swapped.
	_ = bytes.Replace(b, old, new, bytes.Count(old, b))

	// An arithmetic wrapper around the Count is not the exact cap.
	_ = bytes.Replace(b, old, new, bytes.Count(b, old)+1)
	_ = bytes.Replace(b, old, new, bytes.Count(b, old)-1)

	// A plain n, a literal, or -1 (PS-clean spellings) never match.
	_ = bytes.Replace(b, old, new, n)
	_ = bytes.Replace(b, old, new, 2)
	_ = bytes.Replace(b, old, new, -1)
	_ = bytes.ReplaceAll(b, old, new)

	// The same names on a non-bytes method set.
	_ = fk.Replace(b, old, new, fk.Count(b, old))
	_ = bytes.Replace(b, old, new, fk.Count(b, old))

	// A strings.Count cap on a bytes.Replace cannot even type-check
	// with the same operands, but the STRINGS spelling of the idiom is
	// PS5012's territory, not this check's.
	_ = strings.Replace(s, "a", "b", strings.Count(s, "a"))

	// bytes.Count feeding anything but the n of the SAME Replace.
	_ = bytes.Count(b, old)
}
