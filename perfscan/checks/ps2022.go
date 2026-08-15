package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2022 reports bytes.Equal(b, []byte(s)) — and the mirror
// bytes.Equal([]byte(s), b) — where b is a plain []byte and s a plain
// string: the direct comparison string(b) == s is bit-identical and
// deletes the string->[]byte conversion outright. It covers exactly the
// SINGLE-conversion shape PS2010 (both operands conversions) does not.
var PS2022 = register(&lint.Check{
	ID:       "PS2022",
	Category: "alloc",
	Slug:     "bytes-equal-one-string-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Equal of a []byte and one []byte(string) conversion is string(b) == s with a conversion deleted",
		Text: `bytes.Equal(b, []byte(s)) with b a plain []byte and s a plain string
spells the comparison string(b) == s through a string->[]byte
conversion. In the rewrite, string(b) used directly as an operand of ==
compiles to a GUARANTEED no-copy view of b on gc (the OBYTES2STRTMP
form) — unconditionally, with no escape-analysis proof needed — so the
After never copies or allocates: one length check plus memequal.

An honest caveat: current gc (verified on Go 1.26) compiles the Before
allocation-free too — bytes.Equal does not retain or mutate its
arguments, so escape analysis turns []byte(s) into a zero-copy view and
the measured difference is nil (see below). The rewrite moves that
guarantee from a conditional compiler optimization into the source: the
Before's elision fails whenever escape analysis loses the proof (an
indirect call to Equal, the conversion refactored out of the
non-escaping argument slot) and never holds on toolchains without the
zero-copy conversion (gccgo, tinygo, older gc) — each of those pays a
heap allocation plus an O(len(s)) copy per call. The After deletes the
conversion outright, so it is copy-free by construction, and states the
intent — "do these bytes spell this string" — directly.

The rewrite is bit-identical by the standard library's own definition:
bytes.Equal(a, b) is documented and implemented as
string(a) == string(b), and string([]byte(s)) == s for every string s
(a string round-tripped through []byte is byte-identical, including
invalid UTF-8 and the empty string), so
bytes.Equal(b, []byte(s)) == (string(b) == s) on ALL inputs — including
b == nil, where string(nil-[]byte) is "". Both forms compare raw bytes
with no rune interpretation. Each operand is evaluated exactly once, in
the original left-to-right order, in both forms (b then s for
bytes.Equal(b, []byte(s)) -> string(b) == s; s then b for the mirror
bytes.Equal([]byte(s), b) -> s == string(b)), so side effects are
preserved in count and order. There is no both-constant hazard (the
PS2010/PS2108 advisory case): a []byte operand is never a compile-time
constant, so the replacement comparison is never constant either.

The automatic fix rewrites the call to string(b) == s (mirror:
s == string(b)). It applies only when type information proves the shape:

  - the callee resolves to the standard library's bytes.Equal (a
    shadowed bytes identifier or a same-named method does not match);
  - EXACTLY ONE argument is a conversion whose target is exactly the
    predeclared []byte (an unnamed slice of the predeclared byte; a
    defined slice type is not matched) and whose operand's static type
    is exactly the predeclared string — a NAMED string type is not
    reported at all, because string(b) == s would then compare under the
    wrong type's semantics (when both arguments are such conversions the
    call belongs to PS2010, which removes both);
  - the OTHER argument's static type is exactly the predeclared []byte —
    a defined byte-slice type is not reported (its comparison semantics
    belong to that type), and an untyped nil is not reported (string(nil)
    does not compile);
  - the predeclared identifier string is not shadowed at the call site
    (a local named string would capture the conversion in the
    replacement) — there the report stays advisory.

The replacement is parenthesized when the surrounding expression binds
tighter than == (for example !bytes.Equal(b, []byte(s)) becomes
!(string(b) == s)); in && / || and delimited contexts no parentheses are
added. In a context that syntactically requires a call — a bare
expression statement, go, or defer — the rewrite would not compile, so
the report stays advisory. A comment inside the replaced call withholds
the fix, and in a cgo file — whose import block is never pruned — the
fix is withheld when it would orphan the bytes import.`,
		Before: `if bytes.Equal(b, []byte(s)) {
	return true
}`,
		After: `if string(b) == s {
	return true
}`,
		MeasuredWin: `BenchmarkPS2022 (one pair sharing a 2944-byte prefix
plus one length-mismatch pair, Apple M2 Pro, Go 1.26): 63.0 ns/op,
0 allocs/op -> 60.4 ns/op, 0 allocs/op — parity, because gc's zero-copy
conversion already elides the allocation in this exact shape. The After
is guaranteed copy-free by construction (string(b) beside == is compiled
as a no-copy view unconditionally) rather than by escape analysis; on
toolchains without the zero-copy conversion the Before pays a full heap
copy of s per call.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2022",
		Doc:  "bytes.Equal of a []byte and one []byte(string) conversion equals string(b) == s without the conversion",
		Run:  runPS2022,
	},
})

func runPS2022(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		var stack []ast.Node
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				stack = stack[:len(stack)-1]
				return true
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				stack = append(stack, n)
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			repl, ok := ps2022Match(pass, call)
			if !ok {
				stack = append(stack, n)
				return true
			}
			fixOK := true
			// A call is a legal statement where a binary expression is not:
			// a bare expression statement, go, and defer syntactically
			// require a call. There the rewrite would not compile — the
			// report stays advisory. ParenExpr ancestors are skipped: a
			// parenthesized statement expression is just as call-only.
			for i := len(stack) - 1; i >= 0; i-- {
				if _, isParen := stack[i].(*ast.ParenExpr); isParen {
					continue
				}
				switch stack[i].(type) {
				case *ast.ExprStmt, *ast.GoStmt, *ast.DeferStmt:
					fixOK = false
				}
				break
			}
			// The replacement spells a conversion with the predeclared
			// identifier string; a local declaration shadowing it at the
			// call site would capture that identifier — withhold the fix.
			if fixOK {
				scope := pass.Pkg.Scope().Innermost(call.Pos())
				if scope == nil {
					fixOK = false
				} else if _, o := scope.LookupParent("string", call.Pos()); o != types.Universe.Lookup("string") {
					fixOK = false
				}
			}
			var fix *analysis.SuggestedFix
			// A comment inside the replaced call would be silently dropped
			// by the re-rendered expression — withhold the fix there.
			if fixOK && repl != "" && !ps2111CommentIn(f, call.Pos(), call.End()) {
				if ps2010NeedsParens(parent, call) {
					repl = "(" + repl + ")"
				}
				fix = &analysis.SuggestedFix{
					Message:   "replace with " + repl,
					TextEdits: []analysis.TextEdit{{Pos: call.Pos(), End: call.End(), NewText: []byte(repl)}},
				}
				fixable++
			}
			sites = append(sites, site{call, fix})
			// Do not descend: a nested match inside the replaced operands
			// would produce overlapping edits.
			return false
		})
		// Removing bytes.Equal may orphan the bytes import; the fix
		// pipeline prunes it — except in a cgo file, whose import block
		// must not be edited. There, withhold the fixes only when they
		// would orphan bytes (no other bytes reference survives).
		emitFixes := pkgRefCount(pass, f, "bytes") > fixable || !ps2110ImportsC(f)
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "bytes.Equal with one []byte(s) conversion spells string(b) == s through a throwaway byte-slice copy; the direct comparison is bit-identical (bytes.Equal is defined as string(a) == string(b)) and copy-free by construction",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2022Match matches call against bytes.Equal with EXACTLY ONE
// []byte(s) string-conversion argument whose other argument is a plain
// []byte, and returns the rendered replacement expression (without outer
// parentheses). Both-conversion calls are PS2010's and are not matched —
// the two checks never double-report. ok is false when the shape does
// not match.
func ps2022Match(pass *analysis.Pass, call *ast.CallExpr) (repl string, ok bool) {
	if !ps2010IsBytesEqual(pass, call) {
		return "", false
	}
	s0, _, conv0 := ps2010StringConvOperand(pass, call.Args[0])
	s1, _, conv1 := ps2010StringConvOperand(pass, call.Args[1])
	if conv0 == conv1 {
		// Neither argument converts a string (nothing to delete), or both
		// do (PS2010 removes both conversions).
		return "", false
	}
	var sExpr, bExpr ast.Expr
	if conv0 {
		sExpr, bExpr = s0, call.Args[1]
	} else {
		sExpr, bExpr = s1, call.Args[0]
	}
	if !ps2022IsPlainByteSlice(pass, bExpr) {
		return "", false
	}
	// A byte side that is itself []byte(x) of a string-underlying x can
	// only be a NAMED string here (a plain string would have matched as a
	// second conversion above and deferred the call to PS2010). Named
	// strings are not reported at all — same stance as PS2010: the
	// comparison belongs to the named type's own semantics.
	if conv, isCall := ps2108Unparen(bExpr).(*ast.CallExpr); isCall &&
		len(conv.Args) == 1 && !conv.Ellipsis.IsValid() &&
		ps2108IsByteSliceConv(pass, conv.Fun) {
		if tv, found := pass.TypesInfo.Types[conv.Args[0]]; found && tv.Type != nil {
			if ub, isBasic := types.Unalias(tv.Type).Underlying().(*types.Basic); isBasic && ub.Info()&types.IsString != 0 {
				return "", false
			}
		}
	}
	sText, okS := ps2107ExprText(sExpr)
	bText, okB := ps2107ExprText(bExpr)
	if !okS || !okB {
		return "", false
	}
	// Preserve the original left-to-right evaluation order.
	if conv0 {
		return sText + " == string(" + bText + ")", true
	}
	return "string(" + bText + ") == " + sText, true
}

// ps2022IsPlainByteSlice reports whether e's static type is exactly the
// predeclared []byte: an unnamed slice of the predeclared byte. Aliases
// match (identical type); defined slice types do not — their comparison
// semantics belong to the named type — and an untyped nil does not
// (string(nil) would not even compile).
func ps2022IsPlainByteSlice(pass *analysis.Pass, e ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[e]
	if !ok || tv.Type == nil {
		return false
	}
	sl, ok := types.Unalias(tv.Type).(*types.Slice)
	if !ok {
		return false
	}
	eb, ok := types.Unalias(sl.Elem()).(*types.Basic)
	return ok && eb.Kind() == types.Byte
}
