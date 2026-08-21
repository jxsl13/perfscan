package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2124 reports strings.Join([]string{a, b, ...}, sep) — a Join whose
// operands are spelled out in an inline slice literal and whose separator
// is a constant string — where interleaving the separator with + produces
// the identical string without materializing the throwaway []string or
// running Join's second pass over it. A single-element literal makes
// strings.Join([]string{a}, sep) the identity on a.
var PS2124 = register(&lint.Check{
	ID:       "PS2124",
	Category: "alloc",
	Slug:     "join-literal-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: `strings.Join over an inline []string literal is a slice-priced +; concatenate with the separator interleaved`,
		Text: `strings.Join([]string{a, b}, sep) materializes a []string just
to enumerate operands that are already spelled out in the source, then
Join sums their lengths and copies every element and separator into the
result. Escape analysis usually keeps the throwaway slice on the stack
(Join's parameter does not escape), but the slice is still built and
walked twice on every call, and when the literal DOES escape it costs a
heap allocation on top. Join's spec places sep exactly between
consecutive elements, so the result is byte-for-byte
a + sep + b + ... — and that + chain is a direct compiler-emitted
concatenation: one length computation, one allocation, one copy per
operand, no slice at all. An empty constant separator degenerates to
a + b + ..., and a single-element literal makes the whole call the
identity on its element (the separator is never used) — that rewrite
deletes the call and the slice outright.

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level
strings.Join (a shadowed strings or a method named Join does not) and
the call must not spread its arguments. The first argument must be an
INLINE []string composite literal — a slice held in a variable may have
other consumers and an unknown length — with at least one element (the
zero-element Join returns "" and is out of scope) and no keyed elements,
whose element positions could differ from their source order. Every
element must have type EXACTLY the predeclared string (untyped constant
strings default to it) — the same guard PS2122/PS2123 apply; Go's
assignability rules already keep named string types out of a []string
literal, so this guard is defense in depth. The separator must
be a CONSTANT string (a literal or a named constant, resolved via
go/types constant evaluation), because the fix repeats its source text
between every pair of elements: a variable separator would multiply its
evaluation and is never matched. Element evaluation order is unchanged —
both the literal and the + chain evaluate left to right, and a constant
separator has no evaluation to reorder.

The fix keeps every element byte-verbatim in place and edits only the
scaffolding: the strings.Join([]string{ prefix is dropped, each
inter-element comma becomes " + sep + " (just " + " for an empty
separator, whose interleaving is the empty string), and the }, sep)
suffix is dropped. String-typed operands are primaries or + chains, so
the operands never need parentheses; the WHOLE replacement is
parenthesized when the surrounding context binds tighter than + (an
index, a slice, a binary operand), exactly as PS2121 decides it — except
that a single-element rewrite whose element is a primary needs none in
any context. When the rewrite DROPS the separator text (a single
element, or an empty constant value) a package-qualified separator would
lose that file's reference to its package, so such fixes are withheld.
Each rewrite removes the file's strings.Join selector, so — like
PS2107/PS2118/PS2122/PS2123 — the fixes are withheld (advisory report
only) when applying all of them would rewrite the file's last strings
reference and orphan the import. A comment inside the rewritten
scaffolding is never silently dropped: it suppresses the fix and keeps
the report.`,
		Before: `s := strings.Join([]string{host, port}, ":")`,
		After:  `s := host + ":" + port`,
		MeasuredWin: `BenchmarkPS2124 (a dir + "/" + file + "/" + name join of
three plain strings, 12/9/17 bytes, once per op, Apple M2 Pro):
strings.Join([]string{a, b, c}, "/") 33.5 ns/op vs
a + "/" + b + "/" + c 32.0 ns/op, both 48 B/op, 1 alloc/op. Escape
analysis keeps the throwaway slice on the stack here, so the measured
win is the ~5% of building and double-walking it; the win grows to a
whole heap allocation when the literal escapes, and the single-element
identity deletes the call entirely. The rewrite is never slower and the
result is bit-identical.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2124",
		Doc:  "strings.Join over an inline []string literal with a constant separator; interleaved + concatenation is identical and cheaper",
		Run:  runPS2124,
	},
})

func runPS2124(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: fixes are suppressed when applying all of them
		// would rewrite the file's last strings reference and orphan the
		// import (the runner never prunes imports; same guard as
		// PS2107/PS2118/PS2122/PS2123).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level strings.Join:
			// a shadowed strings resolves sel.Sel to some other object, and
			// a method named Join carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Join" || fn.Pkg() == nil || fn.Pkg().Path() != "strings" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The elements must be spelled out INLINE: a slice stored in a
			// variable first may have other consumers and an unknown
			// length. The literal must type as an unnamed []string, hold at
			// least one element (the zero-element Join is "" — out of
			// scope), and use no keyed elements, whose slice positions
			// could differ from their source order.
			lit, ok := call.Args[0].(*ast.CompositeLit)
			if !ok || len(lit.Elts) == 0 {
				return true
			}
			sl, ok := pass.TypesInfo.TypeOf(lit).(*types.Slice)
			if !ok || !types.Identical(sl.Elem(), types.Typ[types.String]) {
				return true
			}
			for _, el := range lit.Elts {
				if _, keyed := el.(*ast.KeyValueExpr); keyed {
					return true
				}
				// Every element must be EXACTLY the predeclared string
				// (untyped constant strings default to it) — the same guard
				// as PS2122/PS2123. Assignability already keeps named
				// string types out of a []string literal; defense in depth.
				t := pass.TypesInfo.TypeOf(el)
				if t == nil || !types.Identical(types.Default(t), types.Typ[types.String]) {
					return true
				}
			}
			// The separator's source text is repeated between every pair of
			// elements, so it must be a CONSTANT string — a variable
			// separator would multiply its evaluation and is never matched.
			tv, ok := pass.TypesInfo.Types[call.Args[1]]
			if !ok || tv.Value == nil || tv.Value.Kind() != constant.String {
				return true
			}
			sepVal := constant.StringVal(tv.Value)
			var parent ast.Node
			if len(stack) > 0 {
				parent = stack[len(stack)-1]
			}
			needParens := ps2121NeedsParens(parent, call)
			if len(lit.Elts) == 1 {
				// The identity rewrite substitutes the lone element
				// verbatim: a primary (anything but a + chain) is
				// self-delimiting in every context.
				if _, isBinary := lit.Elts[0].(*ast.BinaryExpr); !isBinary {
					needParens = false
				}
			}
			fix := ps2124Fix(pass, f, call, lit, sepVal, needParens)
			if fix != nil {
				fixable++
			}
			sites = append(sites, site{call, fix})
			return true
		})
		// Each fixable call holds exactly one strings reference (its
		// selector's package identifier); if those are ALL of the file's
		// strings references, the fixes would orphan the import — advisory
		// only.
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "strings") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "strings.Join over an inline []string literal allocates the throwaway slice and copies it again inside Join; with a constant separator the interleaved + concatenation builds the identical string",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2124Fix builds the e0 + sep + e1 + ... replacement (or the bare e0
// for a single element). Every element stays byte-verbatim in place; only
// the scaffolding is edited — the strings.Join([]string{ prefix becomes
// the (optional) opening parenthesis, each inter-element comma becomes
// " + sep + " (" + " for an empty separator), and the }, sep) suffix
// becomes the (optional) closing one. String-typed operands are primaries
// or + chains (the only string-producing binary operator is the
// associative + itself), so neither the elements nor the interleaved
// separator ever need parentheses. A rewrite that DROPS the separator
// text (single element, or empty constant value) is withheld when the
// separator mentions a package qualifier, which the drop would orphan. A
// comment inside any scaffolding span would be destroyed by the edits —
// the fix is withheld then and the report stays advisory.
func ps2124Fix(pass *analysis.Pass, f *ast.File, call *ast.CallExpr, lit *ast.CompositeLit, sepVal string, needParens bool) *analysis.SuggestedFix {
	elts := lit.Elts
	if ps2111CommentIn(f, call.Pos(), elts[0].Pos()) ||
		ps2111CommentIn(f, elts[len(elts)-1].End(), call.End()) {
		return nil
	}
	for i := 0; i+1 < len(elts); i++ {
		if ps2111CommentIn(f, elts[i].End(), elts[i+1].Pos()) {
			return nil
		}
	}
	sepDropped := len(elts) == 1 || sepVal == ""
	if sepDropped && ps2124MentionsPkgName(pass, call.Args[1]) {
		return nil
	}
	joiner := []byte(" + ")
	if !sepDropped {
		joiner = []byte(" + " + exprTextRendered(call.Args[1]) + " + ")
	}
	open, closing := "", ""
	if needParens {
		open, closing = "(", ")"
	}
	edits := make([]analysis.TextEdit, 0, len(elts)+1)
	edits = append(edits, analysis.TextEdit{Pos: call.Pos(), End: elts[0].Pos(), NewText: []byte(open)})
	for i := 0; i+1 < len(elts); i++ {
		edits = append(edits, analysis.TextEdit{Pos: elts[i].End(), End: elts[i+1].Pos(), NewText: joiner})
	}
	edits = append(edits, analysis.TextEdit{Pos: elts[len(elts)-1].End(), End: call.End(), NewText: []byte(closing)})
	return &analysis.SuggestedFix{
		Message:   "replace strings.Join with interleaved + concatenation",
		TextEdits: edits,
	}
}

// ps2124MentionsPkgName reports whether e contains an identifier that
// resolves to an imported package — text whose removal could orphan that
// import.
func ps2124MentionsPkgName(pass *analysis.Pass, e ast.Expr) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if _, isPkg := pass.TypesInfo.Uses[id].(*types.PkgName); isPkg {
				found = true
				return false
			}
		}
		return true
	})
	return found
}
