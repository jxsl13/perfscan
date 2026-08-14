package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2141 reports fmt.Appendf(buf, "%s", s) — a lone %s over a plain string or
// unnamed []byte — where the direct append(buf, s...) writes the identical
// bytes without fmt's formatter machinery.
var PS2141 = register(&lint.Check{
	ID:       "PS2141",
	Category: "alloc",
	Slug:     "appendf-single-s",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "fmt.Appendf(buf, \"%s\", s) runs fmt's formatter to append bytes append writes directly",
		Text: `fmt.Appendf parses its format string, boxes the argument into an
interface, and walks fmt's formatter state machine through a pooled pp buffer —
even when the format is a lone %s splicing one string onto the buffer. For that
shape the result is byte-for-byte append(buf, s...): the builtin append has a
special case for appending a string to a []byte, so it needs no fmt at all and
no import.

The match is deliberately narrow — it is the whole safety story. The callee
must resolve via type information to the package-level fmt.Appendf (a shadowed
fmt or a method named Appendf does not), the call must not spread its arguments
(no ...), and the format must be a string literal whose value is EXACTLY "%s":
any literal text, a flag, a width, %% or any other verb disqualifies it. The
value argument must have type EXACTLY the predeclared string or an UNNAMED
[]byte: %s writes a string's or a byte slice's bytes verbatim (invalid UTF-8 and
nil included, matching append), but a NAMED string or []byte type could
implement fmt.Stringer or fmt.Formatter, which %s would honor and append would
not — so named types stay advisory. The destination buf must be an unnamed
[]byte, so append(buf, s...) reproduces fmt.Appendf's []byte return type exactly;
a named []byte destination keeps the advisory report.

The fix keeps buf and the value argument byte-verbatim in place and edits only
the scaffolding: fmt.Appendf becomes append, the format literal (with its
surrounding commas) is dropped, and a trailing ... is added to spread the value.
Each rewrite removes the file's fmt.Appendf selector, so — like PS2107/PS2122 —
the fixes are withheld (advisory report only) when applying all of them would
rewrite the file's last fmt reference and orphan the import. A comment inside the
rewritten scaffolding suppresses the fix and keeps the report.`,
		Before: `buf = fmt.Appendf(buf, "%s", name)`,
		After:  `buf = append(buf, name...)`,
		MeasuredWin: `BenchmarkPS2141 (a 25-byte string appended to a preallocated
buffer, Apple M2 Pro): fmt.Appendf ~34.9 ns/op, 16 B/op, 1 alloc/op vs
append(buf, s...) ~2.1 ns/op, 0 B/op, 0 allocs/op (~17x faster, and the
allocation disappears — Appendf boxes the string argument into an interface,
which the builtin append never does; both write into the buffer's existing
capacity).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2141",
		Doc:  "fmt.Appendf with a lone %s over a string/[]byte; append(buf, s...) is identical and cheaper",
		Run:  runPS2141,
	},
})

func runPS2141(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// Collect first: fixes are suppressed when applying all of them would
		// rewrite the file's last fmt reference and orphan the import (the
		// runner never prunes imports; same guard as PS2107/PS2122).
		type site struct {
			call *ast.CallExpr
			fix  *analysis.SuggestedFix
		}
		var sites []site
		fixable := 0
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 3 || call.Ellipsis.IsValid() {
				return true
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
			// The format must be the string LITERAL "%s" exactly.
			lit, ok := call.Args[1].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if format, err := strconv.Unquote(lit.Value); err != nil || format != "%s" {
				return true
			}
			// Only report %s over a string or byte slice — the shape append can
			// reproduce. %s over any other type (an int, a Stringer-less struct)
			// is different formatting entirely and is not this pattern.
			if !ps2141Stringish(pass.TypesInfo.TypeOf(call.Args[2])) {
				return true
			}
			// The destination must be an unnamed []byte and the value a plain
			// string or unnamed []byte — otherwise the rewrite is not provably
			// bit-identical (named types may implement Stringer/Formatter, or
			// change the returned slice type) and no fix is offered.
			fix := (*analysis.SuggestedFix)(nil)
			if ps2141ByteSliceUnnamed(pass.TypesInfo.TypeOf(call.Args[0])) &&
				ps2141AppendableVerbatim(pass.TypesInfo.TypeOf(call.Args[2])) {
				fix = ps2141Fix(f, call, sel)
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
				Message: "fmt.Appendf with a lone %s over a string/[]byte boxes the argument and walks fmt's formatter state machine; append(buf, s...) writes the identical bytes directly",
			}
			if emitFixes && st.fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*st.fix}
			}
			pass.Report(diag)
		}
	}
	return nil, nil
}

// ps2141Stringish reports whether %s over a value of type t is a string append
// in spirit: its underlying type is the predeclared string, or a slice whose
// element's underlying type is byte. Covers named and unnamed string/[]byte —
// the fix guards narrow it to the provably bit-identical subset.
func ps2141Stringish(t types.Type) bool {
	if t == nil {
		return false
	}
	u := types.Default(t).Underlying()
	if b, ok := u.(*types.Basic); ok {
		return b.Info()&types.IsString != 0
	}
	if sl, ok := u.(*types.Slice); ok {
		eb, ok := sl.Elem().Underlying().(*types.Basic)
		return ok && eb.Kind() == types.Byte
	}
	return false
}

// ps2141ByteSliceUnnamed reports whether t is exactly the unnamed []byte type —
// the destination whose type append(buf, ...) preserves as fmt.Appendf's []byte
// return.
func ps2141ByteSliceUnnamed(t types.Type) bool {
	sl, ok := t.(*types.Slice)
	if !ok {
		return false
	}
	b, ok := sl.Elem().(*types.Basic)
	return ok && b.Kind() == types.Byte
}

// ps2141AppendableVerbatim reports whether %s over a value of type t is
// byte-identical to spreading it into append: a value whose DEFAULT type is the
// predeclared string, or an UNNAMED []byte. A named string/[]byte type may carry
// a String()/Format() method that %s honors and append does not, so it is out.
func ps2141AppendableVerbatim(t types.Type) bool {
	if t == nil {
		return false
	}
	// An untyped constant string materializes as the predeclared string.
	if basic, ok := types.Default(t).(*types.Basic); ok {
		return basic.Kind() == types.String
	}
	// An UNNAMED []byte (a *types.Slice, not a *types.Named) cannot carry
	// methods, so %s and append agree on its bytes.
	if sl, ok := t.(*types.Slice); ok {
		b, ok := sl.Elem().(*types.Basic)
		return ok && b.Kind() == types.Byte
	}
	return false
}

// ps2141Fix rewrites fmt.Appendf(buf, "%s", s) to append(buf, s...): buf and the
// value stay byte-verbatim in place; only the scaffolding is edited. A comment
// inside a rewritten scaffolding span would be destroyed — the fix is withheld
// then and the report stays advisory.
func ps2141Fix(f *ast.File, call *ast.CallExpr, sel *ast.SelectorExpr) *analysis.SuggestedFix {
	// The dropped spans: buf..value (the format literal and its commas) and
	// value..) (where the ... is inserted).
	if ps2111CommentIn(f, call.Args[0].End(), call.Args[2].Pos()) ||
		ps2111CommentIn(f, call.Args[2].End(), call.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "replace fmt.Appendf with append",
		TextEdits: []analysis.TextEdit{
			{Pos: sel.Pos(), End: sel.End(), NewText: []byte("append")},
			{Pos: call.Args[0].End(), End: call.Args[2].Pos(), NewText: []byte(", ")},
			{Pos: call.Args[2].End(), End: call.End(), NewText: []byte("...)")},
		},
	}
}
