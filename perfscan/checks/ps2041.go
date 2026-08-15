package checks

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2041 reports fmt.Appendln(buf, a, b) where EVERY operand is a string —
// the shape where Appendln's UNCONDITIONAL formatting rule (exactly one ' '
// between adjacent operands, exactly one trailing '\n', no format-string
// interpretation) makes the result byte-for-byte the nested builtin chain
// append(append(append(append(buf, a...), ' '), b...), '\n'). The fmt.Append
// twin of PS5029 (fmt.Sprintln over plain strings -> concatenation) and the
// Appendln sibling of PS2033 (fmt.Appendf "%s%s" -> nested append chain).
var PS2041 = register(&lint.Check{
	ID:       "PS2041",
	Category: "alloc",
	Slug:     "appendln-strings-append-chain",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendln over strings boxes every operand and runs fmt's printer; a nested append chain writes the same bytes directly",
		Text: `fmt.Appendln(buf, a...) appends fmt.Sprintln(a...) to buf: it boxes
every operand into an interface (each box is a heap allocation when it
escapes into the printer), acquires a pp printer from fmt's sync.Pool,
walks doPrintln's per-operand %v formatting through the pooled buffer,
and finally copies that buffer onto buf. Unlike Sprint/Append's
CONDITIONAL spacing rule, doPrintln is UNCONDITIONAL: it writes exactly
one ' ' between adjacent operands and exactly one trailing '\n', with no
format-string interpretation (a '%' in an operand is printed literally).
%v over a value whose type is EXACTLY the predeclared string takes fmt's
reflection-free fast path that writes the string's raw bytes verbatim —
no quoting, no UTF-8 validation, empty strings and invalid UTF-8
included. So when every operand is a plain string the result is
byte-for-byte the nested builtin chain
append(append(append(append(buf, a...), ' '), b...), '\n'): the builtin
append has a dedicated string->[]byte special case and a single-byte
append for the literal separators, so the chain needs no fmt, no pool,
no reflection and no allocation beyond buf's own growth — unlike
PS5029's string result, the append form never materialises an
intermediate string. The single-operand fmt.Appendln(buf, s) rewrites to
append(append(buf, s...), '\n').

The match is deliberately narrow — it is the whole safety story. The
callee must resolve via type information to the package-level
fmt.Appendln (a shadowed fmt or a method named Appendln does not), the
call must not spread its arguments (no ...), and there must be at least
one operand (the zero-operand fmt.Appendln(buf), which appends just
"\n", is out of scope). Every operand's underlying type must be string —
%v over a []byte prints the DECIMAL slice representation "[104 105]",
nothing like appending its bytes, so []byte, numeric, interface and
error operands are not this pattern at all and are never reported.
Three further gates keep the fix bit-identical, each pinned by a
divergence this check's equivalence suite reproduces:

  - Every operand's type must be EXACTLY the predeclared string (untyped
    constants included). A NAMED string type could carry
    String()/Format() that %v honors and append would not.
  - Every operand AFTER the first must be inert — an identifier, a
    string literal, or a package-qualified identifier — because the
    chain performs its first write to buf between evaluating the first
    and second operands, while fmt.Appendln writes only after evaluating
    everything (all arguments are evaluated at the call site). A later
    operand that runs user code could observe buf's array through
    another slice mid-rewrite. The FIRST operand may be any expression
    of type string: both forms evaluate buf, then it, before the first
    write. Evaluation order and count of buf and every operand are
    otherwise identical (left-to-right, exactly once).
  - buf must be an unnamed []byte so the chain reproduces fmt.Appendln's
    []byte return type exactly. The resulting slice's length and bytes
    are identical; only its unobserved spare CAPACITY may differ (the
    chain may grow more than once), the same standard PS2033 and PS2141
    ship under.

Shapes outside the gates — named operand or destination types, non-inert
later operands — keep an advisory report with no fix. Each rewrite
removes the file's fmt.Appendln selector, so — like PS2033/PS2141/PS5033
— the fixes are withheld (advisory report only) when applying all of
them would rewrite the file's last fmt reference and orphan the import.
A comment inside the rewritten scaffolding suppresses the fix and keeps
the report; the span between buf and the first operand is untouched, so
a comment there survives.`,
		Before: `buf = fmt.Appendln(buf, host, port)`,
		After:  `buf = append(append(append(append(buf, host...), ' '), port...), '\n')`,
		MeasuredWin: `BenchmarkPS2041 (two 12/13-byte strings appended to a
preallocated buffer, Apple M2 Pro, go1.26): fmt.Appendln(buf, a, b)
~61.0 ns/op, 32 B/op, 2 allocs/op vs the nested append chain
~3.7 ns/op, 0 B/op, 0 allocs/op (~16x faster, and the two allocations —
one interface box per string operand — disappear; both write into the
buffer's existing capacity).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2041",
		Doc:  "fmt.Appendln over strings writes exactly one space between operands and one trailing newline; a nested append chain is identical and cheaper",
		Run:  runPS2041,
	},
})

// ps2041Msg is the diagnostic text (shared by the fixed and advisory paths).
const ps2041Msg = "fmt.Appendln over string operands boxes each into an interface and walks fmt's printer through a pooled buffer, writing exactly one space between operands and one trailing newline; a nested append chain with ' ' and '\\n' writes the identical bytes directly"

func runPS2041(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: fixes are suppressed when applying all of them would
		// rewrite the file's last fmt reference and orphan the import (the
		// runner never prunes imports; same guard as PS2033/PS2141/PS5033).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) < 2 || call.Ellipsis.IsValid() {
				return true // zero-operand fmt.Appendln(buf) appends just "\n" — out of scope
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// Type info pins the callee to the package-level fmt.Appendln:
			// a shadowed fmt resolves sel.Sel to some other object, and a
			// method named Appendln carries a receiver.
			fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
			if !ok || fn.Name() != "Appendln" || fn.Pkg() == nil || fn.Pkg().Path() != "fmt" {
				return true
			}
			if sig, isSig := fn.Type().(*types.Signature); !isSig || sig.Recv() != nil {
				return true
			}
			// Only report string operands — the shape the append chain can
			// reproduce at all. %v over a []byte prints the decimal slice
			// representation, over an int a number: different formatting,
			// not this pattern, never reported.
			operands := call.Args[1:]
			for _, arg := range operands {
				if !ps5033StringUnderlying(pass.TypesInfo.TypeOf(arg)) {
					return true
				}
			}
			// The fix demands the provably bit-identical subset (shared with
			// PS2033): unnamed []byte destination, every operand EXACTLY the
			// predeclared string, and every operand after the first inert
			// (the chain writes to buf between operand evaluations;
			// fmt.Appendln evaluates every argument at the call site).
			fix := (*analysis.SuggestedFix)(nil)
			if ps2033FixableShape(pass.TypesInfo, call.Args[0], operands) {
				fix = ps2041Fix(f, call, sel)
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
				Message: ps2041Msg,
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2041Fix rewrites fmt.Appendln(buf, a, b) to
// append(append(append(append(buf, a...), ' '), b...), '\n'): buf and every
// operand stay byte-verbatim in place; only the scaffolding is edited. One
// operand needs 2 appends (its spread and the '\n'), each further operand 2
// more (its ' ' separator and its spread), so n operands nest 2n appends; the
// call's own parentheses serve the innermost level. The span between buf and
// the first operand (the comma) is untouched, so a comment there survives. A
// comment inside any rewritten scaffolding span would be destroyed — the fix
// is withheld then and the report stays advisory.
func ps2041Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	args := call.Args
	last := len(args) - 1
	// The edited spans: the callee itself, each inter-operand comma (which
	// becomes a "...), ' '), " chain link), and lastOperand..) (where the
	// final ... and the '\n' append land).
	if ps2111CommentIn(f, sel.Pos(), sel.End()) ||
		ps2111CommentIn(f, args[last].End(), call.End()) {
		return nil
	}
	for i := 1; i < last; i++ {
		if ps2111CommentIn(f, args[i].End(), args[i+1].Pos()) {
			return nil
		}
	}
	operands := len(args) - 1
	edits := []analysis.TextEdit{
		{Pos: sel.Pos(), End: sel.End(), NewText: []byte(strings.Repeat("append(", 2*operands-1) + "append")},
	}
	for i := 1; i < last; i++ {
		edits = append(edits, analysis.TextEdit{Pos: args[i].End(), End: args[i+1].Pos(), NewText: []byte("...), ' '), ")})
	}
	edits = append(edits, analysis.TextEdit{Pos: args[last].End(), End: call.End(), NewText: []byte(`...), '\n')`)})
	return &analysis.SuggestedFix{
		Message:   "replace fmt.Appendln with a nested append chain",
		TextEdits: edits,
	}
}
