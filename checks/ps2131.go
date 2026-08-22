package checks

import (
	"go/ast"
	"go/constant"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2131 reports the package-level regexp convenience functions
// (regexp.MatchString / Match / MatchReader) called with a compile-time
// constant pattern — each of which recompiles the regexp on every call.
var PS2131 = register(&lint.Check{
	ID:       "PS2131",
	Category: "alloc",
	Slug:     "regexp-match-recompiles",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "regexp.MatchString/Match/MatchReader recompiles the pattern on every call",
		Text: `The package-level regexp helpers regexp.MatchString(pattern, s),
regexp.Match(pattern, b) and regexp.MatchReader(pattern, r) are pure
convenience: each one Compile()s the pattern from scratch, runs the match, and
throws the compiled program away. In a hot path — or any code called more than
once — that recompilation dominates: a single regexp.MatchString call over a
short input measures ~2000 ns and ~56 allocations, versus ~115 ns and ZERO
allocations for a package-level regexp.MustCompile(pattern) whose MatchString
method is reused (~17x faster).

Only a COMPILE-TIME CONSTANT pattern is reported: it can be lifted to a
package-level

    var re = regexp.MustCompile(pattern)

and every call site becomes re.MatchString(s) / re.Match(b) / re.MatchReader(r).
A pattern built at runtime is genuinely dynamic and left alone.

DELIBERATELY advisory — no automatic fix. The helpers return (bool, error) and
report a malformed pattern through that error; the reused-*Regexp form returns
only the bool (MustCompile panics on a bad pattern at init instead). Collapsing
the two return values and introducing a package-level var is a restructuring a
human should place and name, not a mechanical in-place edit — the same reason
PS2005/PS2127 own the in-loop and in-function regexp.MustCompile hoists.`,
		Before: `if ok, _ := regexp.MatchString("^[a-z]+$", s); ok {
	// recompiles "^[a-z]+$" on every call
}`,
		After: `var lowerRe = regexp.MustCompile("^[a-z]+$") // package scope, compiled once

// ... then:
if lowerRe.MatchString(s) {
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2131",
		Doc:  "regexp.MatchString/Match/MatchReader with a constant pattern recompiles every call",
		Run:  runPS2131,
	},
})

var regexpMatchFuncs = map[string]bool{"MatchString": true, "Match": true, "MatchReader": true}

func runPS2131(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 {
				return true
			}
			name, ok := astutil.PkgFuncCall(pass.TypesInfo, call.Fun, "regexp", regexpMatchFuncs)
			if !ok {
				return true
			}
			// Only a compile-time constant string pattern is actionable: it can
			// be hoisted to a package-level MustCompile. A runtime-built pattern
			// is genuinely dynamic.
			v := pass.TypesInfo.Types[call.Args[0]].Value
			if v == nil || v.Kind() != constant.String {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "regexp." + name + " with a constant pattern recompiles the regexp on every call (~17x slower, dozens of allocations); hoist `var re = regexp.MustCompile(pattern)` to package scope and reuse re." + name + " — advisory: the (bool, error) return collapses to the reused-regexp form, so it is not applied automatically",
			})
			return true
		})
	}
	return nil, nil
}
