package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"
	"unicode/utf8"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/lint"
)

var PS6122 = register(&lint.Check{
	ID: "PS6122", Category: "verify", Slug: "constant-simd-shift-codegen",
	Level: lint.LevelAggressive, AutoFix: false,
	Doc: lint.Documentation{Title: "a constant-distance SIMD shift in a repeated path merits target code-generation inspection",
		Text: `Constant-distance archsimd ShiftAllLeft and ShiftAllRight calls may still
lower through a scalar-to-vector broadcast and a variable vector shift. PS6122
recognizes exact typed simd/archsimd methods in reachable loops that may repeat and
their directly called local helpers. It accepts only non-zero distances below
the receiver element width. Zero needs no shift; width and larger have the
API's all-zero or signed-right all-zero/all-one boundary semantics and are not
immediate-shift candidates in this version.

The typed scope is the eight 128-bit ARM64 Int/Uint8x16, 16x8, 32x4 and 64x2
vectors with the SDK's value-receiver, uint64-distance, same-result signatures.
Loop scope is source-proved constant counting loops and range loops whose
cardinality may exceed one. Direct synchronous local helpers are followed once
from a proven repeating call edge. Deferred and asynchronous calls and
uninvoked function literals are excluded.

The Go AST does not prove emitted instructions. Inspect the pinned compiler,
GOEXPERIMENT, GOOS, GOARCH and ISA output, and suppress the finding when an
immediate shift is already emitted, for example with:

//perfscan:ignore PS6122 go1.x target/ISA emits an immediate shift

Signed ShiftAllRight is arithmetic;
unsigned ShiftAllRight is logical. Validate edge vectors and every supported
distance, then benchmark the complete consumer and preallocated leaf. There is
NO automatic fix, compiler-defect conclusion, speedup claim, or source rewrite.`,
		Before: `func exponentBits(k archsimd.Int64x2) archsimd.Uint64x2 {
	return k.Add(archsimd.BroadcastInt64x2(1023)).ShiftAllLeft(52).ToBits()
}`,
		After: `// Keep source unchanged. Inspect pinned target assembly; if it broadcasts
// 52 and uses a variable shift, evaluate a compiler/backend improvement.`,
		MeasuredWin: `Owner issue #965 observed Go 1.27.1 with GOEXPERIMENT=simd on
Darwin ARM64/v8.0 broadcasting 52 before sshl. This is code-generation evidence,
not a measured performance result or proof that every compiler does so.`},
	Analyzer: &analysis.Analyzer{Name: "PS6122", Doc: "constant archsimd ShiftAll calls in repeated paths", Run: runPS6122},
})

var ps6122Types = map[string]uint64{
	"Int8x16": 8, "Uint8x16": 8,
	"Int16x8": 16, "Uint16x8": 16,
	"Int32x4": 32, "Uint32x4": 32,
	"Int64x2": 64, "Uint64x2": 64,
}

type ps6122Flow struct {
	body      *ast.BlockStmt
	graph     *cfg.CFG
	parents   map[ast.Node]ast.Node
	reachable map[*cfg.Block]bool
	blocks    map[token.Pos]*cfg.Block
}

func ps6122NewFlow(pass *analysis.Pass, body *ast.BlockStmt) *ps6122Flow {
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
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
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6088NonreturnCall(pass, call) })
	flow := &ps6122Flow{body: body, graph: graph, parents: parents, reachable: map[*cfg.Block]bool{}, blocks: map[token.Pos]*cfg.Block{}}
	for _, block := range graph.Blocks {
		for _, root := range block.Nodes {
			ast.Inspect(root, func(node ast.Node) bool {
				if node != nil {
					flow.blocks[node.Pos()] = block
				}
				return true
			})
		}
	}
	if len(graph.Blocks) == 0 {
		return flow
	}
	pending := []*cfg.Block{graph.Blocks[0]}
	for len(pending) != 0 {
		block := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if flow.reachable[block] {
			continue
		}
		flow.reachable[block] = true
		pending = append(pending, ps6122Successors(pass, flow, block, token.NoPos)...)
	}
	return flow
}

func runPS6122(pass *analysis.Pass) (any, error) {
	return runPS6122Package(pass, "simd/archsimd")
}
func runPS6122Package(pass *analysis.Pass, packagePath string) (any, error) {
	decls := ps6115Declarations(pass)
	seen := map[token.Pos]bool{}
	inspectedHelpers := map[*ast.FuncDecl]bool{}
	for _, decl := range decls {
		if decl == nil || decl.Body == nil {
			continue
		}
		flow := ps6122NewFlow(pass, decl.Body)
		literalFlows := map[*ast.FuncLit]*ps6122Flow{}
		ps6122WalkExecuted(decl.Body, func(n ast.Node) bool {
			var body *ast.BlockStmt
			switch loop := n.(type) {
			case *ast.ForStmt:
				if ps6122For(pass, loop) && ps6122StableLoop(pass, loop.Body, loop.Init.(*ast.AssignStmt).Lhs[0]) {
					body = loop.Body
				}
			case *ast.RangeStmt:
				if ps6122RangeTrips(pass, loop) >= 2 {
					body = loop.Body
				}
			}
			if body == nil || !ps6122CanEnter(pass, decl.Body, flow.parents, body.Pos()) {
				return true
			}
			eligible := func(pos token.Pos) bool {
				candidateFlow := flow
				for _, literal := range ps6122EnclosingLiterals(decl.Body, pos) {
					candidateFlow = literalFlows[literal]
					if candidateFlow == nil {
						candidateFlow = ps6122NewFlow(pass, literal.Body)
						literalFlows[literal] = candidateFlow
					}
					if !ps6122CanEnter(pass, literal.Body, candidateFlow.parents, pos) {
						return false
					}
				}
				return ps6122CanEnter(pass, decl.Body, flow.parents, pos) && ps6122PositionRepeats(pass, candidateFlow, pos)
			}
			ps6122InspectReachable(pass, body, seen, packagePath, eligible)
			ps6122WalkExecuted(body, func(inner ast.Node) bool {
				call, ok := inner.(*ast.CallExpr)
				if !ok || !eligible(call.Pos()) {
					return true
				}
				helper := decls[ps6087FunctionID(pass, call)]
				if helper != nil && helper != decl && helper.Body != nil && !inspectedHelpers[helper] {
					helperFlow := ps6122NewFlow(pass, helper.Body)
					helperLiteralFlows := map[*ast.FuncLit]*ps6122Flow{}
					inspectedHelpers[helper] = true
					ps6122InspectReachable(pass, helper.Body, seen, packagePath, func(pos token.Pos) bool {
						for _, literal := range ps6122EnclosingLiterals(helper.Body, pos) {
							siteFlow := helperLiteralFlows[literal]
							if siteFlow == nil {
								siteFlow = ps6122NewFlow(pass, literal.Body)
								helperLiteralFlows[literal] = siteFlow
							}
							if !ps6122CanEnter(pass, literal.Body, siteFlow.parents, pos) || !ps6122PositionCanReturn(pass, siteFlow, pos) {
								return false
							}
						}
						return ps6122CanEnter(pass, helper.Body, helperFlow.parents, pos) && ps6122PositionCanReturn(pass, helperFlow, pos)
					})
				}
				return true
			})
			return true
		})
	}
	return nil, nil
}

func ps6122WalkExecuted(node ast.Node, visit func(ast.Node) bool) {
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil || !visit(n) {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok {
			if literal, ok := ps2110Unparen(call.Fun).(*ast.FuncLit); ok {
				ps6122WalkExecuted(literal.Body, visit)
			}
		}
		_, literal := n.(*ast.FuncLit)
		return !literal
	})
}

func ps6122CanEnter(pass *analysis.Pass, function *ast.BlockStmt, parents map[ast.Node]ast.Node, pos token.Pos) bool {
	if !ps6122SynchronousRegion(parents, pos) || ps6122BlockedByPrefix(pass, function, pos) {
		return false
	}
	canEnter := true
	ast.Inspect(function, func(n ast.Node) bool {
		if !canEnter || n == nil || pos < n.Pos() || pos >= n.End() {
			return false
		}
		switch x := n.(type) {
		case *ast.FuncLit:
			// Function literals are separate execution regions. Immediate invocations
			// are visited by ps6122WalkExecuted; other literal bodies are never candidates.
			return false
		case *ast.IfStmt:
			if truth, known := ps6122Bool(pass, x.Cond); known {
				if x.Body.Pos() <= pos && pos < x.Body.End() && !truth {
					canEnter = false
				}
				if x.Else != nil && x.Else.Pos() <= pos && pos < x.Else.End() && truth {
					canEnter = false
				}
			}
		case *ast.ForStmt:
			if x.Body.Pos() <= pos && pos < x.Body.End() && ps6122ForZero(pass, x) {
				canEnter = false
			}
		case *ast.RangeStmt:
			if x.Body.Pos() <= pos && pos < x.Body.End() && ps6122RangeTrips(pass, x) <= 0 {
				canEnter = false
			}
		case *ast.SwitchStmt:
			if x.Body.Pos() <= pos && pos < x.Body.End() && !ps6122SelectedSwitchContains(pass, x, pos) {
				canEnter = false
			}
		case *ast.CommClause:
			if x.Pos() <= pos && pos < x.End() && x.Comm != nil && ps6122StatementBlocks(pass, x.Comm) {
				canEnter = false
			}
		}
		return true
	})
	return canEnter
}

func ps6122BlockedByPrefix(pass *analysis.Pass, block *ast.BlockStmt, pos token.Pos) bool {
	for _, statement := range block.List {
		if statement.End() <= pos && ps6122StatementBlocks(pass, statement) {
			return true
		}
		if statement.Pos() <= pos && pos < statement.End() {
			blocked := false
			ast.Inspect(statement, func(node ast.Node) bool {
				nested, ok := node.(*ast.BlockStmt)
				if ok && nested != block && nested.Pos() <= pos && pos < nested.End() {
					blocked = ps6122BlockedByPrefix(pass, nested, pos)
					return false
				}
				return !blocked
			})
			if blocked {
				return true
			}
		}
	}
	return false
}

func ps6122BlockedAfter(pass *analysis.Pass, block *ast.BlockStmt, pos token.Pos, returnCounts bool) bool {
	for _, statement := range block.List {
		if statement.Pos() > pos {
			if returnCounts && ps6122StatementReturns(pass, statement) {
				return false
			}
			if _, branch := statement.(*ast.BranchStmt); branch {
				return false
			}
			if ps6122StatementBlocksContext(pass, statement, returnCounts) {
				return true
			}
		}
		if statement.Pos() <= pos && pos < statement.End() {
			blocked := false
			ast.Inspect(statement, func(node ast.Node) bool {
				nested, ok := node.(*ast.BlockStmt)
				if ok && nested != block && nested.Pos() <= pos && pos < nested.End() {
					blocked = ps6122BlockedAfter(pass, nested, pos, returnCounts)
					return false
				}
				return !blocked
			})
			if blocked {
				return true
			}
		}
	}
	return false
}

func ps6122StatementReturns(pass *analysis.Pass, statement ast.Stmt) bool {
	switch value := statement.(type) {
	case *ast.ReturnStmt:
		return true
	case *ast.RangeStmt:
		trips := ps6122RangeTrips(pass, value)
		return (trips == 1 || trips == 2) && ps6122BlockAlwaysReturns(pass, value.Body)
	case *ast.ForStmt:
		trips := ps6122ForTripClass(pass, value)
		return (trips == 1 || trips == 2) && ps6122BlockAlwaysReturns(pass, value.Body)
	}
	return false
}

func ps6122BlockAlwaysReturns(pass *analysis.Pass, body *ast.BlockStmt) bool {
	if len(body.List) == 1 {
		return ps6122StatementReturns(pass, body.List[0])
	}
	return false
}

func ps6122StatementBlocks(pass *analysis.Pass, statement ast.Stmt) bool {
	return ps6122StatementBlocksContext(pass, statement, false)
}

func ps6122StatementBlocksContext(pass *analysis.Pass, statement ast.Stmt, returnCounts bool) bool {
	switch value := statement.(type) {
	case *ast.RangeStmt:
		trips := ps6122RangeTrips(pass, value)
		return trips < 0 || (trips == 1 || trips == 2) && !ps6122BodyCanComplete(pass, value.Body, returnCounts)
	case *ast.ForStmt:
		trips := ps6122ForTripClass(pass, value)
		return (trips == 1 || trips == 2) && !ps6122BodyCanComplete(pass, value.Body, returnCounts)
	case *ast.SendStmt:
		return ps6122ExplicitNil(pass, value.Chan)
	case *ast.ExprStmt:
		unary, ok := ps2110Unparen(value.X).(*ast.UnaryExpr)
		return ok && unary.Op == token.ARROW && ps6122ExplicitNil(pass, unary.X)
	case *ast.SelectStmt:
		if len(value.Body.List) == 0 {
			return true
		}
		if len(value.Body.List) != 1 {
			return false
		}
		clause, ok := value.Body.List[0].(*ast.CommClause)
		return ok && clause.Comm != nil && ps6122StatementBlocks(pass, clause.Comm)
	case *ast.SwitchStmt:
		if len(value.Body.List) == 0 {
			return false
		}
		hasDefault := false
		for _, raw := range value.Body.List {
			clause, ok := raw.(*ast.CaseClause)
			if !ok || ps6122BodyCanComplete(pass, &ast.BlockStmt{Lbrace: clause.Colon, List: clause.Body, Rbrace: clause.End()}, returnCounts) {
				return false
			}
			hasDefault = hasDefault || clause.List == nil
		}
		return hasDefault
	}
	return false
}

func ps6122BodyCanComplete(pass *analysis.Pass, body *ast.BlockStmt, returnCounts bool) bool {
	if len(body.List) == 1 {
		if statement, ok := body.List[0].(*ast.SelectStmt); ok && len(statement.Body.List) == 0 {
			return false
		}
	}
	flow := ps6122NewFlow(pass, body)
	if len(flow.graph.Blocks) == 0 {
		return true
	}
	return ps6122BlockCanExit(pass, flow, flow.graph.Blocks[0], map[*cfg.Block]bool{}, returnCounts)
}

func ps6122SynchronousRegion(parents map[ast.Node]ast.Node, pos token.Pos) bool {
	var innermost ast.Node
	for node := range parents {
		if node.Pos() <= pos && pos < node.End() && (innermost == nil || node.End()-node.Pos() < innermost.End()-innermost.Pos()) {
			innermost = node
		}
	}
	for node := innermost; node != nil; node = parents[node] {
		switch x := node.(type) {
		case *ast.GoStmt, *ast.DeferStmt:
			return false
		case *ast.FuncLit:
			call, ok := parents[x].(*ast.CallExpr)
			if !ok || ps2110Unparen(call.Fun) != x {
				return false
			}
		}
	}
	return true
}

func ps6122ForZero(pass *analysis.Pass, loop *ast.ForStmt) bool {
	if loop.Cond != nil {
		value := pass.TypesInfo.Types[ps2110Unparen(loop.Cond)].Value
		if value != nil && value.Kind() == constant.Bool {
			return !constant.BoolVal(value)
		}
	}
	init, ok := loop.Init.(*ast.AssignStmt)
	if !ok || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return false
	}
	cond, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS || ps6114ExprObject(pass, init.Lhs[0]) == nil || ps6114ExprObject(pass, cond.X) != ps6114ExprObject(pass, init.Lhs[0]) {
		return false
	}
	start := pass.TypesInfo.Types[init.Rhs[0]].Value
	bound := pass.TypesInfo.Types[cond.Y].Value
	if start == nil || bound == nil || start.Kind() != constant.Int || bound.Kind() != constant.Int {
		return false
	}
	return constant.Compare(start, token.GEQ, bound)
}

func ps6122SelectedSwitchContains(pass *analysis.Pass, statement *ast.SwitchStmt, pos token.Pos) bool {
	tag := constant.MakeBool(true)
	if statement.Tag != nil {
		tag = pass.TypesInfo.Types[ps2110Unparen(statement.Tag)].Value
		if tag == nil {
			return true
		}
	}
	selected, containing, fallback := -1, -1, -1
	for index, raw := range statement.Body.List {
		clause, ok := raw.(*ast.CaseClause)
		if !ok {
			return false
		}
		if clause.Pos() <= pos && pos < clause.End() {
			containing = index
		}
		if clause.List == nil {
			fallback = index
		}
		for _, expression := range clause.List {
			value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
			if value == nil {
				return true
			}
			if selected < 0 && constant.Compare(tag, token.EQL, value) {
				selected = index
			}
		}
	}
	if selected < 0 {
		selected = fallback
	}
	return containing < 0 || selected == containing
}

func ps6122BlockAt(pass *analysis.Pass, flow *ps6122Flow, pos token.Pos) *cfg.Block {
	block := flow.blocks[pos]
	if flow.reachable[block] && !ps6122BlockHasBlockingTransfer(pass, flow, block, token.NoPos, pos) {
		return block
	}
	return nil
}

func ps6122BlockHasBlockingTransfer(pass *analysis.Pass, flow *ps6122Flow, block *cfg.Block, after, before token.Pos) bool {
	for _, node := range block.Nodes {
		if after.IsValid() && node.End() <= after || before.IsValid() && node.Pos() >= before {
			continue
		}
		blocked := false
		ast.Inspect(node, func(n ast.Node) bool {
			if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.ARROW && ps6122ExplicitNil(pass, unary.X) && ps6122ExpressionExecuted(pass, flow.parents, unary) && !ps6122SelectHasDefault(flow.parents, unary) {
				blocked = true
				return false
			}
			statement, ok := n.(ast.Stmt)
			if ok {
				if ps6122StatementBlocks(pass, statement) && !ps6122SelectHasDefault(flow.parents, statement) {
					blocked = true
				}
				if _, selected := statement.(*ast.SelectStmt); selected {
					return false
				}
			}
			return !blocked
		})
		if blocked {
			return true
		}
	}
	return false
}

func ps6122ExpressionExecuted(pass *analysis.Pass, parents map[ast.Node]ast.Node, node ast.Node) bool {
	for child, parent := node, parents[node]; parent != nil; child, parent = parent, parents[parent] {
		binary, ok := parent.(*ast.BinaryExpr)
		if !ok || binary.Y.Pos() > child.Pos() || child.End() > binary.Y.End() {
			continue
		}
		left, known := ps6122Bool(pass, binary.X)
		if known && (binary.Op == token.LAND && !left || binary.Op == token.LOR && left) {
			return false
		}
	}
	return true
}

func ps6122SelectHasDefault(parents map[ast.Node]ast.Node, node ast.Node) bool {
	original := node
	var communication ast.Stmt
	for node != nil {
		if clause, ok := node.(*ast.CommClause); ok {
			communication = clause.Comm
		}
		if statement, ok := node.(*ast.SelectStmt); ok {
			isCommunication := communication == original
			if expression, ok := communication.(*ast.ExprStmt); ok && ps2110Unparen(expression.X) == original {
				isCommunication = true
			}
			if !isCommunication {
				return false
			}
			for _, raw := range statement.Body.List {
				clause := raw.(*ast.CommClause)
				if clause.Comm == nil {
					return true
				}
			}
			return false
		}
		node = parents[node]
	}
	return false
}

func ps6122Successors(pass *analysis.Pass, flow *ps6122Flow, block *cfg.Block, after token.Pos) []*cfg.Block {
	if ps6122BlockHasBlockingTransfer(pass, flow, block, after, token.NoPos) {
		return nil
	}
	if len(block.Succs) == 2 && len(block.Nodes) != 0 {
		if condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr); ok {
			if loop, ok := flow.parents[condition].(*ast.ForStmt); ok && ps6122ForZero(pass, loop) {
				return block.Succs[1:]
			}
			if truth, known := ps6122Bool(pass, condition); known {
				if truth {
					return block.Succs[:1]
				}
				return block.Succs[1:]
			}
		}
	}
	return ps6099ReachableSuccessors(pass, block, flow.parents)
}

func ps6122Bool(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	expression = ps2110Unparen(expression)
	if value := pass.TypesInfo.Types[expression].Value; value != nil && value.Kind() == constant.Bool {
		return constant.BoolVal(value), true
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			truth, known := ps6122Bool(pass, value.X)
			return !truth, known
		}
	case *ast.BinaryExpr:
		left, leftKnown := ps6122Bool(pass, value.X)
		right, rightKnown := ps6122Bool(pass, value.Y)
		switch value.Op {
		case token.LOR:
			if leftKnown && left || rightKnown && right {
				return true, true
			}
			return false, leftKnown && rightKnown
		case token.LAND:
			if leftKnown && !left || rightKnown && !right {
				return false, true
			}
			return left && right, leftKnown && rightKnown
		case token.EQL, token.NEQ:
			if leftKnown && rightKnown {
				equal := left == right
				return equal == (value.Op == token.EQL), true
			}
		}
	}
	return false, false
}

func ps6122PositionRepeats(pass *analysis.Pass, flow *ps6122Flow, pos token.Pos) bool {
	start := ps6122BlockAt(pass, flow, pos)
	if start == nil {
		return false
	}
	seen := map[*cfg.Block]bool{}
	var reaches func(*cfg.Block) bool
	reaches = func(block *cfg.Block) bool {
		for _, successor := range ps6122Successors(pass, flow, block, token.NoPos) {
			if successor == start {
				return true
			}
			if flow.reachable[successor] && !seen[successor] {
				seen[successor] = true
				if reaches(successor) {
					return true
				}
			}
		}
		return false
	}
	seen[start] = true
	if ps6122BlockHasBlockingTransfer(pass, flow, start, pos, token.NoPos) || ps6122BlockedAfter(pass, flow.body, pos, false) {
		return false
	}
	return reaches(start)
}

func ps6122PositionCanReturn(pass *analysis.Pass, flow *ps6122Flow, pos token.Pos) bool {
	start := ps6122BlockAt(pass, flow, pos)
	if start == nil {
		return false
	}
	if ps6122BlockedAfter(pass, flow.body, pos, true) {
		return false
	}
	return ps6122BlockCanExit(pass, flow, start, map[*cfg.Block]bool{}, true)
}

func ps6122BlockCanExit(pass *analysis.Pass, flow *ps6122Flow, block *cfg.Block, seen map[*cfg.Block]bool, returnCounts bool) bool {
	if seen[block] || !flow.reachable[block] {
		return false
	}
	seen[block] = true
	if ps6122BlockHasBlockingTransfer(pass, flow, block, token.NoPos, token.NoPos) {
		return false
	}
	if len(block.Succs) == 0 {
		if block.Return() != nil {
			return returnCounts
		}
		if statement, ok := block.Stmt.(*ast.SelectStmt); ok && len(statement.Body.List) == 0 {
			return false
		}
		for _, node := range block.Nodes {
			nonreturn := false
			ast.Inspect(node, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if ok && ps6088NonreturnCall(pass, call) {
					nonreturn = true
					return false
				}
				return !nonreturn
			})
			if nonreturn {
				return false
			}
			if statement, ok := node.(*ast.SelectStmt); ok && len(statement.Body.List) == 0 {
				return false
			}
		}
		return block.Kind != cfg.KindUnreachable
	}
	for _, successor := range ps6122Successors(pass, flow, block, token.NoPos) {
		if ps6122BlockCanExit(pass, flow, successor, seen, returnCounts) {
			return true
		}
	}
	return false
}

func ps6122InspectReachable(pass *analysis.Pass, body *ast.BlockStmt, seen map[token.Pos]bool, packagePath string, eligible func(token.Pos) bool) {
	ps6122WalkExecuted(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && eligible(call.Pos()) {
			ps6122Inspect(pass, call, seen, packagePath)
		}
		return true
	})
}

func ps6122EnclosingLiterals(root ast.Node, pos token.Pos) []*ast.FuncLit {
	var result []*ast.FuncLit
	ast.Inspect(root, func(node ast.Node) bool {
		if node == nil || pos < node.Pos() || pos >= node.End() {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok {
			result = append(result, literal)
		}
		return true
	})
	return result
}

func ps6122Inspect(pass *analysis.Pass, node ast.Node, seen map[token.Pos]bool, packagePath string) {
	call, ok := node.(*ast.CallExpr)
	if !ok || seen[call.Pos()] {
		return
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != packagePath || (fn.Name() != "ShiftAllLeft" && fn.Name() != "ShiftAllRight") || sig.Recv() == nil || sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 1 || sig.Results().Len() != 1 || len(call.Args) != 1 {
		return
	}
	named, ok := types.Unalias(sig.Recv().Type()).(*types.Named)
	if !ok {
		return
	}
	width, exactType := ps6122Types[named.Obj().Name()]
	if !exactType || named.Obj().Pkg() == nil || named.Obj().Pkg().Path() != packagePath || !types.Identical(sig.Recv().Type(), sig.Results().At(0).Type()) || !types.Identical(sig.Params().At(0).Type(), types.Typ[types.Uint64]) {
		return
	}
	v := pass.TypesInfo.Types[ps2110Unparen(call.Args[0])].Value
	if v == nil || v.Kind() != constant.Int {
		return
	}
	shift, exact := constant.Uint64Val(v)
	if !exact || shift == 0 || shift >= width {
		return
	}
	seen[call.Pos()] = true
	kind := "left"
	if fn.Name() == "ShiftAllRight" {
		if strings.HasPrefix(named.Obj().Name(), "Int") {
			kind = "arithmetic right"
		} else {
			kind = "logical right"
		}
	}
	pass.Reportf(call.Pos(), "constant %d-bit archsimd %s shift by %d occurs in a loop path that may repeat; inspect pinned compiler/GOEXPERIMENT/GOOS/GOARCH/ISA emission for an avoidable distance broadcast and variable shift, suppress if an immediate shift is already emitted, validate all supported distances and signed/unsigned edge bits, and benchmark the complete consumer plus preallocated leaf (PS6122 advisory, no automatic fix or speedup claim)", width, kind, shift)
}

// ps6122RangeTrips returns 0 for no iterations, 1 for exactly one, and 2 for
// two-or-more. 3 is a dynamic cardinality that may repeat, and -1 is a
// source-proved permanently blocking channel range.
func ps6122RangeTrips(pass *analysis.Pass, loop *ast.RangeStmt) int {
	t := pass.TypesInfo.TypeOf(loop.X)
	if t == nil {
		return 0
	}
	if ps6122ExplicitNil(pass, loop.X) {
		switch t.Underlying().(type) {
		case *types.Chan:
			return -1
		case *types.Slice, *types.Map:
			return 0
		}
	}
	base := t
	if pointer, ok := base.Underlying().(*types.Pointer); ok {
		base = pointer.Elem()
	}
	if a, ok := base.Underlying().(*types.Array); ok {
		return ps6122TripClass(a.Len())
	}
	if v := pass.TypesInfo.Types[ps2110Unparen(loop.X)].Value; v != nil {
		switch v.Kind() {
		case constant.Int:
			n, exact := constant.Int64Val(v)
			if !exact {
				return 0
			}
			return ps6122TripClass(n)
		case constant.String:
			return ps6122TripClass(int64(utf8.RuneCountInString(constant.StringVal(v))))
		}
	}
	if literal, ok := ps2110Unparen(loop.X).(*ast.CompositeLit); ok {
		switch literal.Type.(type) {
		case *ast.ArrayType, *ast.MapType:
			return ps6122TripClass(int64(len(literal.Elts)))
		}
	}
	return 3
}

func ps6122ExplicitNil(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	if identifier, ok := expression.(*ast.Ident); ok {
		_, nilObject := pass.TypesInfo.Uses[identifier].(*types.Nil)
		return nilObject
	}
	call, ok := expression.(*ast.CallExpr)
	return ok && len(call.Args) == 1 && pass.TypesInfo.Types[ps2110Unparen(call.Fun)].IsType() && ps6122ExplicitNil(pass, call.Args[0])
}

func ps6122TripClass(n int64) int {
	if n <= 0 {
		return 0
	}
	if n == 1 {
		return 1
	}
	return 2
}

func ps6122StableLoop(pass *analysis.Pass, body *ast.BlockStmt, index ast.Expr) bool {
	obj := ps6114ExprObject(pass, index)
	stable := obj != nil
	ast.Inspect(body, func(n ast.Node) bool {
		if !stable || n == nil {
			return false
		}
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if ps6114ExprObject(pass, lhs) == obj {
					stable = false
				}
			}
		case *ast.IncDecStmt:
			if ps6114ExprObject(pass, x.X) == obj {
				stable = false
			}
		case *ast.UnaryExpr:
			if x.Op == token.AND && ps6114ExprObject(pass, x.X) == obj {
				stable = false
			}
		case *ast.RangeStmt:
			if x.Tok == token.ASSIGN && (ps6114ExprObject(pass, x.Key) == obj || ps6114ExprObject(pass, x.Value) == obj) {
				stable = false
			}
		}
		return stable
	})
	return stable
}

func ps6122For(pass *analysis.Pass, loop *ast.ForStmt) bool {
	return ps6122ForTripClass(pass, loop) == 2
}

func ps6122ForTripClass(pass *analysis.Pass, loop *ast.ForStmt) int {
	init, ok := loop.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return -1
	}
	id, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return -1
	}
	obj := pass.TypesInfo.Defs[id]
	cond, ok := loop.Cond.(*ast.BinaryExpr)
	if obj == nil || !ok || cond.Op != token.LSS || ps6114ExprObject(pass, cond.X) != obj {
		return -1
	}
	boundValue := pass.TypesInfo.Types[cond.Y].Value
	startValue := pass.TypesInfo.Types[init.Rhs[0]].Value
	if boundValue == nil || boundValue.Kind() != constant.Int || startValue == nil || startValue.Kind() != constant.Int {
		return -1
	}
	trips, exact := constant.Int64Val(constant.BinaryOp(boundValue, token.SUB, startValue))
	if !exact {
		return -1
	}
	post, ok := loop.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC || ps6114ExprObject(pass, post.X) != obj {
		return -1
	}
	return ps6122TripClass(trips)
}
