package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6085 implements owner issue #808 as a fail-closed, source-only advisory.
var PS6085 = register(&lint.Check{
	ID:          "PS6085",
	Category:    "verify",
	Slug:        "bounded-state-affine-lookup-expansion",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"stateExpandedLookupContracts"},
	Doc: lint.Documentation{
		Title: "a hot loop repeatedly adjusts a bounded immutable lookup row for a small state",
		Text: `PS6085 finds a deliberately narrow Go-source analogue of owner issue
#808. An exact configured hot function must select one of two compile-time
floating-point offsets, then add that offset to every lane of one row from an
exact configured unexported package-level array before multiplying in the same element
type. The complete lane loop must have the fixed width of the table's final
dimension. Direct indexing, a by-value row alias, and a confined read-only row
pointer are supported.

The contract supplies facts source syntax cannot establish: profile evidence,
post-initialization immutability, exact-dtype reproducibility, an explicit
expanded-table byte budget, and mandatory initialization-order safety, cache,
exact-output, and paired-benchmark gates. The analyzer independently rejects
duplicate contracts, non-array or non-float tables, visible writes after init,
escaping table aliases, dynamic or partial lane loops, more than two state
values, non-constant state values, mismatched rounding types, adjustment
without the later multiply, and candidates outside the exact owner.

There is NO automatic fix. A package table populated by init cannot safely feed
an earlier variable initializer: build the expanded table from the original
immutable source data, or populate it only after the source table is complete.
Precompute each entry in the original lookup element type and preserve the add
before any later multiply. The diagnostic states both old and expanded byte
footprints: it is not a universal speedup.

The measured owner optimization was retained in ARM64 assembly, which a Go
analysis pass cannot inspect. PS6085 instead recognizes its exact Go fallback
shape; it does not claim to discover the measured assembly instruction site.
Use //perfscan:ignore PS6085 after a reviewed decision.`,
		Before: `delta := float32(0.125)
if code&1 != 0 {
	delta = -0.125
}
row := &grid[index]
for lane := range 8 {
	weight := scale * (row[lane] + delta)
	consume(weight)
}`,
		After: `// Candidate only; initialize after preserving source-table order.
row := &expandedGrid[state][index]
for lane := range 8 {
	weight := scale * row[lane]
	consume(weight)
}`,
		MeasuredWin: `Owner issue #808 measured an independently initialized
two-state 128 KiB float32 grid replacing a 64 KiB grid plus two vector adds in
GoAI's direct-F32 IQ1_S ARM64 row dot. Five alternating fresh-process pairs on
Apple M2 Pro improved K4096 by 8.35% (p=0.016) and M1/N64/K1024 by 12.7%
(p=0.056), while scalar controls and allocations were unchanged; a broad
M1/N4096/K1024 screen was statistically neutral. Those results motivate the
advisory and its required cache/benchmark warning, not a universal speedup.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6085",
		Doc:  "configured hot loop repeatedly adjusts every lane of a bounded immutable lookup row",
		Run:  runPS6085,
	},
})

type ps6085Table struct {
	object       *types.Var
	element      types.Type
	basic        *types.Basic
	dimensions   []int64
	originalByte int64
}

type ps6085Candidate struct {
	adjustment *ast.BinaryExpr
	states     int64
	laneCount  int64
	stateWrite token.Pos
}

type ps6085LookupUse struct {
	element   *ast.IndexExpr
	rowIndex  ast.Expr
	rowObject types.Object
}

type ps6085Write struct {
	statement ast.Stmt
	block     *ast.BlockStmt
	rhs       ast.Expr
	define    bool
	inIf      *ast.IfStmt
}

func runPS6085(pass *analysis.Pass) (any, error) {
	sets := config.Current()
	return runPS6085WithContracts(pass, sets.StateExpandedLookupContracts)
}

func runPS6085WithContracts(pass *analysis.Pass, configured []config.StateExpandedLookupContract) (any, error) {
	contracts := ps6085Contracts(configured)
	if len(contracts) == 0 {
		return nil, nil
	}
	owners := make(map[string]*ast.FuncDecl)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if object != nil {
				owners[ps6081ObjectID(object)] = function
			}
		}
	}
	packageParents := ps6085ParentMap(pass.Files)
	for _, contract := range contracts {
		owner := owners[contract.OwnerSite]
		if owner == nil || !ps6085OwnerKind(pass, owner, contract.OwnerKind) {
			continue
		}
		table, ok := ps6085ResolveTable(pass, contract.TableObject)
		if !ok {
			continue
		}
		aliases := ps6085TableAliases(pass, table.object)
		if !ps6085TableSafe(pass, packageParents, owner, table.object, aliases) {
			continue
		}
		candidate, ok := ps6085MatchOwner(pass, owner, table, contract.MaxStateCardinality)
		if !ok || candidate.states <= 0 || table.originalByte > contract.MaxExpandedBytes/candidate.states {
			continue
		}
		expanded := table.originalByte * candidate.states
		if expanded > contract.MaxExpandedBytes || expanded > config.MaxStateExpandedLookupBytes ||
			table.originalByte > math.MaxInt64-expanded {
			continue
		}
		combined := table.originalByte + expanded
		related := []analysis.RelatedInformation{{
			Pos: table.object.Pos(), End: table.object.Pos() + token.Pos(len(table.object.Name())),
			Message: "configured immutable base lookup is declared here",
		}}
		if candidate.stateWrite.IsValid() {
			related = append(related, analysis.RelatedInformation{
				Pos: candidate.stateWrite, Message: "the second exact affine state is selected here",
			})
		}
		pass.Report(analysis.Diagnostic{
			Pos: candidate.adjustment.Pos(),
			End: candidate.adjustment.End(),
			Message: contract.Name + ": configured hot loop adds one of " + strconv.FormatInt(candidate.states, 10) +
				" exact " + table.basic.Name() + " states across every " + strconv.FormatInt(candidate.laneCount, 10) +
				"-lane row of immutable " + contract.TableObject + "; a state-expanded lookup grows the " +
				strconv.FormatInt(table.originalByte, 10) + "-byte base to " + strconv.FormatInt(expanded, 10) +
				" bytes (" + strconv.FormatInt(combined, 10) + " bytes combined while the base remains live) within the configured " +
				strconv.FormatInt(contract.MaxExpandedBytes, 10) +
				"-byte budget—preserve element-type rounding and initialization order, review cache footprint, and require exact-output plus paired benchmarks (PS6085 advisory, no automatic fix; no universal win)",
			Related: related,
		})
	}
	return nil, nil
}

func ps6085Contracts(configured []config.StateExpandedLookupContract) []*config.StateExpandedLookupContract {
	names := make(map[string]int)
	claims := make(map[string]int)
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() {
			names[contract.Name]++
			claims[ps6085ContractKey(contract)]++
		}
	}
	result := make([]*config.StateExpandedLookupContract, 0, len(configured))
	for index := range configured {
		contract := &configured[index]
		if contract.Valid() && names[contract.Name] == 1 && claims[ps6085ContractKey(contract)] == 1 {
			result = append(result, contract)
		}
	}
	slices.SortFunc(result, func(left, right *config.StateExpandedLookupContract) int {
		return strings.Compare(left.Name, right.Name)
	})
	return result
}

func ps6085ContractKey(contract *config.StateExpandedLookupContract) string {
	return contract.OwnerSite + "\x00" + string(contract.OwnerKind) + "\x00" + contract.TableObject
}

func ps6085OwnerKind(pass *analysis.Pass, owner *ast.FuncDecl, kind config.StateExpandedLookupCallableKind) bool {
	function, _ := pass.TypesInfo.Defs[owner.Name].(*types.Func)
	signature, _ := function.Type().(*types.Signature)
	if signature == nil {
		return false
	}
	if kind == config.StateExpandedLookupFunction {
		return signature.Recv() == nil
	}
	return kind == config.StateExpandedLookupMethod && signature.Recv() != nil
}

func ps6085ResolveTable(pass *analysis.Pass, id string) (ps6085Table, bool) {
	prefix := pass.Pkg.Path() + "."
	if !strings.HasPrefix(id, prefix) {
		return ps6085Table{}, false
	}
	name := strings.TrimPrefix(id, prefix)
	if name == "" || strings.IndexByte(name, '.') >= 0 {
		return ps6085Table{}, false
	}
	object, _ := pass.Pkg.Scope().Lookup(name).(*types.Var)
	if object == nil {
		return ps6085Table{}, false
	}
	dimensions, element, basic, ok := ps6085ArrayShape(object.Type())
	if !ok || object.Exported() || len(dimensions) != 2 || dimensions[len(dimensions)-1] <= 0 {
		return ps6085Table{}, false
	}
	bytesPerElement := int64(0)
	switch basic.Kind() {
	case types.Float32:
		bytesPerElement = 4
	case types.Float64:
		bytesPerElement = 8
	default:
		return ps6085Table{}, false
	}
	entries := int64(1)
	for _, dimension := range dimensions {
		if dimension <= 0 || entries > (1<<62)/dimension {
			return ps6085Table{}, false
		}
		entries *= dimension
	}
	if entries > (1<<62)/bytesPerElement {
		return ps6085Table{}, false
	}
	return ps6085Table{object: object, element: element, basic: basic, dimensions: dimensions, originalByte: entries * bytesPerElement}, true
}

func ps6085ArrayShape(value types.Type) ([]int64, types.Type, *types.Basic, bool) {
	var dimensions []int64
	for {
		element := types.Unalias(value)
		container := element
		if named, ok := container.(*types.Named); ok {
			container = named.Underlying()
		}
		array, ok := container.(*types.Array)
		if !ok {
			value = element
			break
		}
		dimensions = append(dimensions, array.Len())
		value = array.Elem()
	}
	element := types.Unalias(value)
	underlying := element
	if named, ok := underlying.(*types.Named); ok {
		underlying = named.Underlying()
	}
	basic, ok := underlying.(*types.Basic)
	return dimensions, element, basic, ok
}

func ps6085ParentMap(files []*ast.File) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	for _, file := range files {
		var stack []ast.Node
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			if len(stack) != 0 {
				parents[node] = stack[len(stack)-1]
			}
			stack = append(stack, node)
			return true
		})
	}
	return parents
}

func ps6085TableAliases(pass *analysis.Pass, table *types.Var) map[types.Object]bool {
	aliases := make(map[types.Object]bool)
	changed := true
	for changed {
		changed = false
		for _, file := range pass.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.AssignStmt:
					if len(value.Lhs) != len(value.Rhs) {
						return true
					}
					for index, right := range value.Rhs {
						if !ps6085MayAlias(pass, right, table, aliases) {
							continue
						}
						left, _ := ps2110Unparen(value.Lhs[index]).(*ast.Ident)
						object := pass.TypesInfo.ObjectOf(left)
						if object != nil && !aliases[object] {
							aliases[object] = true
							changed = true
						}
					}
				case *ast.ValueSpec:
					if len(value.Names) != len(value.Values) {
						return true
					}
					for index, right := range value.Values {
						if !ps6085MayAlias(pass, right, table, aliases) {
							continue
						}
						object := pass.TypesInfo.Defs[value.Names[index]]
						if object != nil && !aliases[object] {
							aliases[object] = true
							changed = true
						}
					}
				}
				return true
			})
		}
	}
	return aliases
}

func ps6085MayAlias(pass *analysis.Pass, expression ast.Expr, table *types.Var, aliases map[types.Object]bool) bool {
	typeOf := pass.TypesInfo.TypeOf(expression)
	if typeOf == nil {
		return false
	}
	underlying := types.Unalias(typeOf).Underlying()
	switch underlying.(type) {
	case *types.Pointer, *types.Slice:
	default:
		return false
	}
	root := ps6085RootObject(pass, expression)
	return root == table || aliases[root]
}

func ps6085RootObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(value)
	case *ast.IndexExpr:
		return ps6085RootObject(pass, value.X)
	case *ast.IndexListExpr:
		return ps6085RootObject(pass, value.X)
	case *ast.SliceExpr:
		return ps6085RootObject(pass, value.X)
	case *ast.SelectorExpr:
		return ps6085RootObject(pass, value.X)
	case *ast.StarExpr:
		return ps6085RootObject(pass, value.X)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6085RootObject(pass, value.X)
		}
	}
	return nil
}

func ps6085TableSafe(pass *analysis.Pass, parents map[ast.Node]ast.Node, owner *ast.FuncDecl, table *types.Var, aliases map[types.Object]bool) bool {
	writers := make(map[*ast.FuncDecl]bool)
	for _, file := range pass.Files {
		safe := true
		ast.Inspect(file, func(node ast.Node) bool {
			if !safe {
				return false
			}
			switch value := node.(type) {
			case *ast.Ident:
				object := pass.TypesInfo.ObjectOf(value)
				if object == table && ps6085InsideFuncLit(parents, value) {
					safe = false
					return false
				}
				if aliases[object] && (ps6085InsideFuncLit(parents, value) || !ps6085AliasUseSafe(pass, parents, value, aliases)) {
					safe = false
					return false
				}
			case *ast.CompositeLit:
				if ps6085CompositeContainsAlias(pass, value, table, aliases) {
					safe = false
					return false
				}
			case *ast.SliceExpr:
				if ps6085MayAlias(pass, value, table, aliases) {
					safe = false
					return false
				}
			case *ast.UnaryExpr:
				if value.Op == token.AND && ps6085RootObject(pass, value.X) == table {
					if _, row := ps2110Unparen(value.X).(*ast.IndexExpr); !row {
						safe = false
						return false
					}
				}
			case *ast.SelectorExpr:
				root := ps6085RootObject(pass, value.X)
				if pass.TypesInfo.Selections[value] != nil && (root == table || aliases[root]) {
					safe = false
					return false
				}
			case *ast.AssignStmt:
				for _, left := range value.Lhs {
					if ps6085WritesTable(pass, left, table, aliases) {
						writer := ps6085InitFunction(parents, value)
						if writer == nil {
							safe = false
							return false
						}
						writers[writer] = true
					}
				}
				for index, right := range value.Rhs {
					if !ps6085MayAlias(pass, right, table, aliases) {
						continue
					}
					if len(value.Lhs) != len(value.Rhs) || index >= len(value.Lhs) || !ps6085LocalAliasTarget(pass, value.Lhs[index], aliases) {
						safe = false
						return false
					}
				}
			case *ast.ValueSpec:
				for index, right := range value.Values {
					if !ps6085MayAlias(pass, right, table, aliases) {
						continue
					}
					if len(value.Names) != len(value.Values) || index >= len(value.Names) || !ps6085LocalAliasObject(pass, pass.TypesInfo.Defs[value.Names[index]], aliases) {
						safe = false
						return false
					}
				}
			case *ast.IncDecStmt:
				if ps6085WritesTable(pass, value.X, table, aliases) {
					writer := ps6085InitFunction(parents, value)
					if writer == nil {
						safe = false
						return false
					}
					writers[writer] = true
				}
			case *ast.CallExpr:
				if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok {
					root := ps6085RootObject(pass, selector.X)
					if root == table || aliases[root] {
						safe = false
						return false
					}
				}
				for _, argument := range value.Args {
					if ps6085MayAlias(pass, argument, table, aliases) && !ps6085SafeBuiltin(pass, value) {
						safe = false
						return false
					}
				}
			case *ast.ReturnStmt:
				for _, result := range value.Results {
					if ps6085MayAlias(pass, result, table, aliases) {
						safe = false
						return false
					}
				}
			case *ast.SendStmt:
				if ps6085MayAlias(pass, value.Value, table, aliases) {
					safe = false
					return false
				}
			}
			return true
		})
		if !safe {
			return false
		}
	}
	if len(writers) > 1 {
		return false
	}
	return len(writers) == 0 || ps6085InitOrderSafe(pass, parents, owner, table)
}

func ps6085InitOrderSafe(pass *analysis.Pass, parents map[ast.Node]ast.Node, owner *ast.FuncDecl, table *types.Var) bool {
	ownerObject, _ := pass.TypesInfo.Defs[owner.Name].(*types.Func)
	functions := make(map[*types.Func]*ast.FuncDecl)
	var initializers []*ast.FuncDecl
	var packageInitializers []ast.Expr
	for _, file := range pass.Files {
		safe := true
		ast.Inspect(file, func(node ast.Node) bool {
			if !safe {
				return false
			}
			switch value := node.(type) {
			case *ast.Ident:
				if pass.TypesInfo.Uses[value] == table && ps6085InPackageInitializer(parents, value) {
					safe = false
					return false
				}
			}
			return true
		})
		if !safe {
			return false
		}
		for _, declaration := range file.Decls {
			if general, ok := declaration.(*ast.GenDecl); ok {
				for _, specification := range general.Specs {
					if values, ok := specification.(*ast.ValueSpec); ok {
						packageInitializers = append(packageInitializers, values.Values...)
					}
				}
			}
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			object, _ := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if object != nil {
				functions[object] = function
			}
			if function.Recv == nil && function.Name.Name == "init" {
				initializers = append(initializers, function)
			}
		}
	}
	if ownerObject == nil {
		return false
	}
	for _, initializer := range packageInitializers {
		if ps6085NodeReachesOwner(pass, initializer, ownerObject, functions) {
			return false
		}
	}
	for _, initializer := range initializers {
		if ps6085NodeReachesOwner(pass, initializer.Body, ownerObject, functions) {
			return false
		}
	}
	return true
}

func ps6085NodeReachesOwner(pass *analysis.Pass, root ast.Node, owner *types.Func, functions map[*types.Func]*ast.FuncDecl) bool {
	visiting := make(map[*types.Func]bool)
	var reachesNode func(ast.Node) bool
	reachesFunction := func(function *types.Func) bool {
		if function == owner {
			return true
		}
		declaration := functions[function]
		if declaration == nil || visiting[function] {
			return false
		}
		visiting[function] = true
		reaches := reachesNode(declaration.Body)
		visiting[function] = false
		return reaches
	}
	reachesNode = func(root ast.Node) bool {
		reaches := false
		ast.Inspect(root, func(node ast.Node) bool {
			if reaches {
				return false
			}
			if identifier, ok := node.(*ast.Ident); ok && pass.TypesInfo.Uses[identifier] == owner {
				reaches = true
				return false
			}
			literal, nested := node.(*ast.FuncLit)
			if nested {
				return literal == root
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || pass.TypesInfo.Types[call.Fun].IsType() {
				return true
			}
			if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok && reachesNode(literal.Body) {
				reaches = true
				return false
			}
			for _, argument := range call.Args {
				argumentType := pass.TypesInfo.TypeOf(argument)
				if argumentType == nil {
					continue
				}
				if _, callable := types.Unalias(argumentType).Underlying().(*types.Signature); !callable {
					continue
				}
				if literal, ok := ps2110Unparen(argument).(*ast.FuncLit); ok {
					if reachesNode(literal.Body) {
						reaches = true
						return false
					}
					continue
				}
				function, _ := ps6081ObjectExpr(pass, argument).(*types.Func)
				if function == nil || function.Pkg() != pass.Pkg || functions[function] == nil || reachesFunction(function) {
					reaches = true
					return false
				}
			}
			object := ps6081ObjectExpr(pass, call.Fun)
			switch function := object.(type) {
			case *types.Builtin:
				return true
			case *types.Func:
				if reachesFunction(function) {
					reaches = true
					return false
				}
				if function.Pkg() == pass.Pkg && functions[function] == nil {
					reaches = true
					return false
				}
				return true
			default:
				if _, callable := types.Unalias(pass.TypesInfo.TypeOf(call.Fun)).Underlying().(*types.Signature); callable {
					reaches = true
					return false
				}
				return true
			}
		})
		return reaches
	}
	return reachesNode(root)
}

func ps6085InPackageInitializer(parents map[ast.Node]ast.Node, node ast.Node) bool {
	seenValue := false
	for current := parents[node]; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.ValueSpec:
			seenValue = true
		case *ast.FuncDecl, *ast.FuncLit:
			return false
		case *ast.File:
			return seenValue
		}
	}
	return false
}

func ps6085InsideFuncLit(parents map[ast.Node]ast.Node, node ast.Node) bool {
	for current := parents[node]; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.FuncLit:
			return true
		case *ast.FuncDecl, *ast.File:
			return false
		}
	}
	return false
}

func ps6085AliasUseSafe(pass *analysis.Pass, parents map[ast.Node]ast.Node, identifier *ast.Ident, aliases map[types.Object]bool) bool {
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil || !aliases[object] {
		return false
	}
	if pass.TypesInfo.Defs[identifier] == object {
		return true
	}
	current := ast.Node(identifier)
	for {
		parent := parents[current]
		switch value := parent.(type) {
		case *ast.ParenExpr:
			current = value
			continue
		case *ast.StarExpr:
			current = value
			continue
		case *ast.IndexExpr:
			return ps6085Contains(value.X, current)
		case *ast.RangeStmt:
			return ps6085Contains(value.X, current)
		case *ast.CallExpr:
			return ps6085SafeBuiltin(pass, value)
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if ps2110Unparen(left) == identifier {
					return true
				}
			}
			if len(value.Lhs) != len(value.Rhs) {
				return false
			}
			for index, right := range value.Rhs {
				if ps2110Unparen(right) != identifier {
					continue
				}
				left, _ := ps2110Unparen(value.Lhs[index]).(*ast.Ident)
				target := pass.TypesInfo.ObjectOf(left)
				return target != nil && aliases[target] && ps6085DirectAliasType(target.Type())
			}
		case *ast.ValueSpec:
			for index, right := range value.Values {
				if ps2110Unparen(right) != identifier || index >= len(value.Names) {
					continue
				}
				target := pass.TypesInfo.Defs[value.Names[index]]
				return target != nil && aliases[target] && ps6085DirectAliasType(target.Type())
			}
		}
		return false
	}
}

func ps6085CompositeContainsAlias(pass *analysis.Pass, literal *ast.CompositeLit, table *types.Var, aliases map[types.Object]bool) bool {
	for _, element := range literal.Elts {
		expression := element
		if keyed, ok := expression.(*ast.KeyValueExpr); ok {
			expression = keyed.Value
		}
		if expression != nil && ps6085MayAlias(pass, expression, table, aliases) {
			return true
		}
	}
	return false
}

func ps6085DirectAliasType(value types.Type) bool {
	switch types.Unalias(value).Underlying().(type) {
	case *types.Pointer, *types.Slice:
		return true
	default:
		return false
	}
}

func ps6085WritesTable(pass *analysis.Pass, expression ast.Expr, table *types.Var, aliases map[types.Object]bool) bool {
	root := ps6085RootObject(pass, expression)
	if root == table {
		return true
	}
	if !aliases[root] {
		return false
	}
	identifier, direct := ps2110Unparen(expression).(*ast.Ident)
	return !direct || pass.TypesInfo.ObjectOf(identifier) != root
}

func ps6085InitFunction(parents map[ast.Node]ast.Node, node ast.Node) *ast.FuncDecl {
	for current := node; current != nil; current = parents[current] {
		switch value := current.(type) {
		case *ast.FuncLit:
			return nil
		case *ast.FuncDecl:
			if value.Recv == nil && value.Name.Name == "init" {
				return value
			}
			return nil
		}
	}
	return nil
}

func ps6085LocalAliasTarget(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]bool) bool {
	identifier, _ := ps2110Unparen(expression).(*ast.Ident)
	return identifier != nil && ps6085LocalAliasObject(pass, pass.TypesInfo.ObjectOf(identifier), aliases)
}

func ps6085LocalAliasObject(pass *analysis.Pass, object types.Object, aliases map[types.Object]bool) bool {
	return object != nil && aliases[object] && object.Parent() != pass.Pkg.Scope() && ps6085DirectAliasType(object.Type())
}

func ps6085SafeBuiltin(pass *analysis.Pass, call *ast.CallExpr) bool {
	identifier, _ := ps2110Unparen(call.Fun).(*ast.Ident)
	builtin, _ := pass.TypesInfo.Uses[identifier].(*types.Builtin)
	return builtin != nil && (builtin.Name() == "len" || builtin.Name() == "cap")
}

func ps6085MatchOwner(pass *analysis.Pass, owner *ast.FuncDecl, table ps6085Table, maxStates int) (ps6085Candidate, bool) {
	parents := ps6085OwnerParents(owner.Body)
	unreachable := ps2144Unreachable(pass, owner.Body)
	var match ps6085Candidate
	matches := 0
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		adjustment, ok := node.(*ast.BinaryExpr)
		if !ok || adjustment.Op != token.ADD || ps2144PositionIn(adjustment.Pos(), unreachable) {
			return true
		}
		lookup, state, ok := ps6085LookupAndState(pass, owner, adjustment, table)
		if !ok {
			return true
		}
		scale, ok := ps6085MultipliedInElementType(pass, parents, adjustment, table.element)
		if !ok {
			return true
		}
		laneCount, laneLoop, ok := ps6085CompleteLaneLoop(pass, owner, parents, adjustment, lookup, scale, table.dimensions[len(table.dimensions)-1])
		if !ok {
			return true
		}
		states, stateWrite, ok := ps6085TwoStateSelection(pass, owner, parents, state, adjustment, laneLoop, table, maxStates)
		if !ok {
			return true
		}
		matches++
		if matches == 1 {
			match = ps6085Candidate{adjustment: adjustment, states: states, laneCount: laneCount, stateWrite: stateWrite}
		}
		return true
	})
	return match, matches == 1
}

func ps6085OwnerParents(body *ast.BlockStmt) map[ast.Node]ast.Node {
	parents := make(map[ast.Node]ast.Node)
	var stack []ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			parents[node] = stack[len(stack)-1]
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		stack = append(stack, node)
		return true
	})
	return parents
}

func ps6085LookupAndState(pass *analysis.Pass, owner *ast.FuncDecl, adjustment *ast.BinaryExpr, table ps6085Table) (ps6085LookupUse, types.Object, bool) {
	for _, pair := range [][2]ast.Expr{{adjustment.X, adjustment.Y}, {adjustment.Y, adjustment.X}} {
		lookup, ok := ps6085Lookup(pass, owner, pair[0], table)
		if !ok {
			continue
		}
		identifier, _ := ps2110Unparen(pair[1]).(*ast.Ident)
		state := pass.TypesInfo.ObjectOf(identifier)
		variable, local := state.(*types.Var)
		if identifier == nil || !local || variable.IsField() || !types.Identical(pass.TypesInfo.TypeOf(adjustment), table.element) ||
			!types.Identical(variable.Type(), table.element) {
			continue
		}
		return lookup, state, true
	}
	return ps6085LookupUse{}, nil, false
}

func ps6085Lookup(pass *analysis.Pass, owner *ast.FuncDecl, expression ast.Expr, table ps6085Table) (ps6085LookupUse, bool) {
	lookup, ok := ps2110Unparen(expression).(*ast.IndexExpr)
	if !ok || !types.Identical(pass.TypesInfo.TypeOf(lookup), table.element) {
		return ps6085LookupUse{}, false
	}
	if !ps6085RowTypeMatches(pass.TypesInfo.TypeOf(lookup.X), table) {
		return ps6085LookupUse{}, false
	}
	rowIndex, rowObject, ok := ps6085ResolveRowIndex(pass, owner, lookup.X, table, lookup.Pos())
	if !ok {
		return ps6085LookupUse{}, false
	}
	return ps6085LookupUse{element: lookup, rowIndex: rowIndex, rowObject: rowObject}, true
}

func ps6085ResolveRowIndex(pass *analysis.Pass, owner *ast.FuncDecl, expression ast.Expr, table ps6085Table, before token.Pos) (ast.Expr, types.Object, bool) {
	value := ps2110Unparen(expression)
	if star, ok := value.(*ast.StarExpr); ok {
		value = ps2110Unparen(star.X)
	}
	if row, ok := value.(*ast.IndexExpr); ok {
		return row.Index, nil, ps6085RootObject(pass, row.X) == table.object
	}
	identifier, ok := value.(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil {
		return nil, nil, false
	}
	rhs, ok := ps6085SingleDefinition(pass, owner, object)
	if !ok || rhs.Pos() >= before {
		return nil, nil, false
	}
	value = ps2110Unparen(rhs)
	if address, ok := value.(*ast.UnaryExpr); ok && address.Op == token.AND {
		value = ps2110Unparen(address.X)
	}
	if row, ok := value.(*ast.IndexExpr); ok {
		return row.Index, object, ps6085RootObject(pass, row.X) == table.object
	}
	return nil, nil, false
}

func ps6085RowTypeMatches(value types.Type, table ps6085Table) bool {
	value = types.Unalias(value)
	if pointer, ok := value.(*types.Pointer); ok {
		value = types.Unalias(pointer.Elem())
	}
	if named, ok := value.(*types.Named); ok {
		value = named.Underlying()
	}
	row, ok := value.(*types.Array)
	return ok && row.Len() == table.dimensions[1] && types.Identical(types.Unalias(row.Elem()), table.element)
}

func ps6085MultipliedInElementType(pass *analysis.Pass, parents map[ast.Node]ast.Node, adjustment *ast.BinaryExpr, element types.Type) (ast.Expr, bool) {
	current := ast.Node(adjustment)
	for {
		parent := parents[current]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			current = paren
			continue
		}
		multiply, ok := parent.(*ast.BinaryExpr)
		if !ok || multiply.Op != token.MUL || !types.Identical(pass.TypesInfo.TypeOf(multiply), element) {
			return nil, false
		}
		other := multiply.X
		if ps6085Contains(other, adjustment) {
			other = multiply.Y
		} else if !ps6085Contains(multiply.Y, adjustment) {
			return nil, false
		}
		return other, types.Identical(pass.TypesInfo.TypeOf(other), element) && ps6085SideEffectFree(other)
	}
}

func ps6085Contains(root ast.Node, target ast.Node) bool {
	found := false
	ast.Inspect(root, func(node ast.Node) bool {
		if node == target {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6085SideEffectFree(expression ast.Expr) bool {
	safe := true
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.CallExpr:
			safe = false
			return false
		case *ast.UnaryExpr:
			if value.Op == token.ARROW {
				safe = false
				return false
			}
		}
		return safe
	})
	return safe
}

func ps6085CompleteLaneLoop(pass *analysis.Pass, owner *ast.FuncDecl, parents map[ast.Node]ast.Node, adjustment ast.Node, lookup ps6085LookupUse, scale ast.Expr, want int64) (int64, ast.Node, bool) {
	indexIdentifier, _ := ps2110Unparen(lookup.element.Index).(*ast.Ident)
	indexObject := pass.TypesInfo.ObjectOf(indexIdentifier)
	if indexObject == nil {
		return 0, nil, false
	}
	for current := adjustment; current != nil && current != owner; current = parents[current] {
		switch loop := current.(type) {
		case *ast.RangeStmt:
			index, extent, ok := ps6085RangeLoop(pass, owner, loop)
			if ok && index == indexObject {
				if extent != want || !ps6085LaneLoopSafe(parents, adjustment, loop, loop.Body) ||
					!ps6085Invariant(pass, lookup.rowIndex, indexObject) || !ps6085Invariant(pass, scale, indexObject) ||
					!ps6085RowObjectReadOnly(pass, owner, lookup, loop) ||
					!ps6085LoopValuesStable(pass, owner, loop.Body, indexObject, lookup.rowIndex, scale) {
					return 0, nil, false
				}
				return extent, loop, true
			}
		case *ast.ForStmt:
			index, extent, ok := ps6085ForLoop(pass, loop)
			if ok && index == indexObject {
				if extent != want || !ps6085LaneLoopSafe(parents, adjustment, loop, loop.Body) ||
					!ps6085Invariant(pass, lookup.rowIndex, indexObject) || !ps6085Invariant(pass, scale, indexObject) ||
					!ps6085RowObjectReadOnly(pass, owner, lookup, loop) ||
					!ps6085LoopValuesStable(pass, owner, loop.Body, indexObject, lookup.rowIndex, scale) {
					return 0, nil, false
				}
				return extent, loop, true
			}
		}
	}
	return 0, nil, false
}

func ps6085Invariant(pass *analysis.Pass, expression ast.Expr, lane types.Object) bool {
	if expression == nil || !ps6085SideEffectFree(expression) {
		return false
	}
	invariant := true
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == lane {
			invariant = false
			return false
		}
		return invariant
	})
	return invariant
}

func ps6085LaneLoopSafe(parents map[ast.Node]ast.Node, adjustment ast.Node, loop ast.Node, body *ast.BlockStmt) bool {
	for current := adjustment; current != nil && current != body; current = parents[current] {
		switch current.(type) {
		case *ast.IfStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return false
		case *ast.ForStmt, *ast.RangeStmt:
			if current != loop {
				return false
			}
		}
	}
	safe := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch node.(type) {
		case *ast.BranchStmt, *ast.ReturnStmt:
			safe = false
			return false
		}
		return true
	})
	return safe
}

func ps6085RowObjectReadOnly(pass *analysis.Pass, owner *ast.FuncDecl, lookup ps6085LookupUse, laneLoop ast.Node) bool {
	if lookup.rowObject == nil {
		return true
	}
	safe := true
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if !safe {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(identifier) != lookup.rowObject {
			return true
		}
		if pass.TypesInfo.Defs[identifier] == lookup.rowObject || ps6085Contains(lookup.element.X, identifier) {
			return true
		}
		if loop, ok := laneLoop.(*ast.RangeStmt); ok && ps6085Contains(loop.X, identifier) {
			return true
		}
		safe = false
		return false
	})
	return safe
}

func ps6085LoopValuesStable(pass *analysis.Pass, owner *ast.FuncDecl, body *ast.BlockStmt, lane types.Object, expressions ...ast.Expr) bool {
	protected := map[types.Object]bool{lane: true}
	for _, expression := range expressions {
		valid := true
		ast.Inspect(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			object := pass.TypesInfo.ObjectOf(identifier)
			switch value := object.(type) {
			case nil, *types.Const, *types.Builtin, *types.PkgName, *types.TypeName:
				return true
			case *types.Var:
				if value.IsField() || value.Parent() == pass.Pkg.Scope() || !ps6085StableScalar(value.Type()) {
					valid = false
					return false
				}
				protected[value] = true
				return true
			default:
				valid = false
				return false
			}
		})
		if !valid {
			return false
		}
	}
	ownerSafe := true
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if !ownerSafe {
			return false
		}
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.AND && protected[ps6085RootObject(pass, value.X)] {
				ownerSafe = false
				return false
			}
		case *ast.SelectorExpr:
			if pass.TypesInfo.Selections[value] != nil && protected[ps6085RootObject(pass, value.X)] {
				ownerSafe = false
				return false
			}
		case *ast.FuncLit:
			if ps6085ContainsProtected(pass, value, protected) {
				ownerSafe = false
			}
			return false
		}
		return true
	})
	if !ownerSafe {
		return false
	}
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				if protected[ps6085RootObject(pass, left)] {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			if protected[ps6085RootObject(pass, value.X)] {
				stable = false
				return false
			}
		case *ast.RangeStmt:
			if protected[ps6085RootObject(pass, value.Key)] || protected[ps6085RootObject(pass, value.Value)] {
				stable = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6085ContainsProtected(pass, value.X, protected) {
				stable = false
				return false
			}
		case *ast.CallExpr:
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok && protected[ps6085RootObject(pass, selector.X)] {
				stable = false
				return false
			}
		case *ast.FuncLit:
			if ps6085ContainsProtected(pass, value.Body, protected) {
				stable = false
			}
			return false
		}
		return stable
	})
	return stable
}

func ps6085StableScalar(value types.Type) bool {
	_, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok
}

func ps6085ContainsProtected(pass *analysis.Pass, node ast.Node, protected map[types.Object]bool) bool {
	found := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && protected[pass.TypesInfo.ObjectOf(identifier)] {
			found = true
			return false
		}
		return !found
	})
	return found
}

func ps6085RangeLoop(pass *analysis.Pass, owner *ast.FuncDecl, loop *ast.RangeStmt) (types.Object, int64, bool) {
	identifier, _ := ps2110Unparen(loop.Key).(*ast.Ident)
	index := pass.TypesInfo.Defs[identifier]
	if identifier == nil || identifier.Name == "_" || index == nil || loop.Value != nil {
		return nil, 0, false
	}
	extent, ok := ps6085RangeExtent(pass, owner, loop.X)
	return index, extent, ok && extent > 0
}

func ps6085RangeExtent(pass *analysis.Pass, owner *ast.FuncDecl, expression ast.Expr) (int64, bool) {
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Int {
		extent, ok := constant.Int64Val(value)
		return extent, ok && extent > 0
	}
	typeOf := types.Unalias(pass.TypesInfo.TypeOf(expression))
	if pointer, ok := typeOf.(*types.Pointer); ok {
		typeOf = types.Unalias(pointer.Elem())
	}
	if named, ok := typeOf.(*types.Named); ok {
		typeOf = named.Underlying()
	}
	if array, ok := typeOf.(*types.Array); ok {
		return array.Len(), array.Len() > 0
	}
	identifier, _ := ps2110Unparen(expression).(*ast.Ident)
	object := pass.TypesInfo.ObjectOf(identifier)
	if object == nil {
		return 0, false
	}
	rhs, ok := ps6085SingleDefinition(pass, owner, object)
	if !ok {
		return 0, false
	}
	slice, ok := ps2110Unparen(rhs).(*ast.SliceExpr)
	if !ok || slice.Slice3 || slice.Low == nil || slice.High == nil {
		return 0, false
	}
	return ps6085SliceWidth(pass, slice.Low, slice.High)
}

func ps6085SliceWidth(pass *analysis.Pass, low, high ast.Expr) (int64, bool) {
	binary, ok := ps2110Unparen(high).(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		return 0, false
	}
	for _, pair := range [][2]ast.Expr{{binary.X, binary.Y}, {binary.Y, binary.X}} {
		if !ps6085SameExpression(pass, low, pair[0]) {
			continue
		}
		value := pass.TypesInfo.Types[pair[1]].Value
		if value == nil || value.Kind() != constant.Int {
			continue
		}
		width, exact := constant.Int64Val(value)
		return width, exact && width > 0
	}
	return 0, false
}

func ps6085SameExpression(pass *analysis.Pass, left, right ast.Expr) bool {
	leftID, leftOK := ps2110Unparen(left).(*ast.Ident)
	rightID, rightOK := ps2110Unparen(right).(*ast.Ident)
	return leftOK && rightOK && pass.TypesInfo.ObjectOf(leftID) != nil && pass.TypesInfo.ObjectOf(leftID) == pass.TypesInfo.ObjectOf(rightID)
}

func ps6085SingleDefinition(pass *analysis.Pass, owner *ast.FuncDecl, object types.Object) (ast.Expr, bool) {
	var result ast.Expr
	count := 0
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				return true
			}
			for index, left := range value.Lhs {
				identifier, _ := ps2110Unparen(left).(*ast.Ident)
				if pass.TypesInfo.ObjectOf(identifier) == object {
					count++
					result = value.Rhs[index]
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) != len(value.Values) {
				return true
			}
			for index, name := range value.Names {
				if pass.TypesInfo.Defs[name] == object {
					count++
					result = value.Values[index]
				}
			}
		case *ast.IncDecStmt:
			if ps6085RootObject(pass, value.X) == object {
				count++
			}
		}
		return true
	})
	return result, count == 1 && result != nil
}

func ps6085ForLoop(pass *analysis.Pass, loop *ast.ForStmt) (types.Object, int64, bool) {
	initial, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initial.Tok != token.DEFINE || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 || !ps6085IntegerConstant(pass, initial.Rhs[0], 0) {
		return nil, 0, false
	}
	identifier, _ := ps2110Unparen(initial.Lhs[0]).(*ast.Ident)
	index := pass.TypesInfo.Defs[identifier]
	condition, ok := loop.Cond.(*ast.BinaryExpr)
	if identifier == nil || index == nil || !ok || condition.Op != token.LSS || ps6085RootObject(pass, condition.X) != index {
		return nil, 0, false
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC || ps6085RootObject(pass, post.X) != index {
		return nil, 0, false
	}
	value := pass.TypesInfo.Types[condition.Y].Value
	if value == nil || value.Kind() != constant.Int {
		return nil, 0, false
	}
	extent, exact := constant.Int64Val(value)
	return index, extent, exact && extent > 0
}

func ps6085IntegerConstant(pass *analysis.Pass, expression ast.Expr, want int64) bool {
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	got, exact := constant.Int64Val(value)
	return exact && got == want
}

func ps6085TwoStateSelection(pass *analysis.Pass, owner *ast.FuncDecl, parents map[ast.Node]ast.Node, state types.Object, use ast.Node, laneLoop ast.Node, table ps6085Table, maxStates int) (int64, token.Pos, bool) {
	if maxStates < 2 || ps6085HasGoto(owner.Body) {
		return 0, token.NoPos, false
	}
	writes := ps6085StateWrites(pass, owner, parents, state)
	if len(writes) != 2 || !writes[0].define || writes[1].define || writes[1].inIf == nil || writes[1].inIf.Else != nil ||
		len(writes[1].inIf.Body.List) != 1 || writes[1].inIf.Body.List[0] != writes[1].statement ||
		pass.TypesInfo.Types[writes[1].inIf.Cond].Value != nil || writes[0].block == nil {
		return 0, token.NoPos, false
	}
	if parent := parents[writes[1].inIf]; parent != writes[0].block {
		return 0, token.NoPos, false
	}
	definitionIndex := ps6085StatementIndex(writes[0].block, writes[0].statement)
	ifIndex := ps6085StatementIndex(writes[0].block, writes[1].inIf)
	useStatement := ps6085DirectStatement(parents, use, writes[0].block)
	useIndex := ps6085StatementIndex(writes[0].block, useStatement)
	if definitionIndex < 0 || ifIndex <= definitionIndex || useIndex <= ifIndex {
		return 0, token.NoPos, false
	}
	if ps6085Contains(laneLoop, writes[0].statement) || ps6085Contains(laneLoop, writes[1].statement) ||
		!ps6085StateConfined(pass, owner, state, use, writes) {
		return 0, token.NoPos, false
	}
	left, leftOK := ps6085AssignedConstant(pass, writes[0].rhs, table)
	right, rightOK := ps6085AssignedConstant(pass, writes[1].rhs, table)
	if !leftOK || !rightOK || left == right {
		return 0, token.NoPos, false
	}
	return 2, writes[1].statement.Pos(), true
}

func ps6085StateConfined(pass *analysis.Pass, owner *ast.FuncDecl, state types.Object, use ast.Node, writes []ps6085Write) bool {
	allowed := make(map[*ast.Ident]bool)
	for _, root := range []ast.Node{use, writes[0].statement, writes[1].statement} {
		ast.Inspect(root, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && pass.TypesInfo.ObjectOf(identifier) == state {
				allowed[identifier] = true
			}
			return true
		})
	}
	confined := true
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if !confined {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.ObjectOf(identifier) == state && !allowed[identifier] {
			confined = false
			return false
		}
		return true
	})
	return confined
}

func ps6085StateWrites(pass *analysis.Pass, owner *ast.FuncDecl, parents map[ast.Node]ast.Node, state types.Object) []ps6085Write {
	var writes []ps6085Write
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				for _, left := range value.Lhs {
					if ps6085RootObject(pass, left) == state {
						writes = append(writes, ps6085Write{})
					}
				}
				return true
			}
			for index, left := range value.Lhs {
				identifier, _ := ps2110Unparen(left).(*ast.Ident)
				if pass.TypesInfo.ObjectOf(identifier) != state {
					continue
				}
				statement := ast.Stmt(value)
				writes = append(writes, ps6085Write{
					statement: statement,
					block:     ps6085EnclosingBlock(parents, value),
					rhs:       value.Rhs[index],
					define:    value.Tok == token.DEFINE && pass.TypesInfo.Defs[identifier] == state,
					inIf:      ps6085EnclosingIf(parents, value),
				})
			}
		case *ast.ValueSpec:
			for _, name := range value.Names {
				if pass.TypesInfo.Defs[name] == state {
					writes = append(writes, ps6085Write{})
				}
			}
		case *ast.IncDecStmt:
			if ps6085RootObject(pass, value.X) == state {
				writes = append(writes, ps6085Write{})
			}
		case *ast.RangeStmt:
			if ps6085RootObject(pass, value.Key) == state || ps6085RootObject(pass, value.Value) == state {
				writes = append(writes, ps6085Write{})
			}
		}
		return true
	})
	slices.SortFunc(writes, func(left, right ps6085Write) int {
		if left.statement == nil {
			if right.statement == nil {
				return 0
			}
			return -1
		}
		if right.statement == nil {
			return 1
		}
		if left.statement.Pos() < right.statement.Pos() {
			return -1
		}
		if left.statement.Pos() > right.statement.Pos() {
			return 1
		}
		return 0
	})
	return writes
}

func ps6085EnclosingBlock(parents map[ast.Node]ast.Node, node ast.Node) *ast.BlockStmt {
	for current := parents[node]; current != nil; current = parents[current] {
		if block, ok := current.(*ast.BlockStmt); ok {
			return block
		}
	}
	return nil
}

func ps6085EnclosingIf(parents map[ast.Node]ast.Node, node ast.Node) *ast.IfStmt {
	for current := parents[node]; current != nil; current = parents[current] {
		switch value := current.(type) {
		case *ast.IfStmt:
			return value
		case *ast.FuncDecl, *ast.ForStmt, *ast.RangeStmt:
			return nil
		}
	}
	return nil
}

func ps6085DirectStatement(parents map[ast.Node]ast.Node, node ast.Node, block *ast.BlockStmt) ast.Stmt {
	for current := node; current != nil; current = parents[current] {
		if parents[current] == block {
			statement, _ := current.(ast.Stmt)
			return statement
		}
	}
	return nil
}

func ps6085StatementIndex(block *ast.BlockStmt, statement ast.Stmt) int {
	if block == nil || statement == nil {
		return -1
	}
	for index, candidate := range block.List {
		if candidate == statement {
			return index
		}
	}
	return -1
}

func ps6085AssignedConstant(pass *analysis.Pass, expression ast.Expr, table ps6085Table) (uint64, bool) {
	typed := pass.TypesInfo.Types[expression]
	if typed.Value == nil || typed.Value.Kind() != constant.Int && typed.Value.Kind() != constant.Float {
		return 0, false
	}
	source := typed.Type
	if source == nil || !types.Identical(source, table.element) && !types.AssignableTo(source, table.element) {
		return 0, false
	}
	switch table.basic.Kind() {
	case types.Float32:
		value, _ := constant.Float32Val(typed.Value)
		if math.IsInf(float64(value), 0) {
			return 0, false
		}
		return uint64(math.Float32bits(value)), true
	case types.Float64:
		value, _ := constant.Float64Val(typed.Value)
		if math.IsInf(value, 0) {
			return 0, false
		}
		return math.Float64bits(value), true
	default:
		return 0, false
	}
}

func ps6085HasGoto(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if branch, ok := node.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			found = true
			return false
		}
		return !found
	})
	return found
}
