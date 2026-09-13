package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Interior-cell closure only proves that an address cannot expose its enclosing
// owner. Loaded interface values and their native effects are separate proof
// obligations; this is not a buffer alias or native immutability summary.
func ps6136InteriorOwnerCells(pass *analysis.Pass, pkg *ssa.Package, owner *types.Named, workspace *types.Var) map[ast.Node]bool {
	proved := make(map[ast.Node]bool)
	if pass == nil || pkg == nil || owner == nil || workspace == nil {
		return proved
	}
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.Contains(comment.Text, "go:linkname") {
					return proved
				}
			}
		}
	}
	structure, ok := owner.Underlying().(*types.Struct)
	if !ok {
		return proved
	}
	closed, seen := make(map[*types.Var]bool), make(map[*types.Var]bool)
	positions := make(map[token.Pos]*types.Var)
	graph := &ps6136CellUseGraph{pkg: pkg, remaining: 65536, active: make(map[ps6136CellUse]bool), done: make(map[ps6136CellUse]bool)}
	for _, function := range ps6136SourceFunctions(pkg) {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				field, ok := instruction.(*ssa.FieldAddr)
				if !ok || !types.Identical(field.X.Type(), types.NewPointer(owner)) || field.Field < 0 || field.Field >= structure.NumFields() {
					continue
				}
				member := structure.Field(field.Field)
				if member == workspace || !ps6136FlatCell(member.Type()) {
					continue
				}
				if !seen[member] {
					closed[member] = true
					seen[member] = true
				}
				// Include every source occurrence, including implicit receiver
				// addresses at other call sites, before approving any address.
				if !graph.close(ps6136CellUse{value: field}, 0) {
					closed[member] = false
				}
				if field.Pos().IsValid() {
					positions[field.Pos()] = member
				}
			}
		}
	}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			address, ok := node.(*ast.UnaryExpr)
			if !ok || address.Op != token.AND {
				return true
			}
			selector, ok := ps2110Unparen(address.X).(*ast.SelectorExpr)
			if !ok || !types.Identical(pass.TypesInfo.TypeOf(selector.X), types.NewPointer(owner)) {
				return true
			}
			selection := pass.TypesInfo.Selections[selector]
			if selection == nil || selection.Kind() != types.FieldVal || len(selection.Index()) != 1 {
				return true
			}
			field, _ := selection.Obj().(*types.Var)
			if closed[field] && positions[selector.Sel.Pos()] == field {
				proved[address] = true // Never approve its enclosing call/assignment.
			}
			return true
		})
	}
	return proved
}

// A flat cell cannot contain owner/collection metadata or an interior pointer.
// Interface payloads are loaded values, not addresses of the containing cell.
func ps6136FlatCell(typ types.Type) bool {
	structure, ok := typ.Underlying().(*types.Struct)
	if !ok || structure.NumFields() == 0 {
		return false
	}
	for i := 0; i < structure.NumFields(); i++ {
		switch field := structure.Field(i).Type().Underlying().(type) {
		case *types.Basic:
			if field.Kind() == types.UnsafePointer {
				return false
			}
		case *types.Interface:
		default:
			return false
		}
	}
	return true
}

type ps6136CellUse struct {
	value    ssa.Value
	writable bool // Only an actual cell field, never the whole cell.
}

type ps6136CellUseGraph struct {
	pkg       *ssa.Package
	remaining int
	active    map[ps6136CellUse]bool
	done      map[ps6136CellUse]bool
}

// This context-independent effect summary examines all SSA uses, not a selected
// path or allocation instance. Actual call arguments select the exact formals;
// recursive forwarding is unknown, even if another branch would terminate.
func (graph *ps6136CellUseGraph) close(use ps6136CellUse, depth int) bool {
	if graph.remaining <= 0 || depth > 128 || use.value == nil || graph.active[use] {
		return false
	}
	graph.remaining--
	if graph.done[use] {
		return true
	}
	users := use.value.Referrers()
	if users == nil {
		return false
	}
	graph.active[use] = true
	defer delete(graph.active, use)
	for _, instruction := range *users {
		if graph.remaining <= 0 || instruction.Parent() != use.value.Parent() {
			return false
		}
		graph.remaining--
		switch user := instruction.(type) {
		case *ssa.DebugRef:
		case *ssa.FieldAddr:
			if user.X != use.value || use.writable || !graph.close(ps6136CellUse{value: user, writable: true}, depth+1) {
				return false
			}
		case *ssa.UnOp:
			if !use.writable || user.Op != token.MUL || user.X != use.value {
				return false
			}
		case *ssa.Store:
			if !use.writable || user.Addr != use.value || user.Val == use.value {
				return false
			}
		case *ssa.BinOp:
			other := user.X
			if other == use.value {
				other = user.Y
			}
			nilValue, ok := other.(*ssa.Const)
			if (user.Op != token.EQL && user.Op != token.NEQ) || !ok || !nilValue.IsNil() {
				return false
			}
		case *ssa.Call:
			callee := user.Call.StaticCallee()
			if user.Call.IsInvoke() || callee == nil || callee.Pkg != graph.pkg || len(callee.Blocks) == 0 || len(callee.FreeVars) != 0 || len(callee.Params) != len(user.Call.Args) {
				return false
			}
			if declaration, ok := callee.Syntax().(*ast.FuncDecl); !ok || declaration.Body == nil {
				return false
			}
			matched := false
			for index, argument := range user.Call.Args {
				if argument != use.value {
					continue
				}
				matched = true
				if !types.Identical(argument.Type(), callee.Params[index].Type()) || !graph.close(ps6136CellUse{value: callee.Params[index], writable: use.writable}, depth+1) {
					return false
				}
			}
			if !matched {
				return false
			}
		default:
			return false // Includes returns, aliases, captures, casts, go/defer.
		}
	}
	graph.done[use] = true
	return true
}
