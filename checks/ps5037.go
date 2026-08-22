package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5037 reports utf8.RuneCountInString(s) / utf8.RuneCount(b) whose int
// result is only compared against the literal 0 (or the equivalent
// 1-boundary) to answer an EMPTINESS question — "is there anything here
// at all?" — where len(s) / len(b) gives the identical boolean in O(1)
// instead of decoding every rune of the input. The rune-count emptiness
// sibling of PS2027 (buf.String() == "" -> buf.Len() == 0): the same
// O(n) -> O(1) motif, applied to the unicode/utf8 counting pair.
var PS5037 = register(&lint.Check{
	ID:       "PS5037",
	Category: "arith",
	Slug:     "runecount-emptiness-to-len",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "utf8.RuneCountInString/RuneCount compared against 0 scans the whole input just to test emptiness; len(...) is the O(1) bit-identical test",
		Text: `utf8.RuneCountInString(s) and utf8.RuneCount(b)
unconditionally walk the ENTIRE input — the ASCII fast path advances
byte by byte (with a word-at-a-time assist) and the general path
decodes every rune — to produce an exact count. A comparison against 0
throws that exactness away: it only asks whether the input is empty,
and len(s) / len(b) answers the same question by reading the
string/slice header's length word. For a non-empty argument the scan
walks every byte just to return a value >= 1 that len would imply for
free — a genuine O(n) -> O(1) work reduction, not a readability
rewrite. The win is scan time and grows with the input; len never
allocates, and neither does RuneCountInString — while RuneCount on
current gc even pays a heap copy of the tail from the first non-ASCII
byte (it re-materializes string(p[n:]); the artifact PS2024 documents
and measures), which the rewrite removes as a bonus.

Exactly six comparison shapes are emptiness-equivalent, and only those
are matched (with the literal on either side of the operator):

	RuneCountInString(s) == 0  →  len(s) == 0
	RuneCountInString(s) != 0  →  len(s) != 0
	RuneCountInString(s) >  0  →  len(s) >  0
	RuneCountInString(s) <= 0  →  len(s) <= 0
	RuneCountInString(s) >= 1  →  len(s) >= 1
	RuneCountInString(s) <  1  →  len(s) <  1

(and identically for utf8.RuneCount on a []byte). The predicate is
total and bit-identical: an empty input has 0 runes and len 0, and ANY
non-empty input has at least one rune AND len >= 1 — including invalid
UTF-8, where the decode loop yields exactly one utf8.RuneError rune of
width 1 for every erroneous byte, so a non-empty input can never count
0 runes (both counters are >= 1 exactly when len >= 1; nil and empty
[]byte both count 0). Each of the six (operator, literal) pairs
therefore evaluates identically on both sides for every input. The fix
replaces ONLY the callee text (utf8.RuneCountInString -> len): the
argument expression stays byte-verbatim in place and is evaluated
exactly once on both sides (side effects preserved in count and
order), and the comparison node itself — operator, literal, and its
context-adopted type (a NAMED bool context included) — is untouched,
so nothing around the expression can change meaning. Comparisons
against any other constant (== 1 asks "exactly one RUNE", which is NOT
"one byte"; > 1, == 2, ...) genuinely use the rune count and are never
touched, as are results bound to a variable, compared against a
non-literal, or used arithmetically.

The automatic fix applies only when type information proves the callee
is the standard library's unicode/utf8 function (through any import
alias or a dot import; a shadowed identifier or same-named local never
matches) and the predeclared builtin len is not shadowed at the call
site. It is withheld (the report stays advisory) when the argument is
a CONSTANT string — len("...") is a compile-time constant while the
call is not, which can change compile-time properties (duplicate
switch cases; the PS2010 stance) — when a comment sits inside the
replaced callee text, or when the rewrite would orphan an import the
fix pipeline cannot prune: a dot-imported unicode/utf8 whose last
reference is the matched call, or a qualified last reference inside a
cgo file (whose import block must not be edited).

Two degenerate shapes stay silent entirely. utf8.RuneCount(nil) with
the untyped nil literal is skipped — len(nil) does not compile, and
the comparison is statically decidable anyway. And an argument that is
itself PS2024's throwaway conversion (utf8.RuneCount([]byte(s)) on a
plain string, utf8.RuneCountInString(string(b)) on a plain []byte) is
PS2024's site: it rewrites the call to the zero-copy sibling, and this
check picks the sibling up on a later pass — the same composition
class as the existing PS2125 -> PS2024 chain — while reporting both
here would attach two overlapping edits to one callee.

An honest caveat: this exact idiom is uncommon — rune-count emptiness
tests mostly arise from mechanical translations of other languages'
"count == 0" habits. Its value is closing the emptiness-test family
(PS2027's motif) so the O(n) spelling cannot survive a -fix run.`,
		Before: `if utf8.RuneCountInString(s) == 0 {
	return errEmpty
}
ok := utf8.RuneCount(b) > 0`,
		After: `if len(s) == 0 {
	return errEmpty
}
ok := len(b) > 0`,
		MeasuredWin: `BenchmarkPS5037 (Apple M2 Pro, go1.26): mixed-width
~4.3KB string — utf8.RuneCountInString(s) == 0 at 3286 ns/op vs
len(s) == 0 at 0.40 ns/op (~8000x); pure-ASCII ~3.5KB string (the
word-at-a-time fast path) — 1142 ns/op vs 0.40 ns/op (~2900x); the
[]byte twin on the mixed bytes — utf8.RuneCount(b) > 0 at 3602 ns/op,
4864 B/op, 1 allocs/op (RuneCount's non-ASCII tail copy on current gc)
vs len(b) > 0 at 0.39 ns/op, 0 B/op, 0 allocs/op. The win is the whole
scan (plus that allocation) and scales with the input.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5037",
		Doc:  "utf8.RuneCountInString/RuneCount compared against 0 for emptiness instead of the O(1) len test",
		Run:  runPS5037,
	},
})

func runPS5037(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			bin   *ast.BinaryExpr
			fun   ast.Expr // callee node to replace with "len" (SelectorExpr or bare Ident)
			isSel bool
			msg   string
			fixOK bool
		}
		var sites []site
		selFixable, totalFixable := 0, 0
		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			switch bin.Op {
			case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			default:
				return true
			}
			call, fun, name, litVal, litOnLeft, ok := ps5037Match(pass, bin)
			if !ok {
				return true
			}
			// Normalize to the call-on-left spelling: `0 < RuneCount(...)`
			// means `RuneCount(...) > 0`, so mirror the operator when the
			// literal is on the left. The fix itself is side-agnostic — it
			// only renames the callee — so no edit depends on this.
			op := bin.Op
			if litOnLeft {
				op = ps5104Mirror(op)
			}
			if !ps5037Emptiness(op, litVal) {
				// == 1, > 1, >= 0, < 0, ... genuinely use the rune count
				// (or are constant) — not emptiness.
				return true
			}
			arg := call.Args[0]
			// utf8.RuneCount(nil): len(nil) does not compile, and the
			// comparison is statically decidable — stay silent.
			if t := pass.TypesInfo.TypeOf(arg); t != nil {
				if b, isBasic := t.(*types.Basic); isBasic && b.Kind() == types.UntypedNil {
					return true
				}
			}
			// PS2024's throwaway-conversion shapes are its sites: it
			// rewrites the call to the zero-copy sibling first, and this
			// check picks the sibling up on a later pass (the same
			// composition class as the existing PS2125 -> PS2024 chain).
			// Reporting here too would attach two overlapping edits to
			// one callee.
			if ps5037PS2024Shape(pass, name, arg) {
				return true
			}
			kind := "string"
			if name == "RuneCount" {
				kind = "[]byte"
			}
			cmp := op.String() + " " + strconv.FormatInt(litVal, 10)
			msg := "utf8." + name + "(...) " + cmp + " scans the entire " + kind +
				" just to test emptiness; len(" + ps2125ExprText(arg) + ") " + cmp +
				" is the bit-identical O(1) test"
			// A CONSTANT string argument would make len(...) a
			// compile-time constant while the call is not — which can
			// change compile-time properties (duplicate switch cases; the
			// PS2010 stance). A comment inside the replaced callee text
			// would be destroyed by the edit. A shadowed len would capture
			// the rewrite. All three downgrade the report to advisory.
			argTV := pass.TypesInfo.Types[arg]
			fixOK := argTV.Value == nil &&
				!ps2111CommentIn(f, fun.Pos(), fun.End()) &&
				ps5037LenUsable(pass, call)
			_, isSel := fun.(*ast.SelectorExpr)
			if fixOK {
				totalFixable++
				if isSel {
					selFixable++
				}
			}
			sites = append(sites, site{bin, fun, isSel, msg, fixOK})
			return true
		})
		if len(sites) == 0 {
			continue
		}
		// Import accounting. A qualified fix removes one utf8 reference;
		// when the last one goes, the fix pipeline prunes the import —
		// except in a cgo file, whose import block must not be edited
		// (the PS2010 stance). A DOT import is never pruned by the
		// pipeline at all, so a fix consuming the file's last reference
		// to any unicode/utf8 object would leave an "imported and not
		// used" error — withhold every fix in the file then.
		emitFixes := pkgRefCount(pass, f, "unicode/utf8") > selFixable || !ps2110ImportsC(f)
		if ps5037DotUtf8(f) && ps5037Utf8ObjRefCount(pass, f) <= totalFixable {
			emitFixes = false
		}
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.bin.Pos(),
				End:     st.bin.End(),
				Message: st.msg,
			}
			if st.fixOK && emitFixes {
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with the O(1) len(...) emptiness test",
					TextEdits: []analysis.TextEdit{
						{Pos: st.fun.Pos(), End: st.fun.End(), NewText: []byte("len")},
					},
				}}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5037Match reports whether bin compares a direct utf8.RuneCount /
// utf8.RuneCountInString call against the literal integer constant 0 or
// 1, returning the call, the callee node the fix replaces (the whole
// SelectorExpr, so an import alias vanishes with it, or the bare ident
// under a dot import), the function name, the literal's value, and
// which side the literal is on.
func ps5037Match(pass *analysis.Pass, bin *ast.BinaryExpr) (call *ast.CallExpr, fun ast.Expr, name string, litVal int64, litOnLeft, ok bool) {
	if c, fn, nm := ps5037CountCall(pass, bin.X); c != nil {
		if v, isLit := ps5037ZeroOneLit(pass, bin.Y); isLit {
			return c, fn, nm, v, false, true
		}
	}
	if c, fn, nm := ps5037CountCall(pass, bin.Y); c != nil {
		if v, isLit := ps5037ZeroOneLit(pass, bin.X); isLit {
			return c, fn, nm, v, true, true
		}
	}
	return nil, nil, "", 0, false, false
}

// ps5037CountCall returns e as a direct (possibly parenthesized) call of
// unicode/utf8's RuneCount or RuneCountInString, together with the
// callee node whose text the fix replaces. Type information pins the
// callee to the standard library (via ps2024Utf8Callee): a shadowed
// utf8 identifier, a same-named local function, or a function value
// stored in a variable resolves to a different object and is rejected.
func ps5037CountCall(pass *analysis.Pass, e ast.Expr) (call *ast.CallExpr, fun ast.Expr, name string) {
	c, ok := ps2108Unparen(e).(*ast.CallExpr)
	if !ok || len(c.Args) != 1 || c.Ellipsis.IsValid() {
		return nil, nil, ""
	}
	nameID := ps2024Utf8Callee(pass, c)
	if nameID == nil || (nameID.Name != "RuneCount" && nameID.Name != "RuneCountInString") {
		return nil, nil, ""
	}
	return c, ps2108Unparen(c.Fun), nameID.Name
}

// ps5037ZeroOneLit reports whether e is (possibly parenthesized) a
// direct integer literal of value 0 or 1, returning that value. A
// variable or named constant holding 0 is deliberately NOT matched: the
// check only rewrites the spelling that is provably an emptiness test
// against the literal.
func ps5037ZeroOneLit(pass *analysis.Pass, e ast.Expr) (int64, bool) {
	e = ps2108Unparen(e)
	lit, ok := e.(*ast.BasicLit)
	if !ok || lit.Kind != token.INT {
		return 0, false
	}
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Value == nil || tv.Value.Kind() != constant.Int {
		return 0, false
	}
	v, exact := constant.Int64Val(tv.Value)
	if !exact || (v != 0 && v != 1) {
		return 0, false
	}
	return v, true
}

// ps5037Emptiness classifies the normalized (call-on-left) comparison
// `count <op> <lit>` as one of the six emptiness forms. Any other
// (op, literal) pair uses the actual rune count (== 1 is "exactly one
// RUNE", which len cannot answer) or is constant (>= 0, < 0) and is not
// matched.
func ps5037Emptiness(op token.Token, v int64) bool {
	switch v {
	case 0:
		return op == token.EQL || op == token.NEQ || op == token.GTR || op == token.LEQ
	case 1:
		return op == token.GEQ || op == token.LSS
	}
	return false
}

// ps5037PS2024Shape reports whether arg is exactly the throwaway
// conversion PS2024 auto-fixes on this callee: []byte(x) of a plain
// string under RuneCount, or string(b) of a plain []byte under
// RuneCountInString. Those calls are PS2024's to rewrite (to the
// zero-copy sibling); this check stays silent there and picks the
// sibling up on a later pass. A NAMED operand is not PS2024's shape —
// PS2024 skips it entirely — so the plain len rewrite here stands (and
// cannot churn: PS2125's byte arm requires a plain-string operand, and
// no check matches len(string(...))).
func ps5037PS2024Shape(pass *analysis.Pass, name string, arg ast.Expr) bool {
	switch name {
	case "RuneCount":
		conv, isRune := ps2125Conversion(pass, ps2108Unparen(arg))
		if conv == nil || isRune {
			return false
		}
		xt := pass.TypesInfo.TypeOf(conv.Args[0])
		return xt != nil && types.Identical(types.Default(xt), types.Typ[types.String])
	case "RuneCountInString":
		conv := ps2024StringConv(pass, ps2108Unparen(arg))
		return conv != nil && ps2022IsPlainByteSlice(pass, conv.Args[0])
	}
	return false
}

// ps5037LenUsable reports whether the predeclared builtin len is not
// shadowed at the call site — a local `len` would capture the rewrite.
func ps5037LenUsable(pass *analysis.Pass, call *ast.CallExpr) bool {
	scope := pass.Pkg.Scope().Innermost(call.Pos())
	if scope == nil {
		return false
	}
	_, obj := scope.LookupParent("len", call.Pos())
	return obj == types.Universe.Lookup("len")
}

// ps5037DotUtf8 reports whether f dot-imports unicode/utf8. The fix
// pipeline never prunes a dot import, so a fix consuming the file's
// last reference to a unicode/utf8 object must be withheld.
func ps5037DotUtf8(f *ast.File) bool {
	for _, imp := range f.Imports {
		if imp.Name != nil && imp.Name.Name == "." && imp.Path.Value == `"unicode/utf8"` {
			return true
		}
	}
	return false
}

// ps5037Utf8ObjRefCount counts identifiers in f that resolve to an
// object DECLARED in unicode/utf8 (functions, constants — not the
// package qualifier itself, which pkgRefCount covers). This is the
// usage a dot import needs to stay legal.
func ps5037Utf8ObjRefCount(pass *analysis.Pass, f *ast.File) int {
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj := pass.TypesInfo.Uses[id]
		if obj == nil {
			return true
		}
		if _, isPkg := obj.(*types.PkgName); isPkg {
			return true
		}
		if obj.Pkg() != nil && obj.Pkg().Path() == "unicode/utf8" {
			n++
		}
		return true
	})
	return n
}
