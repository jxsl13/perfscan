package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS5021 reports copy(dst, []byte(s)) — a string->[]byte conversion whose
// only consumer is the builtin copy, which reads the throwaway slice and
// copies its bytes AGAIN into dst — and rewrites it to copy(dst, s): the
// builtin's spec special case (destination assignable to []byte, source of
// string type) copies the string's bytes directly in a single memmove. The
// rewrite is bit-identical on every input: same byte count returned
// (min(len(dst), len(s)) either way, since len([]byte(s)) == len(s)), same
// bytes written (byte-level, no UTF-8 interpretation), same single
// evaluation of dst and s. See the Doc for the honest gc >= 1.22 story
// (escape analysis already makes THIS exact shape zero-copy; the rewrite
// turns that optimization-dependent behavior into a spec guarantee) and
// equiv_PS5021_test.go for the runtime differential, including an
// unsafe-aliased overlapping source.
var PS5021 = register(&lint.Check{
	ID:       "PS5021",
	Category: "alloc",
	Slug:     "copy-string-source-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "copy(dst, []byte(s)) converts s just to copy from the conversion's throwaway result; copy(dst, s) copies the string's bytes directly — the builtin special-cases a string source",
		Text: `The builtin copy accepts, as a spec-level special case, a
destination assignable to []byte with a source of string type:
copy(dst, s) copies the string's bytes straight into dst in one
memmove. copy(dst, []byte(s)) instead spells out a string->[]byte
conversion — nominally runtime.stringtoslicebyte, an allocation (heap
once s outgrows the 32-byte stack buffer) plus a full len(s)-byte copy
— whose result the builtin then reads and copies AGAIN: two copies and
a discarded temporary for one byte transfer.

Honest gc note: since Go 1.22 the gc compiler's escape analysis
(the on-by-default ZeroCopy optimization) recognizes that copy never
mutates its source and lowers the conversion in THIS exact argument
position to a zero-copy reinterpretation of the string's memory — no
allocation, no temporary copy. On that toolchain the rewrite is
near-parity: a small constant per call (the conversion's slice-header
scaffolding), with the single memmove dominating both forms. The full
allocation + double-copy cost is real on Go < 1.22, on toolchains
without the optimization, and
the moment the conversion drifts out of the argument slot (hoisted into
a variable escape analysis can no longer prove unmutated). The rewrite
replaces an optimization-dependent behavior with a spec guarantee — and
is shorter.

The rewrite is bit-identical on every input. copy returns
min(len(dst), len(src)) and copies exactly that many bytes;
len([]byte(s)) == len(s), so the count and the bytes written are
identical — byte-level semantics, no UTF-8 interpretation, invalid
UTF-8 preserved. dst and s are each evaluated exactly once in both
forms (the conversion operand is kept byte-verbatim, so even a
call-typed s keeps its single evaluation), and copy is specified to
handle overlapping source and destination, so even an unsafe-built s
aliasing dst behaves identically (the differential test pins this).

The automatic fix applies only when the callee is the predeclared
builtin copy (pinned through type information — a shadowed copy or a
same-named method never matches), the source argument is a conversion
spelled with a literal byte-slice array type ([]byte or []uint8,
resolving to exactly []byte — so no named types whose spelling the fix
would delete, and no cross-package alias whose removal could orphan an
import), and the conversion operand is of string type. A conversion
through a defined byte-slice type (Bytes(s)) or an alias spelling
(bs(s)) is still reported — the temporary is just as redundant — but
stays advisory. A comment inside the deleted conversion scaffolding
withholds the fix.`,
		Before: `n := copy(dst, []byte(s))`,
		After:  `n := copy(dst, s)`,
		MeasuredWin: `BenchmarkPS5021 (64-byte string, Apple M2 Pro, go1.26):
copy(dst, []byte(s)) 2.0 ns/op, 0 B/op -> copy(dst, s) 1.8 ns/op,
0 B/op — near-parity, because on gc >= 1.22 the zero-copy
optimization already eliminates the conversion's allocation and
temporary copy for this exact shape, leaving only its slice-header
scaffolding; at 4 KiB both forms converge to the same ~48 ns single
memmove. Where that optimization does not apply — Go < 1.22, other
toolchains, or the conversion hoisted out of the argument slot
(measured via -gcflags=-d=zerocopy=0) — the Before is 17.4 ns/op
with a 64 B allocation at 64 bytes (~9.5x) and 415 ns/op with a
4096 B allocation at 4 KiB (~8.7x): a full extra copy plus an
allocation per call, scaling with the bytes moved.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5021",
		Doc:  "copy(dst, []byte(s)) materializes a throwaway byte slice the builtin immediately copies again; copy(dst, s) uses the builtin's string-source special case to copy the bytes directly, bit-identically",
		Run:  runPS5021,
	},
})

const ps5021Msg = "copy(dst, []byte(s)) converts s to a throwaway byte slice the builtin immediately copies again; copy(dst, s) copies the string's bytes directly — the builtin special-cases a string source, bit-identically"

// ps5021ByteSlice is the rewrite's required conversion target type, []byte.
var ps5021ByteSlice = types.NewSlice(types.Typ[types.Uint8])

func runPS5021(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			conv, matched := ps5021Match(pass, call)
			if !matched {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: ps5021Msg,
			}
			if fix := ps5021Fix(pass, f, conv); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			// Keep descending: a nested match can only sit inside the
			// verbatim operand spans, whose edits never overlap this
			// site's deletion edits.
			return true
		})
	}
	return nil, nil
}

// ps5021Match matches call against copy(dst, T(s)) where copy is the
// predeclared builtin (pinned through type information — a shadowed copy
// or a same-named method never matches), T is a byte-slice type (a slice
// whose element type is exactly uint8, so the copy(dst, s) rewrite always
// type-checks: any dst whose element type admits a []byte source is
// assignable to []byte) and s is of string type (untyped string constants
// included; untyped nil and byte-slice operands are not). It returns the
// conversion call.
func ps5021Match(pass *analysis.Pass, call *ast.CallExpr) (conv *ast.CallExpr, ok bool) {
	if len(call.Args) != 2 || call.Ellipsis.IsValid() {
		return nil, false
	}
	id, isIdent := ps2108Unparen(call.Fun).(*ast.Ident)
	if !isIdent {
		return nil, false
	}
	if b, isBuiltin := pass.TypesInfo.Uses[id].(*types.Builtin); !isBuiltin || b.Name() != "copy" {
		return nil, false
	}
	conv, isCall := ps2108Unparen(call.Args[1]).(*ast.CallExpr)
	if !isCall || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false
	}
	tv, hasTV := pass.TypesInfo.Types[conv.Fun]
	if !hasTV || !tv.IsType() {
		return nil, false
	}
	slice, isSlice := tv.Type.Underlying().(*types.Slice)
	if !isSlice || !types.Identical(slice.Elem(), types.Typ[types.Uint8]) {
		return nil, false
	}
	operand, hasOp := pass.TypesInfo.Types[conv.Args[0]]
	if !hasOp {
		return nil, false
	}
	basic, isBasic := operand.Type.Underlying().(*types.Basic)
	if !isBasic || basic.Info()&types.IsString == 0 {
		return nil, false
	}
	return conv, true
}

// ps5021Fix builds the copy(dst, s) rewrite for one site, or nil when a
// guard fails and the report must stay advisory. The fix deletes only the
// conversion scaffolding — the type spelling and its parentheses — keeping
// dst and the operand byte-verbatim in place: both are evaluated exactly
// once in both forms, so no purity guard is needed. The conversion must be
// spelled with a literal byte-slice array type resolving to exactly []byte:
// a defined type's spelling is user vocabulary the fix must not delete, and
// a cross-package alias could be the import's last reference.
func ps5021Fix(pass *analysis.Pass, f *ast.File, conv *ast.CallExpr) *analysis.SuggestedFix {
	at, isArray := ps2108Unparen(conv.Fun).(*ast.ArrayType)
	if !isArray || at.Len != nil {
		return nil
	}
	if _, eltIsIdent := ps2108Unparen(at.Elt).(*ast.Ident); !eltIsIdent {
		return nil
	}
	if !types.Identical(pass.TypesInfo.Types[conv.Fun].Type, ps5021ByteSlice) {
		return nil
	}
	// The deleted spans are the conversion punctuation around the kept
	// operand; a comment anywhere there would be silently destroyed —
	// advisory then.
	operand := conv.Args[0]
	if ps2111CommentIn(f, conv.Pos(), operand.Pos()) ||
		ps2111CommentIn(f, operand.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "drop the redundant []byte conversion: the builtin copy accepts a string source directly",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: operand.Pos()},
			{Pos: operand.End(), End: conv.End()},
		},
	}
}
