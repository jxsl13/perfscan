package checks

import (
	"bytes"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"strconv"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/crossover"
	"golang.org/x/tools/go/analysis"
)

// ps6131Harness starts from observed production declarations, not supplied
// clone text. A factory must produce exactly the operation's typed arguments.
func ps6131Harness(pass *analysis.Pass, source *ps6131Source, c *config.DispatchCrossoverContract) (*crossover.HarnessModel, error) {
	if source == nil || source.owner == nil {
		return nil, errors.New("unbound production operation")
	}
	owner, ok := pass.TypesInfo.Defs[source.owner.Name].(*types.Func)
	if !ok {
		return nil, errors.New("missing typed production function")
	}
	signature := owner.Type().(*types.Signature)
	factory := source.functions[c.OperationInputFactory]
	if factory == nil {
		return nil, errors.New("input factory is absent from typed scan partition")
	}
	object, ok := pass.TypesInfo.Defs[factory.Name].(*types.Func)
	if !ok {
		return nil, errors.New("input factory is not a function")
	}
	input := object.Type().(*types.Signature)
	if input.Recv() != nil || input.Variadic() || input.TypeParams().Len() != 0 || input.Params().Len() != 1 || !types.Identical(input.Params().At(0).Type(), types.Typ[types.Int]) || input.Results().Len() != signature.Params().Len() {
		return nil, errors.New("input factory must take int and return exact operation argument tuple")
	}
	for i := range input.Results().Len() {
		if !types.Identical(input.Results().At(i).Type(), signature.Params().At(i).Type()) {
			return nil, errors.New("input factory argument dtype differs from production")
		}
	}
	m := &crossover.HarnessModel{Boundary: source.boundary, Package: pass.Pkg.Name(), PackagePath: pass.Pkg.Path(), ArgumentCount: signature.Params().Len(), ResultCount: signature.Results().Len(), Dtype: source.dtype, Imports: map[string]string{}}
	var printed bytes.Buffer
	if err := format.Node(&printed, pass.Fset, source.owner.Type); err != nil {
		return nil, err
	}
	m.Signature = printed.String()
	var err error
	m.SerialBody, err = ps6131ForcedBody(pass.Fset, source, true)
	if err != nil {
		return nil, err
	}
	m.ParallelBody, err = ps6131ForcedBody(pass.Fset, source, false)
	if err != nil {
		return nil, err
	}
	// Only package qualifications used in the complete copied body/header are
	// imported. Unused original imports must not affect generated builds.
	ast.Inspect(source.owner, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if pkg, ok := pass.TypesInfo.Uses[id].(*types.PkgName); ok {
			m.Imports[pkg.Imported().Path()] = id.Name
		}
		return true
	})
	errorType := types.Universe.Lookup("error").Type()
	for i := range signature.Results().Len() {
		if types.Identical(signature.Results().At(i).Type(), errorType) {
			m.ErrorResults = append(m.ErrorResults, i)
		}
	}
	// The raw-slice route is typed and returns the allocated destination.
	if _, ok := ps6131Signature(signature, false); ok {
		m.OutputExtent = "len(_r0)"
		m.InputData = "_arg0"
		m.OutputData = "_r0"
		m.OutputGuard = "true"
		return m, nil
	}
	// Tensor/context operations require a separately proved output-storage and
	// dtype join. Do not silently accept arbitrary return APIs or configured
	// expressions as shape facts.
	if err := ps6131TensorExtent(pass, source, signature, m); err == nil {
		return m, nil
	}
	return nil, errors.New("unsupported production output shape")
}

func ps6131TensorExtent(pass *analysis.Pass, source *ps6131Source, sig *types.Signature, m *crossover.HarnessModel) error {
	fail := errors.New("tensor output extent and dtype join not qualified")
	if sig.Results().Len() != 2 || len(m.ErrorResults) != 1 || m.ErrorResults[0] != 1 || len(source.owner.Body.List) != 4 {
		return fail
	}
	returned, ok := source.owner.Body.List[len(source.owner.Body.List)-1].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 2 {
		return fail
	}
	literal, ok := returned.Results[0].(*ast.CompositeLit)
	if !ok || len(literal.Elts) != 1 {
		return fail
	}
	out := pass.TypesInfo.ObjectOf(ps6131Ident(literal.Elts[0]))
	if out == nil {
		return fail
	}
	nilResult, ok := ps2110Unparen(returned.Results[1]).(*ast.Ident)
	if !ok || nilResult.Name != "nil" {
		return fail
	}
	condition, ok := ps2110Unparen(source.guard.Cond).(*ast.BinaryExpr)
	if !ok {
		return fail
	}
	length, ok := ps2110Unparen(condition.X).(*ast.CallExpr)
	if !ok || len(length.Args) != 1 {
		return fail
	}
	dst := pass.TypesInfo.ObjectOf(ps6131Ident(length.Args[0]))
	// The measured boundary uses the exact returned tensor's storage length,
	// not an independent temporary's requested size or a config expression.
	method := "F32"
	if source.dtype == "float64" {
		method = "F64"
	}
	matched := false
	ast.Inspect(source.owner.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != len(assign.Rhs) {
			return true
		}
		for i, lhs := range assign.Lhs {
			if pass.TypesInfo.ObjectOf(ps6131Ident(lhs)) != dst {
				continue
			}
			call, ok := assign.Rhs[i].(*ast.CallExpr)
			if !ok || len(call.Args) != 0 {
				continue
			}
			view, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || view.Sel.Name != method {
				continue
			}
			storage, ok := view.X.(*ast.CallExpr)
			if !ok || len(storage.Args) != 0 {
				continue
			}
			selector, ok := storage.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Storage" || pass.TypesInfo.ObjectOf(ps6131Ident(selector.X)) != out {
				continue
			}
			matched = true
		}
		return true
	})
	if !matched {
		return fail
	}
	parents := ps6071Parents(source.owner.Body)
	clause, ok := parents[source.guard].(*ast.CaseClause)
	if !ok || len(clause.List) != 1 {
		return fail
	}
	block, ok := parents[clause].(*ast.BlockStmt)
	if !ok {
		return fail
	}
	sw, ok := parents[block].(*ast.SwitchStmt)
	if !ok || sw.Tag == nil || source.owner.Body.List[2] != sw || len(clause.Body) != 3 || clause.Body[1] != source.guard {
		return fail
	}
	outputDefinition, ok := source.owner.Body.List[1].(*ast.AssignStmt)
	if !ok || outputDefinition.Tok != token.DEFINE || len(outputDefinition.Lhs) != 1 || len(outputDefinition.Rhs) != 1 || pass.TypesInfo.ObjectOf(ps6131Ident(outputDefinition.Lhs[0])) != out {
		return fail
	}
	// Direct storage receivers and the exact returned object are the only
	// permitted output-object references. Rebinding, aliases and escapes do not
	// establish that the measured predicate governs the returned storage.
	allowedOut := map[*ast.Ident]bool{ps6131Ident(outputDefinition.Lhs[0]): true, ps6131Ident(literal.Elts[0]): true}
	ast.Inspect(source.owner.Body, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == "Storage" {
			id := ps6131Ident(selector.X)
			if id != nil && pass.TypesInfo.ObjectOf(id) == out {
				allowedOut[id] = true
			}
		}
		return true
	})
	validOut := true
	ast.Inspect(source.owner.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == out && !allowedOut[id] {
			validOut = false
		}
		return true
	})
	if !validOut {
		return fail
	}
	// Require the dtype case to select a real typed constant and the tag to
	// depend directly on an operation argument; labels alone are not facts.
	selector, ok := clause.List[0].(*ast.SelectorExpr)
	if !ok {
		return fail
	}
	dtype, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Const)
	if !ok || dtype.Name() != method {
		return fail
	}
	dependent := false
	ast.Inspect(sw.Tag, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		object := pass.TypesInfo.ObjectOf(id)
		for i := range sig.Params().Len() {
			if object == sig.Params().At(i) {
				dependent = true
			}
		}
		return true
	})
	if !dependent {
		return fail
	}
	render := func(expr ast.Expr) (string, error) {
		type reference struct{ name, replacement string }
		references := make([]reference, 0, 8)
		ast.Inspect(expr, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				ref := reference{name: id.Name}
				for i := range sig.Params().Len() {
					if pass.TypesInfo.ObjectOf(id) == sig.Params().At(i) {
						ref.replacement = "_arg" + strconv.Itoa(i)
					}
				}
				references = append(references, ref)
			}
			return true
		})
		var text bytes.Buffer
		if err := format.Node(&text, pass.Fset, expr); err != nil {
			return "", err
		}
		copy, err := parser.ParseExpr(text.String())
		if err != nil {
			return "", err
		}
		index := 0
		consistent := true
		ast.Inspect(copy, func(n ast.Node) bool {
			id, ok := n.(*ast.Ident)
			if !ok {
				return true
			}
			if index >= len(references) || id.Name != references[index].name {
				consistent = false
				return false
			}
			if references[index].replacement != "" {
				id.Name = references[index].replacement
			}
			index++
			return true
		})
		if !consistent || index != len(references) {
			return "", errors.New("typed expression identifiers changed while formatting")
		}
		text.Reset()
		if err := format.Node(&text, token.NewFileSet(), copy); err != nil {
			return "", err
		}
		return text.String(), nil
	}
	tag, err := render(sw.Tag)
	if err != nil {
		return err
	}
	arm, err := render(clause.List[0])
	if err != nil {
		return err
	}
	serialCall := ps6032StatementCall(source.guard.Body.List[0])
	if serialCall == nil || len(serialCall.Args) != 2 {
		return fail
	}
	src := pass.TypesInfo.ObjectOf(ps6131Ident(serialCall.Args[1]))
	if src == nil {
		return fail
	}
	storageDefinition, ok := clause.Body[0].(*ast.AssignStmt)
	if !ok || storageDefinition.Tok != token.DEFINE || len(storageDefinition.Lhs) != 2 || len(storageDefinition.Rhs) != 2 {
		return fail
	}
	left0, left1 := pass.TypesInfo.ObjectOf(ps6131Ident(storageDefinition.Lhs[0])), pass.TypesInfo.ObjectOf(ps6131Ident(storageDefinition.Lhs[1]))
	if !(left0 == src && left1 == dst || left0 == dst && left1 == src) {
		return fail
	}
	assignment := func(object types.Object) ast.Expr {
		var expression ast.Expr
		ambiguous := false
		ast.Inspect(source.owner.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for i, lhs := range assign.Lhs {
				if pass.TypesInfo.ObjectOf(ps6131Ident(lhs)) == object {
					if expression != nil || i >= len(assign.Rhs) {
						ambiguous = true
						continue
					}
					expression = assign.Rhs[i]
				}
			}
			return true
		})
		if ambiguous {
			return nil
		}
		return expression
	}
	inputView, ok := assignment(src).(*ast.CallExpr)
	if !ok || len(inputView.Args) != 0 {
		return fail
	}
	inputSelector, ok := inputView.Fun.(*ast.SelectorExpr)
	if !ok || inputSelector.Sel.Name != method {
		return fail
	}
	storageCall, ok := inputSelector.X.(*ast.CallExpr)
	if !ok || len(storageCall.Args) != 0 {
		return fail
	}
	storageSelector, ok := storageCall.Fun.(*ast.SelectorExpr)
	if !ok || storageSelector.Sel.Name != "Storage" {
		return fail
	}
	contiguousObject := pass.TypesInfo.ObjectOf(ps6131Ident(storageSelector.X))
	if contiguousObject == nil {
		return fail
	}
	inputDefinition, ok := source.owner.Body.List[0].(*ast.AssignStmt)
	if !ok || inputDefinition.Tok != token.DEFINE || len(inputDefinition.Lhs) != 1 || len(inputDefinition.Rhs) != 1 || pass.TypesInfo.ObjectOf(ps6131Ident(inputDefinition.Lhs[0])) != contiguousObject {
		return fail
	}
	allowedInput := map[*ast.Ident]bool{ps6131Ident(inputDefinition.Lhs[0]): true}
	ast.Inspect(source.owner.Body, func(n ast.Node) bool {
		if selector, ok := n.(*ast.SelectorExpr); ok && selector.Sel.Name == "Storage" {
			if id := ps6131Ident(selector.X); id != nil && pass.TypesInfo.ObjectOf(id) == contiguousObject {
				allowedInput[id] = true
			}
		}
		return true
	})
	validInput := true
	ast.Inspect(source.owner.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == contiguousObject && !allowedInput[id] {
			validInput = false
		}
		return true
	})
	if !validInput {
		return fail
	}
	contiguous, ok := assignment(contiguousObject).(*ast.CallExpr)
	if !ok || len(contiguous.Args) != 0 {
		return fail
	}
	contiguousSelector, ok := contiguous.Fun.(*ast.SelectorExpr)
	if !ok || contiguousSelector.Sel.Name != "Contiguous" {
		return fail
	}
	tagCall, ok := sw.Tag.(*ast.CallExpr)
	if !ok || len(tagCall.Args) != 0 {
		return fail
	}
	tagSelector, ok := tagCall.Fun.(*ast.SelectorExpr)
	if !ok || tagSelector.Sel.Name != "Dtype" {
		return fail
	}
	inputReceiver, err := render(contiguousSelector.X)
	if err != nil {
		return err
	}
	tagReceiver, err := render(tagSelector.X)
	if err != nil {
		return err
	}
	if inputReceiver != tagReceiver {
		return fail
	}
	inputExpression, err := render(contiguous)
	if err != nil {
		return err
	}
	m.InputData = "(" + inputExpression + ").Storage()." + method + "()"
	m.OutputData = "_r0[0].Storage()." + method + "()"
	m.OutputGuard = "len(_r0)==1 && _r0[0]!=nil"
	m.OutputExtent = "len(_r0[0].Storage()." + method + "())"
	m.DtypeGuard = "if " + tag + " != " + arm + " { b.Fatal(\"factory selected an unqualified dtype branch\") }"
	m.WorkerCallback = true
	return nil
}
