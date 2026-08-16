package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// ps5046FindToMatch maps a nil-returning single-argument *regexp.Regexp Find
// method (whose result is only compared against nil for match presence) to the
// boolean twin that answers the same question without allocating the result.
var ps5046FindToMatch = map[string]string{
	"Find":                    "Match",
	"FindIndex":               "Match",
	"FindSubmatch":            "Match",
	"FindSubmatchIndex":       "Match",
	"FindStringIndex":         "MatchString",
	"FindStringSubmatch":      "MatchString",
	"FindStringSubmatchIndex": "MatchString",
}

// PS5046 reports re.FindStringIndex(s) != nil and its Find/Index/Submatch
// siblings used purely as a match-presence test — where re.MatchString(s) /
// re.Match(b) answers the same boolean without allocating the returned index or
// submatch slice. The regexp-method presence-test analog of PS5031/PS5104
// (Index/Count compared for membership).
var PS5046 = register(&lint.Check{
	ID:       "PS5046",
	Category: "alloc",
	Slug:     "regexp-find-nil-to-match",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "re.FindStringIndex(s) != nil allocates an index/submatch slice just to test for a match; re.MatchString(s) returns the same bool with none",
		Text: `The single-argument *regexp.Regexp Find methods — Find, FindIndex,
FindSubmatch, FindSubmatchIndex and their FindString* twins — return a freshly
allocated []int / [][]byte / []string on a match and nil on no match. Comparing
that result against nil (== nil or != nil) is only asking "did the pattern
match?" — for which re.Match(b) / re.MatchString(s) return the identical bool
running the same automaton, without allocating (or filling) the result slice.

The rewrite is BIT-IDENTICAL. Every one of these Find methods returns non-nil
exactly when the pattern matches the subject and nil otherwise — including the
empty-match case, where it returns a non-nil (possibly empty) slice while Match
returns true — so Find*(x) != nil equals Match/MatchString(x) and Find*(x) == nil
equals its negation, for every subject. The subject is evaluated exactly once in
both forms, and each Find method's parameter type already matches its Match twin
(string -> MatchString, []byte -> Match), so no conversion is introduced.

The match is deliberately narrow: a comparison (== or !=) between nil and a
single-argument call of one of these methods on the standard-library
*regexp.Regexp, where the nil operand is the predeclared nil. The fix renames the
method to its Match twin, drops the nil comparison, and — for the == nil arm,
which means "no match" — prefixes a single ! (which binds tighter than any
surrounding operator, so no parentheses are needed). A comment between the call
and nil keeps the report advisory.`,
		Before: `if re.FindStringIndex(s) != nil {`,
		After:  `if re.MatchString(s) {`,
		MeasuredWin: `On a 384-byte subject (Apple M2 Pro, go1.26): re.FindStringIndex(s) != nil ` +
			`76 ns/op, 16 B/op, 1 alloc/op vs re.MatchString(s) 58 ns/op, 0 B/op, 0 allocs/op — ` +
			`the index slice (and, for the Submatch forms, a [][]byte/[]string) is allocated ` +
			`and filled only to be discarded by the nil test; MatchString runs the same match ` +
			`with none.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5046",
		Doc:  "re.Find*(x) compared against nil for match presence instead of re.Match/MatchString(x)",
		Run:  runPS5046,
	},
})

func runPS5046(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok || (bin.Op != token.EQL && bin.Op != token.NEQ) {
				return true
			}
			// Identify which side is the call and which is the nil operand.
			var call *ast.CallExpr
			var callOnLeft bool
			var other ast.Expr
			if c, ok := bin.X.(*ast.CallExpr); ok {
				call, callOnLeft, other = c, true, bin.Y
			} else if c, ok := bin.Y.(*ast.CallExpr); ok {
				call, callOnLeft, other = c, false, bin.X
			} else {
				return true
			}
			if len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			if !ps2032IsUntypedNil(pass, other) {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "regexp" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() == nil {
				return true
			}
			newMethod, known := ps5046FindToMatch[fn.Name()]
			if !known {
				return true
			}
			// == nil means "no match" -> negate the Match twin.
			neg := bin.Op == token.EQL
			diag := analysis.Diagnostic{
				Pos:     bin.Pos(),
				End:     bin.End(),
				Message: "re." + fn.Name() + "(x) " + bin.Op.String() + " nil allocates an index/submatch slice just to test for a match; re." + newMethod + "(x) returns the same bool with none",
			}
			// The comment guard covers the deleted nil-comparison span (between
			// the call and nil, in whichever order).
			var commentClear bool
			if callOnLeft {
				commentClear = !ps2109CommentBetween(f, call.End(), bin.End())
			} else {
				commentClear = !ps2109CommentBetween(f, bin.Pos(), call.Pos())
			}
			if commentClear {
				edits := []analysis.TextEdit{
					{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(newMethod)},
				}
				if callOnLeft {
					// Drop " <op> nil" after the call; prepend "!" for == nil.
					edits = append(edits, analysis.TextEdit{Pos: call.End(), End: bin.End()})
					if neg {
						edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: call.Pos(), NewText: []byte("!")})
					}
				} else {
					// Drop "nil <op> " before the call, replacing it with "!"
					// (== nil) or nothing (!= nil).
					repl := []byte(nil)
					if neg {
						repl = []byte("!")
					}
					edits = append(edits, analysis.TextEdit{Pos: bin.Pos(), End: call.Pos(), NewText: repl})
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message:   "replace with re." + newMethod + "(x)",
					TextEdits: edits,
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}
