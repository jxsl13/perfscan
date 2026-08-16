package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2051 reports re.Match([]byte(s)) and re.MatchString(string(b)) on an
// already-compiled *regexp.Regexp — a whole-input conversion copy passed to the
// wrong twin method — where re.MatchString(s) / re.Match(b) scans the value in
// place. The regexp-method analog of PS2019/PS3004 (predicate over a converted
// operand): both drop one heap allocation plus a full O(len) copy of the input.
var PS2051 = register(&lint.Check{
	ID:       "PS2051",
	Category: "alloc",
	Slug:     "regexp-match-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "re.Match([]byte(s)) copies the whole input to feed the wrong twin; re.MatchString(s) scans it in place",
		Text: `re.Match([]byte(s)) converts the string s to a fresh []byte — a heap
allocation and a full O(len) copy of the ENTIRE subject on every call — purely to
satisfy Match's []byte parameter; re.MatchString(string(b)) does the mirror for a
[]byte b. *regexp.Regexp exposes Match([]byte) bool and MatchString(string) bool
that funnel to the exact same matcher over regexp's internal input abstraction
(inputBytes / inputString), so calling the twin that matches the value you
already hold scans it in place and drops the conversion:

  re.Match([]byte(s))       -> re.MatchString(s)
  re.MatchString(string(b)) -> re.Match(b)

The rewrite is BIT-IDENTICAL. Match and MatchString run the identical automaton
over the same bytes — decoding the same runes, mapping invalid UTF-8 to the same
RuneError — and return a bool, so the nil-vs-empty conversion trap that PS2109/
PS2136 guard cannot arise (there is no output slice/string). The subject is
evaluated exactly once in both forms, and the match work is unchanged.

The match is deliberately narrow — it is the whole safety story:
  - the callee is a METHOD (a receiver is present) named Match or MatchString on
    the standard-library *regexp.Regexp, with exactly one argument and no spread.
    The package-level regexp.Match(pattern, b) recompiles a pattern and takes two
    arguments — a different function (PS2131's territory) — and never matches;
  - Match's argument is exactly []byte(x) and MatchString's is exactly string(x),
    a single-argument conversion whose target is the predeclared []byte / string;
  - the CONVERTED operand x has type EXACTLY the predeclared string (for the ->
    MatchString direction) or an UNNAMED []byte (for the -> Match direction), so
    x is assignable to the twin method's parameter and the rewrite always
    type-checks. A named string type is convertible via []byte(S) but is NOT
    assignable to a string parameter, so it stays advisory.
A comment inside the unwrapped conversion scaffolding keeps the report advisory.`,
		Before: `if re.Match([]byte(s)) {`,
		After:  `if re.MatchString(s) {`,
		MeasuredWin: `On a 3 KB subject (Apple M2 Pro, go1.26): re.Match([]byte(s)) 3076 B/op, ` +
			`1 alloc/op vs re.MatchString(s) 3 B/op, 0 allocs/op — same ~30 us match time, ` +
			`the full input copy eliminated. A pure allocation win that scales with the ` +
			`subject length; the match work itself is unchanged.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2051",
		Doc:  "re.Match([]byte(s)) / re.MatchString(string(b)) instead of the twin method that avoids the input copy",
		Run:  runPS2051,
	},
})

func runPS2051(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// The callee must be the *regexp.Regexp method Match or MatchString
			// (a receiver present — the package-level regexp.Match takes two
			// arguments and never reaches here).
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != "regexp" {
				return true
			}
			sig, ok := fn.Type().(*types.Signature)
			if !ok || sig.Recv() == nil {
				return true
			}
			method := fn.Name()
			var newMethod string
			switch method {
			case "Match":
				newMethod = "MatchString"
			case "MatchString":
				newMethod = "Match"
			default:
				return true
			}
			// The argument must be exactly the matching single-arg conversion:
			// []byte(x) for Match, string(x) for MatchString.
			inner, toBytes, ok := ps2051Conv(pass, call.Args[0])
			if !ok || toBytes != (method == "Match") {
				return true
			}
			innerT := pass.TypesInfo.TypeOf(inner)
			if innerT == nil {
				return true
			}
			var fixable bool
			if method == "Match" {
				// []byte(x): x's underlying must be string for the conversion to
				// be a real copy (an already-[]byte operand is a no-op, not this
				// shape). The fix additionally needs x assignable to MatchString's
				// string parameter — EXACTLY the predeclared string; a named
				// string type is convertible but not assignable, so it stays
				// advisory.
				u, isBasic := types.Default(innerT).Underlying().(*types.Basic)
				if !isBasic || u.Info()&types.IsString == 0 {
					return true
				}
				b, isPlain := types.Default(innerT).(*types.Basic)
				fixable = isPlain && b.Kind() == types.String
			} else {
				// string(x): x's underlying must be a byte slice. The fix needs x
				// assignable to Match's []byte parameter — an UNNAMED []byte.
				sl, isSlice := innerT.Underlying().(*types.Slice)
				if !isSlice {
					return true
				}
				eb, okElem := sl.Elem().Underlying().(*types.Basic)
				if !okElem || eb.Kind() != types.Byte {
					return true
				}
				fixable = ps2141ByteSliceUnnamed(innerT)
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "re." + method + "(" + convKind(toBytes) + "(x)) copies the whole input to feed the wrong twin; re." + newMethod + "(x) scans it in place",
			}
			conv := ps2109Unparen(call.Args[0]).(*ast.CallExpr)
			if fixable &&
				!ps2109CommentBetween(f, conv.Pos(), inner.Pos()) &&
				!ps2109CommentBetween(f, inner.End(), conv.End()) {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with re." + newMethod + "(x)",
					TextEdits: []analysis.TextEdit{
						// Rename the method ...
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte(newMethod)},
						// ... and unwrap the conversion around the operand.
						{Pos: conv.Pos(), End: inner.Pos()},
						{Pos: inner.End(), End: conv.End()},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

func convKind(toBytes bool) string {
	if toBytes {
		return "[]byte"
	}
	return "string"
}

// ps2051Conv reports whether e is a single-argument conversion to the
// predeclared []byte (toBytes=true) or string (toBytes=false), returning the
// converted operand. A function call, a named target type, and a spread are all
// rejected.
func ps2051Conv(pass *analysis.Pass, e ast.Expr) (inner ast.Expr, toBytes, ok bool) {
	call, isCall := ps2109Unparen(e).(*ast.CallExpr)
	if !isCall || len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return nil, false, false
	}
	tv, isType := pass.TypesInfo.Types[call.Fun]
	if !isType || !tv.IsType() {
		return nil, false, false
	}
	switch t := types.Unalias(tv.Type).(type) {
	case *types.Basic:
		if t.Kind() == types.String {
			return call.Args[0], false, true
		}
	case *types.Slice:
		if b, ok := t.Elem().(*types.Basic); ok && b.Kind() == types.Byte {
			return call.Args[0], true, true
		}
	}
	return nil, false, false
}
