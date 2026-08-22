package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2121 reports len(strings.Split(s, sep)) — allocating the entire
// []string of pieces just to count them and throw the slice away —
// where strings.Count(s, sep) + 1 computes the same integer with a
// single scan and zero allocation. The bytes twin is matched the same
// way. Only fires when the separator is provably non-empty: for the
// empty separator the two expressions genuinely differ.
var PS2121 = register(&lint.Check{
	ID:       "PS2121",
	Category: "alloc",
	Slug:     "len-of-split",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "len(strings.Split(s, sep)) allocates every piece just to count them; strings.Count(s, sep)+1 is allocation-free",
		Text: `strings.Split allocates a []string holding every piece of the
input; taking only its len discards that slice immediately. For a
NON-EMPTY separator the piece count is exactly the number of
non-overlapping separator occurrences plus one, for all inputs
including the empty string: Split("", sep) is [""] with len 1, and
Count("", sep)+1 is 0+1 = 1. strings.Count(s, sep) + 1 therefore
returns the bit-identical integer with one scan and zero allocation.
The same identity holds for bytes.Split versus bytes.Count.

The empty separator is the one case where the identity BREAKS:
strings.Count(s, "") is utf8.RuneCountInString(s)+1 while
len(strings.Split(s, "")) is utf8.RuneCountInString(s). The check
therefore only matches when the separator is PROVABLY non-empty. For
strings that means the separator is a constant (a string literal or a
named constant, resolved via go/types constant evaluation) whose value
is not "". For bytes it means the separator is a []byte("...")
conversion of a non-empty constant string or a []byte{...} composite
literal with at least one element. A variable separator — even one
that happens to be non-empty at run time — is never reported: the
check stays silent rather than advising something it cannot prove.

The callee is pinned with type information to the package-level
strings.Split / bytes.Split (a shadowed strings/bytes or a local or
method Split does not resolve there), and the outer len must be the
predeclared builtin (a shadowed len is rejected the same way). Only
the direct len(Split(...)) composition is matched: a Split result
stored in a variable first may have other consumers, and SplitN,
SplitAfter, SplitAfterN and Fields count differently or take
different arguments, so none of them are matched.

The fix keeps both argument expressions verbatim in place — same
text, same evaluation order — and edits only the scaffolding: len( is
dropped, Split becomes Count, and the closing parenthesis gains + 1.
Count lives in the same package as Split, so an aliased qualifier is
reused unchanged and the import can never be orphaned. Because a
primary expression is replaced by a binary one, the result is wrapped
in parentheses whenever the surrounding context could bind tighter
than + (2 * len(...), -len(...), a shift operand); in self-delimiting
contexts (an assignment or return value, a call argument, an index) no
parentheses are added. A comment inside the rewritten scaffolding
downgrades the fix to an advisory report.`,
		Before: `n := len(strings.Split(s, ","))`,
		After:  `n := strings.Count(s, ",") + 1`,
		MeasuredWin: `BenchmarkPS2121 (a ~1.3KB line of 64 comma-separated
~19-byte fields, counted once per op, Apple M2 Pro): len(Split)
750 ns/op, 1152 B/op, 1 alloc/op vs Count+1 27.7 ns/op, 0 B/op,
0 allocs/op (~27x faster). The entire 64-header []string vanishes,
the per-piece slicing goes with it, and Count rides the vectorized
single-byte scan in internal/bytealg; the win grows with the piece
count and matters most on hot paths where the discarded slice churns
the GC.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2121",
		Doc:  "len(strings.Split(s, sep)) allocates the result slice just to count pieces; strings.Count(s, sep)+1 doesn't",
		Run:  runPS2121,
	},
})

func runPS2121(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			lenCall, ok := n.(*ast.CallExpr)
			if !ok || len(lenCall.Args) != 1 || lenCall.Ellipsis.IsValid() {
				return true
			}
			// The outer callee must be the predeclared builtin len — a
			// shadowed len resolves to some other object and is rejected.
			id, ok := lenCall.Fun.(*ast.Ident)
			if !ok || id.Name != "len" {
				return true
			}
			if pass.TypesInfo.Uses[id] != types.Universe.Lookup("len") {
				return true
			}
			// The argument must be DIRECTLY the Split call: a slice stored
			// in a variable first may have other consumers — out of scope.
			inner, ok := lenCall.Args[0].(*ast.CallExpr)
			if !ok || len(inner.Args) != 2 || inner.Ellipsis.IsValid() {
				return true
			}
			sel, ok := inner.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level strings.Split
			// or bytes.Split: a shadowed strings/bytes resolves sel.Sel to
			// some other object, and a method carries a receiver. SplitN,
			// SplitAfter, SplitAfterN and Fields count differently and are
			// never matched.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Split" || fn.Pkg() == nil {
				return true
			}
			pkgPath := fn.Pkg().Path()
			if pkgPath != "strings" && pkgPath != "bytes" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The identity len(Split(s, sep)) == Count(s, sep)+1 requires a
			// NON-EMPTY separator; without proof the check stays silent.
			if !ps2121SepNonEmpty(pass, pkgPath, inner.Args[1]) {
				return true
			}
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			needParens := ps2121NeedsParens(parent, lenCall)
			diag := analysis.Diagnostic{
				Pos: lenCall.Pos(),
				End: lenCall.End(),
				Message: fmt.Sprintf(
					"len(%[1]s.Split(...)) allocates the whole result slice just to count the pieces; %[1]s.Count(...) + 1 is the same value with zero allocation (non-empty separator)",
					pkgPath),
			}
			// Three edits keep both arguments verbatim: drop "len(", rename
			// Split -> Count, and fold "+ 1" into the closing parenthesis.
			// A comment inside either scaffolding span would be destroyed —
			// the fix is withheld then and the report stays advisory.
			if !ps2111CommentIn(f, lenCall.Pos(), inner.Pos()) &&
				!ps2111CommentIn(f, inner.End(), lenCall.End()) {
				open, closing := "", " + 1"
				if needParens {
					open, closing = "(", " + 1)"
				}
				diag.SuggestedFixes = []analysis.SuggestedFix{{
					Message: "replace with " + pkgPath + ".Count(...) + 1",
					TextEdits: []analysis.TextEdit{
						{Pos: lenCall.Pos(), End: inner.Pos(), NewText: []byte(open)},
						{Pos: sel.Sel.Pos(), End: sel.Sel.End(), NewText: []byte("Count")},
						{Pos: inner.End(), End: lenCall.End(), NewText: []byte(closing)},
					},
				}}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

// ps2121SepNonEmpty reports whether the Split separator is PROVABLY
// non-empty — the precondition for len(Split(x, sep)) == Count(x, sep)+1.
// For strings the separator must be a constant (literal or named,
// resolved via go/types constant evaluation) with a non-empty value. For
// bytes it must be a []byte("...") conversion of a non-empty constant
// string or a []byte{...} composite literal with at least one element
// (a keyed element still forces len > 0). Anything else — in particular
// a variable separator — is not provable and the check stays silent.
func ps2121SepNonEmpty(pass *analysis.Pass, pkgPath string, sep ast.Expr) bool {
	if pkgPath == "strings" {
		tv, ok := pass.TypesInfo.Types[sep]
		return ok && tv.Value != nil &&
			tv.Value.Kind() == constant.String &&
			constant.StringVal(tv.Value) != ""
	}
	switch x := ps2121Unparen(sep).(type) {
	case *ast.CallExpr:
		// A conversion like []byte("x"): the "callee" is a type and the
		// operand a non-empty constant string. The conversion of a
		// non-empty string yields a slice of its (positive) byte length.
		if len(x.Args) != 1 || x.Ellipsis.IsValid() {
			return false
		}
		if ftv, ok := pass.TypesInfo.Types[x.Fun]; !ok || !ftv.IsType() {
			return false
		}
		atv, ok := pass.TypesInfo.Types[x.Args[0]]
		return ok && atv.Value != nil &&
			atv.Value.Kind() == constant.String &&
			constant.StringVal(atv.Value) != ""
	case *ast.CompositeLit:
		// []byte{...}: any element — positional or keyed — makes the
		// literal's length at least one.
		return len(x.Elts) > 0
	}
	return false
}

// ps2121NeedsParens reports whether the Count(...) + 1 replacement for
// the len(...) primary expression must be parenthesized in its context.
// Self-delimiting positions — an assignment or return value, a call
// argument, an index, a composite-literal element — need none; every
// other parent (a binary or unary operand, a switch tag, ...) gets
// parentheses: a redundant pair is harmless and gofmt-stable, a missing
// one would reassociate the expression (2 * len(...) must become
// 2 * (Count(...) + 1), never 2*Count(...) + 1).
func ps2121NeedsParens(parent ast.Node, lenCall *ast.CallExpr) bool {
	switch p := parent.(type) {
	case nil:
		return false
	case *ast.AssignStmt, *ast.ReturnStmt, *ast.ValueSpec, *ast.KeyValueExpr,
		*ast.ParenExpr, *ast.ExprStmt, *ast.CompositeLit:
		return false
	case *ast.SendStmt:
		return false
	case *ast.CallExpr:
		// Safe as an argument; anything else (the callee slot cannot hold
		// an int, but stay defensive) is parenthesized.
		for _, a := range p.Args {
			if a == ast.Expr(lenCall) {
				return false
			}
		}
		return true
	case *ast.IndexExpr:
		// a[len(...)] — the brackets delimit the index.
		return p.Index != ast.Expr(lenCall)
	case *ast.SliceExpr:
		// a[len(...):...] — the brackets and colons delimit each index.
		return p.X == ast.Expr(lenCall)
	}
	return true
}

// ps2121Unparen strips any parentheses around e.
func ps2121Unparen(e ast.Expr) ast.Expr {
	for {
		p, ok := e.(*ast.ParenExpr)
		if !ok {
			return e
		}
		e = p.X
	}
}
