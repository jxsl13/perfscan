package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2010 reports bytes.Equal([]byte(s1), []byte(s2)) where both operands
// are conversions of plain strings: the direct comparison s1 == s2 is
// bit-identical and skips both heap copies.
var PS2010 = register(&lint.Check{
	ID:       "PS2010",
	Category: "alloc",
	Slug:     "bytes-equal-string-conversions",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "bytes.Equal of two []byte(string) conversions is the string comparison with two extra allocations",
		Text: `bytes.Equal([]byte(s1), []byte(s2)) with s1 and s2 plain strings
spells the string comparison s1 == s2 through two string->[]byte
conversions. s1 == s2 performs the same length check and byte comparison
directly on the strings — guaranteed zero allocations, no copies, and an
O(1) short-circuit on a length mismatch.

An honest caveat: current gc (verified on Go 1.26) compiles EXACTLY this
composition allocation-free too — bytes.Equal does not retain or mutate
its arguments, so escape analysis turns both conversions into zero-copy
views of the strings and the measured difference is nil (see below). The
rewrite moves that guarantee from a compiler optimization into the
source: it survives toolchains and build modes without the zero-copy
conversion (gccgo, tinygo, older gc), keeps holding when the expression
is later refactored out of the exact shape escape analysis proves (a
conversion whose slice is stored or passed onward allocates and copies
for real), and states the intent — "are these strings equal" — directly.

The rewrite is bit-identical by the standard library's own definition:
bytes.Equal(a, b) is documented and implemented as string(a) == string(b),
and string([]byte(x)) == x for every string x (a string round-tripped
through []byte is byte-identical, including invalid UTF-8 and empty
strings). Each operand is evaluated exactly once, left to right, in both
forms, so side effects are preserved in count and order.

The automatic fix rewrites the call to s1 == s2. It applies only when
type information proves the shape:

  - the callee resolves to the standard library's bytes.Equal (a shadowed
    bytes identifier or a same-named method does not match);
  - BOTH arguments are conversions whose target is exactly the
    predeclared []byte (an unnamed slice of the predeclared byte; a
    defined slice type is not matched) and whose operand's static type is
    exactly the predeclared string — a NAMED string type is not reported
    at all, because s1 == s2 could then fail to compile (mixed named and
    unnamed strings are not comparable) and the semantics belong to the
    named type.

When BOTH operands are string constants the report stays advisory:
s1 == s2 of two constants is itself a compile-time constant while
bytes.Equal(...) is not, which could change compile-time properties such
as duplicate-switch-case detection (same stance as PS2108). The
replacement is parenthesized when the surrounding expression binds
tighter than == (for example !bytes.Equal(...) becomes !(s1 == s2));
in && / || and delimited contexts no parentheses are added. In a context
that syntactically requires a call — a bare expression statement, go, or
defer — the rewrite would not compile, so the report stays advisory. A
comment inside the replaced call withholds the fix, and in a cgo file —
whose import block is never pruned — the fix is withheld when it would
orphan the bytes import.`,
		Before: `if bytes.Equal([]byte(s1), []byte(s2)) {
	return true
}`,
		After: `if s1 == s2 {
	return true
}`,
		MeasuredWin: `BenchmarkPS2010 (one pair sharing a 2944-byte prefix
plus one length-mismatch pair, Apple M2 Pro, Go 1.26): 58.8 ns/op,
0 allocs/op -> 59.2 ns/op, 0 allocs/op — parity, because gc's zero-copy
conversion already elides both allocations in this exact shape. The After
is guaranteed allocation-free by construction rather than by escape
analysis; on toolchains without the zero-copy conversion the Before pays
two full heap copies per call.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2010",
		Doc:  "bytes.Equal of two []byte(string) conversions equals the direct string comparison without the copies",
		Run:  runPS2010,
	},
})

func runPS2010(pass *analysis.Pass) (any, error) {
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
			a, b, fixOK := ps2010Match(pass, call)
			if a == nil {
				stack = append(stack, n)
				return true
			}
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
			var fix *analysis.SuggestedFix
			aText, okA := ps2107ExprText(a)
			bText, okB := ps2107ExprText(b)
			// A comment inside the replaced call would be silently dropped
			// by the re-rendered expression — withhold the fix there.
			if fixOK && okA && okB && !ps2111CommentIn(f, call.Pos(), call.End()) {
				repl := aText + " == " + bText
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
				Message: "bytes.Equal([]byte(s1), []byte(s2)) allocates two throwaway byte-slice copies just to compare strings; s1 == s2 is bit-identical (bytes.Equal is defined as string(a) == string(b)) and allocation-free",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2010Match matches call against bytes.Equal([]byte(s1), []byte(s2))
// with s1 and s2 statically plain strings. It returns the two string
// operands (nil when the shape does not match) and whether the rewrite is
// provably safe to apply automatically. Two CONSTANT operands are
// reported but not fixable: s1 == s2 would then be a compile-time
// constant while the call is not, which can change compile-time
// properties (e.g. duplicate switch cases) — the same stance as PS2108.
func ps2010Match(pass *analysis.Pass, call *ast.CallExpr) (a, b ast.Expr, fixable bool) {
	if !ps2010IsBytesEqual(pass, call) {
		return nil, nil, false
	}
	a, aConst, ok := ps2010StringConvOperand(pass, call.Args[0])
	if !ok {
		return nil, nil, false
	}
	b, bConst, ok := ps2010StringConvOperand(pass, call.Args[1])
	if !ok {
		return nil, nil, false
	}
	return a, b, !(aConst && bConst)
}

// ps2010IsBytesEqual reports whether call is a direct call of the
// package-level function bytes.Equal. Type information pins the callee to
// the standard library: a local variable, field, or method spelled Equal
// — or a shadowed `bytes` identifier — does not resolve to a
// receiver-less *types.Func with package path "bytes" and is rejected.
func ps2010IsBytesEqual(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || call.Ellipsis != token.NoPos {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Equal" || fn.Pkg() == nil || fn.Pkg().Path() != "bytes" {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	return ok && sig.Recv() == nil
}

// ps2010StringConvOperand matches e (possibly parenthesized) against a
// conversion []byte(s) whose target is exactly the predeclared []byte and
// whose operand s has static type exactly the predeclared string. It
// returns the operand, whether it is a constant, and whether it matched.
// A NAMED string operand does not match at all: mixed named and unnamed
// strings are not comparable with ==, and even two operands of the same
// named type belong to that type's own semantics.
func ps2010StringConvOperand(pass *analysis.Pass, e ast.Expr) (arg ast.Expr, isConst, ok bool) {
	conv, isCall := ps2108Unparen(e).(*ast.CallExpr)
	if !isCall || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false, false
	}
	if !ps2108IsByteSliceConv(pass, conv.Fun) {
		return nil, false, false
	}
	a := conv.Args[0]
	tv, found := pass.TypesInfo.Types[a]
	if !found || tv.Type == nil {
		return nil, false, false
	}
	basic, isBasic := types.Unalias(tv.Type).(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		return nil, false, false
	}
	return a, tv.Value != nil, true
}

// ps2010NeedsParens reports whether the replacement `s1 == s2` must be
// parenthesized in parent, which held the (primary) call expression. ==
// binds looser than every unary and arithmetic operator but tighter than
// && and ||, so parentheses are needed exactly when the parent binds
// tighter than ==: a unary operand (!bytes.Equal(...)), a binary parent
// of higher precedence, or the RIGHT operand of an equal-precedence
// comparison (== is left-associative). Delimited contexts — condition,
// call argument, index bracket, return, assignment — need none. Unknown
// parents are parenthesized defensively: wrapping a complete operand in
// parentheses never changes meaning.
func ps2010NeedsParens(parent ast.Node, call *ast.CallExpr) bool {
	switch p := parent.(type) {
	case nil:
		return false
	case *ast.AssignStmt, *ast.ReturnStmt, *ast.ValueSpec, *ast.KeyValueExpr,
		*ast.ParenExpr, *ast.CompositeLit, *ast.SendStmt,
		*ast.IfStmt, *ast.ForStmt, *ast.SwitchStmt, *ast.CaseClause,
		*ast.RangeStmt:
		// ExprStmt / GoStmt / DeferStmt never reach here: those contexts
		// syntactically require a call and the fix is withheld upstream.
		return false
	case *ast.CallExpr:
		// Safe as an argument; the callee slot cannot hold a bool, but
		// stay defensive.
		for _, arg := range p.Args {
			if arg == ast.Expr(call) {
				return false
			}
		}
		return true
	case *ast.IndexExpr:
		// m[bytes.Equal(...)] — the brackets delimit the index.
		return p.Index != ast.Expr(call)
	case *ast.BinaryExpr:
		eqPrec := token.EQL.Precedence()
		if prec := p.Op.Precedence(); prec != eqPrec {
			// && and || bind looser than == (no parens); anything
			// tighter needs them.
			return prec > eqPrec
		}
		// Equal precedence: comparisons are left-associative, so only
		// the RIGHT operand needs parentheses.
		return p.Y == ast.Expr(call)
	}
	return true
}
