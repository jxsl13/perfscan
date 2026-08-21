package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6064 implements owner issue #781. It finds straight-line pairs of the
// same softplus callable evaluated at x and -x, including immutable aliases.
var PS6064 = register(&lint.Check{
	ID:       "PS6064",
	Category: "arith",
	Slug:     "softplus-complements-recompute-stable-base",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "softplus complements recompute the same expensive stable base",
		Text: `Stable loss and activation code often evaluates both softplus(x)
and softplus(-x) even though the calls share their expensive transcendental
base:

  b = log1p(exp(-abs(x)))
  softplus(x)  = b + max(x, 0)
  softplus(-x) = b + max(-x, 0)

Computing b once removes one exponential and one logarithm per element. Likely
consumers include sigmoid focal loss, logistic objectives, contrastive/reward
losses, and fused activation VJPs.

This check implements owner issue #781. Within each straight-line block it
resolves immutable local aliases and reports calls to the exact same function,
function value, or method on the same receiver when their single float32 or
float64 arguments normalize to x and -x. It supports named floats, repeated
index/selector expressions, explicit dtype conversions, unary plus, either
call order, and pairs inside range/for bodies. Calls separated by control flow,
different callees or method receivers, mutable aliases, constants, integer
arguments, and side-effecting argument expressions stay silent. An
//perfscan:shared-softplus-base-validated annotation suppresses a deliberately
retained pair.

The replacement is deliberately advisory. Preserve the established rounding
barriers and evaluation order: if an F32 graph evaluates standalone softplus
in F64 and narrows on each store, keep b in F64 for both additions and narrow
the two results separately. In an f32-native SIMD path, keep b and both max/add
operations in F32 in their established order. Test +0 and -0, subnormals,
finite extremes, both infinities, quiet and signaling NaNs, and random raw-bit
inputs; validate each result against the unfused pair, not only a real-number
identity. A naive shared expression can change which NaN sign/payload wins an
addition; retain an explicit NaN slow path when the established two-call result
cannot be reproduced from the shared base. Gate the completed forward and
VJP/backward workload in same-binary, order-alternating campaigns rather than
extrapolating two removed scalar math calls to end-to-end leverage.

There is NO automatic fix because the softplus callable is project-defined and
the correct shared-base dtype, narrowing points, SIMD evaluation order, graph
fan-out, and VJP boundary cannot be inferred from call syntax alone.`,
		Before: `positive := softplus(x)
negative := softplus(-x)`,
		After: `// Preserve the established dtype and rounding barriers.
b := log1p(exp(-abs(x)))
positive := b + max(x, 0)
negative := b + max(-x, 0)`,
		MeasuredWin: `In the owner GoAI Apple M2 Pro Sigmoid Focal campaign,
sharing this base reduced a 2,097,152-element forward pilot from about 22.68 ms
to 13.22 ms. Three independent paired count-7 campaigns for the completed
fusion measured 1.50x-1.81x forward and 2.21x-2.55x forward+backward gains at
349,440 and 2,097,152 elements. Default F32/F64 stayed bit-exact against the
composite oracle; GOEXPERIMENT=simd F32 remained within its frozen tolerance.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6064",
		Doc:  "paired softplus complements recompute the same exp/log stable base",
		Run:  runPS6064,
	},
})

type ps6064Flow struct {
	initializers map[types.Object]ast.Expr
	writes       map[types.Object]int
}

type ps6064Call struct {
	call     *ast.CallExpr
	callee   types.Object
	receiver ast.Expr
	base     ast.Expr
	negative bool
	dtype    types.BasicKind
	name     string
}

func runPS6064(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || ps6064ValidatedAnnotation(file, function) {
				continue
			}
			flow := ps6064FunctionFlow(pass, function)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				block, ok := node.(*ast.BlockStmt)
				if !ok {
					return true
				}
				ps6064InspectBlock(pass, block, flow)
				return true
			})
		}
	}
	return nil, nil
}

func ps6064InspectBlock(pass *analysis.Pass, block *ast.BlockStmt, flow ps6064Flow) {
	var candidates []ps6064Call
	paired := map[*ast.CallExpr]bool{}
	flush := func() {
		candidates = candidates[:0]
		clear(paired)
	}
	for _, statement := range block.List {
		if !ps6064StraightStatement(statement) {
			flush()
			continue
		}
		var calls []*ast.CallExpr
		ast.Inspect(statement, func(node ast.Node) bool {
			if node != statement {
				switch node.(type) {
				case *ast.BlockStmt, *ast.FuncLit:
					return false
				}
			}
			if call, ok := node.(*ast.CallExpr); ok {
				calls = append(calls, call)
			}
			return true
		})
		for _, call := range calls {
			current, ok := ps6064SoftplusCall(pass, flow, call)
			if !ok {
				continue
			}
			for index := range candidates {
				previous := &candidates[index]
				if paired[previous.call] || previous.negative == current.negative || previous.dtype != current.dtype ||
					previous.callee != current.callee || !ps6064SameOptionalExpr(pass, previous.receiver, current.receiver) ||
					!ps6064SameExpr(pass, previous.base, current.base) {
					continue
				}
				dtype := "float64"
				if current.dtype == types.Float32 {
					dtype = "float32"
				}
				pass.Reportf(current.call.Pos(), "paired %s %s calls evaluate complementary x and -x inputs but recompute log1p(exp(-abs(x))); compute that stable base once, preserve the established %s/F64 rounding and per-result narrowing barriers, retain native SIMD operation order, validate signed zero/infinities/quiet+signaling NaNs and raw-bit parity (including a NaN slow path when payload/sign propagation differs), and gate complete forward+VJP workloads (advisory, no automatic fix)", dtype, current.name, dtype)
				paired[previous.call] = true
				paired[current.call] = true
				break
			}
			candidates = append(candidates, current)
		}
		if _, terminal := statement.(*ast.ReturnStmt); terminal {
			flush()
		}
	}
}

func ps6064StraightStatement(statement ast.Stmt) bool {
	switch statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt, *ast.ExprStmt, *ast.IncDecStmt, *ast.ReturnStmt, *ast.SendStmt, *ast.EmptyStmt:
		return true
	default:
		return false
	}
}

func ps6064SoftplusCall(pass *analysis.Pass, flow ps6064Flow, call *ast.CallExpr) (ps6064Call, bool) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return ps6064Call{}, false
	}
	var object types.Object
	var receiver ast.Expr
	var name string
	switch callee := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		object = pass.TypesInfo.ObjectOf(callee)
		name = callee.Name
	case *ast.SelectorExpr:
		object = pass.TypesInfo.ObjectOf(callee.Sel)
		name = callee.Sel.Name
		if pass.TypesInfo.Selections[callee] != nil {
			receiver = callee.X
		}
	default:
		return ps6064Call{}, false
	}
	if object == nil || !strings.Contains(ps6007NormalizeName(name), "softplus") || receiver != nil && !ps6064StableExpr(pass, flow, receiver) {
		return ps6064Call{}, false
	}
	base, negative, ok := ps6064NormalizeArgument(pass, flow, call.Args[0], map[types.Object]bool{})
	if !ok {
		return ps6064Call{}, false
	}
	typeValue := pass.TypesInfo.TypeOf(base)
	if typeValue == nil {
		return ps6064Call{}, false
	}
	basic, ok := typeValue.Underlying().(*types.Basic)
	if !ok || basic.Kind() != types.Float32 && basic.Kind() != types.Float64 {
		return ps6064Call{}, false
	}
	if value, known := pass.TypesInfo.Types[base]; known && value.Value != nil {
		return ps6064Call{}, false
	}
	return ps6064Call{call: call, callee: object, receiver: receiver, base: base, negative: negative, dtype: basic.Kind(), name: name}, true
}

func ps6064NormalizeArgument(pass *analysis.Pass, flow ps6064Flow, expression ast.Expr, seen map[types.Object]bool) (ast.Expr, bool, bool) {
	negative := false
	for {
		expression = ps2110Unparen(expression)
		if unary, ok := expression.(*ast.UnaryExpr); ok && (unary.Op == token.ADD || unary.Op == token.SUB) {
			if unary.Op == token.SUB {
				negative = !negative
			}
			expression = unary.X
			continue
		}
		identifier, ok := expression.(*ast.Ident)
		if !ok {
			break
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		initializer, hasInitializer := flow.initializers[object]
		if object == nil || seen[object] || flow.writes[object] != 0 || !hasInitializer || !ps6064AliasExpression(pass, initializer) {
			break
		}
		seen[object] = true
		expression = initializer
	}
	if !ps6064StableExpr(pass, flow, expression) {
		return nil, false, false
	}
	return expression, negative, true
}

func ps6064AliasExpression(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr, *ast.StarExpr:
		return true
	case *ast.UnaryExpr:
		return (value.Op == token.ADD || value.Op == token.SUB) && ps6064AliasExpression(pass, value.X)
	case *ast.CallExpr:
		typeValue, ok := pass.TypesInfo.Types[ps2110Unparen(value.Fun)]
		return ok && typeValue.IsType() && len(value.Args) == 1 && !value.Ellipsis.IsValid() && ps6064AliasExpression(pass, value.Args[0])
	default:
		return false
	}
}

func ps6064StableExpr(pass *analysis.Pass, flow ps6064Flow, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		return object != nil && flow.writes[object] == 0
	case *ast.BasicLit:
		return true
	case *ast.SelectorExpr:
		return pass.TypesInfo.ObjectOf(value.Sel) != nil && ps6064StableExpr(pass, flow, value.X)
	case *ast.IndexExpr:
		return ps6064StableExpr(pass, flow, value.X) && ps6064StableExpr(pass, flow, value.Index)
	case *ast.StarExpr:
		return ps6064StableExpr(pass, flow, value.X)
	case *ast.UnaryExpr:
		return (value.Op == token.ADD || value.Op == token.SUB) && ps6064StableExpr(pass, flow, value.X)
	case *ast.CallExpr:
		typeValue, ok := pass.TypesInfo.Types[ps2110Unparen(value.Fun)]
		return ok && typeValue.IsType() && len(value.Args) == 1 && !value.Ellipsis.IsValid() && ps6064StableExpr(pass, flow, value.Args[0])
	default:
		return false
	}
}

func ps6064SameOptionalExpr(pass *analysis.Pass, left, right ast.Expr) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return ps6064SameExpr(pass, left, right)
}

func ps6064SameExpr(pass *analysis.Pass, left, right ast.Expr) bool {
	left, right = ps2110Unparen(left), ps2110Unparen(right)
	switch a := left.(type) {
	case *ast.Ident:
		b, ok := right.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(a) != nil && pass.TypesInfo.ObjectOf(a) == pass.TypesInfo.ObjectOf(b)
	case *ast.BasicLit:
		b, ok := right.(*ast.BasicLit)
		return ok && a.Kind == b.Kind && a.Value == b.Value
	case *ast.SelectorExpr:
		b, ok := right.(*ast.SelectorExpr)
		return ok && pass.TypesInfo.ObjectOf(a.Sel) != nil && pass.TypesInfo.ObjectOf(a.Sel) == pass.TypesInfo.ObjectOf(b.Sel) && ps6064SameExpr(pass, a.X, b.X)
	case *ast.IndexExpr:
		b, ok := right.(*ast.IndexExpr)
		return ok && ps6064SameExpr(pass, a.X, b.X) && ps6064SameExpr(pass, a.Index, b.Index)
	case *ast.StarExpr:
		b, ok := right.(*ast.StarExpr)
		return ok && ps6064SameExpr(pass, a.X, b.X)
	case *ast.UnaryExpr:
		b, ok := right.(*ast.UnaryExpr)
		return ok && a.Op == b.Op && ps6064SameExpr(pass, a.X, b.X)
	case *ast.CallExpr:
		b, ok := right.(*ast.CallExpr)
		if !ok || len(a.Args) != 1 || len(b.Args) != 1 || a.Ellipsis.IsValid() || b.Ellipsis.IsValid() {
			return false
		}
		aType, aOK := pass.TypesInfo.Types[ps2110Unparen(a.Fun)]
		bType, bOK := pass.TypesInfo.Types[ps2110Unparen(b.Fun)]
		return aOK && bOK && aType.IsType() && bType.IsType() && types.Identical(aType.Type, bType.Type) && ps6064SameExpr(pass, a.Args[0], b.Args[0])
	default:
		return false
	}
}

func ps6064FunctionFlow(pass *analysis.Pass, function *ast.FuncDecl) ps6064Flow {
	flow := ps6064Flow{initializers: map[types.Object]ast.Expr{}, writes: map[types.Object]int{}}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, name := range value.Names {
					if object := pass.TypesInfo.Defs[name]; object != nil {
						flow.initializers[object] = value.Values[index]
					}
				}
			}
		case *ast.AssignStmt:
			for index, target := range value.Lhs {
				identifier, ok := ps2110Unparen(target).(*ast.Ident)
				if !ok || identifier.Name == "_" {
					continue
				}
				if value.Tok == token.DEFINE {
					if object := pass.TypesInfo.Defs[identifier]; object != nil && len(value.Lhs) == len(value.Rhs) {
						flow.initializers[object] = value.Rhs[index]
					}
					continue
				}
				if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
					flow.writes[object]++
				}
			}
		case *ast.IncDecStmt:
			if identifier, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
				if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
					flow.writes[object]++
				}
			}
		}
		return true
	})
	return flow
}

func ps6064ValidatedAnnotation(file *ast.File, function *ast.FuncDecl) bool {
	for _, group := range file.Comments {
		if group != function.Doc && !(group.End() <= function.Pos() && function.Pos()-group.End() <= 3) && !(group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()) {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscansharedsoftplusbasevalidated") || strings.Contains(text, "perfscansoftpluscomplementvalidated") {
				return true
			}
		}
	}
	return false
}
