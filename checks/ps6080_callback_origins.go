package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

type ps6080CallbackDefinition struct {
	node       ast.Node
	expression ast.Expr
	field      *types.Var
}

type ps6080CallbackDefinitions map[*ps6080CallbackDefinition]bool

type ps6080CallbackObjectFlow struct {
	entry  map[*cfg.Block]ps6080CallbackDefinitions
	writes map[ast.Node]*ps6080CallbackDefinition
}

type ps6080CallbackOriginScope struct {
	flow    *ps6122Flow
	objects map[ps6080CallbackStorage]*ps6080CallbackObjectFlow
}

type ps6080CallbackStorage struct {
	object types.Object
	field  *types.Var
}

type ps6080CallbackOrigins struct {
	pass        *analysis.Pass
	function    *ps6080Function
	parameters  map[types.Object]int
	parents     map[ast.Node]ast.Node
	invocations map[*ast.FuncLit][]*ast.CallExpr
	scopes      map[*ast.BlockStmt]*ps6080CallbackOriginScope
}

func (origins *ps6080CallbackOrigins) scope(body *ast.BlockStmt) *ps6080CallbackOriginScope {
	if existing := origins.scopes[body]; existing != nil {
		return existing
	}
	result := &ps6080CallbackOriginScope{flow: ps6122NewFlow(origins.pass, body), objects: make(map[ps6080CallbackStorage]*ps6080CallbackObjectFlow)}
	origins.scopes[body] = result
	return result
}

// One finite reaching-definition solution is shared by every query for an
// object in a body. A nil definition denotes its incoming value. Writes kill
// old definitions; CFG joins union alternatives, including loop backedges.
func (origins *ps6080CallbackOrigins) objectFlow(body *ast.BlockStmt, object types.Object, field *types.Var) *ps6080CallbackObjectFlow {
	scope := origins.scope(body)
	storage := ps6080CallbackStorage{object: object, field: field}
	if existing := scope.objects[storage]; existing != nil {
		return existing
	}
	result := &ps6080CallbackObjectFlow{entry: make(map[*cfg.Block]ps6080CallbackDefinitions), writes: make(map[ast.Node]*ps6080CallbackDefinition)}
	scope.objects[storage] = result
	ps6071InspectOwnBody(body, func(node ast.Node) bool {
		var value ast.Expr
		projection := field
		written := false
		switch assignment := node.(type) {
		case *ast.AssignStmt:
			for index, lhs := range assignment.Lhs {
				if selector, ok := ps2110Unparen(lhs).(*ast.SelectorExpr); ok && field != nil {
					id, direct := ps2110Unparen(selector.X).(*ast.Ident)
					if direct && origins.pass.TypesInfo.ObjectOf(id) == object && origins.pass.TypesInfo.ObjectOf(selector.Sel) == field {
						written = true
						projection = nil
						if len(assignment.Lhs) == len(assignment.Rhs) {
							value = assignment.Rhs[index]
						}
					}
					continue
				}
				id, ok := ps2110Unparen(lhs).(*ast.Ident)
				if !ok || origins.pass.TypesInfo.ObjectOf(id) != object {
					continue
				}
				written = true
				if assignment.Tok == token.ASSIGN || assignment.Tok == token.DEFINE {
					if len(assignment.Lhs) == len(assignment.Rhs) {
						value = assignment.Rhs[index]
					}
				}
			}
		case *ast.ValueSpec:
			for index, name := range assignment.Names {
				if origins.pass.TypesInfo.Defs[name] != object {
					continue
				}
				written = true
				if len(assignment.Names) == len(assignment.Values) {
					value = assignment.Values[index]
				}
			}
		}
		if written {
			result.writes[node] = &ps6080CallbackDefinition{node: node, expression: value, field: projection}
		}
		return true
	})
	if len(scope.flow.graph.Blocks) == 0 {
		return result
	}
	entry := scope.flow.graph.Blocks[0]
	result.entry[entry] = ps6080CallbackDefinitions{nil: true}
	queue := []*cfg.Block{entry}
	queued := map[*cfg.Block]bool{entry: true}
	for index := 0; index < len(queue); index++ {
		block := queue[index]
		queued[block] = false
		out := result.entry[block]
		for _, node := range block.Nodes {
			if definition := result.writes[node]; definition != nil {
				out = ps6080CallbackDefinitions{definition: true}
			}
		}
		for _, successor := range ps6122Successors(origins.pass, scope.flow, block, token.NoPos) {
			if !scope.flow.reachable[successor] {
				continue
			}
			destination := result.entry[successor]
			if destination == nil {
				destination = make(ps6080CallbackDefinitions)
				result.entry[successor] = destination
			}
			changed := false
			for definition := range out {
				if !destination[definition] {
					destination[definition] = true
					changed = true
				}
			}
			if changed && !queued[successor] {
				queue = append(queue, successor)
				queued[successor] = true
			}
		}
	}
	return result
}

func (origins *ps6080CallbackOrigins) definitionsAt(body *ast.BlockStmt, object types.Object, field *types.Var, at token.Pos) ps6080CallbackDefinitions {
	scope := origins.scope(body)
	block := ps6122BlockAt(origins.pass, scope.flow, at)
	if block == nil {
		return nil
	}
	flow := origins.objectFlow(body, object, field)
	result := flow.entry[block]
	for _, node := range block.Nodes {
		if node.End() > at {
			break
		}
		if definition := flow.writes[node]; definition != nil {
			result = ps6080CallbackDefinitions{definition: true}
		}
	}
	return result
}

func (origins *ps6080CallbackOrigins) definitionsAtExit(body *ast.BlockStmt, object types.Object, field *types.Var) ps6080CallbackDefinitions {
	scope := origins.scope(body)
	flow := origins.objectFlow(body, object, field)
	result := make(ps6080CallbackDefinitions)
	for _, block := range scope.flow.graph.Blocks {
		if !scope.flow.reachable[block] || len(ps6122Successors(origins.pass, scope.flow, block, token.NoPos)) != 0 {
			continue
		}
		out := flow.entry[block]
		for _, node := range block.Nodes {
			if definition := flow.writes[node]; definition != nil {
				out = ps6080CallbackDefinitions{definition: true}
			}
		}
		for definition := range out {
			result[definition] = true
		}
	}
	return result
}

func (origins *ps6080CallbackOrigins) bodyAt(node ast.Node) (*ast.BlockStmt, *ast.FuncLit) {
	if literal := ps6080ContainingLiteral(node, origins.parents); literal != nil {
		return literal.Body, literal
	}
	return origins.function.body, nil
}

func (origins *ps6080CallbackOrigins) at(expression ast.Expr, query ast.Node) ps6080IndexSet {
	type state struct {
		object types.Object
		field  *types.Var
		body   *ast.BlockStmt
		at     token.Pos
		exit   bool
	}
	var result ps6080IndexSet
	var queue []state
	seen := make(map[state]bool)
	var enqueue func(ast.Expr, *ast.BlockStmt, token.Pos)
	enqueue = func(expression ast.Expr, body *ast.BlockStmt, at token.Pos) {
		if expression == nil {
			return
		}
		for {
			expression = ps2110Unparen(expression)
			conversion, ok := expression.(*ast.CallExpr)
			if !ok || len(conversion.Args) != 1 || !origins.pass.TypesInfo.Types[conversion.Fun].IsType() {
				break
			}
			expression = conversion.Args[0]
		}
		if unary, ok := expression.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			enqueue(unary.X, body, at)
			return
		}
		if identifier, ok := expression.(*ast.Ident); ok {
			if object := origins.pass.TypesInfo.ObjectOf(identifier); object != nil {
				queue = append(queue, state{object: object, body: body, at: at})
			}
			return
		}
		if selector, ok := expression.(*ast.SelectorExpr); ok {
			selection := origins.pass.TypesInfo.Selections[selector]
			field, valid := selectionObject(selection).(*types.Var)
			identifier, local := ps2110Unparen(selector.X).(*ast.Ident)
			if valid && selection.Kind() == types.FieldVal && local {
				queue = append(queue, state{object: origins.pass.TypesInfo.ObjectOf(identifier), field: field, body: body, at: at})
			}
			return
		}
		if literal, ok := expression.(*ast.FuncLit); ok {
			ast.Inspect(literal.Body, func(node ast.Node) bool {
				if nested, ok := node.(*ast.FuncLit); ok && nested != literal {
					return false
				}
				call, ok := node.(*ast.CallExpr)
				if ok && !origins.pass.TypesInfo.Types[call.Fun].IsType() {
					enqueue(call.Fun, literal.Body, call.Pos())
				}
				return true
			})
			return
		}
		if composite, ok := expression.(*ast.CompositeLit); ok {
			for _, element := range composite.Elts {
				if pair, keyed := element.(*ast.KeyValueExpr); keyed {
					enqueue(pair.Value, body, at)
				} else {
					enqueue(element, body, at)
				}
			}
		}
	}
	body, _ := origins.bodyAt(query)
	escapeAt := query.Pos()
	enqueue(expression, body, query.Pos())
	for index := 0; index < len(queue); index++ {
		current := queue[index]
		if seen[current] {
			continue
		}
		seen[current] = true
		definitions := origins.definitionsAt(current.body, current.object, current.field, current.at)
		if current.exit {
			definitions = origins.definitionsAtExit(current.body, current.object, current.field)
		}
		for definition := range definitions {
			if definition != nil {
				if definition.field != nil {
					if identifier, ok := ps2110Unparen(definition.expression).(*ast.Ident); ok {
						queue = append(queue, state{object: origins.pass.TypesInfo.ObjectOf(identifier), field: definition.field, body: current.body, at: definition.node.Pos()})
						continue
					}
					composite, ok := ps2110Unparen(definition.expression).(*ast.CompositeLit)
					if !ok {
						continue
					}
					structure, _ := origins.pass.TypesInfo.TypeOf(composite).Underlying().(*types.Struct)
					for index, element := range composite.Elts {
						pair, keyed := element.(*ast.KeyValueExpr)
						if !keyed {
							if structure != nil && index < structure.NumFields() && structure.Field(index) == definition.field {
								enqueue(element, current.body, definition.node.Pos())
							}
							continue
						}
						key, named := ps2110Unparen(pair.Key).(*ast.Ident)
						if named && origins.pass.TypesInfo.ObjectOf(key) == definition.field {
							enqueue(pair.Value, current.body, definition.node.Pos())
						}
					}
				} else {
					enqueue(definition.expression, current.body, definition.node.Pos())
				}
				continue
			}
			if current.body == origins.function.body {
				if parameter, ok := origins.parameters[current.object]; ok {
					ps6080AddIndex(&result, parameter)
				}
				continue
			}
			literal, _ := origins.parents[current.body].(*ast.FuncLit)
			if literal == nil {
				continue
			}
			parameter := ps6080LiteralParameterIndex(origins.pass, literal, current.object)
			for _, invocation := range origins.invocations[literal] {
				outer, _ := origins.bodyAt(invocation)
				if parameter >= 0 {
					if parameter < len(invocation.Args) {
						enqueue(invocation.Args[parameter], outer, invocation.Pos())
					}
				} else {
					deferred := false
					if _, ok := origins.parents[invocation].(*ast.DeferStmt); ok {
						deferred = true
					}
					queue = append(queue, state{object: current.object, body: outer, at: invocation.Pos(), exit: deferred})
				}
			}
			if len(origins.invocations[literal]) == 0 {
				queue = append(queue, state{object: current.object, body: origins.function.body, at: escapeAt})
			}
		}
	}
	return result
}
