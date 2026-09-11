package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// The resolver supplies facts at the expression's program point. A typed name
// alone is not an immutable value: callers must prove reaching bindings, object
// identity, effects and path conditions before supplying a known extent.
type ps6125ExtentResolver func(types.Object, []*types.Var, bool) ps6125Extent

// ps6125ExtentExpression lowers a restricted source expression to geometry.
// Unknown leaves, calls, narrowing conversions and unsupported operators remain
// unknown. Algebraic equality is not a proof that source int arithmetic cannot
// overflow; that separate obligation remains with the eventual diagnostic.
func ps6125ExtentExpression(pass *analysis.Pass, expression ast.Expr, resolve ps6125ExtentResolver) ps6125Extent {
	expression = ps2110Unparen(expression)
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return ps6125Extent{}
	}
	basic, integer := typ.Underlying().(*types.Basic)
	if !integer || basic.Info()&types.IsInteger == 0 {
		return ps6125Extent{}
	}
	if value := pass.TypesInfo.Types[expression].Value; value != nil {
		integer, exact := constant.Int64Val(constant.ToInt(value))
		if exact {
			return ps6125ConstantExtent(integer)
		}
		return ps6125Extent{}
	}
	// Nonconstant narrow/unsigned arithmetic is intentionally not normalized
	// into a signed-int capacity expression with different overflow semantics.
	if basic.Kind() != types.Int {
		return ps6125Extent{}
	}
	if binary, ok := expression.(*ast.BinaryExpr); ok {
		if binary.Op != token.MUL {
			return ps6125Extent{}
		}
		return ps6125MultiplyExtents(ps6125ExtentExpression(pass, binary.X, resolve), ps6125ExtentExpression(pass, binary.Y, resolve))
	}
	length := false
	if call, ok := expression.(*ast.CallExpr); ok {
		if len(call.Args) != 1 || call.Ellipsis.IsValid() || !typedBuiltinName(pass, call.Fun, "len") {
			return ps6125Extent{}
		}
		expression = ps2110Unparen(call.Args[0])
		length = true
	}
	root, fields, ok := ps6125ExtentPath(pass, expression)
	if !ok || resolve == nil {
		return ps6125Extent{}
	}
	return resolve(root, fields, length)
}

// Expand promoted fields using the type checker's complete selection index.
// d.width and d.header.width describe the same path only if Go actually chose
// that exact embedded header field. Sibling header instances remain distinct.
func ps6125ExtentPath(pass *analysis.Pass, expression ast.Expr) (types.Object, []*types.Var, bool) {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		object, variable := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
		return object, nil, variable
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return nil, nil, false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.FieldVal {
		return nil, nil, false
	}
	root, fields, ok := ps6125ExtentPath(pass, selector.X)
	if !ok {
		return nil, nil, false
	}
	typ := selection.Recv()
	for _, index := range selection.Index() {
		typ = types.Unalias(typ)
		if pointer, ok := typ.(*types.Pointer); ok {
			typ = types.Unalias(pointer.Elem())
		}
		structure, ok := typ.Underlying().(*types.Struct)
		if !ok || index < 0 || index >= structure.NumFields() {
			return nil, nil, false
		}
		field := structure.Field(index)
		fields = append(fields, field)
		typ = field.Type()
	}
	return root, fields, true
}
