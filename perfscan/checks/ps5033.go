package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5033 reports fmt.Append(buf, s) with a SINGLE string operand — where the
// direct append(buf, s...) writes the identical bytes without fmt's printer
// machinery. The plain-Append sibling of PS2141 (fmt.Appendf with a lone %s).
var PS5033 = register(&lint.Check{
	ID:       "PS5033",
	Category: "alloc",
	Slug:     "append-single-string",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Append with a single string runs fmt's printer to append bytes append writes directly",
		Text: `fmt.Append(b, a...) appends fmt.Sprint(a...): it boxes every operand
into an interface, acquires a pp printer from fmt's sync.Pool, formats each
operand as %v into the pooled buffer, and copies that buffer onto b. When the
call has exactly ONE operand whose static type is the predeclared string, all
of that machinery reproduces append(buf, s...) byte for byte: with one operand
Sprint's between-operand spacing rule never applies, and %v over a value whose
dynamic type is exactly string takes fmt's reflection-free fast path that
writes the string's raw bytes verbatim — no quoting, no validation, invalid
UTF-8 and the empty string included — which is precisely what the builtin
append's string special case copies. The rewrite needs no fmt at all and no
import, and removes the interface box (the call's only allocation when buf has
capacity).

The match is deliberately narrow — it is the whole safety story. The callee
must resolve via type information to the package-level fmt.Append (a shadowed
fmt or a method named Append does not), the call must have exactly two
arguments and must not spread them (fmt.Append(buf, parts...) passes an
unknown number of operands, and two or more operands engage Sprint's spacing
rule), and the operand's type must have underlying string — %v over a []byte
prints the DECIMAL slice representation "[104 105]", nothing like append, so
byte-slice operands are not this pattern at all and are never reported. The
automatic fix additionally requires the operand's type to be EXACTLY the
predeclared string (an untyped string constant defaults to it): a NAMED string
type could implement fmt.Stringer or fmt.Formatter, which %v would honor and
append would not — named operands keep the advisory report only. The
destination buf must be an unnamed []byte, so append(buf, s...) reproduces
fmt.Append's []byte return type exactly; a named []byte destination keeps the
advisory report. s is evaluated exactly once in the same position in both
forms, and both forms grow buf with the same builtin append, so aliasing and
capacity behavior match too.

The fix keeps buf and the operand byte-verbatim in place and edits only the
scaffolding: fmt.Append becomes append and a trailing ... is added to spread
the operand. Each rewrite removes the file's fmt.Append selector, so — like
PS2107/PS2122/PS2141 — the fixes are withheld (advisory report only) when
applying all of them would rewrite the file's last fmt reference and orphan
the import. A comment inside the rewritten scaffolding suppresses the fix and
keeps the report.`,
		Before: `buf = fmt.Append(buf, name)`,
		After:  `buf = append(buf, name...)`,
		MeasuredWin: `BenchmarkPS5033 (a 25-byte string appended to a preallocated
buffer, Apple M2 Pro, go1.26): fmt.Append ~36.7 ns/op, 16 B/op, 1 alloc/op vs
append(buf, s...) ~2.1 ns/op, 0 B/op, 0 allocs/op (~17x faster, and the
allocation disappears — Append boxes the string operand into an interface,
which the builtin append never does; both write into the buffer's existing
capacity). The same shape of win PS2141 measured for the Appendf("%s")
sibling.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5033",
		Doc:  "fmt.Append with a single string operand; append(buf, s...) is identical and cheaper",
		Run:  runPS5033,
	},
})

func runPS5033(pass *analysis.Pass) (any, error) {
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
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				return true
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
			// Only a single operand whose underlying type is string is this
			// pattern: two or more operands engage Sprint's spacing rule, and
			// %v over a []byte prints "[104 105]" — different formatting
			// entirely, never reported.
			if !ps5033StringUnderlying(pass.TypesInfo.TypeOf(call.Args[1])) {
				return true
			}
			// The fix additionally requires the destination to be an unnamed
			// []byte and the operand EXACTLY the predeclared string —
			// otherwise the rewrite is not provably bit-identical (a named
			// string may implement Stringer/Formatter that %v honors and
			// append does not; a named destination changes the returned slice
			// type) and no fix is offered.
			fix := (*analysis.SuggestedFix)(nil)
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				ps5033PlainString(pass.TypesInfo.TypeOf(call.Args[1])) {
				fix = ps5033Fix(f, call, sel)
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
				Message: "fmt.Append with a single string operand boxes it into an interface and walks fmt's printer through a pooled buffer; append(buf, s...) writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps5033StringUnderlying reports whether a lone fmt.Append operand of type t is
// a string append in spirit: its default type's underlying type is the
// predeclared string. Covers named and unnamed strings — the fix guard narrows
// to the provably bit-identical predeclared subset. []byte is deliberately NOT
// stringish here: %v over a byte slice prints the decimal slice representation,
// so a []byte operand is a different pattern entirely.
func ps5033StringUnderlying(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := types.Default(t).Underlying().(*types.Basic)
	return ok && b.Info()&types.IsString != 0
}

// ps5033PlainString reports whether t is EXACTLY the predeclared string type
// (an untyped string constant defaults to it). A named string type may carry a
// String()/Format() method that %v honors and append does not, so it is out —
// types.Default of a named type is the named type itself, never a *types.Basic.
func ps5033PlainString(t types.Type) bool {
	if t == nil {
		return false
	}
	basic, ok := types.Default(t).(*types.Basic)
	return ok && basic.Kind() == types.String
}

// ps5033Fix rewrites fmt.Append(buf, s) to append(buf, s...): buf and the
// operand stay byte-verbatim in place; only the scaffolding is edited. A
// comment inside a rewritten scaffolding span would be destroyed — the fix is
// withheld then and the report stays advisory. The span between buf and s (the
// comma) is untouched, so a comment there survives the rewrite.
func ps5033Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	// The rewritten spans: the fmt.Append selector (a comment can legally sit
	// between fmt and .Append) and s..) (where the ... is inserted and a
	// trailing comma would be dropped).
	if ps2111CommentIn(f, sel.Pos(), sel.End()) ||
		ps2111CommentIn(f, call.Args[1].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace fmt.Append with append",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Pos(), End: sel.End(), NewText: []byte("append")},
			{Pos: call.Args[1].End(), End: call.End(), NewText: []byte("...)")},
		},
	}
}
