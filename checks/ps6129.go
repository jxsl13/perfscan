package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strconv"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

var PS6129 = register(&lint.Check{
	ID: "PS6129", Category: "verify", Slug: "dense-gemm-zero-suffix",
	Level: lint.LevelAggressive, AutoFix: false, NeedsConfig: true,
	Vocab: []string{"denseRowGEMMFuncs", "causalZeroGEMMContracts"},
	Doc: lint.Documentation{
		Title: "a dense GEMM consumes an explicitly cleared matrix suffix",
		Text: `A row-major matrix whose rows have a common explicitly cleared
suffix can make a dense product execute redundant rectangular arithmetic.
For n rows, sequence width S, live bound L and output width D, A*B performs
n*(S-L)*D such multiply-accumulates when L<S. After a complete transpose,
A^T*B computes (S-L)*n*D multiply-accumulates in zero output rows.

PS6129 uses exact denseRowGEMMFuncs vocabulary, not names such as causal,
probability or gradient. Each entry is a fully qualified package-level
function with the reviewed contract (A,B,C []float32 or []float64,
start,end,depth,columns int): row-major C[start:end,:]=A[start:end,:]*B,
read-only non-retained inputs and store semantics. Interfaces, function
values, methods and incompatible signatures are not matched.

The uncontracted grammar requires a fresh local make([]float32/float64,n*S),
followed by a complete for r := range n loop containing only
clear(A[r*S+L:(r+1)*S]). A complete two-loop transpose into a fresh local
make([]T,S*n) can carry the proof into output-row products. The clear,
transpose and consumers must be consecutive statements in one lexical
block. Geometry is immutable, and fresh buffers may not escape or acquire
aliases. Unrecognized intervening statements invalidate the proof.
Already trimmed dimensions, arbitrary masks, conditional or partial clears,
stale pooled tails, overwritten buffers, aliased outputs and uncontracted
opaque helper semantics stay silent.

For raw pooled scratch and per-row causal bounds, causalZeroGEMMContracts
selects one exact non-generic owner function, unique typed geometry/bound
bindings, source/transpose binding pairs and exact helper identities.
The contract attests exclusive allocator leases (including disjointness from
other operands/outputs), synchronous non-retaining helpers with no returned
results, self-attention
geometry, and bounds(query) returning upper=offset+query+1 when causal or
the full sequence otherwise. A probability helper must overwrite its matrix
and clear masked suffixes on EVERY architecture/compile-time dispatch path,
including nonfinite inputs. These are reviewed project facts, not conclusions
drawn from helper names. Missing, contradictory and duplicate-owner contracts
remain silent and count as missing vocabulary in the runner.

Source still proves exact floating-point/int helper signatures, immutable
geometry, allocator/deference flow, full n*S region extents and disjoint
integer-multiple partitions within a shared scratch lease; canonical full
row views; bounds(i0+r) and a final unconditional builtin clear(row[upper:])
on every row, with all matrix writes confined to the current canonical row
view (whole-matrix writes and opaque buffer helpers within that loop are
rejected because they can recontaminate previously cleared rows); complete
transpose indexing (including jointly filled P/dS
transposes); and the relevant full-dimension GEMM consumer. Direct aliases,
early pool releases, buffer mutation, changed bounds, partial clears,
partial transposes, escaping buffers, geometry addressing/pointer methods,
closures and jump control flow are rejected. Buffer writes invalidate prior
zero proofs; unrelated builtin copies can preserve them. The contracted
flow is deliberately restricted to the owner's direct lexical body.

For causal rows r in [0,n), the common union bound is L=offset+i0+n. Findings
are conditional on causal mode and L<S; noncausal/final bands do not have this
suffix opportunity. The pinned GoAI issue #983 source reproduces all three
dQ/dV/dK products, including its shared raw-scratch transpose partition.
The diagnostic's symbolic work count is conditional on L<S and is not a
measured speedup or a claim that a runtime change is valid.

There is NO automatic fix. 0*NaN and 0*Inf produce NaNs, so skipping them
requires an explicit finite-operand guard and unchanged numeric fallback.
Check signed-zero bits and accumulation order against the frozen registered
production path, all gradients, nonfinite classes and input immutability.
Skipped output rows in pooled raw scratch must be fully overwritten or
cleared before the unchanged ordered reduction reads them. Keep architecture
dispatch scoped to measured targets; ARM64 qualification does not qualify
AMD64. The withdrawn GoAI prototype trimmed dK/dV output rows, kept the full
dQ contraction, and required both finite guards and tail clears.

Measure complete backward operations and the GPT objective, including
repacking, transpose, clears, scheduling, softmax, reduction, allocated bytes
and allocations. Use fixed same-host interleaved A/B campaigns, excluded
warmup, every retained sample, predeclared significance/spread gates and an
exact frozen-production oracle before timing. An invalid-output force-off
control is only an upper bound. Never select favorable runs or relax gates.`,
		Before: `for r := range n {
    clear(a[r*seq+live:(r+1)*seq])
}
gemm(a, b, out, 0, n, seq, width)`,
		After: `// Advisory: evaluate a stride-aware bounded product only after
// finite fallback, exact production oracle and whole-workload gates.
// Clear every skipped scratch output before the ordered reduction.`,
		MeasuredWin: `Issue #983 derives 37.5% fewer multiply-accumulates in
three final products at S512/H8/D64/band128 (26.47% of all five), not a
measured runtime gain. The longer M2 Pro campaign measured S512 -17.63%
(p=.017) and S1024 -16.47% (p=.001), but S256 failed significance and spread
gates, and the GPT objective had no significant latency gain (p=.805),
2.01% more allocated bytes (p=.007), and 0.22% more allocations (p=.005).
Allocation sites and source-level causes were not established. The prototype
was withdrawn; the original runtime was restored. No optimization is promoted.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6129", Doc: "dense GEMM consumes a source-proven zero suffix", Run: runPS6129},
})

type ps6129Zero struct {
	buffer             types.Object
	rows, stride, live ast.Expr
	transpose          bool
}

func runPS6129(pass *analysis.Pass) (any, error) {
	_, err := runPS6129WithFuncs(pass, config.Current().DenseRowGEMMFuncs)
	if err != nil {
		return nil, err
	}
	return runPS6129WithContracts(pass, config.Current().CausalZeroGEMMContracts)
}

func runPS6129WithFuncs(pass *analysis.Pass, funcs map[string]bool) (any, error) {
	if len(funcs) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Type.TypeParams != nil {
				continue
			}
			unsafe := false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch n.(type) {
				case *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
					unsafe = true
				}
				return !unsafe
			})
			if unsafe {
				continue
			}
			// Reuse the typed multi-call collector and lexical-block attribution.
			calls := ps6022Calls(pass, fn.Body, ps6019Parents(fn.Body))
			byCall := make(map[*ast.CallExpr]ps6022Call, len(calls))
			for _, c := range calls {
				byCall[c.call] = c
			}
			ps6032Blocks(fn.Body, func(block *ast.BlockStmt) {
				zeros := map[types.Object]ps6129Zero{}
				for _, stmt := range block.List {
					if z, ok := ps6129Clear(pass, fn.Body, stmt, funcs); ok {
						zeros[z.buffer] = z
						continue
					}
					if z, ok := ps6129Transpose(pass, fn.Body, stmt, zeros, funcs); ok {
						zeros[z.buffer] = z
						continue
					}
					call := ps6032StatementCall(stmt)
					c, ok := byCall[call]
					if !ok || c.block != block || !ps6129GEMM(pass, c, funcs) {
						clear(zeros)
						continue
					}
					// Fresh tracked buffers cannot alias outputs or other inputs.
					for object, z := range zeros {
						if ps6129Object(pass, call.Args[2]) == object {
							delete(zeros, object)
							continue
						}
						if ps6129Object(pass, call.Args[0]) != object || !ps6129Int(pass, call.Args[3], 0) {
							continue
						}
						end, depth := z.rows, z.stride
						if z.transpose {
							end, depth = z.stride, z.rows
						}
						if !ps6129Equal(pass, call.Args[4], end) || !ps6129Equal(pass, call.Args[5], depth) || ps6129Equal(pass, z.live, z.stride) {
							continue
						}
						if z.transpose {
							pass.Reportf(call.Pos(), "dense GEMM computes zero suffix output rows [%s,%s) after a proven full transpose; if the live bound is below the full width, redundant work is (%s-%s)*%s*%s multiply-accumulates; advisory only: preserve finite fallback, signed-zero/accumulation semantics and clear skipped scratch tails before ordered reduction; validate frozen production and complete-workload latency/allocations on each target", exprTextRendered(z.live), exprTextRendered(z.stride), exprTextRendered(z.stride), exprTextRendered(z.live), exprTextRendered(z.rows), exprTextRendered(call.Args[6]))
						} else {
							pass.Reportf(call.Pos(), "dense GEMM contracts over explicitly cleared matrix suffix [%s,%s); if the live bound is below the full width, redundant work is %s*(%s-%s)*%s multiply-accumulates; advisory only: preserve finite fallback, signed-zero/accumulation semantics and clear skipped scratch tails before ordered reduction; validate frozen production and complete-workload latency/allocations on each target", exprTextRendered(z.live), exprTextRendered(z.stride), exprTextRendered(z.rows), exprTextRendered(z.stride), exprTextRendered(z.live), exprTextRendered(call.Args[6]))
						}
					}
				}
			})
		}
	}
	return nil, nil
}

func ps6129GEMM(pass *analysis.Pass, c ps6022Call, funcs map[string]bool) bool {
	fn, sig, ok := typedCallee(pass, c.call.Fun)
	if !ok || fn.Pkg() == nil || sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Results().Len() != 0 || len(c.call.Args) != 7 || sig.Params().Len() != 7 || !funcs[fn.Pkg().Path()+"."+fn.Name()] {
		return false
	}
	for i := 0; i < 3; i++ {
		slice, ok := types.Unalias(sig.Params().At(i).Type()).(*types.Slice)
		if !ok || (!types.Identical(slice.Elem(), types.Typ[types.Float32]) && !types.Identical(slice.Elem(), types.Typ[types.Float64])) {
			return false
		}
		if !types.Identical(sig.Params().At(0).Type(), slice) {
			return false
		}
	}
	for i := 3; i < 7; i++ {
		if !types.Identical(sig.Params().At(i).Type(), types.Typ[types.Int]) {
			return false
		}
	}
	return true
}

func ps6129Object(pass *analysis.Pass, e ast.Expr) types.Object {
	id, ok := ps2110Unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	return identObject(pass, id)
}

func ps6129Equal(pass *analysis.Pass, a, b ast.Expr) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	a, b = ps2110Unparen(a), ps2110Unparen(b)
	switch x := a.(type) {
	case *ast.Ident:
		y, ok := b.(*ast.Ident)
		return ok && identObject(pass, x) != nil && identObject(pass, x) == identObject(pass, y)
	case *ast.BasicLit:
		y, ok := b.(*ast.BasicLit)
		return ok && x.Kind == y.Kind && x.Value == y.Value
	case *ast.BinaryExpr:
		y, ok := b.(*ast.BinaryExpr)
		return ok && x.Op == y.Op && ((ps6129Equal(pass, x.X, y.X) && ps6129Equal(pass, x.Y, y.Y)) || ((x.Op == token.MUL || x.Op == token.ADD) && ps6129Equal(pass, x.X, y.Y) && ps6129Equal(pass, x.Y, y.X)))
	}
	return false
}

func ps6129Int(pass *analysis.Pass, e ast.Expr, value int64) bool {
	return ps6129Equal(pass, e, &ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(value, 10)})
}

func ps6129Product(pass *analysis.Pass, e, a, b ast.Expr) bool {
	return ps6129Equal(pass, e, &ast.BinaryExpr{X: a, Op: token.MUL, Y: b})
}

func ps6129Range(stmt ast.Stmt) (*ast.RangeStmt, *ast.Ident, bool) {
	r, ok := stmt.(*ast.RangeStmt)
	if !ok || r.Tok != token.DEFINE || r.Value != nil {
		return nil, nil, false
	}
	id, ok := r.Key.(*ast.Ident)
	return r, id, ok && id.Name != "_"
}

func ps6129Clear(pass *analysis.Pass, body *ast.BlockStmt, stmt ast.Stmt, funcs map[string]bool) (ps6129Zero, bool) {
	r, index, ok := ps6129Range(stmt)
	if !ok || len(r.Body.List) != 1 {
		return ps6129Zero{}, false
	}
	call := ps6032StatementCall(r.Body.List[0])
	if call == nil || len(call.Args) != 1 {
		return ps6129Zero{}, false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ps6129Zero{}, false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || builtin.Name() != "clear" {
		return ps6129Zero{}, false
	}
	slice, ok := call.Args[0].(*ast.SliceExpr)
	if !ok || slice.Slice3 || slice.Low == nil || slice.High == nil {
		return ps6129Zero{}, false
	}
	low, ok := ps2110Unparen(slice.Low).(*ast.BinaryExpr)
	if !ok || low.Op != token.ADD {
		return ps6129Zero{}, false
	}
	mul, ok := ps2110Unparen(low.X).(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL || !ps6129Equal(pass, mul.X, index) {
		return ps6129Zero{}, false
	}
	stride, live := mul.Y, low.Y
	if ps6129Object(pass, stride) == nil || ps6129Object(pass, live) == nil || ps6129Object(pass, r.X) == nil {
		return ps6129Zero{}, false
	}
	next := &ast.BinaryExpr{X: index, Op: token.ADD, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}
	if !ps6129Product(pass, slice.High, next, stride) {
		return ps6129Zero{}, false
	}
	object := ps6129Object(pass, slice.X)
	if object == nil || !ps6129Fresh(pass, body, object, r.X, stride, funcs) || !ps6129Immutable(pass, body, r.X, stride, live) {
		return ps6129Zero{}, false
	}
	return ps6129Zero{buffer: object, rows: r.X, stride: stride, live: live}, true
}

// A fresh make buffer has only indexed read/write, matched clear-slice and
// reviewed GEMM input/output roles. Reject all aliases, escape and rebinding.
func ps6129Fresh(pass *analysis.Pass, body *ast.BlockStmt, object types.Object, a, b ast.Expr, funcs map[string]bool) bool {
	parents := ps6019Parents(body)
	definitions := 0
	valid := true
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || identObject(pass, id) != object {
			return true
		}
		switch p := parents[id].(type) {
		case *ast.AssignStmt:
			if len(p.Lhs) != 1 || len(p.Rhs) != 1 || p.Lhs[0] != id || p.Tok != token.DEFINE {
				valid = false
				break
			}
			call, ok := p.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) != 2 || !ps6129Product(pass, call.Args[1], a, b) {
				valid = false
				break
			}
			callee, ok := call.Fun.(*ast.Ident)
			if !ok {
				valid = false
				break
			}
			builtin, ok := pass.TypesInfo.Uses[callee].(*types.Builtin)
			if !ok || builtin.Name() != "make" {
				valid = false
				break
			}
			slice, ok := types.Unalias(object.Type()).(*types.Slice)
			if !ok || (!types.Identical(slice.Elem(), types.Typ[types.Float32]) && !types.Identical(slice.Elem(), types.Typ[types.Float64])) {
				valid = false
				break
			}
			definitions++
		case *ast.IndexExpr:
			if p.X != id {
				valid = false
				break
			}
			// Addressing even one element can expose the complete backing array.
			var owner ast.Node = p
			for {
				paren, ok := parents[owner].(*ast.ParenExpr)
				if !ok {
					break
				}
				owner = paren
			}
			if unary, ok := parents[owner].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				valid = false
			}
		case *ast.SliceExpr:
			call, ok := parents[p].(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				valid = false
				break
			}
			callee, ok := call.Fun.(*ast.Ident)
			if !ok {
				valid = false
				break
			}
			built, ok := pass.TypesInfo.Uses[callee].(*types.Builtin)
			if !ok || built.Name() != "clear" {
				valid = false
			}
		case *ast.CallExpr:
			fn, _, ok := typedCallee(pass, p.Fun)
			if !ok || !ps6129GEMM(pass, ps6022Call{call: p, function: fn}, funcs) {
				valid = false
			}
		default:
			valid = false
		}
		return valid
	})
	return valid && definitions == 1
}

func ps6129Immutable(pass *analysis.Pass, body *ast.BlockStmt, expressions ...ast.Expr) bool {
	objects := make(map[types.Object]bool, len(expressions))
	for _, e := range expressions {
		object := ps6129Object(pass, e)
		if object == nil || object.Parent() == pass.Pkg.Scope() {
			return false
		}
		objects[object] = true
	}
	valid := true
	parents := ps6019Parents(body)
	ast.Inspect(body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok || !objects[identObject(pass, id)] {
			return true
		}
		var owner ast.Node = id
		for {
			if paren, ok := parents[owner].(*ast.ParenExpr); ok {
				owner = paren
			} else {
				break
			}
		}
		switch p := parents[owner].(type) {
		case *ast.AssignStmt:
			for _, lhs := range p.Lhs {
				if lhs == owner && (p.Tok != token.DEFINE || pass.TypesInfo.Defs[id] != identObject(pass, id)) {
					valid = false
				}
			}
		case *ast.IncDecStmt:
			valid = false
		case *ast.RangeStmt:
			if p.Tok == token.ASSIGN && (p.Key == owner || p.Value == owner) {
				valid = false
			}
		case *ast.UnaryExpr:
			if p.Op == token.AND {
				valid = false
			}
		}
		return valid
	})
	return valid
}

func ps6129Transpose(pass *analysis.Pass, body *ast.BlockStmt, stmt ast.Stmt, zeros map[types.Object]ps6129Zero, funcs map[string]bool) (ps6129Zero, bool) {
	r, ri, ok := ps6129Range(stmt)
	if !ok || len(r.Body.List) != 1 {
		return ps6129Zero{}, false
	}
	j, ji, ok := ps6129Range(r.Body.List[0])
	if !ok || len(j.Body.List) != 1 {
		return ps6129Zero{}, false
	}
	assignment, ok := j.Body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return ps6129Zero{}, false
	}
	dst, ok := assignment.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return ps6129Zero{}, false
	}
	src, ok := assignment.Rhs[0].(*ast.IndexExpr)
	if !ok {
		return ps6129Zero{}, false
	}
	z, ok := zeros[ps6129Object(pass, src.X)]
	if !ok || z.transpose || !ps6129Equal(pass, r.X, z.rows) || !ps6129Equal(pass, j.X, z.stride) {
		return ps6129Zero{}, false
	}
	from := &ast.BinaryExpr{X: &ast.BinaryExpr{X: ri, Op: token.MUL, Y: z.stride}, Op: token.ADD, Y: ji}
	to := &ast.BinaryExpr{X: &ast.BinaryExpr{X: ji, Op: token.MUL, Y: z.rows}, Op: token.ADD, Y: ri}
	object := ps6129Object(pass, dst.X)
	if !ps6129Equal(pass, src.Index, from) || !ps6129Equal(pass, dst.Index, to) || object == nil || object == z.buffer || !ps6129Fresh(pass, body, object, z.rows, z.stride, funcs) {
		return ps6129Zero{}, false
	}
	z.buffer, z.transpose = object, true
	return z, true
}
