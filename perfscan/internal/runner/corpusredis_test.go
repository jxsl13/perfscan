package runner

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFixRedisRegexpHoistOutOfLoop pins a shape from corpus -fix validation
// (redis/go-redis command.go, part of 22 bit-identical fixes that built clean):
// PS2005 hoists a regexp.MustCompile of a constant pattern OUT of an enclosing
// loop to a uniquely-named local declared immediately before the loop, and
// aliases the original site to it — so the pattern is compiled ONCE instead of
// per iteration. The rewrite is behavior-preserving (the pattern is invariant
// and *regexp.Regexp is safe for concurrent reuse).
//
// Locks: exactly one MustCompile remains (compiled once, not per-iteration), the
// hoist is placed BEFORE the for-loop, and the result compiles.
func TestFixRedisRegexpHoistOutOfLoop(t *testing.T) {
	const src = `package p

import "regexp"

func parse(lines []string) []string {
	var out []string
	for _, ln := range lines {
		re := regexp.MustCompile(` + "`name=(.+?),(.+)$`" + `)
		out = append(out, re.FindString(ln))
	}
	return out
}
`
	got := string(runFixMode(t, src))

	if n := strings.Count(got, "regexp.MustCompile("); n != 1 {
		t.Errorf("MustCompile should appear exactly once after the hoist, got %d:\n%s", n, got)
	}
	// The single MustCompile must sit ABOVE the loop, not inside it.
	compileAt := strings.Index(got, "regexp.MustCompile(")
	forAt := strings.Index(got, "for _, ln := range")
	if compileAt < 0 || forAt < 0 || compileAt > forAt {
		t.Errorf("the hoisted MustCompile must be placed before the for-loop:\n%s", got)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "p.go", got, 0); err != nil {
		t.Errorf("fixed file does not parse: %v\n%s", err, got)
	}
}
