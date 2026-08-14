package checks

import (
	"go/ast"
	"go/constant"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS2112 reports a clipped append chain — two or more spread appends onto
// a provably fresh, empty base slice — which is a hand-rolled slice
// concatenation with one growth step per append; slices.Concat sizes and
// allocates the result once. ADVISORY ONLY: the rewrite is not
// mechanically safe because the two forms produce observably different
// capacities (see Doc.Text).
var PS2112 = register(&lint.Check{
	ID:       "PS2112",
	Category: "alloc",
	Slug:     "clipped-append-concat",
	Level:    lint.LevelIdiomatic,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "append(append([]T(nil), a...), b...) is a hand-rolled concat; slices.Concat says it (advisory)",
		Text: `Chaining spread appends onto an empty base to join slices —
append(append([]T(nil), a...), b...) — allocates for the first spread and
then grows (often reallocating and re-copying) once per further spread,
because no append knows the final length. slices.Concat (Go 1.21+) sums
the operand lengths first and allocates the result exactly once, and it
names the intent. The check REPORTS the pattern and recommends rewriting
to slices.Concat by hand.

Only a chain whose base is PROVABLY fresh and empty is reported —
[]T(nil), []T{}, or make([]T, 0) with no capacity argument. With such a
base the chain cannot reuse an existing backing array, so slices.Concat
yields the same contents with the same operand aliasing: a fresh array,
sharing nothing with the operands. A chain onto a non-empty slice
variable is deliberately NOT reported: there append may extend the
variable's backing array in place — the normal, efficient way to extend
a slice — and Concat would change that. make([]T, 0, cap) is likewise
left alone: the explicit capacity is there to be reused by later
appends.

This check is ADVISORY (no automatic fix), because the two forms are not
bit-identical: slices.Concat clamps the result's capacity to its length
(it Grows to the exact total size), while the chained append
over-allocates via the runtime's size-class rounding (e.g. two 5-element
halves yield len 10, cap 12 under the chain but cap 10 under Concat).
That capacity difference is OBSERVABLE through aliasing: a subsequent
append to the chain's result fits in the spare capacity and REUSES the
backing array — so mutating the original slice afterwards also changes
the appended slice — whereas the same append to Concat's clamped result
REALLOCATES, and the mutation does not propagate. An automatic rewrite
would silently change that behavior. This is the same capacity/size-class
trap that keeps PS2011 and the bytes.SplitSeq rewrite advisory, and it
cannot be gated statically: whether the capacities coincide depends on
the runtime lengths of the operands.

Nil-ness adds a second, independent divergence for two of the three base
forms: with all-empty operands a []T(nil) base yields nil, exactly as
slices.Concat does (Grow(nil, 0) returns nil) — but a []T{} or
make([]T, 0) base yields a NON-nil empty slice where Concat returns nil,
and nil-ness is observable. Even for the nil-conversion base, though,
the capacity divergence above stands, so every base form is advisory.`,
		Before: `merged := append(append([]string(nil), defaults...), overrides...)`,
		After: `// Recommended hand-rewrite — after confirming no caller relies on
// the chain's spare capacity (a later append reusing the backing array):
merged := slices.Concat(defaults, overrides)`,
		MeasuredWin: `BenchmarkPS2112 (two 512-element []string halves, Apple
M2 Pro): 2.5 µs/op, 2 allocs -> 1.5 µs/op, 1 alloc (~1.7x, half the
allocations): the chain's second spread reallocates and re-copies the
first half, Concat sizes the result once.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2112",
		Doc:  "chained spread appends onto an empty base instead of slices.Concat (advisory)",
		Run:  runPS2112,
	},
})

func runPS2112(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		// consumed marks the inner append calls of a reported chain so the
		// preorder walk does not report the sub-chain again.
		consumed := map[*ast.CallExpr]bool{}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || consumed[call] {
				return true
			}
			operands, base, inner := ps2112Chain(pass, call)
			if len(operands) < 2 {
				return true
			}
			if ps2112EmptyBase(pass, base) == ps2112NotEmpty {
				return true
			}
			bt := pass.TypesInfo.TypeOf(base)
			if bt == nil {
				return true
			}
			if _, isSlice := bt.(*types.Slice); !isSlice {
				return true
			}
			// slices.Concat requires one common slice type S: every spread
			// operand must have exactly the base's (unnamed) slice type. A
			// named slice would change Concat's inferred result type; a
			// string spread into []byte would not compile.
			for _, op := range operands {
				ot := pass.TypesInfo.TypeOf(op)
				if ot == nil || !types.Identical(ot, bt) {
					return true
				}
			}
			for _, c := range inner {
				consumed[c] = true
			}
			// Advisory only — no SuggestedFix. slices.Concat clamps the
			// result's capacity to its length while the chain over-allocates
			// (size-class rounding), an observable aliasing divergence under
			// a later append; see Doc.Text and TestEquiv_Concat.
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "chained spread appends onto an empty base hand-roll a concatenation, growing the result per append; slices.Concat sizes and allocates it once",
			})
			return true
		})
	}
	return nil, nil
}

// ps2112Chain peels call as a chain of spread appends:
// append(append(BASE, a...), b...) yields operands [a b] (evaluation and
// source order), the base expression, and the peeled inner append calls.
// A single spread append yields one operand and is not a chain.
func ps2112Chain(pass *analysis.Pass, call *ast.CallExpr) (operands []ast.Expr, base ast.Expr, inner []*ast.CallExpr) {
	if !ps2112SpreadAppend(pass, call) {
		return nil, nil, nil
	}
	cur := call
	for {
		operands = append(operands, cur.Args[1])
		next, ok := cur.Args[0].(*ast.CallExpr)
		if !ok || !ps2112SpreadAppend(pass, next) {
			break
		}
		inner = append(inner, next)
		cur = next
	}
	// operands were collected outermost-first; reverse into source order.
	for i, j := 0, len(operands)-1; i < j; i, j = i+1, j-1 {
		operands[i], operands[j] = operands[j], operands[i]
	}
	return operands, cur.Args[0], inner
}

// ps2112SpreadAppend reports whether call is the builtin append applied to
// exactly one spread operand: append(x, y...). A shadowed append does not
// resolve to *types.Builtin and is rejected.
func ps2112SpreadAppend(pass *analysis.Pass, call *ast.CallExpr) bool {
	if len(call.Args) != 2 || !call.Ellipsis.IsValid() {
		return false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && b.Name() == "append"
}

// ps2112BaseKind classifies the chain's base expression.
type ps2112BaseKind int

const (
	// ps2112NotEmpty: not provably a fresh empty slice — the chain may
	// legitimately extend an existing backing array in place; silent.
	ps2112NotEmpty ps2112BaseKind = iota
	// ps2112NilConv: []T(nil) — empty AND nil, so the chain's result is
	// nil exactly when slices.Concat's is. Still advisory: the capacity
	// divergence (Concat clamps cap to len, the chain over-allocates)
	// applies to every base form.
	ps2112NilConv
	// ps2112EmptyNonNil: []T{} or make([]T, 0) — empty but non-nil: with
	// all-empty operands the chain returns a non-nil empty slice where
	// Concat returns nil, an additional nil-ness divergence on top of the
	// capacity one.
	ps2112EmptyNonNil
)

// ps2112EmptyBase classifies e as one of the recognized fresh empty base
// forms. make([]T, 0, cap) is deliberately NotEmpty: its capacity exists
// to be reused by later appends.
func ps2112EmptyBase(pass *analysis.Pass, e ast.Expr) ps2112BaseKind {
	switch x := e.(type) {
	case *ast.CompositeLit:
		if at, ok := x.Type.(*ast.ArrayType); ok && at.Len == nil && len(x.Elts) == 0 {
			return ps2112EmptyNonNil
		}
	case *ast.CallExpr:
		fun := x.Fun
		for {
			p, ok := fun.(*ast.ParenExpr)
			if !ok {
				break
			}
			fun = p.X
		}
		if at, ok := fun.(*ast.ArrayType); ok && at.Len == nil {
			// []T(nil) / ([]T)(nil) conversion.
			if len(x.Args) == 1 && !x.Ellipsis.IsValid() {
				if tv, ok := pass.TypesInfo.Types[x.Args[0]]; ok && tv.IsNil() {
					return ps2112NilConv
				}
			}
			return ps2112NotEmpty
		}
		if id, ok := fun.(*ast.Ident); ok && len(x.Args) == 2 {
			if b, ok := pass.TypesInfo.Uses[id].(*types.Builtin); ok && b.Name() == "make" {
				if at, ok := x.Args[0].(*ast.ArrayType); ok && at.Len == nil && ps2112IsZero(pass, x.Args[1]) {
					return ps2112EmptyNonNil
				}
			}
		}
	}
	return ps2112NotEmpty
}

// ps2112IsZero reports whether e is the integer constant 0.
func ps2112IsZero(pass *analysis.Pass, e ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[e]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}
