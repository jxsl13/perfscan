package ps5075

import (
	"bytes"
	"math"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

func stringLower(s string) string {
	return strings.ToLower(strings.ToLower(s)) // want `strings\.ToLower is applied 2 times`
}

func byteSpace(b []byte) []byte {
	return bytes.TrimSpace(bytes.TrimSpace(b)) // want `bytes\.TrimSpace is applied 2 times`
}

func cleanPath(p string) string {
	return path.Clean(path.Clean(p)) // want `path\.Clean is applied 2 times`
}

func cleanFilepath(p string) string {
	return filepath.Clean(filepath.Clean(p)) // want `path/filepath\.Clean is applied 2 times`
}

func rounded(x float64) float64 {
	return math.Round(math.Round(math.Round(x))) // want `math\.Round is applied 3 times`
}

func lowerRune(r rune) rune {
	return unicode.ToLower(unicode.ToLower(r)) // want `unicode\.ToLower is applied 2 times`
}

func compact(xs []int) []int {
	return slices.Compact[[]int](slices.Compact[[]int](xs)) // want `slices\.Compact is applied 2 times`
}

// A comment in deleted scaffolding withholds the fix.
func commented(s string) string {
	return strings.TrimSpace( /* retain */ strings.TrimSpace(s)) // want `strings\.TrimSpace is applied 2 times`
}

// --- negatives ---

func single(s string) string { return strings.ToUpper(s) }

func crossCase(s string) string { return strings.ToLower(strings.ToUpper(s)) }

func Clean(s string) string { return s }

func userClean(s string) string { return Clean(Clean(s)) }

type cleaner string

func (c cleaner) Clean() cleaner { return c }

func method(c cleaner) cleaner { return c.Clean().Clean() }

type namedInts []int

// Removing the outer generic call would change the interface's dynamic type
// from []int to namedInts.
func mixedGenericResult(xs namedInts) any {
	return slices.Compact[[]int](slices.Compact[namedInts](xs))
}
