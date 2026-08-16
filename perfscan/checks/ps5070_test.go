package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

// PS5070 is an AutoFix check: fixes are applied and diffed against the
// .go.golden fixture. The suite covers each recognized receiver
// (bytes.Buffer, strings.Builder via an auto-addressed value,
// bufio.Writer, an interface declaring both methods), a named slice of
// byte (assignable to Write's []byte), a sliced argument left in place,
// the comment-in-conversion advisory, and the guards that must stay
// silent: a WriteString-only type, a plain-string argument, a []rune
// argument, the Write-delegates-to-WriteString self-dispatch (which would
// recurse), and a field write inside Write (a different object, still
// reported). See equiv_PS5070_test.go for the runtime proof that
// WriteString(string(b)) and Write(b) write the identical bytes.
func TestPS5070(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5070.Analyzer, "ps5070")
}
