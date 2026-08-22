package checks

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2040 reports fmt.Append(buf, a, b, ...) over TWO OR MORE plain-string
// operands — where the nested builtin chain append(append(buf, a...), b...)
// writes the identical bytes with no fmt machinery. The multi-operand sibling
// of PS5033 (which owns the single-operand shape) and the plain-Append twin of
// PS2033 (fmt.Appendf with a repeated-%s format).
var PS2040 = register(&lint.Check{
	ID:       "PS2040",
	Category: "alloc",
	Slug:     "append-multi-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Append over two or more strings boxes every operand and runs fmt's printer; a nested append chain writes the same bytes directly",
		Text: `fmt.Append(b, a...) appends fmt.Sprint(a...): it boxes EVERY operand
into an interface (a heap allocation per operand when it escapes into the
printer), acquires a pp printer from fmt's sync.Pool, formats each operand as
%v into the pooled buffer, and copies that buffer onto b. Sprint inserts a
space between two operands only when NEITHER is a string — so when EVERY
operand is a string no separator is ever inserted and the result is
byte-for-byte a+b+...+z, exactly the bytes the nested builtin chain
append(append(buf, a...), b...) writes. The builtin append has a dedicated
string->[]byte special case: no interface boxing, no pool round-trip, no
intermediate pp-buffer copy — the same win PS5033 measures for the
single-operand case, multiplied by the operand count. PS5033 owns the lone
operand; this check owns two or more.

The match is deliberately narrow — it is the whole safety story. The callee
must resolve via type information to the package-level fmt.Append (a shadowed
fmt or a method named Append does not), the call must not spread its arguments
(fmt.Append(buf, parts...) passes an unknown number of operands), and EVERY
operand's type must have underlying string: an operand of any other type
re-engages Sprint's spacing rule against its neighbors and formats differently
anyway (%v over a []byte prints the DECIMAL slice representation "[104 105]",
nothing like append), so such calls are not this pattern at all and are never
reported. Three further gates keep the fix bit-identical, each pinned by a
divergence this check's equivalence suite reproduces:

  - Every operand's type must be EXACTLY the predeclared string (untyped
    constants included). A NAMED string type could implement fmt.Stringer or
    fmt.Formatter, which %v honors and append does not — named operands keep
    the advisory report only.
  - Every operand AFTER the first must be inert — an identifier, a string
    literal, or a package-qualified identifier — because the chain performs
    its first write to buf between evaluating the first and second operands,
    while fmt.Append writes only after evaluating everything. A later operand
    that runs user code (any call) could observe buf's array through another
    slice mid-rewrite, and an expression that can panic would move the panic
    across the first write. The FIRST operand may be any expression of type
    string: both forms evaluate buf, then it, before the first write.
    Evaluation order and count of buf and every operand are otherwise
    identical (left-to-right, exactly once).
  - buf must be an unnamed []byte so the chain reproduces fmt.Append's
    []byte return type exactly. The resulting slice's length, bytes and
    nil-ness are identical; only its unobserved spare CAPACITY may differ
    (the chain may grow more than once), the same standard PS5033 and
    PS2033 ship under.

Invalid UTF-8 and the empty string are copied verbatim by both forms, and
strings carry no -0.0/NaN/overflow surface. Shapes outside the gates — named
operand or destination types, non-inert later operands — keep an advisory
report with no fix. Each rewrite removes the file's fmt.Append selector, so —
like PS2033/PS5033 — the fixes are withheld (advisory report only) when
applying all of them would rewrite the file's last fmt reference and orphan
the import. A comment inside the rewritten scaffolding suppresses the fix and
keeps the report.`,
		Before: `buf = fmt.Append(buf, host, ":", port)`,
		After:  `buf = append(append(append(buf, host...), ":"...), port...)`,
		MeasuredWin: `BenchmarkPS2040 (a host + ":" + port join of three strings,
10/1/4 bytes, appended to a preallocated buffer, Apple M2 Pro): fmt.Append
~64.5 ns/op, 32 B/op, 2 allocs/op vs the nested append chain ~4.6 ns/op,
0 B/op, 0 allocs/op (~14x faster, and the allocations — one interface box per
string VARIABLE operand; the constant ":" boxes into a static interface value
at compile time — disappear; both write into the buffer's existing capacity).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2040",
		Doc:  "fmt.Append over two or more string operands; a nested append chain is identical and cheaper",
		Run:  runPS2040,
	},
})

func runPS2040(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: fixes are suppressed when applying all of them would
		// rewrite the file's last fmt reference and orphan the import (the
		// runner never prunes imports; same guard as PS2033/PS5033).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 3 || call.Ellipsis.IsValid() {
				return true // the 2-arg single-operand call is PS5033's shape
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level fmt.Append.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Append" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// EVERY operand must have underlying string. Any other type
			// re-engages Sprint's spacing rule against its neighbors and
			// formats differently anyway (%v over a []byte prints the decimal
			// slice form) — different formatting entirely, never reported.
			operands := call.Args[1:]
			for _, arg := range operands {
				if !ps5033StringUnderlying(pass.TypesInfo.TypeOf(arg)) {
					return true
				}
			}
			// The fix demands the provably bit-identical subset: unnamed
			// []byte destination, every operand EXACTLY the predeclared
			// string (a named string may carry String()/Format() that %v
			// honors and append does not), and every operand after the first
			// inert (the chain writes to buf between operand evaluations;
			// fmt.Append writes after all of them).
			fix := (*analysis.SuggestedFix)(nil)
			if ps2040FixableShape(pass.TypesInfo, call.Args[0], operands) {
				fix = ps2040Fix(f, call, sel)
				if fix != nil {
					fixable++
				}
			}
			sites = append(sites, site{call, fix})
			return true
		})
		emitFixes := fixable > 0 && pkgRefCount(pass, f, "fmt") > fixable
		for _, st := range sites {
			diag := analysis.Diagnostic{
				Pos:     st.call.Pos(),
				End:     st.call.End(),
				Message: "fmt.Append over two or more string operands boxes each into an interface and walks fmt's printer through a pooled buffer; a nested append chain writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2040FixableShape reports whether the destination and operands are inside
// the bit-identical subset: unnamed []byte destination, every operand exactly
// the predeclared string, and every operand after the first inert.
func ps2040FixableShape(info *types.Info, dest ast.Expr, operands []ast.Expr) bool {
	if !ps2141ByteSliceUnnamed(info.TypeOf(dest)) {
		return false
	}
	for i, arg := range operands {
		if !ps5033PlainString(info.TypeOf(arg)) {
			return false
		}
		if i > 0 && !ps2033Inert(info, arg) {
			return false
		}
	}
	return true
}

// ps2040Fix rewrites fmt.Append(buf, a, b) to append(append(buf, a...), b...):
// buf and every operand stay byte-verbatim in place; only the scaffolding is
// edited. A comment inside a rewritten scaffolding span would be destroyed —
// the fix is withheld then and the report stays advisory. The span between buf
// and the first operand (the comma) is untouched, so a comment there survives
// the rewrite.
func ps2040Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	args := call.Args
	last := len(args) - 1
	// The edited spans: the fmt.Append selector (a comment can legally sit
	// between fmt and .Append), each inter-operand comma (which becomes a
	// "...), " chain link), and lastOperand..) (where the final ... lands and
	// a trailing comma would be dropped).
	if ps2111CommentIn(f, sel.Pos(), sel.End()) ||
		ps2111CommentIn(f, args[last].End(), call.End()) {
		return nil
	}
	for i := 1; i < last; i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	// One nested append per operand: N-1 extra "append(" prefixes reuse the
	// call's own parentheses for the innermost level.
	edits := []analysis.TextEdit{
		{Pos: sel.Pos(), End: sel.End(), NewText: []byte(strings.Repeat("append(", len(args)-2) + "append")},
	}
	for i := 1; i < last; i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte("...), ")})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[last].End(), End: call.End(), NewText: []byte("...)")})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Append with a nested append chain",
		TextEdits: edits,
	}
}
