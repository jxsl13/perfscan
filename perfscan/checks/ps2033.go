package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2033 reports fmt.Appendf(buf, "%s%s", a, b) — a format that is NOTHING but
// two or more %s verbs over string operands — where the nested builtin chain
// append(append(buf, a...), b...) writes the identical bytes with zero
// allocations and no fmt machinery. The multi-operand sibling of PS2141 (which
// owns the lone-%s shape).
var PS2033 = register(&lint.Check{
	ID:       "PS2033",
	Category: "alloc",
	Slug:     "appendf-repeated-s",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf(buf, \"%s%s\", a, b) boxes every operand and runs fmt's formatter; a nested append chain writes the same bytes directly",
		Text: `fmt.Appendf parses its format string, boxes EACH operand into an
interface (each box is a heap allocation when it escapes into the printer),
walks fmt's formatter state machine through a pooled pp buffer, and finally
copies that buffer into buf. When the format is nothing but %s verbs over plain
strings, the result is byte-for-byte the nested builtin chain
append(append(buf, a...), b...): the builtin append has a dedicated
string->[]byte special case, so the chain needs no fmt, no pool, no reflection
and no allocation beyond buf's own growth. PS2141 owns the single-%s shape;
this check owns "%s" repeated two or more times.

The match is deliberately narrow — it is the whole safety story. The callee
must resolve via type information to the package-level fmt.Appendf, the call
must not spread its arguments (no ...), and the format must be a string literal
that is EXACTLY "%s" repeated len(args) times: any literal text, flag, width,
%% or other verb disqualifies it. Three further gates keep the fix
bit-identical, each pinned by a divergence this check's equivalence suite
reproduces:

  - Every operand's type must be EXACTLY the predeclared string (untyped
    constants included). A NAMED string type could carry String()/Format()
    that %s honors and append would not. A []byte operand — although %s
    accepts it and single-operand PS2141 fixes it — is EXCLUDED here even
    unnamed: if its bytes alias buf's spare capacity, the FIRST append in the
    chain overwrites them before a later append reads them, while fmt.Appendf
    reads every operand before touching buf (measured divergence:
    "abXYcd" vs "abXYXY").
  - Every operand AFTER the first must be inert — an identifier, a string
    literal, or a package-qualified identifier — because the chain performs
    its first write to buf between evaluating the first and second operands,
    while fmt.Appendf writes only after evaluating everything. A later operand
    that runs user code (any call) could observe buf's array through another
    slice mid-rewrite (measured divergence: "XXabXX" vs "XXabab"). The FIRST
    operand may be any expression of type string: both forms evaluate buf,
    then it, before the first write. Evaluation order and count of buf and
    every operand are otherwise identical (left-to-right, exactly once).
  - buf must be an unnamed []byte so the chain reproduces fmt.Appendf's
    []byte return type exactly. The resulting slice's length and bytes are
    identical; only its unobserved spare CAPACITY may differ (the chain may
    grow twice), the same standard PS2141 ships under.

Shapes outside the gates — named operand or destination types, []byte
operands, non-inert later operands — keep an advisory report with no fix.
Each rewrite removes the file's fmt.Appendf selector, so — like
PS2107/PS2122/PS2141 — the fixes are withheld (advisory report only) when
applying all of them would rewrite the file's last fmt reference and orphan
the import. A comment inside the rewritten scaffolding suppresses the fix and
keeps the report.`,
		Before: `buf = fmt.Appendf(buf, "%s%s", a, b)`,
		After:  `buf = append(append(buf, a...), b...)`,
		MeasuredWin: `BenchmarkPS2033 (two 12/13-byte strings appended to a
preallocated buffer, Apple M2 Pro): fmt.Appendf(buf, "%s%s", a, b)
~62.5 ns/op, 32 B/op, 2 allocs/op vs append(append(buf, a...), b...)
~3.2 ns/op, 0 B/op, 0 allocs/op (~19x faster, and the two allocations — one
interface box per string operand — disappear; both write into the buffer's
existing capacity).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2033",
		Doc:  "fmt.Appendf whose format is only repeated %s verbs over strings; a nested append chain is identical and cheaper",
		Run:  runPS2033,
	},
})

func runPS2033(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: fixes are suppressed when applying all of them would
		// rewrite the file's last fmt reference and orphan the import (the
		// runner never prunes imports; same guard as PS2107/PS2122/PS2141).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 4 || call.Ellipsis.IsValid() {
				return true // 3-arg lone-%s calls are PS2141's shape
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendf.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Appendf" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// The format must be the string LITERAL "%s" repeated exactly
			// once per operand (>= 2 operands here).
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			operands := call.Args[2:]
			if format, err := strconv.Unquote(lit.Value); err != nil ||
				format != strings.Repeat("%s", len(operands)) {
				return true
			}
			// Only report %s over strings/byte slices — the shape append can
			// reproduce at all. %s over any other type (an int, a
			// Stringer-less struct) is different formatting, not this pattern.
			for _, arg := range operands {
				if !ps2141Stringish(pass.TypesInfo.TypeOf(arg)) {
					return true
				}
			}
			// The fix demands the provably bit-identical subset: unnamed
			// []byte destination, every operand EXACTLY the predeclared
			// string ([]byte operands may alias buf's spare capacity, which
			// the chain's first append would clobber), and every operand
			// after the first inert (the chain writes to buf between operand
			// evaluations; fmt.Appendf writes after all of them).
			fix := (*analysis.SuggestedFix)(nil)
			if ps2033FixableShape(pass.TypesInfo, call.Args[0], operands) {
				fix = ps2033Fix(f, call, sel)
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
				Message: "fmt.Appendf whose format is only repeated %s verbs boxes every operand and walks fmt's formatter state machine; a nested append chain writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2033FixableShape reports whether the destination and operands are inside
// the bit-identical subset: unnamed []byte destination, every operand exactly
// the predeclared string, and every operand after the first inert.
func ps2033FixableShape(info *types.Info, dest ast.Expr, operands []ast.Expr) bool {
	if !ps2141ByteSliceUnnamed(info.TypeOf(dest)) {
		return false
	}
	for i, arg := range operands {
		if !ps2033PlainString(info.TypeOf(arg)) {
			return false
		}
		if i > 0 && !ps2033Inert(info, arg) {
			return false
		}
	}
	return true
}

// ps2033PlainString reports whether t's default type is EXACTLY the
// predeclared string. A named string type may carry a String()/Format() method
// that %s honors and append does not; a []byte — safe for single-operand
// PS2141 — is excluded here because the chain's first append could clobber a
// later []byte operand aliasing buf's spare capacity.
func ps2033PlainString(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := types.Default(t).(*types.Basic)
	return ok && b.Kind() == types.String
}

// ps2033Inert reports whether evaluating e runs no user code and cannot panic:
// a string literal, a plain identifier (variable or constant read), or a
// package-qualified identifier. Anything else — a call, an index or slice
// expression (may panic), a field selector (may dereference a nil pointer) —
// is not inert: the chain evaluates it AFTER the first append has written into
// buf's capacity, while fmt.Appendf evaluates it before writing anything.
func ps2033Inert(info *types.Info, e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.BasicLit:
		return x.Kind == token.STRING
	case *ast.Ident:
		return true
	case *ast.ParenExpr:
		return ps2033Inert(info, x.X)
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok {
			if _, isPkg := info.Uses[id].(*types.PkgName); isPkg {
				return true
			}
		}
	}
	return false
}

// ps2033Fix rewrites fmt.Appendf(buf, "%s%s", a, b) to
// append(append(buf, a...), b...): buf and every operand stay byte-verbatim in
// place; only the scaffolding is edited. A comment inside a rewritten
// scaffolding span would be destroyed — the fix is withheld then and the
// report stays advisory.
func ps2033Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	args := call.Args
	last := len(args) - 1
	// The edited spans: the callee itself, buf..firstOperand (the format
	// literal and its commas), each inter-operand comma (which becomes a
	// "...), " chain link), and lastOperand..) (where the final ... lands).
	if ps2111CommentIn(f, sel.Pos(), sel.End()) ||
		ps2111CommentIn(f, args[0].End(), args[2].Pos()) ||
		ps2111CommentIn(f, args[last].End(), call.End()) {
		return nil
	}
	for i := 2; i < last; i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	// One nested append per operand: N-1 extra "append(" prefixes reuse the
	// call's own parentheses for the innermost level.
	edits := []analysis.TextEdit{
		{Pos: sel.Pos(), End: sel.End(), NewText: []byte(strings.Repeat("append(", len(args)-3) + "append")},
		{Pos: args[0].End(), End: args[2].Pos(), NewText: []byte(", ")},
	}
	for i := 2; i < last; i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte("...), ")})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[last].End(), End: call.End(), NewText: []byte("...)")})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Appendf with a nested append chain",
		TextEdits: edits,
	}
}
