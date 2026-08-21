package checks

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6076 implements owner issue #793. It finds a range-invariant packing or
// materialization repeated by every callback invocation of a configured
// parallel fan-out helper.
var PS6076 = register(&lint.Check{
	ID:          "PS6076",
	Category:    "verify",
	Slug:        "parallel-callback-invariant-packing",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a parallel band callback rebuilds the same invariant packed data",
		Text: `A fast leaf kernel can lose all of its gain when every parallel row
band first rebuilds the same immutable packed operand. With B callback bands,
an S-byte panel is transformed B times and writes approximately B*S bytes;
only one transform and S bytes belong to the complete operation.

This check implements owner issue #793 using the configured fanOutHelpers
vocabulary. It inspects directly supplied callback literals and tracks the
callback range parameters plus locals derived from them. It reports either:

  - a local make([]T, length[, capacity]) whose invariant storage is filled by
    copy, a packing/transpose/quantization/materialization call, or a loop that
    writes the buffer from captured immutable-looking data; or
  - a direct pack, transpose, quantize/dequantize, plan, mask, table, reorder,
    clone, materialize, preprocess, prepare, or transform call whose receiver
    and arguments are range-invariant and include captured data.

The proof is deliberately about callback-range invariance, not identifier
spelling. Type objects distinguish shadowed variables; transitively derived
range bounds are treated as dependent. Operations under range-dependent
conditions or loops stay silent, as do per-band slices, per-band copies,
unfilled scratch allocations, indirect function values, named callbacks whose
bodies are unavailable at the call, unconfigured helpers, nested closures, and
//perfscan:parallel-packing-validated functions.

For make-backed buffers the diagnostic estimates packed bytes from the slice
element size and capacity expression. Constant shapes produce an exact byte
count; dynamic invariant shapes produce a symbolic elementBytes*(capacity)
count. For opaque helper results it reports B repeated transforms and asks for
the result capacity times element size to be measured. B means the fan-out
helper's runtime callback-band count because that scheduling policy is not
available from a call site. The diagnostic also prints the scheduled work
domain so the observed band count can be connected to the estimate.

There is NO automatic fix. Hoisting must publish a fully initialized read-only
buffer before workers cross the fan-out barrier. A buffer that is actually
mutable per-band scratch cannot be shared; give it explicit worker ownership
or pool it instead. Benchmark first-call allocation/cache warmup separately
from steady-state reuse, and retain the change only when complete operations
and unaffected consumers pass same-binary alternating-order campaigns.`,
		Before: `parallelFor(rows, func(rowStart, rowEnd int) {
	packed := make([]float64, depth*columns)
	for p := 0; p < depth; p++ {
		copy(packed[p*columns:], weights[p*columns:(p+1)*columns])
	}
	runRows(rowStart, rowEnd, packed)
})`,
		After: `packed := make([]float64, depth*columns)
packReadOnly(packed, weights) // complete-operation boundary
parallelFor(rows, func(rowStart, rowEnd int) {
	runRows(rowStart, rowEnd, packed)
})`,
		MeasuredWin: `The owner Apple M2 Pro campaign found that packing immutable
B panels inside each parallel row-band callback reduced full operations to
roughly 0.2-0.7x baseline despite a 2.67x microkernel. Hoisting one published
packed buffer before fan-out produced 1.649-2.471x direct GEMM and 2.04-2.38x
public MatMul gains across alternating-order count-7 dense and ragged shape
campaigns; unaffected Conv consumers remained statistically neutral. This
operation-scoped packing plus 4x8 NEON design merged in GoAI PR #1126 at
58a2fa4e3f1716a81326e2093100cffe70e2ab6b after two complete 15-of-15 CI
matrices.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6076",
		Doc:  "parallel callback repeats range-invariant packing or materialization",
		Run:  runPS6076,
	},
})

type ps6076Allocation struct {
	call      *ast.CallExpr
	object    types.Object
	name      string
	footprint string
}

type ps6076Work struct {
	position  token.Pos
	operation string
	dest      string
	sources   []string
	footprint string
}

func runPS6076(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || ps6076Validated(function) {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				helper, ok := ps6076FanoutHelper(pass, call, fanout)
				if !ok {
					return true
				}
				for _, argument := range call.Args {
					callback, ok := ps2110Unparen(argument).(*ast.FuncLit)
					if !ok {
						continue
					}
					work, ok := ps6076CallbackWork(pass, callback)
					if !ok {
						continue
					}
					pass.Reportf(work.position, "%s callback repeats range-invariant %s from captured source %s into %s over scheduled work domain %s; %s Hoist and fully initialize one read-only packed buffer at the complete-operation boundary before the fan-out barrier. If this is mutable per-band scratch, retain worker ownership or pool it instead; measure first-call allocation/cache warmup separately from steady state (advisory, no automatic fix)",
						helper,
						work.operation,
						ps6076QuotedList(work.sources),
						work.dest,
						ps6076WorkDomain(call),
						ps6076Duplication(work.footprint),
					)
				}
				return true
			})
		}
	}
	return nil, nil
}

func ps6076FanoutHelper(pass *analysis.Pass, call *ast.CallExpr, fanout map[string]bool) (string, bool) {
	function, signature, ok := typedCallee(pass, call.Fun)
	if !ok {
		return "", false
	}
	candidates := []string{function.Name(), astutil.CalleeName(ps2110Unparen(call.Fun)), exprTextRendered(call.Fun)}
	if function.Pkg() != nil {
		candidates = append(candidates, function.Pkg().Name()+"."+function.Name())
	}
	if receiver := ps6076ReceiverName(signature); receiver != "" {
		candidates = append(candidates, receiver+"."+function.Name())
	}
	for _, candidate := range candidates {
		if fanout[candidate] {
			return candidate, true
		}
	}
	return "", false
}

func ps6076ReceiverName(signature *types.Signature) string {
	if signature == nil || signature.Recv() == nil {
		return ""
	}
	typeValue := types.Unalias(signature.Recv().Type())
	if pointer, ok := typeValue.(*types.Pointer); ok {
		typeValue = types.Unalias(pointer.Elem())
	}
	named, ok := typeValue.(*types.Named)
	if !ok || named.Obj() == nil {
		return ""
	}
	return named.Obj().Name()
}

func ps6076CallbackWork(pass *analysis.Pass, callback *ast.FuncLit) (ps6076Work, bool) {
	dependent := ps6076RangeDependent(pass, callback)
	parents := ps6071Parents(callback.Body)
	allocations := ps6076Allocations(pass, callback, parents, dependent)
	for _, allocation := range allocations {
		operation, sources, ok := ps6076AllocationWork(pass, callback, allocation, parents, dependent)
		if !ok {
			continue
		}
		return ps6076Work{
			position: allocation.call.Pos(), operation: operation, dest: allocation.name,
			sources: sources, footprint: allocation.footprint,
		}, true
	}
	return ps6076DirectTransform(pass, callback, parents, dependent)
}

func ps6076RangeDependent(pass *analysis.Pass, callback *ast.FuncLit) map[types.Object]bool {
	dependent := make(map[types.Object]bool)
	if callback.Type.Params != nil {
		for _, field := range callback.Type.Params.List {
			for _, name := range field.Names {
				if object := pass.TypesInfo.ObjectOf(name); object != nil {
					dependent[object] = true
				}
			}
		}
	}
	for {
		changed := false
		ast.Inspect(callback.Body, func(node ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok && literal != callback {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for index, left := range value.Lhs {
					right := ast.Node(nil)
					if len(value.Rhs) == len(value.Lhs) {
						right = value.Rhs[index]
					} else if len(value.Rhs) == 1 {
						right = value.Rhs[0]
					}
					if right != nil && ps6076Depends(pass, right, dependent) {
						changed = ps6076MarkObject(pass, left, dependent) || changed
					}
				}
			case *ast.ValueSpec:
				for index, name := range value.Names {
					if index < len(value.Values) && ps6076Depends(pass, value.Values[index], dependent) {
						changed = ps6076MarkObject(pass, name, dependent) || changed
					}
				}
			case *ast.RangeStmt:
				if ps6076Depends(pass, value.X, dependent) {
					changed = ps6076MarkObject(pass, value.Key, dependent) || changed
					changed = ps6076MarkObject(pass, value.Value, dependent) || changed
				}
			}
			return true
		})
		if !changed {
			return dependent
		}
	}
}

func ps6076MarkObject(pass *analysis.Pass, expression ast.Expr, set map[types.Object]bool) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil || set[object] {
		return false
	}
	set[object] = true
	return true
}

func ps6076Depends(pass *analysis.Pass, node ast.Node, dependent map[types.Object]bool) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := candidate.(*ast.Ident)
		if ok && dependent[pass.TypesInfo.Uses[identifier]] {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6076Allocations(pass *analysis.Pass, callback *ast.FuncLit, parents map[ast.Node]ast.Node, dependent map[types.Object]bool) []ps6076Allocation {
	var result []ps6076Allocation
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		if literal, ok := node.(*ast.FuncLit); ok && literal != callback {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || !ps6076Builtin(pass, call.Fun, "make") || len(call.Args) < 2 || len(call.Args) > 3 {
			return true
		}
		sliceType, ok := types.Unalias(pass.TypesInfo.TypeOf(call.Args[0])).Underlying().(*types.Slice)
		if !ok {
			return true
		}
		shape := call.Args[len(call.Args)-1]
		for _, dimension := range call.Args[1:] {
			if ps6076Depends(pass, dimension, dependent) {
				return true
			}
		}
		if ps6076DependentControl(pass, call, callback, parents, dependent) {
			return true
		}
		object, name, ok := ps6076AssignedObject(pass, call, parents)
		if !ok || object.Pos() < callback.Pos() || object.Pos() >= callback.End() {
			return true
		}
		result = append(result, ps6076Allocation{
			call: call, object: object, name: name,
			footprint: ps6076SliceFootprint(pass, sliceType.Elem(), shape),
		})
		return true
	})
	return result
}

func ps6076AssignedObject(pass *analysis.Pass, expression ast.Expr, parents map[ast.Node]ast.Node) (types.Object, string, bool) {
	node := ast.Node(expression)
	for {
		parent := parents[node]
		if _, ok := parent.(*ast.ParenExpr); ok {
			node = parent
			continue
		}
		switch value := parent.(type) {
		case *ast.AssignStmt:
			for index, right := range value.Rhs {
				if right != node || index >= len(value.Lhs) {
					continue
				}
				identifier, ok := ps2110Unparen(value.Lhs[index]).(*ast.Ident)
				if !ok || identifier.Name == "_" {
					return nil, "", false
				}
				object := pass.TypesInfo.ObjectOf(identifier)
				return object, identifier.Name, object != nil
			}
		case *ast.ValueSpec:
			for index, right := range value.Values {
				if right != node || index >= len(value.Names) {
					continue
				}
				identifier := value.Names[index]
				object := pass.TypesInfo.ObjectOf(identifier)
				return object, identifier.Name, object != nil
			}
		}
		return nil, "", false
	}
}

func ps6076SliceFootprint(pass *analysis.Pass, element types.Type, shape ast.Expr) string {
	elementBytes := pass.TypesSizes.Sizeof(element)
	if elementBytes <= 0 {
		return ""
	}
	if value, ok := pass.TypesInfo.Types[ps2110Unparen(shape)]; ok && value.Value != nil && value.Value.Kind() == constant.Int {
		if count, exact := constant.Int64Val(value.Value); exact && count >= 0 && count <= math.MaxInt64/elementBytes {
			return fmt.Sprintf("%d bytes", count*elementBytes)
		}
	}
	return fmt.Sprintf("%d*(%s) bytes", elementBytes, exprTextRendered(shape))
}

func ps6076AllocationWork(pass *analysis.Pass, callback *ast.FuncLit, allocation ps6076Allocation, parents map[ast.Node]ast.Node, dependent map[types.Object]bool) (string, []string, bool) {
	var operation string
	var sources []string
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if operation != "" {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal != callback {
			return false
		}
		if node.Pos() <= allocation.call.Pos() {
			return true
		}
		switch value := node.(type) {
		case *ast.CallExpr:
			if ps6076DependentControl(pass, value, callback, parents, dependent) {
				return true
			}
			if ps6076Builtin(pass, value.Fun, "copy") && len(value.Args) == 2 &&
				ps6076RootObject(pass, value.Args[0]) == allocation.object && !ps6076Depends(pass, value.Args[1], dependent) {
				candidateSources := ps6076ExternalSources(pass, value.Args[1], callback, allocation.object)
				if len(candidateSources) > 0 {
					operation, sources = "copy", candidateSources
					return false
				}
			}
			if name, ok := ps6076TransformName(pass, value); ok && !ps6076Depends(pass, value, dependent) &&
				ps6076MentionsObject(pass, value, allocation.object) {
				candidateSources := ps6076ExternalSources(pass, value, callback, allocation.object)
				if len(candidateSources) > 0 {
					operation, sources = name, candidateSources
					return false
				}
			}
		case *ast.ForStmt:
			if !ps6076Depends(pass, value, dependent) &&
				!ps6076DependentControl(pass, value, callback, parents, dependent) &&
				ps6076WritesObject(pass, value.Body, allocation.object) {
				candidateSources := ps6076ExternalSources(pass, value, callback, allocation.object)
				if len(candidateSources) > 0 {
					operation, sources = "packing loop", candidateSources
					return false
				}
			}
		case *ast.RangeStmt:
			if !ps6076Depends(pass, value, dependent) &&
				!ps6076DependentControl(pass, value, callback, parents, dependent) &&
				ps6076WritesObject(pass, value.Body, allocation.object) {
				candidateSources := ps6076ExternalSources(pass, value, callback, allocation.object)
				if len(candidateSources) > 0 {
					operation, sources = "packing loop", candidateSources
					return false
				}
			}
		}
		return true
	})
	return operation, sources, operation != ""
}

func ps6076DirectTransform(pass *analysis.Pass, callback *ast.FuncLit, parents map[ast.Node]ast.Node, dependent map[types.Object]bool) (ps6076Work, bool) {
	var result ps6076Work
	ast.Inspect(callback.Body, func(node ast.Node) bool {
		if result.position.IsValid() {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal != callback {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || ps6076Depends(pass, call, dependent) || ps6076DependentControl(pass, call, callback, parents, dependent) {
			return true
		}
		name, ok := ps6076TransformName(pass, call)
		if !ok {
			return true
		}
		sources := ps6076ExternalSources(pass, call, callback, nil)
		if len(sources) == 0 {
			return true
		}
		_, destination, assigned := ps6076AssignedObject(pass, call, parents)
		if !assigned {
			destination = "a callback-local result"
		}
		result = ps6076Work{
			position: call.Pos(), operation: name, dest: destination,
			sources: sources, footprint: ps6076CallFootprint(pass, call),
		}
		return false
	})
	return result, result.position.IsValid()
}

func ps6076TransformName(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	function, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return "", false
	}
	name := strings.ToLower(function.Name())
	if !ps6007ContainsAny(name,
		"pack", "transpose", "quant", "dequant", "plan", "mask", "table",
		"reorder", "clone", "materialize", "preprocess", "prepare", "transform",
	) {
		return "", false
	}
	return function.Name(), true
}

func ps6076CallFootprint(pass *analysis.Pass, call *ast.CallExpr) string {
	typeValue := types.Unalias(pass.TypesInfo.TypeOf(call))
	if array, ok := typeValue.Underlying().(*types.Array); ok {
		if size := pass.TypesSizes.Sizeof(array); size >= 0 {
			return fmt.Sprintf("%d bytes", size)
		}
	}
	function, _, ok := typedCallee(pass, call.Fun)
	if ok && strings.Contains(strings.ToLower(function.Name()), "clone") && len(call.Args) == 1 {
		if sliceType, ok := types.Unalias(pass.TypesInfo.TypeOf(call.Args[0])).Underlying().(*types.Slice); ok {
			if elementBytes := pass.TypesSizes.Sizeof(sliceType.Elem()); elementBytes > 0 {
				return fmt.Sprintf("%d*len(%s) bytes", elementBytes, exprTextRendered(call.Args[0]))
			}
		}
	}
	return ""
}

func ps6076Builtin(pass *analysis.Pass, expression ast.Expr, name string) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return ok && builtin.Name() == name
}

func ps6076DependentControl(pass *analysis.Pass, node ast.Node, callback *ast.FuncLit, parents map[ast.Node]ast.Node, dependent map[types.Object]bool) bool {
	for current := node; current != nil && current != callback.Body; current = parents[current] {
		switch parent := parents[current].(type) {
		case *ast.IfStmt:
			if ps6076Depends(pass, parent.Cond, dependent) {
				return true
			}
		case *ast.ForStmt:
			if ps6076Depends(pass, parent.Init, dependent) || ps6076Depends(pass, parent.Cond, dependent) ||
				ps6076Depends(pass, parent.Post, dependent) {
				return true
			}
		case *ast.RangeStmt:
			if ps6076Depends(pass, parent.X, dependent) {
				return true
			}
		case *ast.SwitchStmt:
			if ps6076Depends(pass, parent.Init, dependent) || ps6076Depends(pass, parent.Tag, dependent) {
				return true
			}
		case *ast.TypeSwitchStmt:
			if ps6076Depends(pass, parent.Init, dependent) || ps6076Depends(pass, parent.Assign, dependent) {
				return true
			}
		}
	}
	return false
}

func ps6076RootObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.IndexExpr:
		return ps6076RootObject(pass, value.X)
	case *ast.IndexListExpr:
		return ps6076RootObject(pass, value.X)
	case *ast.SliceExpr:
		return ps6076RootObject(pass, value.X)
	case *ast.SelectorExpr:
		return ps6076RootObject(pass, value.X)
	case *ast.StarExpr:
		return ps6076RootObject(pass, value.X)
	}
	return nil
}

func ps6076WritesObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if found {
			return false
		}
		if _, nested := candidate.(*ast.FuncLit); nested {
			return false
		}
		assignment, ok := candidate.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			if ps6076RootObject(pass, left) == object {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func ps6076MentionsObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[identifier] == object {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6076ExternalSources(pass *analysis.Pass, node ast.Node, callback *ast.FuncLit, excluded types.Object) []string {
	seen := make(map[types.Object]bool)
	var names []string
	ast.Inspect(node, func(candidate ast.Node) bool {
		if literal, ok := candidate.(*ast.FuncLit); ok && literal != callback {
			return false
		}
		identifier, ok := candidate.(*ast.Ident)
		if !ok {
			return true
		}
		object, ok := pass.TypesInfo.Uses[identifier].(*types.Var)
		if !ok || object == excluded || seen[object] ||
			(object.Pos() >= callback.Pos() && object.Pos() < callback.End()) ||
			!ps6076ReferenceData(object.Type()) {
			return true
		}
		seen[object] = true
		names = append(names, object.Name())
		return true
	})
	slices.Sort(names)
	return names
}

func ps6076ReferenceData(typeValue types.Type) bool {
	switch underlying := types.Unalias(typeValue).Underlying().(type) {
	case *types.Slice, *types.Array, *types.Pointer, *types.Map:
		return true
	case *types.Basic:
		return underlying.Kind() == types.String
	}
	return false
}

func ps6076WorkDomain(call *ast.CallExpr) string {
	var parts []string
	for _, argument := range call.Args {
		if _, callback := ps2110Unparen(argument).(*ast.FuncLit); callback {
			continue
		}
		parts = append(parts, exprTextRendered(argument))
		if len(parts) == 3 {
			break
		}
	}
	if len(parts) == 0 {
		return "<runtime-defined>"
	}
	return strings.Join(parts, ", ")
}

func ps6076Duplication(footprint string) string {
	if footprint == "" {
		return "With B callback bands, the transform executes B times (B-1 avoidable); measure packed result capacity times element size to quantify duplicated bytes."
	}
	return "Packed footprint is " + footprint + " per callback; with B callback bands, estimated packing traffic is B*" + footprint + " and avoidable duplication is (B-1)*" + footprint + "."
}

func ps6076QuotedList(names []string) string {
	quoted := make([]string, len(names))
	for index, name := range names {
		quoted[index] = "'" + name + "'"
	}
	return strings.Join(quoted, ", ")
}

func ps6076Validated(function *ast.FuncDecl) bool {
	if function.Doc == nil {
		return false
	}
	for _, comment := range function.Doc.List {
		if strings.Contains(strings.ToLower(comment.Text), "perfscan:parallel-packing-validated") {
			return true
		}
	}
	return false
}
