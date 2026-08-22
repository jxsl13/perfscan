package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6065 implements owner issue #782. It follows results from locally visible
// approximate exp/sigmoid implementations with a nonzero underflow clamp into
// fused gradient multiplications.
var PS6065 = register(&lint.Check{
	ID:       "PS6065",
	Category: "verify",
	Slug:     "approx-exp-clamp-residue-amplified-in-vjp",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an approximate exp clamp residue can be amplified in a fused VJP",
		Text: `A fast approximate exp kernel may clamp a deep-negative input to a
finite threshold, or return a small positive normal floor, instead of
underflowing to zero. That can be acceptable under a forward absolute-error
budget but unsafe after fusion: an unbounded upstream gradient or product can
amplify the tiny residue into a material backward error.

The characteristic sigmoid/softplus path is:

  e := approximateExp(-abs(x)) // e has a nonzero below-clamp floor
  s := e / (1 + e)
  dx := upstream * s           // a large upstream resurrects the floor

This check implements owner issue #782. It first resolves package-local
functions or methods whose names identify approximate/fast/native/SIMD/vector
exp or sigmoid code and whose bodies demonstrably clamp a negative input to a
finite negative threshold or return a positive nonzero floor below such a
threshold. It then follows immutable local dataflow through unary/binary
expressions, conversions, and wrapper calls. A multiplication in a fused,
backward, VJP, gradient, loss, attention, or activation function is reported
when exactly one side depends on that clamped approximation and the other is a
runtime floating term with no proved bound.

Zero-underflow implementations, exact exp calls, constants, non-floating
multipliers, mutable aliases, forward-only functions, and approximate callees
whose nonzero clamp cannot be established from source stay silent. A
//perfscan:exp-clamp-amplification-validated annotation suppresses a site with
an external bound/parity proof.

Choose among: a cold exact fallback below the approximation threshold; an
explicit zero mask only where the mathematical and machine contract permits
it; or a proved multiplier bound that keeps floor*maxAbsMultiplier inside the
frozen error budget. Do not blindly zero the tail: the established expression
may require Inf*0 = NaN, and signed zero, subnormal, NaN payload/quieting, or
rounding behavior can differ. Plant extreme finite inputs on both sides of the
threshold, both infinities, quiet and signaling NaNs, both zero signs,
subnormals, maximum upstream magnitudes, and the Inf*0 case. Validate forward
and VJP/backward parity at the established dtype and gate the complete fused
workload.

There is NO automatic fix because the clamp threshold/floor, approximation
error contract, upstream bound, exact fallback, dtype rounding, NaN policy,
and safe masking semantics are project-specific.`,
		Before: `e := fastApproxExp(-abs(x))
sigmoid := e / (1 + e)
dx := upstream * sigmoid`,
		After: `if belowApproxClamp(x) {
	// Use the exact scalar tail, or a zero mask only with a proved machine contract.
	return exactVJP(upstream, x)
}
return upstream * approximateSigmoid(x)`,
		MeasuredWin: `In the owner Apple M2 SIMD sigmoid-focal backward kernel,
an extreme finite logit amplified the approximation floor and changed an
expected gradient from about 0.025 to 0.225. A cold exact scalar fallback only
for the below-clamp tail restored the established result while retaining the
fast fused path for ordinary inputs.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6065",
		Doc:  "approximate exp underflow floor is multiplied by a potentially unbounded VJP term",
		Run:  runPS6065,
	},
})

func runPS6065(pass *analysis.Pass) (any, error) {
	clamped := ps6065ClampedApproxFunctions(pass)
	if len(clamped) == 0 {
		return nil, nil
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || clamped[pass.TypesInfo.Defs[function.Name]] ||
				!ps6065GradientContext(function.Name.Name) || ps6065ValidatedAnnotation(file, function) {
				continue
			}
			flow := ps6064FunctionFlow(pass, function)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				if _, nested := node.(*ast.FuncLit); nested {
					return false
				}
				product, ok := node.(*ast.BinaryExpr)
				if !ok || product.Op != token.MUL {
					return true
				}
				leftSource := ps6065ApproxDependency(pass, flow, clamped, product.X, map[types.Object]bool{})
				rightSource := ps6065ApproxDependency(pass, flow, clamped, product.Y, map[types.Object]bool{})
				if (leftSource == nil) == (rightSource == nil) {
					return true
				}
				source, multiplier := leftSource, product.Y
				if source == nil {
					source, multiplier = rightSource, product.X
				}
				if !ps6065RuntimeFloat(pass, multiplier) {
					return true
				}
				pass.Reportf(product.OpPos, "%s has a nonzero deep-negative exp/sigmoid clamp and its residue is multiplied by a potentially unbounded runtime term in %s; bound floor*maxAbsMultiplier or use a cold exact tail/semantically proved zero mask, preserving dtype rounding, planted extreme finite/Inf/NaN/signed-zero/subnormal parity, and Inf*0=NaN where applicable before gating the complete fused VJP (advisory, no automatic fix)", source.Name(), function.Name.Name)
				return true
			})
		}
	}
	return nil, nil
}

func ps6065ClampedApproxFunctions(pass *analysis.Pass) map[types.Object]bool {
	result := map[types.Object]bool{}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || !ps6065ApproxName(function.Name.Name) || !ps6065HasNonzeroClamp(pass, function.Body) {
				continue
			}
			if object := pass.TypesInfo.Defs[function.Name]; object != nil {
				result[object] = true
			}
		}
	}
	return result
}

func ps6065ApproxName(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "exp", "sigmoid") && ps6007ContainsAny(name, "approx", "fast", "simd", "vector", "native")
}

func ps6065GradientContext(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "fused", "backward", "vjp", "gradient", "grad", "loss", "attention", "activation")
}

func ps6065HasNonzeroClamp(pass *analysis.Pass, body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.IfStmt:
			thresholdVariables, threshold := ps6065NegativeThresholdCondition(pass, value.Cond)
			if !threshold {
				return true
			}
			ast.Inspect(value.Body, func(inner ast.Node) bool {
				if found {
					return false
				}
				switch statement := inner.(type) {
				case *ast.ReturnStmt:
					for _, expression := range statement.Results {
						if number, ok := ps6065FloatConstant(pass, expression); ok && number > 0 {
							found = true
						}
					}
				case *ast.AssignStmt:
					for index, expression := range statement.Rhs {
						if index >= len(statement.Lhs) {
							continue
						}
						identifier, identifierOK := ps2110Unparen(statement.Lhs[index]).(*ast.Ident)
						if number, numberOK := ps6065FloatConstant(pass, expression); identifierOK && numberOK && number < 0 && thresholdVariables[pass.TypesInfo.ObjectOf(identifier)] {
							found = true
						}
					}
				}
				return !found
			})
		case *ast.CallExpr:
			if ps6065NegativeClampCall(pass, value) {
				found = true
			}
		}
		return !found
	})
	return found
}

func ps6065NegativeThresholdCondition(pass *analysis.Pass, expression ast.Expr) (map[types.Object]bool, bool) {
	variables := map[types.Object]bool{}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		binary, ok := node.(*ast.BinaryExpr)
		if !ok || binary.Op != token.LSS && binary.Op != token.LEQ && binary.Op != token.GTR && binary.Op != token.GEQ {
			return true
		}
		left, leftConstant := ps6065FloatConstant(pass, binary.X)
		right, rightConstant := ps6065FloatConstant(pass, binary.Y)
		found = leftConstant && left < 0 && !rightConstant || rightConstant && right < 0 && !leftConstant
		if found {
			runtime := binary.X
			if leftConstant {
				runtime = binary.Y
			}
			ast.Inspect(runtime, func(inner ast.Node) bool {
				if identifier, ok := inner.(*ast.Ident); ok {
					if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
						variables[object] = true
					}
				}
				return true
			})
		}
		return !found
	})
	return variables, found
}

func ps6065NegativeClampCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	name := ""
	switch function := ps2110Unparen(call.Fun).(type) {
	case *ast.Ident:
		name = function.Name
	case *ast.SelectorExpr:
		name = function.Sel.Name
	}
	name = ps6007NormalizeName(name)
	if !ps6007ContainsAny(name, "max", "clamp", "clip", "saturate") {
		return false
	}
	negative, runtime := false, false
	for _, argument := range call.Args {
		if number, ok := ps6065FloatConstant(pass, argument); ok {
			negative = negative || number < 0
		} else {
			runtime = true
		}
	}
	return negative && runtime
}

func ps6065FloatConstant(pass *analysis.Pass, expression ast.Expr) (float64, bool) {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int && value.Value.Kind() != constant.Float {
		return 0, false
	}
	number, _ := constant.Float64Val(value.Value)
	return number, true
}

func ps6065ApproxDependency(pass *analysis.Pass, flow ps6064Flow, clamped map[types.Object]bool, expression ast.Expr, seen map[types.Object]bool) types.Object {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		object := pass.TypesInfo.ObjectOf(value)
		if object == nil || seen[object] || flow.writes[object] != 0 {
			return nil
		}
		initializer, ok := flow.initializers[object]
		if !ok {
			return nil
		}
		seen[object] = true
		source := ps6065ApproxDependency(pass, flow, clamped, initializer, seen)
		delete(seen, object)
		return source
	case *ast.UnaryExpr:
		return ps6065ApproxDependency(pass, flow, clamped, value.X, seen)
	case *ast.BinaryExpr:
		if source := ps6065ApproxDependency(pass, flow, clamped, value.X, seen); source != nil {
			return source
		}
		return ps6065ApproxDependency(pass, flow, clamped, value.Y, seen)
	case *ast.CallExpr:
		if object := ps6065CalledObject(pass, value.Fun); clamped[object] {
			return object
		}
		for _, argument := range value.Args {
			if source := ps6065ApproxDependency(pass, flow, clamped, argument, seen); source != nil {
				return source
			}
		}
	}
	return nil
}

func ps6065CalledObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch function := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(function)
	case *ast.SelectorExpr:
		return pass.TypesInfo.ObjectOf(function.Sel)
	default:
		return nil
	}
}

func ps6065RuntimeFloat(pass *analysis.Pass, expression ast.Expr) bool {
	typeValue := pass.TypesInfo.TypeOf(expression)
	if typeValue == nil {
		return false
	}
	basic, ok := typeValue.Underlying().(*types.Basic)
	if !ok || basic.Kind() != types.Float32 && basic.Kind() != types.Float64 {
		return false
	}
	value, known := pass.TypesInfo.Types[ps2110Unparen(expression)]
	return !known || value.Value == nil
}

func ps6065ValidatedAnnotation(file *ast.File, function *ast.FuncDecl) bool {
	for _, group := range file.Comments {
		if group != function.Doc && !(group.End() <= function.Pos() && function.Pos()-group.End() <= 3) && !(group.Pos() >= function.Body.Pos() && group.End() <= function.Body.End()) {
			continue
		}
		for _, comment := range group.List {
			text := ps6058Compact(comment.Text)
			if strings.Contains(text, "perfscanexpclampamplificationvalidated") || strings.Contains(text, "perfscanboundedexpclampresidue") {
				return true
			}
		}
	}
	return false
}
