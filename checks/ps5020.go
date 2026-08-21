package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS5020 reports append(dst, []byte(s)...) — appending a string's bytes
// through a throwaway []byte conversion — and rewrites it to
// append(dst, s...), the builtin string-append special form, which
// appends the identical bytes without materializing an intermediate
// byte slice. Bit-identical for every string (see the Doc and the
// runtime differential in equiv_PS5020_test.go): both forms append
// exactly the raw bytes of s, with no UTF-8 interpretation, evaluate
// dst and s exactly once each, and produce a result with identical
// contents, length, capacity and nil-ness.
var PS5020 = register(&lint.Check{
	ID:       "PS5020",
	Category: "alloc",
	Slug:     "append-string-conversion",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "append(dst, []byte(s)...) copies s through a throwaway byte slice; append(dst, s...) appends the identical bytes directly",
		Text: `append(dst, []byte(s)...) first converts s to a fresh []byte —
historically a runtime.stringtoslicebyte call that allocates a slice
(heap once len(s) exceeds the 32-byte stack buffer) and copies len(s)
bytes — and then append copies those same bytes a SECOND time into dst.
The language's string-append special form append(dst, s...) (spec:
"append also accepts a second argument with core type bytestring") is a
single copy straight from the string's data into dst's backing array:
no intermediate slice, half the memory traffic, and the canonical
spelling of "append a string to a byte slice".

An honest caveat: since go1.22 cmd/compile's zero-copy escape analysis
rewrites EXACTLY this shape — a []byte(s) conversion whose result never
escapes and is never mutated, which the direct spread argument always
satisfies — to reuse the string's memory, so on current gc the two
forms compile to the same growslice+memmove and the measured difference
is nil (see below). The rewrite moves that guarantee from a compiler
special case into the source: it survives toolchains and build modes
without the optimization (gc before 1.22, gccgo, tinygo), keeps holding
when a refactor hoists the conversion out of the exact spread position
the optimization matches (a stored []byte(s) allocates for real), and
states the intent directly.

The rewrite is bit-identical for every string s. Both forms append
exactly the raw bytes of s — the string-append special form performs no
UTF-8 interpretation, so invalid UTF-8 rounds through verbatim. Empty s
appends zero elements in both (a nil dst stays nil). dst and s are each
evaluated exactly once in both forms, so side effects, evaluation order
and evaluation count match — which is why the fix applies even when s
is a call. []byte(s) is a fresh copy that cannot alias dst, and
append(dst, s...) reads from immutable string memory, so there is no
aliasing divergence either; resulting length and capacity are identical
because append growth depends only on len(dst), cap(dst) and the number
of appended elements — all unchanged.

The shape is matched only when it is provably the redundant conversion:

  - the callee must be the predeclared append (a shadowed append is
    rejected via type information), in its spread form with exactly two
    arguments;
  - the spread argument must be DIRECTLY a conversion whose target type
    is the unnamed []byte ([]uint8 and byte/uint8-aliases spell the
    identical type; an alias of []byte itself is resolved) — a NAMED
    byte-slice type, a composite literal []byte{...}, or a function
    call that merely looks like a conversion never matches;
  - the conversion operand must have core type string (the predeclared
    string, an untyped string constant, or a named string type — all of
    which the builtin's bytestring special case accepts; type
    parameters are conservatively skipped). []byte(b) over an existing
    byte slice is a different (no-op) conversion and never matches.

dst needs no extra guard: for the original to type-check its core type
must already be []byte, which is exactly what the string-append special
form requires, so the rewrite always compiles where the original did.

The fix keeps s verbatim in place — same text, same single evaluation —
and only deletes the conversion scaffolding around it ("[]byte(" and
the closing ")"); the ... grammatically covers the whole final
argument, so no parentheses are ever needed, and no import can be
touched. A comment inside the deleted scaffolding downgrades the fix to
an advisory report.`,
		Before: `dst = append(dst, []byte(s)...)`,
		After:  `dst = append(dst, s...)`,
		MeasuredWin: `BenchmarkPS5020 (1 KiB string appended into a reused
2 KiB-cap buffer, Apple M2 Pro, gc 1.26): append(dst, []byte(s)...)
15.0 ns/op, 0 B/op vs append(dst, s...) 15.2 ns/op, 0 B/op — identical,
because go1.22+ escape analysis turns this exact non-escaping,
never-mutated conversion into a zero-copy alias of the string. On
toolchains without that optimization (gc <=1.21, gccgo, tinygo) the
Before pays a full extra allocation+copy of len(s) bytes per call
(~2x the memory traffic, one heap allocation for len(s) > 32); the
After does a single copy on every toolchain, so the rewrite never
loses and locks the fast path into the source.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS5020",
		Doc:  "append(dst, []byte(s)...) converts s to a throwaway byte slice that append immediately copies out of; append(dst, s...) appends the identical bytes directly via the builtin string-append special form",
		Run:  runPS5020,
	},
})

const ps5020Msg = "append(dst, []byte(s)...) converts s to a throwaway byte slice that append immediately copies out of; append(dst, s...) appends the identical bytes directly"

func runPS5020(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			conv, matched := ps5020Match(pass, call)
			if !matched {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: ps5020Msg,
			}
			if fix := ps5020Fix(f, conv); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			// Keep descending: a nested match can only sit inside the
			// operand span the fix keeps verbatim, so edits never overlap.
			return true
		})
	}
	return nil, nil
}

// ps5020Match matches call against append(dst, []byte(s)...) — the
// callee pinned to the predeclared append builtin by type information,
// the spread argument directly a conversion to the unnamed []byte, and
// the conversion operand of core type string. It returns the conversion
// call.
func ps5020Match(pass *analysis.Pass, call *ast.CallExpr) (conv *ast.CallExpr, ok bool) {
	// The spread form of append always has exactly two arguments.
	if !call.Ellipsis.IsValid() || len(call.Args) != 2 {
		return nil, false
	}
	fn, isIdent := ps2108Unparen(call.Fun).(*ast.Ident)
	if !isIdent {
		return nil, false
	}
	if b, isBuiltin := pass.TypesInfo.Uses[fn].(*types.Builtin); !isBuiltin || b.Name() != "append" {
		return nil, false
	}
	conv, isCall := ps2108Unparen(call.Args[1]).(*ast.CallExpr)
	if !isCall || len(conv.Args) != 1 || conv.Ellipsis.IsValid() {
		return nil, false
	}
	// The "callee" must be a TYPE — a genuine conversion, not a function
	// call — and that type must be the unnamed []byte exactly ([]uint8
	// spells the identical type; aliases of []byte resolve to it). A
	// named byte-slice type is deliberately out of scope.
	tv, has := pass.TypesInfo.Types[conv.Fun]
	if !has || !tv.IsType() {
		return nil, false
	}
	slice, isSlice := types.Unalias(tv.Type).(*types.Slice)
	if !isSlice {
		return nil, false
	}
	elem, isBasic := types.Unalias(slice.Elem()).(*types.Basic)
	if !isBasic || elem.Kind() != types.Uint8 {
		return nil, false
	}
	// The operand must have core type string: predeclared string, an
	// untyped string constant, or a named string type — every one of
	// which append's bytestring special case accepts, so the rewrite
	// always still type-checks. A type parameter's underlying is an
	// interface and is conservatively skipped; []byte(b) over a byte
	// slice is a no-op conversion, not this pattern, and never matches.
	xt := pass.TypesInfo.TypeOf(conv.Args[0])
	if xt == nil {
		return nil, false
	}
	if basic, isStr := xt.Underlying().(*types.Basic); !isStr || basic.Info()&types.IsString == 0 {
		return nil, false
	}
	return conv, true
}

// ps5020Fix deletes the conversion scaffolding around the operand —
// "[]byte(" and the closing ")" — keeping the operand byte-verbatim in
// place (same text, same single evaluation). The ... spread covers the
// whole final argument grammatically, so the bare operand needs no
// parentheses in that position, and no import is ever touched. A
// comment inside either deleted span would be silently destroyed, so
// the report stays advisory then.
func ps5020Fix(f *ast.File, conv *ast.CallExpr) *analysis.SuggestedFix {
	x := conv.Args[0]
	if ps2111CommentIn(f, conv.Pos(), x.Pos()) || ps2111CommentIn(f, x.End(), conv.End()) {
		return nil
	}
	return &analysis.SuggestedFix{
		Message: "drop the []byte conversion and append the string directly",
		TextEdits: []analysis.TextEdit{
			{Pos: conv.Pos(), End: x.Pos()},
			{Pos: x.End(), End: conv.End()},
		},
	}
}
