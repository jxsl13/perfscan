package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6012 implements owner issue #772: a batch index drives per-item Slice
// dispatches whose results are collected and concatenated back into a batch.
var PS6012 = register(&lint.Check{
	ID:       "PS6012",
	Category: "verify",
	Slug:     "batch-slice-concat-dispatch-loop",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a batch-indexed Slice dispatch loop concatenates the results back into one tensor",
		Text: `A loop that slices one batch item, runs movement or elementwise
backend operations, appends the result, and concatenates the collection after
the loop turns one batch boundary into O(B) command submissions. The data was
already packed; recovering a packed tensor after per-item dispatch often means
the graph boundary, rather than the algorithm, introduced the loop.

This check implements owner issue #772 with object-aware AST matching. It
requires all of these signals in one lexical block:

  - a runtime-bound counted or range loop with a real induction variable;
  - a backend Execute/Dispatch/Submit/RunOp-style call, or a tensor-returning
    thin op helper, whose Slice start/end arguments reference that variable;
  - append or index assignment into a collection inside the loop; and
  - a backend or tensor-helper Concat after the loop that consumes the same
    collection object.

The diagnostic counts syntactic backend calls in the loop and in the Concat
statement, reporting a formula such as 2B+1 or 4B+3. Two or more calls per
iteration are identified as the higher-leverage form. Constant-bounded loops
and fixed arrays are excluded. A nearby explicit perfscan retention marker,
measured retained-winner comment, or comment documenting required independent
sequence semantics suppresses the finding.

The formula is an operation-count estimate, not a speedup promise. Validate a
rewrite with a structural op-count assertion and order-controlled B=1/8/32
end-to-end benchmarks. A flat end-to-end result is a valid reason to retain the
original even when its command count scales with B.

There is NO automatic fix. Hoisting Slice/Concat across a batch boundary can
change shapes, broadcasting, aliasing, and independent-sequence semantics;
only project-specific graph knowledge can prove the rewrite.`,
		Before: `parts := make([]Tensor, 0, batch)
for i := 0; i < batch; i++ {
	row := backend.Execute(OpSlice{Start: i, End: i + 1}, input)
	parts = append(parts, backend.Execute(OpReshape{}, row))
}
return backend.Execute(OpConcat{}, parts...)`,
		After: `// Express the packed batch boundary directly, then prove it:
got := backend.Execute(BatchedBoundary{}, input)
assertBackendOpCount(t, 5) // independent of B
// Benchmark B=1/8/32 end to end before promotion.`,
		MeasuredWin: `The ViT case behind issue #772 issued 51 backend operations
at B=8. A mutation-proven packed prototype reduced that boundary to five
batch-independent operations and preserved exact F64 forward values at B=1,
4, and 8. It was still correctly rejected: M2 end-to-end training remained
flat and forward speedup did not repeatedly clear the frozen 1.25x gate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6012",
		Doc:  "batch-indexed Slice dispatch loop feeds a post-loop Concat",
		Run:  runPS6012,
	},
})

type ps6012Loop struct {
	body       *ast.BlockStmt
	index      types.Object
	boundLabel string
}

func runPS6012(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			seen := make(map[ast.Stmt]bool)
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				block, ok := node.(*ast.BlockStmt)
				if !ok {
					return true
				}
				for pos, stmt := range block.List {
					loop, ok := ps6012RuntimeLoop(pass, stmt)
					if !ok || seen[stmt] || ps6012Suppressed(pass, file, fn, stmt) {
						continue
					}
					seen[stmt] = true
					perIteration, sliceCall := ps6012LoopDispatches(pass, loop)
					if perIteration == 0 || sliceCall == nil {
						continue
					}
					collections := ps6012Collections(pass, loop)
					if len(collections) == 0 {
						continue
					}
					postCalls, collection, ok := ps6012FindConcat(pass, block.List[pos+1:], collections)
					if !ok {
						continue
					}
					leverage := ""
					if perIteration >= 2 {
						leverage = "; high-leverage form has two or more backend calls per batch item"
					}
					pass.Reportf(sliceCall.Pos(), "batch-indexed Slice dispatches feed %s and a post-loop Concat; estimated backend dispatches scale as %s (%d per iteration + %d post-loop, runtime bound %s)%s; add a structural op-count test and B=1/8/32 end-to-end benchmark before rewriting", collection.Name(), ps6012Formula(perIteration, postCalls), perIteration, postCalls, loop.boundLabel, leverage)
				}
				return true
			})
		}
	}
	return nil, nil
}

func ps6012RuntimeLoop(pass *analysis.Pass, stmt ast.Stmt) (ps6012Loop, bool) {
	switch loop := stmt.(type) {
	case *ast.ForStmt:
		init, ok := loop.Init.(*ast.AssignStmt)
		if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
			return ps6012Loop{}, false
		}
		indexID, ok := ps2110Unparen(init.Lhs[0]).(*ast.Ident)
		if !ok || indexID.Name == "_" {
			return ps6012Loop{}, false
		}
		index := pass.TypesInfo.Defs[indexID]
		if index == nil || !ps6012Zero(pass, init.Rhs[0]) || !ps6012UnitPost(pass, loop.Post, index) {
			return ps6012Loop{}, false
		}
		cond, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if !ok || cond.Op != token.LSS || !ps6012Mentions(pass, cond.X, index) || ps6012CompileTimeBound(pass, cond.Y) {
			return ps6012Loop{}, false
		}
		return ps6012Loop{body: loop.Body, index: index, boundLabel: ps6012BoundLabel(cond.Y)}, true

	case *ast.RangeStmt:
		indexID, ok := ps2110Unparen(loop.Key).(*ast.Ident)
		if !ok || indexID.Name == "_" {
			return ps6012Loop{}, false
		}
		index := pass.TypesInfo.Defs[indexID]
		if index == nil {
			index = pass.TypesInfo.Uses[indexID]
		}
		if index == nil || ps6012CompileTimeRange(pass, loop.X) {
			return ps6012Loop{}, false
		}
		return ps6012Loop{body: loop.Body, index: index, boundLabel: ps6012BoundLabel(loop.X)}, true
	}
	return ps6012Loop{}, false
}

func ps6012Zero(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Sign(tv.Value) == 0
}

func ps6012UnitPost(pass *analysis.Pass, stmt ast.Stmt, index types.Object) bool {
	switch post := stmt.(type) {
	case *ast.IncDecStmt:
		return post.Tok == token.INC && ps6012Mentions(pass, post.X, index)
	case *ast.AssignStmt:
		if post.Tok != token.ADD_ASSIGN || len(post.Lhs) != 1 || len(post.Rhs) != 1 || !ps6012Mentions(pass, post.Lhs[0], index) {
			return false
		}
		tv, ok := pass.TypesInfo.Types[ps2110Unparen(post.Rhs[0])]
		return ok && tv.Value != nil && tv.Value.Kind() == constant.Int && constant.Compare(tv.Value, token.EQL, constant.MakeInt64(1))
	}
	return false
}

func ps6012CompileTimeBound(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	return ok && tv.Value != nil
}

func ps6012CompileTimeRange(pass *analysis.Pass, expr ast.Expr) bool {
	if ps6012CompileTimeBound(pass, expr) {
		return true
	}
	t := pass.TypesInfo.TypeOf(expr)
	if t == nil {
		return false
	}
	if _, fixed := types.Unalias(t).Underlying().(*types.Array); fixed {
		return true
	}
	if ptr, ok := types.Unalias(t).Underlying().(*types.Pointer); ok {
		_, fixed := types.Unalias(ptr.Elem()).Underlying().(*types.Array)
		return fixed
	}
	return false
}

func ps6012BoundLabel(expr ast.Expr) string {
	if text := simpleExprText(ps2110Unparen(expr)); text != "" {
		return text
	}
	return "batch dimension"
}

func ps6012Mentions(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		id, ok := n.(*ast.Ident)
		if ok && (pass.TypesInfo.Uses[id] == object || pass.TypesInfo.Defs[id] == object) {
			found = true
		}
		return !found
	})
	return found
}

func ps6012LoopDispatches(pass *analysis.Pass, loop ps6012Loop) (int, *ast.CallExpr) {
	count := 0
	var sliceCall *ast.CallExpr
	ast.Inspect(loop.body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6012BackendCall(pass, call) {
			return true
		}
		count++
		if sliceCall == nil && ps6012SliceDrivenCall(pass, call, loop.index) {
			sliceCall = call
		}
		return true
	})
	return count, sliceCall
}

func ps6012BackendCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	if ps6007ContainsAny(name, "execute", "dispatch", "submit", "runop", "applyop", "launchkernel", "enqueueop") {
		return true
	}
	if !ps6007ContainsAny(name, "slice", "concat", "reshape", "gather", "scatter", "elementwise", "addtensor", "multensor") {
		return false
	}
	return ps6012TensorResult(sig)
}

func ps6012TensorResult(sig *types.Signature) bool {
	for i := 0; i < sig.Results().Len(); i++ {
		name := strings.ToLower(types.TypeString(sig.Results().At(i).Type(), func(*types.Package) string { return "" }))
		if ps6007ContainsAny(name, "tensor", "devicevalue", "graphvalue", "nodevalue") {
			return true
		}
	}
	return false
}

func ps6012SliceDrivenCall(pass *analysis.Pass, call *ast.CallExpr, index types.Object) bool {
	if !ps6012Mentions(pass, call, index) || !ps6012NodeHasText(call, "slice") {
		return false
	}
	return ps6012NodeHasText(call, "start", "end", "begin", "limit", "offset") || ps6012CalleeHasText(pass, call, "slice")
}

func ps6012NodeHasText(node ast.Node, terms ...string) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		var text string
		switch value := n.(type) {
		case *ast.Ident:
			text = value.Name
		case *ast.BasicLit:
			text = value.Value
		}
		text = strings.ToLower(text)
		for _, term := range terms {
			if strings.Contains(text, term) {
				found = true
				break
			}
		}
		return !found
	})
	return found
}

func ps6012CalleeHasText(pass *analysis.Pass, call *ast.CallExpr, term string) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	return ok && strings.Contains(strings.ToLower(fn.Name()), term)
}

func ps6012Collections(pass *analysis.Pass, loop ps6012Loop) map[types.Object]bool {
	var assignments []*ast.AssignStmt
	ast.Inspect(loop.body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if assign, ok := node.(*ast.AssignStmt); ok && (len(assign.Lhs) == len(assign.Rhs) || len(assign.Rhs) == 1) {
			assignments = append(assignments, assign)
		}
		return true
	})
	// Resolve short chains such as row := Execute(Slice); shaped :=
	// Execute(Reshape, row); parts = append(parts, shaped). Object identity
	// prevents same-spelled variables in nested scopes from leaking in.
	dispatchValues := make(map[types.Object]bool)
	for range len(assignments) + 1 {
		changed := false
		for _, assign := range assignments {
			for i, lhs := range assign.Lhs {
				rhs := ps6012AssignedValue(assign, i)
				id, ok := ps2110Unparen(lhs).(*ast.Ident)
				if !ok || rhs == nil || !ps6012ValueFromDispatch(pass, rhs, dispatchValues) {
					continue
				}
				if object := ps6012IdentObject(pass, id); object != nil && !dispatchValues[object] {
					dispatchValues[object] = true
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}

	collections := make(map[types.Object]bool)
	for _, assign := range assignments {
		for i, lhs := range assign.Lhs {
			rhs := ps6012AssignedValue(assign, i)
			if rhs == nil {
				continue
			}
			if id, ok := ps2110Unparen(lhs).(*ast.Ident); ok {
				object := ps6012IdentObject(pass, id)
				call, ok := ps2110Unparen(rhs).(*ast.CallExpr)
				if object != nil && ok && ps6012AppendTo(pass, call, object, dispatchValues) {
					collections[object] = true
				}
				continue
			}
			indexed, ok := ps2110Unparen(lhs).(*ast.IndexExpr)
			if !ok || !ps6012Mentions(pass, indexed.Index, loop.index) {
				continue
			}
			if !ps6012ValueFromDispatch(pass, rhs, dispatchValues) {
				continue
			}
			if id, ok := ps2110Unparen(indexed.X).(*ast.Ident); ok {
				if object := ps6012IdentObject(pass, id); object != nil {
					collections[object] = true
				}
			}
		}
	}
	return collections
}

func ps6012AssignedValue(assign *ast.AssignStmt, index int) ast.Expr {
	if len(assign.Rhs) == len(assign.Lhs) {
		return assign.Rhs[index]
	}
	if len(assign.Rhs) == 1 {
		return assign.Rhs[0]
	}
	return nil
}

func ps6012AppendTo(pass *analysis.Pass, call *ast.CallExpr, object types.Object, dispatchValues map[types.Object]bool) bool {
	id, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok || len(call.Args) < 2 {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || builtin.Name() != "append" {
		return false
	}
	first, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok || ps6012IdentObject(pass, first) != object {
		return false
	}
	for _, appended := range call.Args[1:] {
		if ps6012ValueFromDispatch(pass, appended, dispatchValues) {
			return true
		}
	}
	return false
}

func ps6012ValueFromDispatch(pass *analysis.Pass, expr ast.Expr, dispatchValues map[types.Object]bool) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if found {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok && ps6012BackendCall(pass, call) {
			found = true
			return false
		}
		if id, ok := node.(*ast.Ident); ok && dispatchValues[ps6012IdentObject(pass, id)] {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6012IdentObject(pass *analysis.Pass, id *ast.Ident) types.Object {
	if object := pass.TypesInfo.Uses[id]; object != nil {
		return object
	}
	return pass.TypesInfo.Defs[id]
}

func ps6012FindConcat(pass *analysis.Pass, after []ast.Stmt, collections map[types.Object]bool) (int, types.Object, bool) {
	for _, stmt := range after {
		var found types.Object
		ast.Inspect(stmt, func(node ast.Node) bool {
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || !ps6012BackendCall(pass, call) || !ps6012NodeHasText(call, "concat") {
				return true
			}
			for collection := range collections {
				if ps6012Mentions(pass, call, collection) && (found == nil || collection.Pos() < found.Pos()) {
					found = collection
				}
			}
			return found == nil
		})
		if found != nil {
			postCalls := ps6012DispatchCount(pass, stmt)
			if postCalls == 0 {
				postCalls = 1
			}
			return postCalls, found, true
		}
	}
	return 0, nil, false
}

func ps6012DispatchCount(pass *analysis.Pass, node ast.Node) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if _, nested := n.(*ast.FuncLit); nested {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && ps6012BackendCall(pass, call) {
			count++
		}
		return true
	})
	return count
}

func ps6012Formula(perIteration, post int) string {
	coefficient := ""
	if perIteration > 1 {
		coefficient = strconv.Itoa(perIteration)
	}
	formula := coefficient + "B"
	if post > 0 {
		formula += "+" + strconv.Itoa(post)
	}
	return formula
}

func ps6012Suppressed(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl, loop ast.Stmt) bool {
	if fn.Doc != nil && ps6012SuppressionText(fn.Doc.Text()) {
		return true
	}
	for _, group := range file.Comments {
		if group == fn.Doc || group.Pos() < fn.Body.Pos() || group.End() > fn.Body.End() {
			continue
		}
		inside := group.Pos() >= loop.Pos() && group.End() <= loop.End()
		beforeLines := pass.Fset.Position(loop.Pos()).Line - pass.Fset.Position(group.End()).Line
		afterLines := pass.Fset.Position(group.Pos()).Line - pass.Fset.Position(loop.End()).Line
		nearby := beforeLines >= 0 && beforeLines <= 4 || afterLines >= 0 && afterLines <= 2
		if (inside || nearby) && ps6012SuppressionText(group.Text()) {
			return true
		}
	}
	return false
}

func ps6012SuppressionText(text string) bool {
	text = strings.ToLower(text)
	if strings.Contains(text, "perfscan:retain-batch-loop") {
		return true
	}
	measured := ps6007ContainsAny(text, "measured", "benchmark", "profiled")
	retained := ps6007ContainsAny(text, "retained winner", "retain this", "retained because", "measured winner")
	independent := strings.Contains(text, "independent") && strings.Contains(text, "sequence") && ps6007ContainsAny(text, "required", "separate", "semantic")
	algorithmic := strings.Contains(text, "algorithmic") && ps6007ContainsAny(text, "required", "bounded", "separate")
	return measured && retained || independent || algorithmic
}
