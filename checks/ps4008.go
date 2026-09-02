package checks

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS4008 reports a matmul-shaped nest whose innermost loop is a serial
// scalar dot accumulator.
var PS4008 = register(&lint.Check{
	ID:       "PS4008",
	Category: "vector",
	Slug:     "serial-dot-matmul",
	Level:    lint.LevelAggressive,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a matmul whose innermost loop is a serial scalar dot accumulator",
		Text: `sum += a[…]*b[…] in the innermost loop of a ≥3-deep nest chains
every fused multiply-add on the previous one: the loop runs at FMA latency,
not throughput. An ikj/axpy loop order (or a small block of independent
accumulators) breaks the dependency chain and lets the core retire several
FMAs per cycle — WHEN the axpy inner loop is SIMD-vectorized.

MEASURED-IN-GO CAVEAT: gc does not auto-vectorize the axpy inner loop, so the
throughput win above is a C/Fortran -O3 outcome, not a gc one. Measured on go1.26
(Apple M2 Pro) across 128/256/512, the ikj form is ≈parity to the
register-accumulating ijk form — modestly SLOWER at small/cache-resident sizes
(the extra bounds-checked "c[i][j] +=" read-modify-writes outweigh the broken
latency chain), ~3% faster only once the matrix spills cache. Treat this as a
STRUCTURAL pointer: the real speedup comes from vectorizing (asm/cgo/a BLAS
call), for which the ikj order is the right shape — not from the reorder alone.
See benchmarks/README.md for why there is no Before/After pair.

L3 (aggressive): reassociating a floating-point reduction changes the
rounding order, so the result is NOT bit-identical. Gate the rewrite with a
tolerance-based oracle or restructure so each output element keeps one
accumulation order (the ikj form does: each c[i][j] still sums k ascending).

Local call summaries preserve derived-index and accumulator facts across
declared function aliases, generic instantiations, method expressions and
method values. Callable arguments contribute effects only when the callee can
invoke them; definite resets, conditional calls, deferred callback work and
asynchronous go calls retain their distinct execution semantics. Nested
invoked closures and shared package objects use the same path-sensitive
summaries. Argument values are snapshotted in Go's left-to-right evaluation
order, read-modify writes retain their prior dependency, and a write through a
may-alias pointer is definite only when every possible value names the same
target. Pointer retargeting by an earlier argument is carried into later
arguments, while an opaque fallback preserves the incoming value unless it
proves an independent overwrite. Saved method receivers, callable struct
fields, fixed-array elements and callable results retain their definition-time
values even when the source is reassigned. Multi-result call expansions
preserve the same snapshots; ambiguous joins and escaped receivers remain
conservative.

The automatic fix rewrites only the canonical shape below when a, b, c are
fixed rectangular float64 arrays whose dimensions prove every access in
bounds. It zeroes the output row first, then accumulates rank-1 updates with
k ascending, preserving each cell's IEEE addition sequence. [][]float64
stays advisory: separate variables may share rows, and ragged source/output
panics expose the original per-cell store order.`,
		Before: `for i := range a {
	for j := range b[0] {
		sum := 0.0
		for k := range b {
			sum += a[i][k] * b[k][j] // latency chain
		}
		c[i][j] = sum
	}
}`,
		After: `for i := range a {
	for j := range b[0] {
		c[i][j] = 0
	}
	for k := range b { // axpy: independent accumulators per j
		for j := range b[0] {
			c[i][j] += a[i][k] * b[k][j]
		}
	}
}`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4008",
		Doc:  "serial scalar dot accumulator in a matmul nest",
		Run:  runPS4008,
	},
})

func runPS4008(pass *analysis.Pass) (any, error) {
	index := ps1006BuildAnalysisIndex(pass)
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	defer ps1006ActiveAnalysisIndexes.Delete(pass)
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			if !astutil.IsLoop(n) {
				return true
			}
			body := astutil.LoopBody(n)
			if body == nil || containsLoop(body) {
				return true
			}
			// Depth ≥3 within the current function: at least two enclosing loops.
			depth := 1
			for index := len(stack) - 1; index >= 0; index-- {
				if _, ok := stack[index].(*ast.FuncLit); ok {
					break
				}
				anc := stack[index]
				if astutil.IsLoop(anc) {
					depth++
				}
			}
			if depth < 3 {
				return true
			}
			kObj := ps4008LoopObject(pass, n)
			if kObj == nil {
				return true
			}
			independent := ps4008IndependentAccumulatorCount(pass, body, kObj, nil, ps4008EnclosingTailGuard(index, pass, stack))
			// A tile with four or more independent accumulators in the same
			// serial loop already exposes enough ILP to hide FMA latency. Count
			// only direct updates and updates guarded by conditions invariant in
			// the serial dimension, so tap-varying masks keep reporting.
			if independent >= 4 {
				return true
			}
			if !ps4008HasSerialAccumulator(pass, body, kObj) {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     n.Pos(),
				End:     body.Lbrace,
				Message: "innermost loop of this nest is a serial scalar dot accumulator running at FMA latency, not throughput; an ikj/axpy order or an accumulator block breaks the chain (reassociation is not bit-identical — gate with a tolerance oracle)",
			}
			if fix := ps4008AxpyFix(pass, stack, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil, nil
}

func containsLoop(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		if n != body {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
		}
		if n != body && astutil.IsLoop(n) {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps4008LoopObject(pass *analysis.Pass, loop ast.Node) types.Object {
	switch value := loop.(type) {
	case *ast.ForStmt:
		assign, ok := value.Init.(*ast.AssignStmt)
		if !ok || len(assign.Lhs) != 1 {
			return nil
		}
		identifier, ok := assign.Lhs[0].(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return nil
		}
		return identObject(pass, identifier)
	case *ast.RangeStmt:
		identifier, ok := value.Key.(*ast.Ident)
		if !ok || identifier.Name == "_" {
			return nil
		}
		return identObject(pass, identifier)
	}
	return nil
}

func ps4008UpdateDerivedDeps(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool) {
	ps4008InvalidateDerivedDeps(pass, statement, deps)
	switch value := statement.(type) {
	case *ast.AssignStmt:
		if value.Tok != token.DEFINE && value.Tok != token.ASSIGN {
			return
		}
		if len(value.Lhs) != len(value.Rhs) {
			for _, lhs := range value.Lhs {
				if identifier, ok := lhs.(*ast.Ident); ok {
					delete(deps, identObject(pass, identifier))
				}
			}
			return
		}
		prev := cloneObjectBoolMap(deps)
		updates := make(map[types.Object]bool, len(value.Lhs))
		for index, lhs := range value.Lhs {
			identifier, ok := lhs.(*ast.Ident)
			if !ok || identifier.Name == "_" {
				continue
			}
			object := identObject(pass, identifier)
			if object == nil {
				continue
			}
			if !ps4008CanTrackDerivedLocal(pass, identifier) {
				delete(deps, object)
				continue
			}
			updates[object] = ps4008ExprDependsOn(pass, value.Rhs[index], inner, prev)
		}
		for object, depends := range updates {
			if depends {
				deps[object] = true
				continue
			}
			delete(deps, object)
		}
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || declaration.Tok != token.VAR {
			return
		}
		for _, raw := range declaration.Specs {
			specification, ok := raw.(*ast.ValueSpec)
			if !ok {
				continue
			}
			prev := cloneObjectBoolMap(deps)
			updates := make(map[types.Object]bool, len(specification.Names))
			for index, name := range specification.Names {
				object := identObject(pass, name)
				if object == nil {
					continue
				}
				if !ps4008CanTrackDerivedLocal(pass, name) {
					delete(deps, object)
					continue
				}
				updates[object] = index < len(specification.Values) && ps4008ExprDependsOn(pass, specification.Values[index], inner, prev)
			}
			for object, depends := range updates {
				if depends {
					deps[object] = true
					continue
				}
				delete(deps, object)
			}
		}
	}
}

func ps4008InvalidateDerivedDeps(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]bool) {
	ps4008InvalidateDerivedDepsAt(pass, statement, deps, ps4008StatementPosition(statement))
}

type ps4008CallableEffectMode uint8

const (
	ps4008DefiniteWrite ps4008CallableEffectMode = iota
	ps4008MayWrite
)

func ps4008StatementPosition(statement ast.Stmt) token.Pos {
	if statement == nil {
		return token.NoPos
	}
	return statement.Pos()
}

func ps4008InvalidateDerivedDepsAt(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]bool, before token.Pos) bool {
	return ps4008InvalidateEffectsAt(pass, statement, deps, before, ps4008DefiniteWrite)
}

func ps4008InvalidateMayWritesAt(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]bool, before token.Pos) bool {
	return ps4008InvalidateEffectsAt(pass, statement, deps, before, ps4008MayWrite)
}

func ps4008InvalidateEffectsAt(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]bool, before token.Pos, mode ps4008CallableEffectMode) bool {
	if len(deps) == 0 || statement == nil {
		return true
	}
	clearAll := false
	callablesKnown := true
	kill := make(map[types.Object]bool)
	ast.Inspect(statement, func(node ast.Node) bool {
		if clearAll {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			// A function literal's body does not run when the literal value is
			// created. Calls of the value are handled through callable
			// provenance below, including immediately-invoked literals.
			return false
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				switch target := ps2110Unparen(lhs).(type) {
				case *ast.StarExpr:
					if ps4008KillAggregateAliasTargets(pass, target.X, deps, kill, before) {
						continue
					}
					if ps4008KillPointerAliasTargets(pass, target.X, deps, kill, before, mode) {
						continue
					}
					if ps4008ExprDependsOn(pass, target.X, nil, deps) {
						clearAll = true
						return false
					}
				case *ast.IndexExpr:
					if ps4008ExprDependsOn(pass, target.X, nil, deps) {
						clearAll = true
						return false
					}
				case *ast.SelectorExpr:
					if ps4008ExprDependsOn(pass, target.X, nil, deps) {
						clearAll = true
						return false
					}
				}
			}
		case *ast.CallExpr:
			// Callable bodies use CFG-derived effects. Dependency transfer keeps
			// only writes guaranteed on every reachable exit; tile-safety scans use
			// any reachable write. Both exclude impossible branches and
			// short-circuited operands.
			if ps4008KillCallableWriteTargetsMode(pass, value.Fun, deps, kill, before, mode) {
				// Known callable write provenance was handled precisely.
			} else {
				if !ps4008CallIsConversionOrBuiltin(pass, value) && ps4008CallableMayCapture(pass, value.Fun) {
					callablesKnown = false
					// An unresolved callable may mutate dependencies captured from
					// its surrounding function even without explicit arguments.
					clearAll = true
					return false
				}
				if ps4008ExprDependsOn(pass, value.Fun, nil, deps) {
					clearAll = true
					return false
				}
			}
			for argumentIndex, arg := range value.Args {
				if unary, ok := ps2110Unparen(arg).(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if ps4008CallWritesArgument(pass, value, argumentIndex, before, mode) {
						ps4008CollectMentionedDeps(pass, unary.X, deps, kill)
					}
					continue
				}
				if ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(arg), make(map[types.Type]bool)) {
					if ps4008CallInvokesArgument(pass, value, argumentIndex, before, mode) {
						if ps4008KillCallableWriteTargetsMode(pass, arg, deps, kill, before, mode) {
							continue
						}
						if ps4008CallableMayCapture(pass, arg) {
							callablesKnown = false
							clearAll = true
							return false
						}
					}
				}
				if typ := pass.TypesInfo.TypeOf(arg); typ != nil {
					if _, isInterface := typ.Underlying().(*types.Interface); isInterface && ps4008KillAggregateAliasTargets(pass, arg, deps, kill, before) {
						continue
					}
				}
				if ps4008IsPointerType(pass.TypesInfo.TypeOf(arg)) {
					if ps4008CallWritesArgument(pass, value, argumentIndex, before, mode) {
						ps4008KillPointerAliasTargets(pass, arg, deps, kill, before, mode)
					}
					continue
				}
				if ps4008CallWritesArgument(pass, value, argumentIndex, before, mode) && ps4008KillAggregateAliasTargets(pass, arg, deps, kill, before) {
					continue
				}
				if ps4008ArgumentMayExposeDerivedStorage(pass, arg, deps) {
					clearAll = true
					return false
				}
			}
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok && ps4008MethodValueReceiverWrites(pass, selector, before, mode) {
				if ps4008IsPointerType(pass.TypesInfo.TypeOf(selector.X)) && ps4008KillPointerAliasTargets(pass, selector.X, deps, kill, before, mode) {
					return true
				}
				if ps4008KillAggregateAliasTargets(pass, selector.X, deps, kill, before) {
					return true
				}
				if ps4008ExprDependsOn(pass, selector.X, nil, deps) {
					clearAll = true
					return false
				}
			}
		}
		return true
	})
	if clearAll {
		clear(deps)
		return callablesKnown
	}
	for object := range kill {
		delete(deps, object)
	}
	return callablesKnown
}

func ps4008KillAggregateAliasTargets(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, kill map[types.Object]bool, before token.Pos) bool {
	targets := make(map[types.Object]bool)
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		base := identObject(pass, identifier)
		for _, target := range ps4008AggregateAliasTargets(pass, base, deps, before) {
			targets[target] = true
		}
		return true
	})
	for target := range targets {
		kill[target] = true
	}
	return len(targets) > 0
}

func ps4008AggregateAliasTargets(pass *analysis.Pass, alias types.Object, deps map[types.Object]bool, before token.Pos) []types.Object {
	if alias == nil {
		return nil
	}
	bestValue := ps4008BestAliasValue(pass, alias, before)
	return ps4008AddressedDerivedTargetsDeep(pass, bestValue, deps, before)
}

func ps4008BestAliasValue(pass *analysis.Pass, alias types.Object, before token.Pos) ast.Expr {
	definition, ok := ps4008BestAliasDefinition(pass, alias, before)
	if !ok {
		return nil
	}
	return definition.value
}

func ps4008BestAliasDefinition(pass *analysis.Pass, alias types.Object, before token.Pos) (ps1006AliasDef, bool) {
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil || alias == nil {
		return ps1006AliasDef{}, false
	}
	definitions := index.aliasDefs[alias]
	position := len(definitions)
	if before.IsValid() {
		position, _ = slices.BinarySearchFunc(definitions, before, func(definition ps1006AliasDef, target token.Pos) int {
			switch {
			case definition.position < target:
				return -1
			case definition.position > target:
				return 1
			default:
				return 0
			}
		})
	}
	for definitionIndex := position - 1; definitionIndex >= 0; definitionIndex-- {
		definition := definitions[definitionIndex]
		if !ps4008SkipAliasDef(pass, definition.position, before) {
			return definition, true
		}
	}
	return ps1006AliasDef{}, false
}

func ps4008PossibleAliasValues(pass *analysis.Pass, alias types.Object, before token.Pos) []ast.Expr {
	definitions := ps4008PossibleAliasDefinitions(pass, alias, before)
	values := make([]ast.Expr, 0, len(definitions))
	for _, definition := range definitions {
		values = append(values, definition.value)
	}
	return values
}

// ps4008PossibleAliasDefinitions retains the source position at which an
// alias value was copied. Callable aliases need that snapshot position:
// `saved := callback` keeps callback's value at the copy even if callback is
// reassigned before saved is invoked.
func ps4008PossibleAliasDefinitions(pass *analysis.Pass, alias types.Object, before token.Pos) []ps1006AliasDef {
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil || alias == nil {
		return nil
	}
	best, haveBest := ps4008BestAliasDefinition(pass, alias, before)
	definitions := make([]ps1006AliasDef, 0, 2)
	if haveBest {
		definitions = append(definitions, best)
	}
	bestShadowsBackedge := haveBest && ps4008TypeMayCarryCallable(alias.Type(), make(map[types.Type]bool)) &&
		ps4008AliasDefinitionShadowsBackedge(index, best.position, before)
	for _, definition := range index.aliasDefs[alias] {
		if haveBest && definition.position == best.position {
			continue
		}
		if ps4008AliasDefinitionUnreachableByLoop(pass, index, definition.position) {
			continue
		}
		if ps4008AliasDefCanReachOnBackedge(index, definition.position, before) {
			if bestShadowsBackedge {
				continue
			}
			definitions = append(definitions, definition)
			continue
		}
		if before.IsValid() && definition.position < before && (!haveBest || definition.position > best.position) {
			// A skipped conditional definition after the last dominating
			// definition is another value that can reach this use.
			definitions = append(definitions, definition)
		}
	}
	exhaustiveWithoutIncoming := haveBest && ps4008ExhaustiveBranchDefinitions(pass, index, definitions[1:], before)
	if exhaustiveWithoutIncoming {
		definitions = definitions[1:]
	}
	// When every lexical definition may be skipped, the incoming value is
	// unknown (for example a parameter assigned only in a zero-trip loop).
	// Returning only the optional definitions would incorrectly make them an
	// exhaustive reaching set; paired if/else definitions are the exception.
	if !haveBest && !ps4008ExhaustiveBranchDefinitions(pass, index, definitions, before) {
		return nil
	}
	if ps4008CallableAliasType(alias.Type()) && !ps4008CallableAliasDefinitionsStable(index, alias, definitions, before) {
		return nil
	}
	return definitions
}

func ps4008CallableAliasType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	_, ok := types.Unalias(typ).Underlying().(*types.Signature)
	return ok
}

// ps4008CallableAliasDefinitionsStable distinguishes tracked assignments,
// which are represented in the reaching-definition set, from opaque writes
// or escapes through the callable's address. The latter can replace the
// function value between its lexical definition and the snapshot and must
// make callable resolution unknown.
func ps4008CallableAliasDefinitionsStable(index *ps1006AnalysisIndex, alias types.Object, definitions []ps1006AliasDef, before token.Pos) bool {
	if index == nil || alias == nil || len(definitions) == 0 || !before.IsValid() {
		return false
	}
	facts := index.functionFactsAt[before]
	if facts == nil {
		return false
	}
	tracked := index.aliasDefs[alias]
	earliest := before
	for _, definition := range definitions {
		if !definition.position.IsValid() || definition.position >= before {
			return false
		}
		if definition.position < earliest {
			earliest = definition.position
		}
	}
	positions := facts.localUnsafe[alias]
	position, _ := slices.BinarySearch(positions, earliest+1)
	for ; position < len(positions) && positions[position] < before; position++ {
		_, found := slices.BinarySearchFunc(tracked, positions[position], func(definition ps1006AliasDef, target token.Pos) int {
			return cmp.Compare(definition.position, target)
		})
		if !found {
			return false
		}
	}
	return true
}

func ps4008AliasDefinitionShadowsBackedge(index *ps1006AnalysisIndex, definition, before token.Pos) bool {
	if index == nil || !definition.IsValid() || !before.IsValid() || definition >= before {
		return false
	}
	for loop := index.loopsAt[before]; loop != nil; loop = loop.parent {
		if loop.bodyStart <= definition && definition <= loop.bodyEnd {
			return true
		}
	}
	return false
}

func ps4008ExhaustiveBranchDefinitions(pass *analysis.Pass, index *ps1006AnalysisIndex, definitions []ps1006AliasDef, before token.Pos) bool {
	if pass == nil || index == nil || len(definitions) < 2 || !before.IsValid() {
		return false
	}
	ranges := make(map[ps1006BranchRange][]token.Pos, len(definitions))
	for _, definition := range definitions {
		if definition.branch.start.IsValid() {
			ranges[definition.branch] = append(ranges[definition.branch], definition.position)
		}
	}
	for branch, positions := range ranges {
		peer, paired := index.branchPeers[branch]
		peerPositions := ranges[peer]
		owner := index.branchOwners[branch]
		if !paired || len(peerPositions) == 0 || owner == nil || index.branchOwners[peer] != owner {
			continue
		}
		if ps4008BranchDefinitionMustExecute(pass, owner, branch, positions) &&
			ps4008BranchDefinitionMustExecute(pass, owner, peer, peerPositions) &&
			ps4008BranchPairReachesUse(pass, index, owner, before) {
			return true
		}
	}
	return false
}

func ps4008BranchDefinitionMustExecute(pass *analysis.Pass, owner *ast.IfStmt, branch ps1006BranchRange, definitions []token.Pos) bool {
	if owner == nil || len(definitions) == 0 {
		return false
	}
	var block *ast.BlockStmt
	switch {
	case owner.Body != nil && owner.Body.Pos() == branch.start && owner.Body.End() == branch.end:
		block = owner.Body
	case owner.Else != nil && owner.Else.Pos() == branch.start && owner.Else.End() == branch.end:
		block, _ = owner.Else.(*ast.BlockStmt)
	}
	for _, definition := range definitions {
		if ps4008DefinitionInUnconditionalPrefix(pass, block, definition) {
			return true
		}
	}
	return false
}

// ps4008BranchPairReachesUse proves the part that a syntactic if/else pair
// alone cannot: every exited enclosing loop enters and reaches the pair.
// Without this proof, a zero-trip loop or a break/continue/goto before the
// pair leaves the incoming definition live after the loop.
func ps4008BranchPairReachesUse(pass *analysis.Pass, index *ps1006AnalysisIndex, owner *ast.IfStmt, before token.Pos) bool {
	if pass == nil || index == nil || owner == nil || !before.IsValid() || owner.End() >= before {
		return false
	}
	ownerFacts := index.functionFactsAt[owner.Pos()]
	if ownerFacts == nil || ownerFacts != index.functionFactsAt[before] {
		return false
	}
	target := owner.Pos()
	for loop := index.loopsAt[owner.Pos()]; loop != nil; loop = loop.parent {
		if !ps4008NodeInUnconditionalPrefix(pass, astutil.LoopBody(loop.node), target) {
			return false
		}
		if !ps4008LoopRangeContains(loop, before) && !ps4008LoopEntryGuaranteed(pass, loop.node) {
			return false
		}
		target = loop.node.Pos()
	}
	return ps4008NodeInUnconditionalPrefix(pass, ps4008FunctionBody(ownerFacts.root), target)
}

func ps4008FunctionBody(root ast.Node) *ast.BlockStmt {
	switch value := root.(type) {
	case *ast.FuncDecl:
		return value.Body
	case *ast.FuncLit:
		return value.Body
	default:
		return nil
	}
}

func ps4008NodeInUnconditionalPrefix(pass *analysis.Pass, block *ast.BlockStmt, target token.Pos) bool {
	if block == nil || !target.IsValid() {
		return false
	}
	for statementIndex := 0; statementIndex < len(block.List); statementIndex++ {
		statement := block.List[statementIndex]
		if statement.Pos() <= target && target <= statement.End() {
			if statement.Pos() == target {
				return true
			}
			switch value := statement.(type) {
			case *ast.BlockStmt:
				return ps4008NodeInUnconditionalPrefix(pass, value, target)
			case *ast.LabeledStmt:
				return ps4008StatementContainsTarget(pass, value.Stmt, target)
			default:
				return false
			}
		}
		if branch, ok := statement.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			labelIndex, found := ps4008ForwardGotoLabelIndex(block, statementIndex, branch.Label, target)
			if !found {
				return false
			}
			statementIndex = labelIndex - 1
			continue
		}
		if !ps4008StatementPreservesFollowingOnLivePaths(pass, statement) {
			return false
		}
	}
	return false
}

func ps4008StatementContainsTarget(pass *analysis.Pass, statement ast.Stmt, target token.Pos) bool {
	if statement == nil || !target.IsValid() || target < statement.Pos() || target > statement.End() {
		return false
	}
	if statement.Pos() == target {
		return true
	}
	switch value := statement.(type) {
	case *ast.BlockStmt:
		return ps4008NodeInUnconditionalPrefix(pass, value, target)
	case *ast.LabeledStmt:
		return ps4008StatementContainsTarget(pass, value.Stmt, target)
	default:
		return false
	}
}

func ps4008ForwardGotoLabelIndex(block *ast.BlockStmt, statementIndex int, label *ast.Ident, target token.Pos) (int, bool) {
	if block == nil || label == nil || label.Name == "" || statementIndex < 0 {
		return 0, false
	}
	for candidateIndex := statementIndex + 1; candidateIndex < len(block.List); candidateIndex++ {
		candidate, ok := block.List[candidateIndex].(*ast.LabeledStmt)
		if !ok || candidate.Label == nil || candidate.Label.Name != label.Name {
			continue
		}
		return candidateIndex, candidate.Pos() <= target
	}
	return 0, false
}

func ps4008StatementPreservesFollowingOnLivePaths(pass *analysis.Pass, statement ast.Stmt) bool {
	return ps4008StatementPreservesFollowingOnLivePathsWithRoot(pass, statement, statement)
}

func ps4008StatementPreservesFollowingOnLivePathsWithRoot(pass *analysis.Pass, statement ast.Stmt, branchRoot ast.Node) bool {
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt, *ast.IncDecStmt, *ast.ExprStmt,
		*ast.EmptyStmt, *ast.DeferStmt, *ast.GoStmt, *ast.SendStmt,
		*ast.ReturnStmt:
		return true
	case *ast.BlockStmt:
		return ps4008BlockPreservesFollowingOnLivePaths(pass, value)
	case *ast.IfStmt:
		return !ps4008ContainsEscapingBranch(pass, branchRoot)
	case *ast.ForStmt:
		return ps4008LoopPreservesFollowingOnLivePaths(pass, value, branchRoot)
	case *ast.RangeStmt:
		return ps4008LoopPreservesFollowingOnLivePaths(pass, value, branchRoot)
	case *ast.SwitchStmt:
		return !ps4008ContainsEscapingBranch(pass, branchRoot)
	case *ast.TypeSwitchStmt:
		return !ps4008ContainsEscapingBranch(pass, branchRoot)
	case *ast.SelectStmt:
		return ps4008SelectHasDefault(value) && !ps4008ContainsEscapingBranch(pass, branchRoot)
	case *ast.LabeledStmt:
		return ps4008StatementPreservesFollowingOnLivePathsWithRoot(pass, value.Stmt, branchRoot)
	default:
		return false
	}
}

func ps4008LoopPreservesFollowingOnLivePaths(pass *analysis.Pass, loop ast.Node, branchRoot ast.Node) bool {
	// An impossible loop cannot execute a transfer in its body. Guaranteed and
	// maybe-trip loops both need the same target-aware scan: only a branch that
	// leaves the loop root can bypass the following statement on a live path.
	if ps4008LoopEntryImpossible(pass, loop) {
		return true
	}
	return !ps4008ContainsEscapingBranch(pass, branchRoot)
}

func ps4008BlockPreservesFollowingOnLivePaths(pass *analysis.Pass, block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	for _, statement := range block.List {
		if !ps4008StatementPreservesFollowingOnLivePaths(pass, statement) {
			return false
		}
	}
	return true
}

func ps4008SelectHasDefault(statement *ast.SelectStmt) bool {
	if statement == nil || statement.Body == nil {
		return false
	}
	for _, rawClause := range statement.Body.List {
		clause, ok := rawClause.(*ast.CommClause)
		if ok && clause.Comm == nil {
			return true
		}
	}
	return false
}

func ps4008ContainsEscapingBranch(pass *analysis.Pass, node ast.Node) bool {
	escapes := false
	astutil.WithStack(node, func(current ast.Node, stack []ast.Node) bool {
		if escapes {
			return false
		}
		if _, nestedCallable := current.(*ast.FuncLit); nestedCallable && current != node {
			return false
		}
		if current != node && astutil.IsLoop(current) && ps4008LoopEntryImpossible(pass, current) {
			return false
		}
		branch, ok := current.(*ast.BranchStmt)
		if !ok {
			return true
		}
		if branch.Label != nil {
			switch branch.Tok {
			case token.BREAK, token.CONTINUE:
				escapes = !ps4008StackContainsLabeledBranchTarget(stack, branch.Label.Name, branch.Tok)
			default:
				// Gotos and any other labeled transfer stay conservative. A
				// goto target need not be an ancestor and therefore cannot be
				// resolved from this subtree stack alone.
				escapes = true
			}
			return !escapes
		}
		switch branch.Tok {
		case token.BREAK:
			escapes = !ps4008StackContainsBreakTarget(stack)
		case token.CONTINUE:
			escapes = !ps4008StackContainsContinueTarget(stack)
		case token.GOTO:
			escapes = true
		case token.FALLTHROUGH:
			// A valid fallthrough stays within the current switch and reaches
			// either a later terminating clause or the following statement.
		default:
			escapes = true
		}
		return !escapes
	})
	return escapes
}

func ps4008StackContainsLabeledBranchTarget(stack []ast.Node, label string, branch token.Token) bool {
	if label == "" {
		return false
	}
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		labeled, ok := stack[stackIndex].(*ast.LabeledStmt)
		if !ok || labeled.Label == nil || labeled.Label.Name != label {
			continue
		}
		switch branch {
		case token.BREAK:
			switch labeled.Stmt.(type) {
			case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
				return true
			}
		case token.CONTINUE:
			switch labeled.Stmt.(type) {
			case *ast.ForStmt, *ast.RangeStmt:
				return true
			}
		}
		return false
	}
	return false
}

func ps4008StackContainsBreakTarget(stack []ast.Node) bool {
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		switch stack[stackIndex].(type) {
		case *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.SelectStmt:
			return true
		}
	}
	return false
}

func ps4008StackContainsContinueTarget(stack []ast.Node) bool {
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		switch stack[stackIndex].(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return true
		}
	}
	return false
}

func ps4008AliasDefCanReachOnBackedge(index *ps1006AnalysisIndex, definition, before token.Pos) bool {
	if index == nil || !definition.IsValid() || !before.IsValid() {
		return false
	}
	for loop := index.loopsAt[before]; loop != nil; loop = loop.parent {
		if before < definition && loop.bodyStart <= definition && definition <= loop.bodyEnd {
			return true
		}
		if loop.postStart.IsValid() && loop.postStart <= definition && definition <= loop.postEnd {
			return true
		}
	}
	return false
}

func ps4008SkipAliasDef(pass *analysis.Pass, position, before token.Pos) bool {
	if before.IsValid() && (!position.IsValid() || position >= before) {
		return true
	}
	index := ps1006AnalysisIndexForPass(pass)
	if ps4008AliasDefinitionMayBeSkippedByExitedLoop(pass, index, position, before) {
		return true
	}
	if ps4008AliasDefCanReachOnBackedge(index, position, before) {
		return true
	}
	defStart, defEnd, defInside := ps4008ConditionalBranchRange(pass, position)
	if !defInside {
		return false
	}
	beforeStart, beforeEnd, beforeInside := ps4008ConditionalBranchRange(pass, before)
	return !beforeInside || defStart > beforeStart || defEnd < beforeEnd
}

// ps4008AliasDefinitionMayBeSkippedByExitedLoop rejects lexical "last
// definitions" that are not actually dominating definitions. A body in a
// for/range statement commonly executes zero times; after the loop, both the
// incoming value and the body assignment can therefore reach a use. The
// caller will keep the skipped definition as a conditional alternative while
// selecting the preceding definition as the dominating value.
//
// A loop definition is allowed to dominate only when entry is statically
// guaranteed and the definition is on the body's unconditional prefix. This
// preserves useful exact-one/fixed-array cases without assuming that a
// branch, break, goto, or other control construct reaches the assignment.
func ps4008AliasDefinitionMayBeSkippedByExitedLoop(pass *analysis.Pass, index *ps1006AnalysisIndex, definition, before token.Pos) bool {
	if index == nil || !definition.IsValid() || !before.IsValid() {
		return false
	}
	for loop := index.loopsAt[definition]; loop != nil; loop = loop.parent {
		if ps4008LoopRangeContains(loop, before) {
			continue
		}
		if !ps4008LoopEntryGuaranteed(pass, loop.node) || !ps4008LoopDefinitionMustExecute(pass, loop.node, definition) {
			return true
		}
	}
	return false
}

func ps4008AliasDefinitionUnreachableByLoop(pass *analysis.Pass, index *ps1006AnalysisIndex, definition token.Pos) bool {
	if index == nil || !definition.IsValid() {
		return false
	}
	for loop := index.loopsAt[definition]; loop != nil; loop = loop.parent {
		if ps4008LoopEntryImpossible(pass, loop.node) {
			return true
		}
	}
	return false
}

func ps4008LoopRangeContains(loop *ps1006LoopRange, position token.Pos) bool {
	if loop == nil || !position.IsValid() {
		return false
	}
	if loop.bodyStart <= position && position <= loop.bodyEnd {
		return true
	}
	if loop.postStart.IsValid() && loop.postStart <= position && position <= loop.postEnd {
		return true
	}
	return loop.iterationStart.IsValid() && loop.iterationStart <= position && position <= loop.iterationEnd
}

func ps4008LoopEntryGuaranteed(pass *analysis.Pass, node ast.Node) bool {
	enters, known := ps4008LoopEntryKnown(pass, node)
	return known && enters
}

func ps4008LoopEntryImpossible(pass *analysis.Pass, node ast.Node) bool {
	enters, known := ps4008LoopEntryKnown(pass, node)
	return known && !enters
}

func ps4008LoopEntryKnown(pass *analysis.Pass, node ast.Node) (bool, bool) {
	if pass == nil || pass.TypesInfo == nil || node == nil {
		return false, false
	}
	switch loop := node.(type) {
	case *ast.ForStmt:
		if loop.Cond == nil {
			return true, true
		}
		if value, known := ps1006BoolConstant(pass, loop.Cond); known {
			return value, true
		}
		initialization, ok := loop.Init.(*ast.AssignStmt)
		if !ok || initialization.Tok != token.DEFINE && initialization.Tok != token.ASSIGN ||
			len(initialization.Lhs) != 1 || len(initialization.Rhs) != 1 {
			return false, false
		}
		variable, ok := ps2110Unparen(initialization.Lhs[0]).(*ast.Ident)
		if !ok {
			return false, false
		}
		condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
		if !ok {
			return false, false
		}
		left, ok := ps2110Unparen(condition.X).(*ast.Ident)
		if !ok || identObject(pass, left) != identObject(pass, variable) {
			return false, false
		}
		initial := pass.TypesInfo.Types[initialization.Rhs[0]].Value
		bound := pass.TypesInfo.Types[condition.Y].Value
		if initial == nil || bound == nil {
			return false, false
		}
		switch condition.Op {
		case token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ:
			return constant.Compare(initial, condition.Op, bound), true
		}
	case *ast.RangeStmt:
		typ := pass.TypesInfo.TypeOf(loop.X)
		if typ != nil {
			underlying := types.Unalias(typ).Underlying()
			if pointer, ok := underlying.(*types.Pointer); ok {
				underlying = types.Unalias(pointer.Elem()).Underlying()
			}
			if array, ok := underlying.(*types.Array); ok {
				return array.Len() > 0, true
			}
		}
		if value := pass.TypesInfo.Types[loop.X].Value; value != nil {
			switch value.Kind() {
			case constant.Int:
				return constant.Sign(value) > 0, true
			case constant.String:
				return len(constant.StringVal(value)) > 0, true
			}
		}
		literal, ok := ps2110Unparen(loop.X).(*ast.CompositeLit)
		if ok {
			_, slice := types.Unalias(pass.TypesInfo.TypeOf(literal)).Underlying().(*types.Slice)
			if slice {
				return len(literal.Elts) > 0, true
			}
		}
	}
	return false, false
}

func ps4008LoopDefinitionMustExecute(pass *analysis.Pass, node ast.Node, definition token.Pos) bool {
	if node == nil || !definition.IsValid() {
		return false
	}
	if loop, ok := node.(*ast.RangeStmt); ok {
		if loop.Key != nil && loop.Key.Pos() <= definition && definition <= loop.Key.End() ||
			loop.Value != nil && loop.Value.Pos() <= definition && definition <= loop.Value.End() {
			return true
		}
	}
	if loop, ok := node.(*ast.ForStmt); ok && loop.Post != nil &&
		loop.Post.Pos() <= definition && definition <= loop.Post.End() {
		return ps4008BlockFallsThrough(loop.Body)
	}
	return ps4008DefinitionInUnconditionalPrefix(pass, astutil.LoopBody(node), definition)
}

func ps4008BlockFallsThrough(block *ast.BlockStmt) bool {
	if block == nil {
		return false
	}
	for _, statement := range block.List {
		switch value := statement.(type) {
		case *ast.AssignStmt, *ast.DeclStmt, *ast.IncDecStmt, *ast.ExprStmt,
			*ast.EmptyStmt, *ast.DeferStmt, *ast.GoStmt, *ast.SendStmt:
		case *ast.BlockStmt:
			if !ps4008BlockFallsThrough(value) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func ps4008DefinitionInUnconditionalPrefix(pass *analysis.Pass, block *ast.BlockStmt, definition token.Pos) bool {
	if block == nil {
		return false
	}
	for _, statement := range block.List {
		if statement.Pos() <= definition && definition <= statement.End() {
			switch value := statement.(type) {
			case *ast.AssignStmt, *ast.DeclStmt, *ast.IncDecStmt:
				return true
			case *ast.BlockStmt:
				return ps4008DefinitionInUnconditionalPrefix(pass, value, definition)
			default:
				return false
			}
		}
		if !ps4008StatementPreservesFollowingOnLivePaths(pass, statement) {
			return false
		}
	}
	return false
}

func ps4008ConditionalBranchRange(pass *analysis.Pass, position token.Pos) (token.Pos, token.Pos, bool) {
	if !position.IsValid() {
		return token.NoPos, token.NoPos, false
	}
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil {
		return token.NoPos, token.NoPos, false
	}
	branch, ok := index.branchAt[position]
	return branch.start, branch.end, ok
}

func ps4008SelectorKey(pass *analysis.Pass, expression ast.Expr) (ps1006SelectorKey, bool) {
	var reversed []string
	for {
		expression = ps2110Unparen(expression)
		switch value := expression.(type) {
		case *ast.StarExpr:
			expression = value.X
		case *ast.IndexExpr:
			reversed = append(reversed, "[]")
			expression = value.X
		case *ast.IndexListExpr:
			reversed = append(reversed, "[]")
			expression = value.X
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if selection == nil {
				return ps1006SelectorKey{}, false
			}
			reversed = append(reversed, ps1006ObjectKey(pass, selection.Obj(), value.Sel.Name))
			expression = value.X
		case *ast.Ident:
			root := identObject(pass, value)
			if root == nil || len(reversed) == 0 {
				return ps1006SelectorKey{}, false
			}
			slices.Reverse(reversed)
			return ps1006SelectorKey{root: root, path: strings.Join(reversed, "/")}, true
		default:
			return ps1006SelectorKey{}, false
		}
	}
}

func ps4008ConstantIndexKey(pass *analysis.Pass, expression *ast.IndexExpr) (ps1006SelectorKey, bool) {
	if pass == nil || pass.TypesInfo == nil || expression == nil {
		return ps1006SelectorKey{}, false
	}
	typ := pass.TypesInfo.TypeOf(expression.X)
	if typ == nil {
		return ps1006SelectorKey{}, false
	}
	array, ok := types.Unalias(typ).Underlying().(*types.Array)
	if !ok || !ps4008TypeMayCarryCallable(array.Elem(), make(map[types.Type]bool)) {
		return ps1006SelectorKey{}, false
	}
	indexValue := pass.TypesInfo.Types[expression.Index].Value
	if indexValue == nil {
		return ps1006SelectorKey{}, false
	}
	index, ok := constant.Int64Val(indexValue)
	if !ok || index < 0 || index >= array.Len() {
		return ps1006SelectorKey{}, false
	}
	identifier, ok := ps2110Unparen(expression.X).(*ast.Ident)
	if !ok {
		return ps1006SelectorKey{}, false
	}
	root := identObject(pass, identifier)
	if root == nil {
		return ps1006SelectorKey{}, false
	}
	return ps1006SelectorKey{root: root, path: "$index:" + strconv.FormatInt(index, 10)}, true
}

func ps4008PossibleConstantIndexValues(pass *analysis.Pass, expression *ast.IndexExpr, before token.Pos) ([]ast.Expr, bool) {
	key, ok := ps4008ConstantIndexKey(pass, expression)
	if !ok {
		return nil, false
	}
	return ps4008PossibleConstantIndexValuesSeen(pass, key, before, make(map[types.Object]bool))
}

func ps4008PossibleConstantIndexValuesSeen(pass *analysis.Pass, key ps1006SelectorKey, before token.Pos, seen map[types.Object]bool) ([]ast.Expr, bool) {
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil || key.root == nil || seen[key.root] {
		return nil, false
	}
	seen[key.root] = true
	definitions := index.selectorDefs[key]
	best, haveBest := ps4008BestDefinition(pass, definitions, before)
	rootDefinition, haveRootDefinition := ps4008BestAliasDefinition(pass, key.root, before)
	rootShadowsBackedge := haveRootDefinition &&
		!ps4008IsPointerType(key.root.Type()) &&
		ps4008TypeMayCarryCallable(key.root.Type(), make(map[types.Type]bool)) &&
		ps4008AliasDefinitionShadowsBackedge(index, rootDefinition.position, before)
	values := make([]ast.Expr, 0, 2)
	selectedDefinitions := make([]ps1006AliasDef, 0, 2)
	if haveBest {
		values = append(values, best.value)
		selectedDefinitions = append(selectedDefinitions, best)
	}
	for _, definition := range definitions {
		if haveBest && definition.position == best.position {
			continue
		}
		if ps4008AliasDefinitionUnreachableByLoop(pass, index, definition.position) {
			continue
		}
		if !rootShadowsBackedge && ps4008AliasDefCanReachOnBackedge(index, definition.position, before) ||
			before.IsValid() && definition.position < before && (!haveBest || definition.position > best.position) {
			values = append(values, definition.value)
			selectedDefinitions = append(selectedDefinitions, definition)
		}
	}
	if haveBest {
		for _, definition := range selectedDefinitions {
			if !ps4008RootProvenanceStable(index, key, definition.position, before) {
				return nil, false
			}
		}
		return values, true
	}
	rootDefinitions := ps4008PossibleAliasDefinitions(pass, key.root, before)
	rootResolved := len(rootDefinitions) != 0
	for _, definition := range rootDefinitions {
		if value, ok := ps4008CompositeIndexValue(pass, definition.value, key.path); ok {
			if !ps4008RootProvenanceStable(index, key, definition.position, before) {
				rootResolved = false
				continue
			}
			values = append(values, value)
			continue
		}
		identifier, ok := ps2110Unparen(definition.value).(*ast.Ident)
		if !ok {
			rootResolved = false
			continue
		}
		root := identObject(pass, identifier)
		nested, dominating := ps4008PossibleConstantIndexValuesSeen(pass, ps1006SelectorKey{root: root, path: key.path}, definition.position, maps.Clone(seen))
		values = append(values, nested...)
		rootResolved = rootResolved && dominating
	}
	return values, rootResolved
}

func ps4008CompositeIndexValue(pass *analysis.Pass, expression ast.Expr, path string) (ast.Expr, bool) {
	if pass == nil || pass.TypesInfo == nil || expression == nil || !strings.HasPrefix(path, "$index:") {
		return nil, false
	}
	wanted, err := strconv.ParseInt(strings.TrimPrefix(path, "$index:"), 10, 64)
	if err != nil || wanted < 0 {
		return nil, false
	}
	literal, ok := ps2110Unparen(expression).(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	typ := pass.TypesInfo.TypeOf(literal)
	if typ == nil {
		return nil, false
	}
	if _, ok := types.Unalias(typ).Underlying().(*types.Array); !ok {
		return nil, false
	}
	next := int64(0)
	for _, element := range literal.Elts {
		value := element
		current := next
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			keyValue := pass.TypesInfo.Types[keyed.Key].Value
			if keyValue == nil {
				return nil, false
			}
			key, exact := constant.Int64Val(keyValue)
			if !exact || key < 0 {
				return nil, false
			}
			current = key
			value = keyed.Value
		}
		if current == wanted {
			return value, true
		}
		next = current + 1
	}
	return nil, false
}

func ps4008PointerObjectTargets(pass *analysis.Pass, expression ast.Expr, before token.Pos, seen map[types.Object]bool) ([]types.Object, bool) {
	if expression == nil {
		return nil, false
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op != token.AND {
			return nil, false
		}
		identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
		if !ok {
			return nil, false
		}
		object := identObject(pass, identifier)
		return []types.Object{object}, object != nil
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if fun, ok := pass.TypesInfo.Types[value.Fun]; ok && fun.IsType() {
				return ps4008PointerObjectTargets(pass, value.Args[0], before, seen)
			}
		}
		return nil, false
	case *ast.StarExpr:
		containers, ok := ps4008PointerObjectTargets(pass, value.X, before, seen)
		if !ok {
			return nil, false
		}
		return ps4008PointerObjectsFromAliases(pass, containers, before, seen)
	case *ast.Ident:
		object := identObject(pass, value)
		if object == nil || seen[object] {
			return nil, false
		}
		seen[object] = true
		return ps4008PointerObjectsFromAliases(pass, []types.Object{object}, before, seen)
	case *ast.SelectorExpr:
		selectorTargets := make(map[types.Object]bool)
		if key, ok := ps4008SelectorKey(pass, value); ok {
			values, dominating := ps4008PossibleSelectorValues(pass, key, before)
			if len(values) != 0 {
				resolved := true
				for _, expression := range values {
					nested, ok := ps4008PointerObjectTargets(pass, expression, before, maps.Clone(seen))
					resolved = resolved && ok
					for _, target := range nested {
						selectorTargets[target] = true
					}
				}
				if dominating {
					result := make([]types.Object, 0, len(selectorTargets))
					for target := range selectorTargets {
						result = append(result, target)
					}
					return result, resolved && len(result) != 0
				}
			}
		}
		objects := ps4008MentionedAliasObjects(pass, value, before, seen)
		for _, object := range objects {
			selectorTargets[object] = true
		}
		result := make([]types.Object, 0, len(selectorTargets))
		for target := range selectorTargets {
			result = append(result, target)
		}
		return result, len(result) != 0
	case *ast.IndexExpr:
		objects := ps4008MentionedAliasObjects(pass, value, before, seen)
		return objects, len(objects) != 0
	}
	return nil, false
}

func ps4008PointerObjectsFromAliases(pass *analysis.Pass, aliases []types.Object, before token.Pos, seen map[types.Object]bool) ([]types.Object, bool) {
	targets := make(map[types.Object]bool)
	resolved := len(aliases) != 0
	for _, alias := range aliases {
		values := ps4008PossibleAliasValues(pass, alias, before)
		if len(values) == 0 {
			resolved = false
			continue
		}
		for _, value := range values {
			nestedSeen := maps.Clone(seen)
			nested, ok := ps4008PointerObjectTargets(pass, value, before, nestedSeen)
			if !ok {
				resolved = false
			}
			for _, target := range nested {
				targets[target] = true
			}
		}
	}
	result := make([]types.Object, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	return result, resolved && len(result) != 0
}

func ps4008MentionedAliasObjects(pass *analysis.Pass, expression ast.Expr, before token.Pos, seen map[types.Object]bool) []types.Object {
	targets := make(map[types.Object]bool)
	for object := range ps1006MentionedObjects(pass, expression) {
		if object == nil || seen[object] {
			continue
		}
		seen[object] = true
		for _, value := range ps4008PossibleAliasValues(pass, object, before) {
			ps4008CollectAddressedObjectsDeep(pass, value, before, seen, targets)
		}
	}
	result := make([]types.Object, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	return result
}

func ps4008CollectAddressedObjectsDeep(pass *analysis.Pass, expression ast.Expr, before token.Pos, seen map[types.Object]bool, targets map[types.Object]bool) {
	if expression == nil {
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				for object := range ps1006MentionedObjects(pass, value.X) {
					if object != nil {
						targets[object] = true
					}
				}
				return false
			}
		case *ast.Ident:
			object := identObject(pass, value)
			if object == nil || seen[object] {
				return true
			}
			seen[object] = true
			for _, nested := range ps4008PossibleAliasValues(pass, object, before) {
				ps4008CollectAddressedObjectsDeep(pass, nested, before, seen, targets)
			}
		}
		return true
	})
}

func ps4008AddressedDerivedTargetsDeep(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, before token.Pos) []types.Object {
	if expression == nil {
		return nil
	}
	addressed := make(map[types.Object]bool)
	ps4008CollectAddressedObjectsDeep(pass, expression, before, make(map[types.Object]bool), addressed)
	targets := make(map[types.Object]bool)
	for object := range addressed {
		if deps[object] {
			targets[object] = true
		}
	}
	result := make([]types.Object, 0, len(targets))
	for object := range targets {
		result = append(result, object)
	}
	return result
}

func ps4008InvalidateDerivedDepsInExpr(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool) {
	if expression == nil {
		return
	}
	ps4008InvalidateDerivedDeps(pass, &ast.ExprStmt{X: expression}, deps)
}

func ps4008ApplyDerivedDependencyStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool) {
	if statement == nil {
		return
	}
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, statement, inner, deps)
	case *ast.ExprStmt:
		ps4008ApplyExpressionDependencyStatement(pass, value, inner, deps)
	default:
		ps4008InvalidateDerivedDeps(pass, statement, deps)
	}
}

func ps4008ApplyExpressionDependencyStatement(pass *analysis.Pass, statement *ast.ExprStmt, inner types.Object, deps map[types.Object]bool) {
	ps4008ApplyExpressionDependencyStatementWithAliases(pass, statement, inner, deps, nil)
}

func ps4008ApplyExpressionDependencyStatementWithAliases(pass *analysis.Pass, statement *ast.ExprStmt, inner types.Object, deps map[types.Object]bool, pointerAliases map[types.Object]ps1006OrderedPointerAlias) {
	if statement == nil || statement.X == nil {
		return
	}
	state := ps1006DependencyState{deps: deps, strideDeps: make(map[types.Object]string), pointerAliases: pointerAliases}
	ps1006ApplyCaseExpression(pass, statement.X, inner, &state)
}

func ps4008KillPointerAliasTargets(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, kill map[types.Object]bool, before token.Pos, mode ps4008CallableEffectMode) bool {
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return false
	}
	if _, ok := typ.Underlying().(*types.Pointer); !ok {
		return false
	}
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		if mode == ps4008MayWrite {
			for object := range deps {
				kill[object] = true
			}
		}
		return true
	}
	if mode == ps4008DefiniteWrite {
		allTargets, allResolved := ps4008PointerObjectTargets(pass, expression, before, make(map[types.Object]bool))
		if !allResolved || len(allTargets) != 1 {
			return true
		}
	}
	alias := identObject(pass, identifier)
	targets, resolved := ps4008PointerAliasTargetsResolved(pass, alias, deps, before, make(map[types.Object]bool))
	if !resolved {
		if mode == ps4008MayWrite {
			for object := range deps {
				kill[object] = true
			}
		}
		return true
	}
	// One runtime dereference writes one runtime target. For a dependency-kill
	// transfer, a set of multiple possible aliases is therefore a may-write to
	// each member, not a definite overwrite of all of them. Tile-safety scans
	// still invalidate every possible target.
	if deps[alias] {
		kill[alias] = true
	}
	for _, target := range targets {
		if deps[target] {
			kill[target] = true
		}
	}
	return true
}

func ps4008PointerAliasTargetsResolved(pass *analysis.Pass, alias types.Object, deps map[types.Object]bool, before token.Pos, seen map[types.Object]bool) ([]types.Object, bool) {
	if alias == nil || seen[alias] {
		return nil, false
	}
	seen[alias] = true
	values := ps4008PossibleAliasValues(pass, alias, before)
	if len(values) == 0 {
		return nil, false
	}
	targetSet := make(map[types.Object]bool)
	resolved := true
	for _, expression := range values {
		if expression == nil {
			resolved = false
			continue
		}
		if targets, ok := ps4008AddressedAliasTargets(pass, expression, deps); ok {
			for _, target := range targets {
				targetSet[target] = true
			}
			continue
		}
		if conversion, ok := ps2110Unparen(expression).(*ast.CallExpr); ok && len(conversion.Args) == 1 && !conversion.Ellipsis.IsValid() {
			if fun, exists := pass.TypesInfo.Types[conversion.Fun]; exists && fun.IsType() {
				if targets, ok := ps4008AddressedAliasTargets(pass, conversion.Args[0], deps); ok {
					for _, target := range targets {
						targetSet[target] = true
					}
					continue
				}
			}
		}
		identifier, ok := ps2110Unparen(expression).(*ast.Ident)
		if !ok {
			resolved = false
			continue
		}
		nestedSeen := make(map[types.Object]bool, len(seen)+1)
		for object := range seen {
			nestedSeen[object] = true
		}
		nested, ok := ps4008PointerAliasTargetsResolved(pass, identObject(pass, identifier), deps, before, nestedSeen)
		if !ok {
			resolved = false
		}
		for _, target := range nested {
			targetSet[target] = true
		}
	}
	targets := make([]types.Object, 0, len(targetSet))
	for target := range targetSet {
		targets = append(targets, target)
	}
	return targets, resolved
}

func ps4008AddressedAliasTargets(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool) ([]types.Object, bool) {
	unary, ok := ps2110Unparen(expression).(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return nil, false
	}
	targets := make(map[types.Object]bool)
	ps4008CollectMentionedDeps(pass, unary.X, deps, targets)
	result := make([]types.Object, 0, len(targets))
	for object := range targets {
		result = append(result, object)
	}
	return result, true
}

func ps4008KillCallableWriteTargetsMode(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, kill map[types.Object]bool, before token.Pos, mode ps4008CallableEffectMode) bool {
	targets, known := ps4008CallableWriteTargetsMode(pass, expression, deps, before, make(map[types.Object]bool), mode)
	if !known {
		return false
	}
	for target := range targets {
		kill[target] = true
	}
	return true
}

func ps4008CallableWriteTargets(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, before token.Pos, seen map[types.Object]bool) (map[types.Object]bool, bool) {
	return ps4008CallableWriteTargetsMode(pass, expression, deps, before, seen, ps4008MayWrite)
}

func ps4008CallableWriteTargetsMode(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, before token.Pos, seen map[types.Object]bool, mode ps4008CallableEffectMode) (map[types.Object]bool, bool) {
	targets := make(map[types.Object]bool)
	merge := func(values []ast.Expr) bool {
		if len(values) == 0 {
			return false
		}
		known := true
		for valueIndex, value := range values {
			nested, ok := ps4008CallableWriteTargetsMode(pass, value, deps, before, maps.Clone(seen), mode)
			known = known && ok
			if mode == ps4008MayWrite || valueIndex == 0 {
				for target := range nested {
					targets[target] = true
				}
				continue
			}
			if mode == ps4008DefiniteWrite {
				for target := range targets {
					if !nested[target] {
						delete(targets, target)
					}
				}
			}
		}
		return known
	}

	switch value := ps2110Unparen(expression).(type) {
	case *ast.FuncLit:
		for target := range ps4008CallableBodyWriteTargetsMode(pass, value.Body, deps, before, mode) {
			targets[target] = true
		}
		return targets, true
	case *ast.Ident:
		alias := identObject(pass, value)
		if alias == nil || seen[alias] {
			return nil, false
		}
		if function, declared := alias.(*types.Func); declared {
			for target := range ps4008DeclaredCallableWriteTargetsMode(pass, function, deps, before, mode) {
				targets[target] = true
			}
			// A declared function cannot capture caller-local candidates, but a
			// local declaration can share package-level objects with its caller.
			// Writes through actual arguments are still handled at the call site.
			return targets, true
		}
		seen[alias] = true
		definitions := ps4008PossibleAliasDefinitions(pass, alias, before)
		if len(definitions) == 0 {
			return nil, false
		}
		known := true
		for definitionIndex, definition := range definitions {
			var nested map[types.Object]bool
			var ok bool
			if definition.resultSelection {
				ordered, resolved := ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{
					expression:  definition.value,
					resultIndex: definition.resultIndex, resultSelection: true,
				}, definition.position, maps.Clone(seen))
				nested = make(map[types.Object]bool)
				ok = resolved && len(ordered) != 0
				for syntaxIndex := range ordered {
					syntax := &ordered[syntaxIndex]
					bodyTargets := ps4008CallableBodyWriteTargetsMode(pass, syntax.syntax.body, deps, definition.position, mode)
					if mode == ps4008MayWrite || syntaxIndex == 0 {
						for target := range bodyTargets {
							nested[target] = true
						}
						continue
					}
					for target := range nested {
						if !bodyTargets[target] {
							delete(nested, target)
						}
					}
				}
			} else {
				nested, ok = ps4008CallableWriteTargetsMode(pass, definition.value, deps, definition.position, maps.Clone(seen), mode)
			}
			known = known && ok
			if mode == ps4008MayWrite || definitionIndex == 0 {
				for target := range nested {
					targets[target] = true
				}
				continue
			}
			for target := range targets {
				if !nested[target] {
					delete(targets, target)
				}
			}
		}
		return targets, known
	case *ast.SelectorExpr:
		function, _ := identObject(pass, value.Sel).(*types.Func)
		for target := range ps4008DeclaredCallableWriteTargetsMode(pass, function, deps, before, mode) {
			targets[target] = true
		}
		selection := pass.TypesInfo.Selections[value]
		if selection == nil {
			if function != nil {
				return targets, true
			}
			return nil, false
		}
		if selection.Kind() == types.MethodVal {
			if ps4008MethodValueReceiverWrites(pass, value, before, mode) {
				ps4008CollectReceiverMutationTargets(pass, value, deps, before, targets, mode)
			}
			return targets, true
		}
		if selection.Kind() == types.MethodExpr {
			// A method expression is an ordinary function whose first argument is
			// the receiver. Receiver writes are therefore accounted for by the
			// argument-effect summary at the call site, not as captured state.
			return targets, true
		}
		if key, ok := ps4008SelectorKey(pass, value); ok {
			values, dominating := ps4008PossibleSelectorValues(pass, key, before)
			if !dominating {
				index := ps1006AnalysisIndexForPass(pass)
				// A known root definition whose field provenance became unstable
				// (escape, reassignment, or opaque mutation) must remain unknown.
				// Scanning its old aggregate syntax would resurrect a callback that
				// no longer definitely occupies this instance's field.
				if index == nil || len(index.aliasDefs[key.root]) == 0 {
					values = append(values, ps4008AggregateCallableValues(pass, value.X, before)...)
				}
			}
			return targets, merge(values)
		}
	case *ast.IndexExpr:
		if typ := pass.TypesInfo.TypeOf(value.X); typ != nil {
			if _, signature := typ.Underlying().(*types.Signature); signature {
				return ps4008CallableWriteTargetsMode(pass, value.X, deps, before, maps.Clone(seen), mode)
			}
		}
		if values, dominating := ps4008PossibleConstantIndexValues(pass, value, before); dominating {
			return targets, merge(values)
		}
		return targets, merge(ps4008AggregateCallableValues(pass, value.X, before))
	case *ast.IndexListExpr:
		return ps4008CallableWriteTargetsMode(pass, value.X, deps, before, maps.Clone(seen), mode)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps4008CallableWriteTargetsMode(pass, value.Args[0], deps, before, maps.Clone(seen), mode)
			}
		}
	case *ast.TypeAssertExpr:
		return ps4008CallableWriteTargetsMode(pass, value.X, deps, before, maps.Clone(seen), mode)
	}
	if values := ps4008AggregateCallableValues(pass, expression, before); len(values) > 0 {
		return targets, merge(values)
	}
	return nil, false
}

func ps4008PossibleSelectorValues(pass *analysis.Pass, key ps1006SelectorKey, before token.Pos) ([]ast.Expr, bool) {
	return ps4008PossibleSelectorValuesSeen(pass, key, before, make(map[types.Object]bool))
}

func ps4008PossibleSelectorValuesSeen(pass *analysis.Pass, key ps1006SelectorKey, before token.Pos, seen map[types.Object]bool) ([]ast.Expr, bool) {
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil || key.root == nil || seen[key.root] {
		return nil, false
	}
	seen[key.root] = true
	definitions := index.selectorDefs[key]
	best, haveBest := ps4008BestDefinition(pass, definitions, before)
	rootDefinition, haveRootDefinition := ps4008BestAliasDefinition(pass, key.root, before)
	rootShadowsBackedge := haveRootDefinition &&
		!ps4008IsPointerType(key.root.Type()) &&
		ps4008TypeMayCarryCallable(key.root.Type(), make(map[types.Type]bool)) &&
		ps4008AliasDefinitionShadowsBackedge(index, rootDefinition.position, before)
	values := make([]ast.Expr, 0, 2)
	selectedDefinitions := make([]ps1006AliasDef, 0, 2)
	if haveBest {
		values = append(values, best.value)
		selectedDefinitions = append(selectedDefinitions, best)
	}
	for _, definition := range definitions {
		if haveBest && definition.position == best.position {
			continue
		}
		if ps4008AliasDefinitionUnreachableByLoop(pass, index, definition.position) {
			continue
		}
		if !rootShadowsBackedge && ps4008AliasDefCanReachOnBackedge(index, definition.position, before) ||
			before.IsValid() && definition.position < before && (!haveBest || definition.position > best.position) {
			values = append(values, definition.value)
			selectedDefinitions = append(selectedDefinitions, definition)
		}
	}
	if haveBest {
		for _, definition := range selectedDefinitions {
			if !ps4008RootProvenanceStable(index, key, definition.position, before) {
				return nil, false
			}
		}
		return values, true
	}
	// A copied aggregate retains the selected callable/pointer field. Follow
	// dominating root-object copies so a field assignment on the source
	// aggregate remains visible through the copy.
	rootDefinitions := ps4008PossibleAliasDefinitions(pass, key.root, before)
	rootResolved := len(rootDefinitions) != 0
	for _, definition := range rootDefinitions {
		if compositeValue, ok := ps4008CompositeSelectorValue(pass, definition.value, key.path); ok {
			if !ps4008RootProvenanceStable(index, key, definition.position, before) {
				rootResolved = false
				continue
			}
			values = append(values, compositeValue)
			continue
		}
		identifier, ok := ps2110Unparen(definition.value).(*ast.Ident)
		if !ok {
			rootResolved = false
			continue
		}
		root := identObject(pass, identifier)
		// Aggregate assignment copies the selected field at this definition;
		// later source-field updates must not retroactively change the copy.
		nested, dominating := ps4008PossibleSelectorValuesSeen(pass, ps1006SelectorKey{root: root, path: key.path}, definition.position, maps.Clone(seen))
		values = append(values, nested...)
		rootResolved = rootResolved && dominating
	}
	return values, rootResolved
}

func ps4008RootProvenanceStable(index *ps1006AnalysisIndex, key ps1006SelectorKey, definition, before token.Pos) bool {
	if index == nil || key.root == nil || !definition.IsValid() || !before.IsValid() || definition >= before {
		return false
	}
	facts := index.functionFactsAt[before]
	if facts == nil {
		return false
	}
	positions := facts.localUnsafe[key.root]
	position, _ := slices.BinarySearch(positions, definition+1)
	for ; position < len(positions) && positions[position] < before; position++ {
		unsafe := positions[position]
		trackedSelectorWrite := false
		for _, selectorDefinition := range index.selectorDefs[key] {
			if selectorDefinition.position == unsafe {
				trackedSelectorWrite = true
				break
			}
		}
		if !trackedSelectorWrite {
			return false
		}
	}
	return true
}

func ps4008CompositeSelectorValue(pass *analysis.Pass, expression ast.Expr, path string) (ast.Expr, bool) {
	if pass == nil || pass.TypesInfo == nil || expression == nil || path == "" {
		return nil, false
	}
	expression = ps2110Unparen(expression)
	if address, ok := expression.(*ast.UnaryExpr); ok && address.Op == token.AND {
		expression = ps2110Unparen(address.X)
	}
	literal, ok := expression.(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	typ := types.Unalias(pass.TypesInfo.TypeOf(literal))
	if typ == nil {
		return nil, false
	}
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	structure, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, false
	}
	segment, remainder, _ := strings.Cut(path, "/")
	fieldIndex := -1
	for index := 0; index < structure.NumFields(); index++ {
		field := structure.Field(index)
		if ps1006ObjectKey(pass, field, field.Name()) == segment {
			fieldIndex = index
			break
		}
	}
	if fieldIndex < 0 {
		return nil, false
	}
	var selected ast.Expr
	positionalIndex := 0
	for _, element := range literal.Elts {
		if keyed, ok := element.(*ast.KeyValueExpr); ok {
			identifier, ok := ps2110Unparen(keyed.Key).(*ast.Ident)
			if ok && ps1006ObjectKey(pass, identObject(pass, identifier), identifier.Name) == segment {
				selected = keyed.Value
				break
			}
			continue
		}
		if positionalIndex == fieldIndex {
			selected = element
			break
		}
		positionalIndex++
	}
	if selected == nil {
		return nil, false
	}
	if remainder == "" {
		return selected, true
	}
	return ps4008CompositeSelectorValue(pass, selected, remainder)
}

func ps4008BestDefinition(pass *analysis.Pass, definitions []ps1006AliasDef, before token.Pos) (ps1006AliasDef, bool) {
	position := len(definitions)
	if before.IsValid() {
		position, _ = slices.BinarySearchFunc(definitions, before, func(definition ps1006AliasDef, target token.Pos) int {
			return cmp.Compare(definition.position, target)
		})
	}
	for definitionIndex := position - 1; definitionIndex >= 0; definitionIndex-- {
		definition := definitions[definitionIndex]
		if !ps4008SkipAliasDef(pass, definition.position, before) {
			return definition, true
		}
	}
	return ps1006AliasDef{}, false
}

func ps4008AggregateCallableValues(pass *analysis.Pass, expression ast.Expr, before token.Pos) []ast.Expr {
	var values []ast.Expr
	seen := make(map[types.Object]bool)
	var inspect func(ast.Expr)
	inspect = func(current ast.Expr) {
		if current == nil {
			return
		}
		ast.Inspect(current, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.FuncLit:
				values = append(values, value)
				return false
			case *ast.SelectorExpr:
				if selection := pass.TypesInfo.Selections[value]; selection != nil && selection.Kind() == types.MethodVal {
					values = append(values, value)
					return false
				}
			case *ast.Ident:
				object := identObject(pass, value)
				if object == nil || seen[object] {
					return true
				}
				seen[object] = true
				for _, nested := range ps4008PossibleAliasValues(pass, object, before) {
					inspect(nested)
				}
			}
			return true
		})
	}
	inspect(expression)
	return values
}

func ps4008CollectReceiverMutationTargets(pass *analysis.Pass, selector *ast.SelectorExpr, deps map[types.Object]bool, before token.Pos, targets map[types.Object]bool, mode ps4008CallableEffectMode) {
	if !ps4008ReceiverMayMutateReferencedStorage(pass, selector) {
		return
	}
	if ps4008IsPointerType(pass.TypesInfo.TypeOf(selector.X)) && ps4008KillPointerAliasTargets(pass, selector.X, deps, targets, before, mode) {
		return
	}
	ps4008KillAggregateAliasTargets(pass, selector.X, deps, targets, before)
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return
	}
	signature, _ := selection.Obj().Type().(*types.Signature)
	if signature == nil || signature.Recv() == nil {
		return
	}
	if _, pointer := signature.Recv().Type().(*types.Pointer); pointer {
		ps4008CollectMentionedDeps(pass, selector.X, deps, targets)
	}
}

func ps4008DeclaredCallableWriteTargetsMode(pass *analysis.Pass, function *types.Func, deps map[types.Object]bool, before token.Pos, mode ps4008CallableEffectMode) map[types.Object]bool {
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil || function == nil {
		return nil
	}
	declaration := index.functionDeclarations[function]
	if declaration == nil {
		return nil
	}
	// Only package-level candidates can be referenced directly by a declared
	// body. Caller locals flow exclusively through receivers and arguments and
	// are summarized at the call site; including them here would make an
	// unresolved formal callback look as if it captured every caller local.
	packageDeps := make(map[types.Object]bool)
	for object := range deps {
		if object != nil && pass != nil && pass.Pkg != nil && object.Parent() == pass.Pkg.Scope() {
			packageDeps[object] = true
		}
	}
	return ps4008CallableBodyWriteTargetsMode(pass, declaration.Body, packageDeps, before, mode)
}

func ps4008CallableBodyWriteTargetsMode(pass *analysis.Pass, body *ast.BlockStmt, deps map[types.Object]bool, before token.Pos, mode ps4008CallableEffectMode) map[types.Object]bool {
	if body == nil {
		return nil
	}
	if !before.IsValid() || before < body.End() {
		// A declaration may appear after its call site. Its own local alias
		// definitions must nevertheless be visible while summarizing the body.
		before = body.End()
	}
	index := ps1006AnalysisIndexForPass(pass)
	cacheKey := ps4008CallableEffectKey{body: body, before: before, deps: ps4008BoolObjectSetKey(deps), mode: mode}
	if index != nil {
		if cached, ok := index.callableEffects[cacheKey]; ok {
			return cloneObjectBoolMap(cached)
		}
		if index.activeCallableBodies[body] {
			if mode == ps4008MayWrite {
				return cloneObjectBoolMap(deps)
			}
			return nil
		}
		index.activeCallableBodies[body] = true
		index.callableVisits++
		defer delete(index.activeCallableBodies, body)
	}
	graph := psAccumulatorGraph{
		universe:     cloneObjectBoolMap(deps),
		invalid:      make(map[types.Object]bool),
		edges:        make(map[types.Object]map[types.Object]bool),
		effectBefore: before,
	}
	if mode == ps4008MayWrite {
		targets := ps4008CallableMayWriteTargetsCFG(pass, body, graph)
		if index != nil {
			index.callableEffects[cacheKey] = cloneObjectBoolMap(targets)
		}
		return targets
	}
	targets := ps4008CallableDefiniteWriteTargetsCFG(pass, body, deps, before)
	if index != nil {
		index.callableEffects[cacheKey] = cloneObjectBoolMap(targets)
	}
	return targets
}

func ps4008CallableDefiniteWriteTargetsCFG(pass *analysis.Pass, body *ast.BlockStmt, candidates map[types.Object]bool, before token.Pos) map[types.Object]bool {
	if body == nil || len(candidates) == 0 {
		return nil
	}
	control := cfg.New(body, func(*ast.CallExpr) bool { return true })
	if control == nil || len(control.Blocks) == 0 {
		return nil
	}
	type blockState struct {
		initialized bool
		written     map[types.Object]bool
	}
	states := make(map[*cfg.Block]blockState, len(control.Blocks))
	entry := control.Blocks[0]
	states[entry] = blockState{initialized: true, written: make(map[types.Object]bool)}
	queue := []*cfg.Block{entry}
	queued := map[*cfg.Block]bool{entry: true}
	exits := make(map[*cfg.Block]map[types.Object]bool)
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		queued[block] = false
		written := cloneObjectBoolMap(states[block].written)
		for _, node := range block.Nodes {
			for target := range ps4008NodeDefiniteWriteTargets(pass, node, candidates, before) {
				written[target] = true
			}
		}
		successors := ps4008ReachableCFGSuccessors(pass, block)
		if len(successors) == 0 {
			exits[block] = written
			continue
		}
		for _, successor := range successors {
			state := states[successor]
			merged := written
			if state.initialized {
				merged = ps4008IntersectObjectSets(state.written, written)
			}
			if state.initialized && ps4008ObjectSetsEqual(state.written, merged) {
				continue
			}
			states[successor] = blockState{initialized: true, written: cloneObjectBoolMap(merged)}
			if !queued[successor] {
				queued[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	var definite map[types.Object]bool
	for _, written := range exits {
		if definite == nil {
			definite = cloneObjectBoolMap(written)
			continue
		}
		definite = ps4008IntersectObjectSets(definite, written)
	}
	return definite
}

func ps4008ReachableCFGSuccessors(pass *analysis.Pass, block *cfg.Block) []*cfg.Block {
	if block == nil {
		return nil
	}
	successors := block.Succs
	if len(successors) == 2 && len(block.Nodes) != 0 {
		if condition, ok := block.Nodes[len(block.Nodes)-1].(ast.Expr); ok {
			if result, known := ps1006BoolConstant(pass, condition); known {
				if result {
					return successors[:1]
				}
				return successors[1:]
			}
		}
	}
	return successors
}

func ps4008NodeDefiniteWriteTargets(pass *analysis.Pass, node ast.Node, candidates map[types.Object]bool, before token.Pos) map[types.Object]bool {
	targets := make(map[types.Object]bool)
	addDirect := func(expression ast.Expr) {
		switch value := ps2110Unparen(expression).(type) {
		case *ast.Ident:
			identifier := value
			if object := identObject(pass, identifier); candidates[object] {
				targets[object] = true
			}
		case *ast.StarExpr:
			if identifier, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
				if object := identObject(pass, identifier); candidates[object] {
					targets[object] = true
				}
			}
		}
	}
	switch value := node.(type) {
	case *ast.AssignStmt:
		for _, lhs := range value.Lhs {
			addDirect(lhs)
		}
		for _, rhs := range value.Rhs {
			ps4008CollectDefiniteExpressionWrites(pass, rhs, candidates, before, targets)
		}
	case *ast.IncDecStmt:
		addDirect(value.X)
	case *ast.ExprStmt:
		ps4008CollectDefiniteExpressionWrites(pass, value.X, candidates, before, targets)
	case *ast.ReturnStmt:
		for _, result := range value.Results {
			ps4008CollectDefiniteExpressionWrites(pass, result, candidates, before, targets)
		}
	case *ast.DeferStmt:
		// Deferred calls execute before every normal return once their defer
		// statement has been reached. The surrounding CFG decides whether the
		// registration itself dominates every reachable exit.
		ps4008CollectDefiniteExpressionWrites(pass, value.Call, candidates, before, targets)
	case *ast.GoStmt:
		// The launched call is asynchronous and is not a definite write before
		// the caller returns. Its function and argument expressions are still
		// evaluated synchronously in source order.
		ps4008CollectCallOperandWrites(pass, value.Call, candidates, before, targets)
	case *ast.SendStmt:
		ps4008CollectDefiniteExpressionWrites(pass, value.Chan, candidates, before, targets)
		ps4008CollectDefiniteExpressionWrites(pass, value.Value, candidates, before, targets)
	case ast.Expr:
		ps4008CollectDefiniteExpressionWrites(pass, value, candidates, before, targets)
	case *ast.ValueSpec:
		for _, name := range value.Names {
			addDirect(name)
		}
		for _, expression := range value.Values {
			ps4008CollectDefiniteExpressionWrites(pass, expression, candidates, before, targets)
		}
	}
	return targets
}

func ps4008CollectCallOperandWrites(pass *analysis.Pass, call *ast.CallExpr, candidates map[types.Object]bool, before token.Pos, targets map[types.Object]bool) {
	if call == nil {
		return
	}
	ps4008CollectDefiniteExpressionWrites(pass, call.Fun, candidates, before, targets)
	for _, argument := range call.Args {
		ps4008CollectDefiniteExpressionWrites(pass, argument, candidates, before, targets)
	}
}

func ps4008CollectDefiniteExpressionWrites(pass *analysis.Pass, expression ast.Expr, candidates map[types.Object]bool, before token.Pos, targets map[types.Object]bool) {
	if expression == nil {
		return
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.FuncLit:
		return
	case *ast.UnaryExpr:
		ps4008CollectDefiniteExpressionWrites(pass, value.X, candidates, before, targets)
		return
	case *ast.BinaryExpr:
		ps4008CollectDefiniteExpressionWrites(pass, value.X, candidates, before, targets)
		if value.Op == token.LAND || value.Op == token.LOR {
			if left, known := ps1006BoolConstant(pass, value.X); !known || value.Op == token.LAND && !left || value.Op == token.LOR && left {
				return
			}
		}
		ps4008CollectDefiniteExpressionWrites(pass, value.Y, candidates, before, targets)
		return
	case *ast.CallExpr:
		written, _ := ps4008CallableWriteTargetsMode(pass, value.Fun, candidates, before, make(map[types.Object]bool), ps4008DefiniteWrite)
		for target := range written {
			targets[target] = true
		}
		for argumentIndex, argument := range value.Args {
			ps4008CollectDefiniteExpressionWrites(pass, argument, candidates, before, targets)
			if ps4008CallWritesArgument(pass, value, argumentIndex, before, ps4008DefiniteWrite) {
				ps4008CollectMentionedDeps(pass, argument, candidates, targets)
				for _, target := range ps4008AddressedDerivedTargetsDeep(pass, argument, candidates, before) {
					targets[target] = true
				}
			}
		}
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == expression {
			return true
		}
		nested, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		ps4008CollectDefiniteExpressionWrites(pass, nested, candidates, before, targets)
		return false
	})
}

func ps4008IntersectObjectSets(left, right map[types.Object]bool) map[types.Object]bool {
	intersection := make(map[types.Object]bool)
	for object := range left {
		if right[object] {
			intersection[object] = true
		}
	}
	return intersection
}

func ps4008ObjectSetsEqual(left, right map[types.Object]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for object := range left {
		if !right[object] {
			return false
		}
	}
	return true
}

func ps4008CallableMayWriteTargetsCFG(pass *analysis.Pass, body *ast.BlockStmt, effects psAccumulatorGraph) map[types.Object]bool {
	if body == nil || len(effects.universe) == 0 {
		return nil
	}
	control := cfg.New(body, func(*ast.CallExpr) bool { return true })
	if control == nil || len(control.Blocks) == 0 {
		return cloneObjectBoolMap(effects.universe)
	}
	queued := map[*cfg.Block]bool{control.Blocks[0]: true}
	queue := []*cfg.Block{control.Blocks[0]}
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		for _, node := range block.Nodes {
			switch value := node.(type) {
			case ast.Stmt:
				effects.apply(pass, value, nil)
			case ast.Expr:
				ps4008ApplyReachableExpressionWrites(pass, value, &effects)
			case *ast.ValueSpec:
				for _, expression := range value.Values {
					ps4008ApplyReachableExpressionWrites(pass, expression, &effects)
				}
				for _, name := range value.Names {
					if object := identObject(pass, name); effects.universe[object] {
						effects.invalid[object] = true
					}
				}
			}
		}
		successors := ps4008ReachableCFGSuccessors(pass, block)
		for _, successor := range successors {
			if successor == nil || queued[successor] {
				continue
			}
			queued[successor] = true
			queue = append(queue, successor)
		}
	}
	return cloneObjectBoolMap(effects.invalid)
}

func ps4008ApplyReachableExpressionWrites(pass *analysis.Pass, expression ast.Expr, effects *psAccumulatorGraph) {
	if expression == nil || effects == nil {
		return
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			ps4008ApplyReachableExpressionWrites(pass, value.X, effects)
			return
		}
	case *ast.BinaryExpr:
		if value.Op == token.LAND || value.Op == token.LOR {
			ps4008ApplyReachableExpressionWrites(pass, value.X, effects)
			if left, known := ps1006BoolConstant(pass, value.X); known && (value.Op == token.LAND && !left || value.Op == token.LOR && left) {
				return
			}
			ps4008ApplyReachableExpressionWrites(pass, value.Y, effects)
			return
		}
	case *ast.FuncLit:
		return
	}
	effects.apply(pass, &ast.ExprStmt{X: expression}, nil)
}

type ps4008CallableEffectKey struct {
	body   *ast.BlockStmt
	before token.Pos
	deps   string
	mode   ps4008CallableEffectMode
}

func ps4008CallIsConversionOrBuiltin(pass *analysis.Pass, call *ast.CallExpr) bool {
	if pass == nil || pass.TypesInfo == nil || call == nil {
		return false
	}
	if value, ok := pass.TypesInfo.Types[call.Fun]; ok && value.IsType() {
		return true
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	_, builtin := identObject(pass, identifier).(*types.Builtin)
	return builtin
}

// ps4008CallDefinitelyOverwritesArgument proves the narrow helper shape used
// to reset a derived scalar: the callee body consists of `*parameter = value`
// followed by at most one return whose expressions are value-only. Merely
// passing &base is a may-write and must retain the old dependency; otherwise a
// no-op callback hides a real strided reduction.
func ps4008CallWritesArgument(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int, before token.Pos, mode ps4008CallableEffectMode) bool {
	if mode == ps4008MayWrite {
		return ps4008CallMayWriteArgument(pass, call, argumentIndex, before)
	}
	return ps4008CallDefinitelyOverwritesArgumentAt(pass, call, argumentIndex, before)
}

type ps4008CallableInvocationKey struct {
	body      *ast.BlockStmt
	parameter types.Object
	element   int
	mode      ps4008CallableEffectMode
}

// ps4008CallInvokesArgument distinguishes passing a callable value from
// executing it. May-write consumers include an argument when any reachable
// path invokes the corresponding formal parameter; definite dependency
// transfer includes it only when every normal return does so.
func ps4008CallInvokesArgument(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int, before token.Pos, mode ps4008CallableEffectMode) bool {
	return ps4008CallInvokesArgumentSeen(pass, call, argumentIndex, before, mode, make(map[ps4008CallableInvocationKey]bool))
}

func ps4008CallInvokesArgumentSeen(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int, before token.Pos, mode ps4008CallableEffectMode, seen map[ps4008CallableInvocationKey]bool) bool {
	if pass == nil || pass.TypesInfo == nil || call == nil || argumentIndex < 0 || argumentIndex >= len(call.Args) {
		return mode == ps4008MayWrite
	}
	if ps4008CallIsConversionOrBuiltin(pass, call) {
		return false
	}
	syntaxes, known := ps4008CallableSyntaxes(pass, call.Fun, before, make(map[types.Object]bool))
	if !known || len(syntaxes) == 0 {
		return mode == ps4008MayWrite
	}
	if mode == ps4008DefiniteWrite {
		for _, syntax := range syntaxes {
			binding := ps4008SyntaxParameterBinding(pass, syntax, argumentIndex)
			if binding.variadic && call.Ellipsis.IsValid() {
				binding.variadicIndex = -2
			}
			if binding.object == nil || !ps4008CallableInvokesParameter(pass, syntax.body, binding.object, binding.variadicIndex, mode, seen) {
				return false
			}
		}
		return true
	}
	for _, syntax := range syntaxes {
		binding := ps4008SyntaxParameterBinding(pass, syntax, argumentIndex)
		if binding.variadic && call.Ellipsis.IsValid() {
			binding.variadicIndex = -2
		}
		if binding.object == nil || ps4008CallableInvokesParameter(pass, syntax.body, binding.object, binding.variadicIndex, mode, seen) {
			return true
		}
	}
	return false
}

func ps4008CallableInvokesParameter(pass *analysis.Pass, body *ast.BlockStmt, parameter types.Object, element int, mode ps4008CallableEffectMode, seen map[ps4008CallableInvocationKey]bool) (invoked bool) {
	if body == nil || parameter == nil {
		return mode == ps4008MayWrite
	}
	key := ps4008CallableInvocationKey{body: body, parameter: parameter, element: element, mode: mode}
	index := ps1006AnalysisIndexForPass(pass)
	if index != nil {
		if cached, ok := index.callableInvocations[key]; ok {
			return cached
		}
	}
	if seen[key] {
		return mode == ps4008MayWrite
	}
	seen[key] = true
	defer delete(seen, key)
	if index != nil {
		defer func() { index.callableInvocations[key] = invoked }()
	}
	control := cfg.New(body, func(*ast.CallExpr) bool { return true })
	if control == nil || len(control.Blocks) == 0 {
		return mode == ps4008MayWrite
	}
	if mode == ps4008MayWrite {
		queued := map[*cfg.Block]bool{control.Blocks[0]: true}
		queue := []*cfg.Block{control.Blocks[0]}
		for len(queue) != 0 {
			block := queue[0]
			queue = queue[1:]
			for _, node := range block.Nodes {
				if ps4008NodeInvokesParameter(pass, node, parameter, element, mode, seen) {
					return true
				}
			}
			for _, successor := range ps4008ReachableCFGSuccessors(pass, block) {
				if successor != nil && !queued[successor] {
					queued[successor] = true
					queue = append(queue, successor)
				}
			}
		}
		return false
	}
	type invocationState struct {
		initialized bool
		invoked     bool
	}
	states := map[*cfg.Block]invocationState{control.Blocks[0]: {initialized: true}}
	queue := []*cfg.Block{control.Blocks[0]}
	queued := map[*cfg.Block]bool{control.Blocks[0]: true}
	var exits []bool
	for len(queue) != 0 {
		block := queue[0]
		queue = queue[1:]
		queued[block] = false
		invoked := states[block].invoked
		for _, node := range block.Nodes {
			invoked = invoked || ps4008NodeInvokesParameter(pass, node, parameter, element, mode, seen)
		}
		successors := ps4008ReachableCFGSuccessors(pass, block)
		if len(successors) == 0 {
			exits = append(exits, invoked)
			continue
		}
		for _, successor := range successors {
			state := states[successor]
			merged := invoked
			if state.initialized {
				merged = state.invoked && invoked
			}
			if state.initialized && state.invoked == merged {
				continue
			}
			states[successor] = invocationState{initialized: true, invoked: merged}
			if !queued[successor] {
				queued[successor] = true
				queue = append(queue, successor)
			}
		}
	}
	if len(exits) == 0 {
		return false
	}
	for _, invoked := range exits {
		if !invoked {
			return false
		}
	}
	return true
}

func ps4008NodeInvokesParameter(pass *analysis.Pass, node ast.Node, parameter types.Object, element int, mode ps4008CallableEffectMode, seen map[ps4008CallableInvocationKey]bool) bool {
	switch value := node.(type) {
	case *ast.AssignStmt:
		for _, expression := range append(slices.Clone(value.Lhs), value.Rhs...) {
			if ps4008ExpressionInvokesParameter(pass, expression, parameter, element, mode, seen) {
				return true
			}
		}
	case *ast.ExprStmt:
		return ps4008ExpressionInvokesParameter(pass, value.X, parameter, element, mode, seen)
	case *ast.ReturnStmt:
		for _, expression := range value.Results {
			if ps4008ExpressionInvokesParameter(pass, expression, parameter, element, mode, seen) {
				return true
			}
		}
	case *ast.DeferStmt:
		return ps4008ExpressionInvokesParameter(pass, value.Call, parameter, element, mode, seen)
	case *ast.GoStmt:
		if mode == ps4008MayWrite && ps4008ExpressionInvokesParameter(pass, value.Call, parameter, element, mode, seen) {
			return true
		}
		return ps4008CallOperandsInvokeParameter(pass, value.Call, parameter, element, mode, seen)
	case *ast.SendStmt:
		return ps4008ExpressionInvokesParameter(pass, value.Chan, parameter, element, mode, seen) ||
			ps4008ExpressionInvokesParameter(pass, value.Value, parameter, element, mode, seen)
	case *ast.IncDecStmt:
		return ps4008ExpressionInvokesParameter(pass, value.X, parameter, element, mode, seen)
	case *ast.ValueSpec:
		for _, expression := range value.Values {
			if ps4008ExpressionInvokesParameter(pass, expression, parameter, element, mode, seen) {
				return true
			}
		}
	case ast.Expr:
		return ps4008ExpressionInvokesParameter(pass, value, parameter, element, mode, seen)
	}
	return false
}

func ps4008CallOperandsInvokeParameter(pass *analysis.Pass, call *ast.CallExpr, parameter types.Object, element int, mode ps4008CallableEffectMode, seen map[ps4008CallableInvocationKey]bool) bool {
	if call == nil {
		return false
	}
	if ps4008ExpressionInvokesParameter(pass, call.Fun, parameter, element, mode, seen) {
		return true
	}
	for _, argument := range call.Args {
		if ps4008ExpressionInvokesParameter(pass, argument, parameter, element, mode, seen) {
			return true
		}
	}
	return false
}

func ps4008ExpressionInvokesParameter(pass *analysis.Pass, expression ast.Expr, parameter types.Object, element int, mode ps4008CallableEffectMode, seen map[ps4008CallableInvocationKey]bool) bool {
	if expression == nil {
		return false
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.FuncLit:
		return false
	case *ast.UnaryExpr:
		return ps4008ExpressionInvokesParameter(pass, value.X, parameter, element, mode, seen)
	case *ast.BinaryExpr:
		if ps4008ExpressionInvokesParameter(pass, value.X, parameter, element, mode, seen) {
			return true
		}
		if value.Op == token.LAND || value.Op == token.LOR {
			left, known := ps1006BoolConstant(pass, value.X)
			short := known && (value.Op == token.LAND && !left || value.Op == token.LOR && left)
			if short || mode == ps4008DefiniteWrite && !known {
				return false
			}
		}
		return ps4008ExpressionInvokesParameter(pass, value.Y, parameter, element, mode, seen)
	case *ast.CallExpr:
		if ps4008CallOperandsInvokeParameter(pass, value, parameter, element, mode, seen) {
			return true
		}
		if ps4008CallIsConversionOrBuiltin(pass, value) {
			return false
		}
		if ps4008CallableExpressionReferencesObject(pass, value.Fun, parameter, element, mode, value.Pos(), make(map[types.Object]bool)) {
			return true
		}
		for argumentIndex, argument := range value.Args {
			if !ps4008CallableExpressionReferencesObject(pass, argument, parameter, element, mode, value.Pos(), make(map[types.Object]bool)) {
				continue
			}
			if ps4008CallInvokesArgumentSeen(pass, value, argumentIndex, value.Pos(), mode, seen) {
				return true
			}
		}
		return false
	}
	invoked := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if invoked {
			return false
		}
		nested, ok := node.(ast.Expr)
		if !ok || nested == expression {
			return true
		}
		if ps4008ExpressionInvokesParameter(pass, nested, parameter, element, mode, seen) {
			invoked = true
		}
		return false
	})
	return invoked
}

func ps4008CallableExpressionReferencesObject(pass *analysis.Pass, expression ast.Expr, target types.Object, element int, mode ps4008CallableEffectMode, before token.Pos, seen map[types.Object]bool) bool {
	if expression == nil || target == nil {
		return false
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if object == target {
			return element < 0 || mode == ps4008MayWrite
		}
		if object == nil || seen[object] {
			return false
		}
		seen[object] = true
		for _, nested := range ps4008PossibleAliasValues(pass, object, before) {
			if ps4008CallableExpressionReferencesObject(pass, nested, target, element, mode, before, maps.Clone(seen)) {
				return true
			}
		}
		return false
	case *ast.FuncLit:
		found := false
		ast.Inspect(value.Body, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok && identObject(pass, identifier) == target && (element < 0 || mode == ps4008MayWrite) {
				found = true
				return false
			}
			return !found
		})
		return found
	case *ast.IndexExpr:
		if element >= 0 && ps4008ExpressionMentionsObject(pass, value.X, target) {
			indexValue := pass.TypesInfo.Types[value.Index].Value
			if indexValue == nil || indexValue.Kind() != constant.Int {
				return mode == ps4008MayWrite
			}
			index, exact := constant.Int64Val(indexValue)
			return exact && index == int64(element)
		}
		if element == -2 {
			return mode == ps4008MayWrite && ps4008ExpressionMentionsObject(pass, value.X, target)
		}
		return ps4008CallableExpressionReferencesObject(pass, value.X, target, element, mode, before, seen)
	case *ast.IndexListExpr:
		return ps4008CallableExpressionReferencesObject(pass, value.X, target, element, mode, before, seen)
	case *ast.TypeAssertExpr:
		return ps4008CallableExpressionReferencesObject(pass, value.X, target, element, mode, before, seen)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps4008CallableExpressionReferencesObject(pass, value.Args[0], target, element, mode, before, seen)
			}
		}
	}
	for _, nested := range ps4008AggregateCallableValues(pass, expression, before) {
		if nested != expression && ps4008CallableExpressionReferencesObject(pass, nested, target, element, mode, before, maps.Clone(seen)) {
			return true
		}
	}
	return false
}

func ps4008CallDefinitelyOverwritesArgumentAt(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int, before token.Pos) bool {
	if pass == nil || pass.TypesInfo == nil || call == nil || argumentIndex < 0 || argumentIndex >= len(call.Args) || call.Ellipsis.IsValid() {
		return false
	}
	syntaxes, known := ps4008CallableSyntaxes(pass, call.Fun, before, make(map[types.Object]bool))
	if !known || len(syntaxes) == 0 {
		return false
	}
	for _, syntax := range syntaxes {
		if !ps4008VariadicBindingIsUnique(pass, syntax, call, argumentIndex) ||
			!ps4008CallableDefinitelyOverwritesParameter(pass, syntax, argumentIndex, before) {
			return false
		}
	}
	return true
}

func ps4008CallableDefinitelyOverwritesParameter(pass *analysis.Pass, syntax ps4008CallableSyntaxValue, argumentIndex int, before token.Pos) bool {
	if _, ok := ps4008CallableOverwriteExpression(pass, syntax, argumentIndex); ok {
		return true
	}
	parameter := ps4008SyntaxParameterObject(pass, syntax, argumentIndex)
	if parameter == nil || syntax.body == nil {
		return false
	}
	return ps4008CallableBodyWriteTargetsMode(pass, syntax.body, map[types.Object]bool{parameter: true}, before, ps4008DefiniteWrite)[parameter]
}

func ps4008CallableOverwriteExpression(pass *analysis.Pass, syntax ps4008CallableSyntaxValue, argumentIndex int) (ast.Expr, bool) {
	parameter := ps4008SyntaxParameterObject(pass, syntax, argumentIndex)
	functionType, body := syntax.functionType, syntax.body
	if parameter == nil || body == nil || len(body.List) == 0 || len(body.List) > 2 {
		return nil, false
	}
	assignment, ok := body.List[0].(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, false
	}
	star, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.StarExpr)
	if !ok {
		return nil, false
	}
	if !ps4008ExpressionMentionsObject(pass, star.X, parameter) || !ps4008DefiniteOverwriteValue(pass, functionType, assignment.Rhs[0]) {
		return nil, false
	}
	if len(body.List) == 1 {
		return assignment.Rhs[0], true
	}
	result, ok := body.List[1].(*ast.ReturnStmt)
	if !ok {
		return nil, false
	}
	for _, expression := range result.Results {
		if !ps4008DefiniteOverwriteValue(pass, functionType, expression) {
			return nil, false
		}
	}
	return assignment.Rhs[0], true
}

func ps4008CallMayWriteArgument(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int, before token.Pos) bool {
	if pass == nil || pass.TypesInfo == nil || call == nil || argumentIndex < 0 || argumentIndex >= len(call.Args) || call.Ellipsis.IsValid() {
		return true
	}
	syntaxes, known := ps4008CallableSyntaxes(pass, call.Fun, before, make(map[types.Object]bool))
	if !known || len(syntaxes) == 0 {
		return true
	}
	for _, syntax := range syntaxes {
		parameter := ps4008SyntaxParameterObject(pass, syntax, argumentIndex)
		if parameter == nil || ps4008CallableMayWriteParameter(pass, syntax.body, parameter) {
			return true
		}
	}
	return false
}

func ps4008CallableMayWriteParameter(pass *analysis.Pass, body *ast.BlockStmt, parameter types.Object) bool {
	if body == nil || parameter == nil {
		return true
	}
	writes := false
	ast.Inspect(body, func(node ast.Node) bool {
		if writes {
			return false
		}
		if literal, ok := node.(*ast.FuncLit); ok && literal.Body != body {
			// Merely constructing a closure does not mutate the argument. If the
			// closure is invoked, the surrounding CallExpr below is conservative.
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if ps4008ExpressionMentionsObject(pass, lhs, parameter) && !ps4008DirectObjectAssignment(pass, lhs, parameter) {
					writes = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if ps4008ExpressionMentionsObject(pass, value.X, parameter) {
				writes = true
				return false
			}
		case *ast.CallExpr:
			if ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(value.Fun), make(map[types.Type]bool)) {
				targets, known := ps4008CallableWriteTargetsMode(pass, value.Fun, map[types.Object]bool{parameter: true}, value.Pos(), make(map[types.Object]bool), ps4008MayWrite)
				if known && targets[parameter] {
					writes = true
					return false
				}
			}
			if ps4008ExpressionMentionsObject(pass, value.Fun, parameter) {
				writes = true
				return false
			}
			for argumentIndex, argument := range value.Args {
				if ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
					if !ps4008CallInvokesArgument(pass, value, argumentIndex, value.Pos(), ps4008MayWrite) {
						continue
					}
					if targets, known := ps4008CallableWriteTargetsMode(pass, argument, map[types.Object]bool{parameter: true}, value.Pos(), make(map[types.Object]bool), ps4008MayWrite); known && targets[parameter] {
						writes = true
						return false
					}
				}
				if ps4008ExpressionMentionsObject(pass, argument, parameter) {
					writes = true
					return false
				}
			}
		case *ast.SendStmt:
			if ps4008ExpressionMentionsObject(pass, value.Chan, parameter) || ps4008ExpressionMentionsObject(pass, value.Value, parameter) {
				writes = true
				return false
			}
		}
		return true
	})
	return writes
}

func ps4008DirectObjectAssignment(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && identObject(pass, identifier) == object
}

func ps4008ExpressionMentionsObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	mentioned := false
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && identObject(pass, identifier) == object {
			mentioned = true
			return false
		}
		return !mentioned
	})
	return mentioned
}

type ps4008CallableSyntaxValue struct {
	functionType     *ast.FuncType
	receiver         *ast.FieldList
	receiverArgument bool
	body             *ast.BlockStmt
}

type ps4008ParameterBinding struct {
	object        types.Object
	variadic      bool
	variadicIndex int
}

func ps4008CallableSyntaxes(pass *analysis.Pass, expression ast.Expr, before token.Pos, seen map[types.Object]bool) ([]ps4008CallableSyntaxValue, bool) {
	if pass == nil || pass.TypesInfo == nil || expression == nil {
		return nil, false
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.FuncLit:
		return []ps4008CallableSyntaxValue{{functionType: value.Type, body: value.Body}}, true
	case *ast.Ident:
		object := identObject(pass, value)
		if function, declared := object.(*types.Func); declared {
			if index := ps1006AnalysisIndexForPass(pass); index != nil {
				if declaration := index.functionDeclarations[function]; declaration != nil {
					return []ps4008CallableSyntaxValue{{functionType: declaration.Type, body: declaration.Body}}, true
				}
			}
			return nil, false
		}
		if object == nil || seen[object] {
			return nil, false
		}
		seen[object] = true
		definitions := ps4008PossibleAliasDefinitions(pass, object, before)
		if len(definitions) == 0 {
			return nil, false
		}
		var syntaxes []ps4008CallableSyntaxValue
		known := true
		for _, definition := range definitions {
			ordered, ok := ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{
				expression: definition.value, valueSnapshot: true,
				resultIndex: definition.resultIndex, resultSelection: definition.resultSelection,
			}, definition.position, maps.Clone(seen))
			nested := make([]ps4008CallableSyntaxValue, 0, len(ordered))
			for syntaxIndex := range ordered {
				syntax := &ordered[syntaxIndex]
				if syntax.receiverSnapshot && syntax.receiver.pointerSnapshot && !syntax.receiver.pointerTargetsKnown {
					ok = false
				}
				nested = append(nested, syntax.syntax)
			}
			known = known && ok
			syntaxes = append(syntaxes, nested...)
		}
		return syntaxes, known && len(syntaxes) != 0
	case *ast.SelectorExpr:
		function, _ := identObject(pass, value.Sel).(*types.Func)
		if index := ps1006AnalysisIndexForPass(pass); index != nil {
			if declaration := index.functionDeclarations[function]; declaration != nil {
				selection := pass.TypesInfo.Selections[value]
				return []ps4008CallableSyntaxValue{{
					functionType:     declaration.Type,
					receiver:         declaration.Recv,
					receiverArgument: selection != nil && selection.Kind() == types.MethodExpr,
					body:             declaration.Body,
				}}, true
			}
		}
		if key, ok := ps4008SelectorKey(pass, value); ok {
			values, _ := ps4008PossibleSelectorValues(pass, key, before)
			return ps4008MergeCallableSyntaxes(pass, values, before, seen)
		}
	case *ast.IndexExpr:
		if typ := pass.TypesInfo.TypeOf(value.X); typ != nil {
			if _, signature := types.Unalias(typ).Underlying().(*types.Signature); signature {
				return ps4008CallableSyntaxes(pass, value.X, before, seen)
			}
		}
		if values, dominating := ps4008PossibleConstantIndexValues(pass, value, before); dominating {
			return ps4008MergeCallableSyntaxes(pass, values, before, seen)
		}
		return nil, false
	case *ast.IndexListExpr:
		return ps4008CallableSyntaxes(pass, value.X, before, seen)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps4008CallableSyntaxes(pass, value.Args[0], before, seen)
			}
		}
	case *ast.TypeAssertExpr:
		return ps4008CallableSyntaxes(pass, value.X, before, seen)
	}
	return nil, false
}

func ps4008MergeCallableSyntaxes(pass *analysis.Pass, values []ast.Expr, before token.Pos, seen map[types.Object]bool) ([]ps4008CallableSyntaxValue, bool) {
	if len(values) == 0 {
		return nil, false
	}
	var syntaxes []ps4008CallableSyntaxValue
	known := true
	for _, value := range values {
		nested, ok := ps4008CallableSyntaxes(pass, value, before, maps.Clone(seen))
		known = known && ok
		syntaxes = append(syntaxes, nested...)
	}
	return syntaxes, known && len(syntaxes) != 0
}

func ps4008SyntaxParameterObject(pass *analysis.Pass, syntax ps4008CallableSyntaxValue, argumentIndex int) types.Object {
	return ps4008SyntaxParameterBinding(pass, syntax, argumentIndex).object
}

func ps4008SyntaxParameterBinding(pass *analysis.Pass, syntax ps4008CallableSyntaxValue, argumentIndex int) ps4008ParameterBinding {
	if argumentIndex < 0 {
		return ps4008ParameterBinding{variadicIndex: -1}
	}
	if syntax.receiverArgument {
		if argumentIndex == 0 {
			if syntax.receiver == nil || len(syntax.receiver.List) != 1 || len(syntax.receiver.List[0].Names) != 1 {
				return ps4008ParameterBinding{variadicIndex: -1}
			}
			return ps4008ParameterBinding{object: identObject(pass, syntax.receiver.List[0].Names[0]), variadicIndex: -1}
		}
		argumentIndex--
	}
	functionType := syntax.functionType
	if functionType == nil || functionType.Params == nil {
		return ps4008ParameterBinding{variadicIndex: -1}
	}
	current := 0
	for fieldIndex, field := range functionType.Params.List {
		variadic := fieldIndex == len(functionType.Params.List)-1
		if _, ok := ps2110Unparen(field.Type).(*ast.Ellipsis); !ok {
			variadic = false
		}
		if len(field.Names) == 0 {
			if current == argumentIndex || variadic && argumentIndex >= current {
				variadicIndex := -1
				if variadic {
					variadicIndex = argumentIndex - current
				}
				return ps4008ParameterBinding{variadic: variadic, variadicIndex: variadicIndex}
			}
			current++
			continue
		}
		for _, name := range field.Names {
			if current == argumentIndex || variadic && argumentIndex >= current {
				variadicIndex := -1
				if variadic {
					variadicIndex = argumentIndex - current
				}
				return ps4008ParameterBinding{object: identObject(pass, name), variadic: variadic, variadicIndex: variadicIndex}
			}
			current++
		}
	}
	return ps4008ParameterBinding{variadicIndex: -1}
}

func ps4008VariadicBindingIsUnique(pass *analysis.Pass, syntax ps4008CallableSyntaxValue, call *ast.CallExpr, argumentIndex int) bool {
	binding := ps4008SyntaxParameterBinding(pass, syntax, argumentIndex)
	if !binding.variadic {
		return true
	}
	if call == nil || call.Ellipsis.IsValid() || binding.object == nil {
		return false
	}
	count := 0
	for index := range call.Args {
		other := ps4008SyntaxParameterBinding(pass, syntax, index)
		if other.object == binding.object {
			count++
		}
	}
	return count == 1
}

func ps4008DefiniteOverwriteValue(pass *analysis.Pass, functionType *ast.FuncType, expression ast.Expr) bool {
	allowed := make(map[types.Object]bool)
	if functionType != nil && functionType.Params != nil {
		for _, field := range functionType.Params.List {
			for _, name := range field.Names {
				object := identObject(pass, name)
				if object != nil && !ps4008TypeMayExposeMutableStorage(object.Type()) && !ps4008TypeMayCarryCallable(object.Type(), make(map[types.Type]bool)) {
					allowed[object] = true
				}
			}
		}
	}
	safe := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !safe {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.IndexExpr, *ast.IndexListExpr, *ast.SelectorExpr, *ast.StarExpr, *ast.FuncLit:
			safe = false
			return false
		case *ast.Ident:
			object := identObject(pass, value)
			if _, constant := object.(*types.Const); constant || allowed[object] {
				return true
			}
			if object != nil {
				safe = false
				return false
			}
		}
		return true
	})
	return safe
}

func ps4008TypeMayCarryCallable(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil || seen[typ] {
		return false
	}
	seen[typ] = true
	switch underlying := typ.Underlying().(type) {
	case *types.Signature, *types.Interface:
		return true
	case *types.Pointer:
		return ps4008TypeMayCarryCallable(underlying.Elem(), seen)
	case *types.Slice:
		return ps4008TypeMayCarryCallable(underlying.Elem(), seen)
	case *types.Array:
		return ps4008TypeMayCarryCallable(underlying.Elem(), seen)
	case *types.Chan:
		return ps4008TypeMayCarryCallable(underlying.Elem(), seen)
	case *types.Map:
		return ps4008TypeMayCarryCallable(underlying.Key(), seen) || ps4008TypeMayCarryCallable(underlying.Elem(), seen)
	case *types.Struct:
		for fieldIndex := 0; fieldIndex < underlying.NumFields(); fieldIndex++ {
			if ps4008TypeMayCarryCallable(underlying.Field(fieldIndex).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps4008IsPointerType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	_, pointer := typ.Underlying().(*types.Pointer)
	return pointer
}

// ps4008CallableMayCapture distinguishes a stored function value from a
// directly named package/local function. A declared function cannot close over
// caller locals; its pointer/reference arguments and method receiver are
// handled separately. An unresolved function-valued variable can.
func ps4008CallableMayCapture(pass *analysis.Pass, expression ast.Expr) bool {
	if pass == nil || pass.TypesInfo == nil || expression == nil {
		return true
	}
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		_, declared := identObject(pass, value).(*types.Func)
		return !declared
	case *ast.SelectorExpr:
		if selection := pass.TypesInfo.Selections[value]; selection != nil {
			return false
		}
		_, declared := identObject(pass, value.Sel).(*types.Func)
		return !declared
	case *ast.FuncLit:
		return false
	case *ast.IndexExpr:
		return ps4008CallableMayCapture(pass, value.X)
	case *ast.IndexListExpr:
		return ps4008CallableMayCapture(pass, value.X)
	case *ast.TypeAssertExpr:
		return ps4008CallableMayCapture(pass, value.X)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps4008CallableMayCapture(pass, value.Args[0])
			}
		}
	}
	return true
}

func ps4008CollectMentionedDeps(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool, dst map[types.Object]bool) {
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if deps[object] {
			dst[object] = true
		}
		return true
	})
}

func ps4008CanTrackDerivedLocal(pass *analysis.Pass, identifier *ast.Ident) bool {
	if identifier == nil {
		return false
	}
	typ := pass.TypesInfo.TypeOf(identifier)
	if typ == nil {
		return true
	}
	return ps4008TypeCanTrackDerivedFact(typ)
}

func ps4008TypeCanTrackDerivedFact(typ types.Type) bool {
	switch underlying := typ.Underlying().(type) {
	case *types.Basic:
		return underlying.Info()&types.IsInteger != 0
	case *types.Pointer, *types.Signature:
		return true
	}
	return false
}

func ps4008ArgumentMayExposeDerivedStorage(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]bool) bool {
	if expression == nil {
		return false
	}
	unparen := ps2110Unparen(expression)
	if unary, ok := unparen.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		return ps4008ExprDependsOn(pass, unary.X, nil, deps)
	}
	if !ps4008ExprDependsOn(pass, expression, nil, deps) {
		return false
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return true
	}
	return ps4008TypeMayExposeMutableStorage(typ)
}

func ps4008TypeMayExposeMutableStorage(typ types.Type) bool {
	switch typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface:
		return true
	}
	return false
}

func ps4008ReceiverMayMutateReferencedStorage(pass *analysis.Pass, selector *ast.SelectorExpr) bool {
	if selector == nil {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return false
	}
	signature, ok := selection.Obj().Type().(*types.Signature)
	if !ok || signature.Recv() == nil {
		return false
	}
	return ps1006TypeMayAliasMutableStorage(signature.Recv().Type(), make(map[types.Type]bool))
}

func ps4008MethodValueReceiverWrites(pass *analysis.Pass, selector *ast.SelectorExpr, before token.Pos, mode ps4008CallableEffectMode) bool {
	if !ps4008ReceiverMayMutateReferencedStorage(pass, selector) {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	function, _ := selection.Obj().(*types.Func)
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil || index.functionDeclarations[function] == nil {
		return true
	}
	declaration := index.functionDeclarations[function]
	syntax := ps4008CallableSyntaxValue{
		functionType:     declaration.Type,
		receiver:         declaration.Recv,
		receiverArgument: true,
		body:             declaration.Body,
	}
	receiver := ps4008SyntaxParameterObject(pass, syntax, 0)
	if receiver == nil {
		// An unnamed receiver cannot be referenced by the local body.
		return false
	}
	if mode == ps4008MayWrite {
		return ps4008CallableMayWriteParameter(pass, declaration.Body, receiver)
	}
	return ps4008CallableDefinitelyOverwritesParameter(pass, syntax, 0, before)
}

func ps4008ExprDependsOn(pass *analysis.Pass, expression ast.Expr, target types.Object, deps map[types.Object]bool) bool {
	if expression == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if target != nil && object == target || deps[object] {
			found = true
			return false
		}
		return true
	})
	return found
}

// ps4008IndependentAccumulatorCount counts distinct `lhs += ...`
// accumulators whose RHS multiplies two indexed operands from the serial
// dimension. Updates guarded by a serial-dimension-dependent condition do not
// count toward the latency-hiding tile.
func ps4008IndependentAccumulatorCount(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, deps map[types.Object]bool, tailGuard ps4008TailGuard) int {
	if body == nil || inner == nil {
		return 0
	}
	initialDeps := cloneObjectBoolMap(deps)
	states := []ps4008AccumulatorPath{{
		deps: initialDeps, pointerAliases: make(map[types.Object]ps1006OrderedPointerAlias), seen: make(map[types.Object]bool, len(body.List)),
		graph: psNewAccumulatorGraph(pass, body), tailGuard: tailGuard,
	}}
	states = ps4008ScanAccumulatorBlock(pass, body, inner, states)
	minCount := 0
	sawActive := false
	for _, state := range states {
		if state.unsafe {
			return 0
		}
		if len(state.seen) == 0 {
			continue
		}
		count := state.graph.independentCount(state.seen)
		if count == 0 {
			return 0
		}
		if !sawActive || count < minCount {
			minCount = count
		}
		sawActive = true
	}
	if !sawActive {
		return 0
	}
	return minCount
}

func ps4008HasSerialAccumulator(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object) bool {
	if body == nil || inner == nil {
		return false
	}
	return ps4008HasSerialAccumulatorInBlock(pass, body, inner, make(map[types.Object]bool, len(body.List)), make(map[types.Object]ps1006OrderedPointerAlias), nil)
}

func ps4008HasSerialAccumulatorInBlock(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, deps map[types.Object]bool, pointerAliases map[types.Object]ps1006OrderedPointerAlias, gotos map[string][]map[types.Object]bool) bool {
	if pointerAliases == nil {
		pointerAliases = make(map[types.Object]ps1006OrderedPointerAlias)
	}
	if gotos == nil {
		gotos = make(map[string][]map[types.Object]bool)
	}
	labels := make(map[string]int)
	skipUntil := ""
	for index, statement := range body.List {
		if skipUntil != "" {
			label, ok := statement.(*ast.LabeledStmt)
			if !ok || label.Label.Name != skipUntil {
				continue
			}
			skipUntil = ""
		}
		statementGotos := ps4008CollectGotoDepsForStatement(pass, statement, inner, deps)
		for label, states := range statementGotos {
			targetIndex, ok := labels[label]
			if !ok || targetIndex >= index {
				continue
			}
			for _, state := range states {
				if ps4008HasSerialAccumulatorInStatements(pass, body.List[targetIndex:index], inner, cloneObjectBoolMap(state)) {
					return true
				}
			}
			delete(statementGotos, label)
		}
		ps4008MergeGotoDeps(gotos, statementGotos)
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if ps4008AccumulatorObject(pass, value, inner, deps) != nil {
				return true
			}
			ps4008UpdateDerivedDeps(pass, value, inner, deps)
			ps1006UpdateOrderedPointerAliasesForStatement(pass, value, pointerAliases)
		case *ast.DeclStmt:
			ps4008UpdateDerivedDeps(pass, value, inner, deps)
			ps1006UpdateOrderedPointerAliasesForStatement(pass, value, pointerAliases)
		case *ast.ExprStmt:
			ps4008ApplyExpressionDependencyStatementWithAliases(pass, value, inner, deps, pointerAliases)
		case *ast.IfStmt:
			preInitDeps := cloneObjectBoolMap(deps)
			preInitAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			if value.Init != nil {
				ps4008UpdateDerivedDeps(pass, value.Init, inner, deps)
			}
			whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, value.Cond, inner, ps1006DependencyState{
				deps: deps, strideDeps: make(map[types.Object]string), pointerAliases: pointerAliases,
			})
			for _, branch := range whenTrue {
				if ps4008HasSerialAccumulatorInBlock(pass, value.Body, inner, cloneObjectBoolMap(branch.deps), clonePS1006OrderedPointerAliases(branch.pointerAliases), nil) {
					return true
				}
			}
			if elseBlock, ok := value.Else.(*ast.BlockStmt); ok {
				for _, branch := range whenFalse {
					if ps4008HasSerialAccumulatorInBlock(pass, elseBlock, inner, cloneObjectBoolMap(branch.deps), clonePS1006OrderedPointerAliases(branch.pointerAliases), nil) {
						return true
					}
				}
			} else if elseIf, ok := value.Else.(*ast.IfStmt); ok {
				for _, branch := range whenFalse {
					if ps4008HasSerialAccumulatorInBlock(pass, &ast.BlockStmt{List: []ast.Stmt{elseIf}}, inner, cloneObjectBoolMap(branch.deps), clonePS1006OrderedPointerAliases(branch.pointerAliases), nil) {
						return true
					}
				}
			}
			exit := ps1006UnionDependencyState(ps1006DependencyExitStatesForIf(pass, value, inner, preInitDeps, make(map[types.Object]string), preInitAliases, false))
			ps4008ReplaceDeps(deps, exit.deps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, exit.pointerAliases)
		case *ast.ForStmt:
			exit := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{
				deps: cloneObjectBoolMap(deps), strideDeps: make(map[types.Object]string), pointerAliases: clonePS1006OrderedPointerAliases(pointerAliases),
			}, false))
			ps4008ReplaceDeps(deps, exit.deps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, exit.pointerAliases)
		case *ast.BlockStmt:
			if ps4008HasSerialAccumulatorInBlock(pass, value, inner, deps, pointerAliases, gotos) {
				return true
			}
		case *ast.LabeledStmt:
			labels[value.Label.Name] = index
			if labelStates := gotos[value.Label.Name]; len(labelStates) > 0 {
				inputs := []map[types.Object]bool{cloneObjectBoolMap(deps)}
				inputs = append(inputs, labelStates...)
				ps4008ReplaceDeps(deps, ps4008UnionDerivedDeps(inputs))
				delete(gotos, value.Label.Name)
			}
			if ps4008HasSerialAccumulatorInStatement(pass, value.Stmt, inner, deps) {
				return true
			}
		case *ast.SwitchStmt:
			preSwitchDeps := cloneObjectBoolMap(deps)
			preSwitchAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			if value.Init != nil {
				ps4008UpdateDerivedDeps(pass, value.Init, inner, deps)
			}
			ps4008InvalidateDerivedDepsInExpr(pass, value.Tag, deps)
			if ps4008HasSerialAccumulatorInCaseClauses(pass, value.Body, inner, deps, value.Tag == nil) {
				return true
			}
			exit := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{
				deps: preSwitchDeps, strideDeps: make(map[types.Object]string), pointerAliases: preSwitchAliases,
			}, false))
			ps4008ReplaceDeps(deps, exit.deps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, exit.pointerAliases)
		case *ast.TypeSwitchStmt:
			preSwitchDeps := cloneObjectBoolMap(deps)
			if value.Init != nil {
				ps4008UpdateDerivedDeps(pass, value.Init, inner, deps)
			}
			ps4008InvalidateDerivedDeps(pass, value.Assign, deps)
			for _, clause := range value.Body.List {
				if caseClause, ok := clause.(*ast.CaseClause); ok && ps4008HasSerialAccumulatorInStatements(pass, caseClause.Body, inner, cloneObjectBoolMap(deps)) {
					return true
				}
			}
			ps4008ReplaceDeps(deps, ps4008UnionDerivedDeps(ps4008DependencyExitStatesForStatement(pass, value, inner, preSwitchDeps, false)))
		case *ast.SelectStmt:
			preSelectDeps := cloneObjectBoolMap(deps)
			preSelectAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			for _, clause := range value.Body.List {
				if commClause, ok := clause.(*ast.CommClause); ok {
					clauseDeps := cloneObjectBoolMap(deps)
					ps4008ApplyDerivedDependencyStatement(pass, commClause.Comm, inner, clauseDeps)
					if ps4008HasSerialAccumulatorInStatements(pass, commClause.Body, inner, clauseDeps) {
						return true
					}
				}
			}
			exit := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{
				deps: preSelectDeps, strideDeps: make(map[types.Object]string), pointerAliases: preSelectAliases,
			}, false))
			ps4008ReplaceDeps(deps, exit.deps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, exit.pointerAliases)
		case *ast.BranchStmt:
			switch value.Tok {
			case token.GOTO:
				if value.Label == nil {
					return false
				}
				targetIndex, ok := labels[value.Label.Name]
				if ok && targetIndex < index && ps4008HasSerialAccumulatorInStatements(pass, body.List[targetIndex:index], inner, cloneObjectBoolMap(deps)) {
					return true
				}
				if ok && targetIndex < index {
					return false
				}
				skipUntil = value.Label.Name
			case token.BREAK, token.CONTINUE:
				return false
			}
		case *ast.ReturnStmt:
			return false
		}
	}
	return false
}

func ps4008ConditionDependencyStates(pass *analysis.Pass, expression ast.Expr, inner types.Object, deps map[types.Object]bool) (whenTrue, whenFalse []map[types.Object]bool) {
	combinedTrue, combinedFalse := ps1006ConditionDependencyStates(pass, expression, inner, ps1006DependencyState{
		deps: cloneObjectBoolMap(deps), strideDeps: make(map[types.Object]string),
	})
	for _, state := range combinedTrue {
		whenTrue = append(whenTrue, state.deps)
	}
	for _, state := range combinedFalse {
		whenFalse = append(whenFalse, state.deps)
	}
	return whenTrue, whenFalse
}

func ps4008DependencyExitStatesForIf(pass *analysis.Pass, statement *ast.IfStmt, inner types.Object, deps map[types.Object]bool, breakExits bool) []map[types.Object]bool {
	if statement == nil {
		return []map[types.Object]bool{cloneObjectBoolMap(deps)}
	}
	state := cloneObjectBoolMap(deps)
	if statement.Init != nil {
		ps4008UpdateDerivedDeps(pass, statement.Init, inner, state)
	}
	whenTrue, whenFalse := ps4008ConditionDependencyStates(pass, statement.Cond, inner, state)
	thenStates := ps4008DependencyExitStatesForBlock(pass, statement.Body, inner, whenTrue, breakExits)
	var elseStates []map[types.Object]bool
	switch elseNode := statement.Else.(type) {
	case *ast.BlockStmt:
		elseStates = ps4008DependencyExitStatesForBlock(pass, elseNode, inner, whenFalse, breakExits)
	case *ast.IfStmt:
		for _, branch := range whenFalse {
			elseStates = append(elseStates, ps4008DependencyExitStatesForIf(pass, elseNode, inner, branch, breakExits)...)
		}
	default:
		elseStates = whenFalse
	}
	return append(thenStates, elseStates...)
}

func ps4008DependencyExitStatesForBlock(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, states []map[types.Object]bool, breakExits bool) []map[types.Object]bool {
	if body == nil {
		return states
	}
	for _, statement := range body.List {
		var next []map[types.Object]bool
		for _, state := range states {
			next = append(next, ps4008DependencyExitStatesForStatement(pass, statement, inner, state, breakExits)...)
		}
		states = next
		if len(states) == 0 {
			return nil
		}
	}
	return states
}

func ps4008DependencyExitStatesForStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool, breakExits bool) []map[types.Object]bool {
	state := cloneObjectBoolMap(deps)
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, value, inner, state)
		return []map[types.Object]bool{state}
	case *ast.ExprStmt:
		ps4008ApplyExpressionDependencyStatement(pass, value, inner, state)
		return []map[types.Object]bool{state}
	case *ast.IfStmt:
		return ps4008DependencyExitStatesForIf(pass, value, inner, state, breakExits)
	case *ast.BlockStmt:
		return ps4008DependencyExitStatesForBlock(pass, value, inner, []map[types.Object]bool{state}, breakExits)
	case *ast.LabeledStmt:
		return ps4008DependencyExitStatesForStatement(pass, value.Stmt, inner, state, breakExits)
	case *ast.SwitchStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state)
		}
		ps4008InvalidateDerivedDepsInExpr(pass, value.Tag, state)
		return ps4008DependencyExitStatesForCaseClauses(pass, value.Body, inner, state, value.Tag == nil)
	case *ast.TypeSwitchStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state)
		}
		ps4008InvalidateDerivedDeps(pass, value.Assign, state)
		return ps4008DependencyExitStatesForCaseClauses(pass, value.Body, inner, state, false)
	case *ast.SelectStmt:
		return ps4008DependencyExitStatesForCommClauses(pass, value.Body, inner, state)
	case *ast.ReturnStmt:
		return nil
	case *ast.BranchStmt:
		if value.Tok == token.BREAK && value.Label == nil && breakExits {
			return []map[types.Object]bool{state}
		}
		return nil
	}
	return []map[types.Object]bool{state}
}

func ps4008DependencyExitStatesForCaseClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, deps map[types.Object]bool, constantBools bool) []map[types.Object]bool {
	if body == nil {
		return []map[types.Object]bool{cloneObjectBoolMap(deps)}
	}
	clauses := ps1006CaseClauses(body)
	selectedStates, remainingStates := ps4008SelectedDependencyStates(pass, clauses, inner, deps, constantBools)
	var states []map[types.Object]bool
	var fallthroughStates []map[types.Object]bool
	for clauseIndex, clause := range clauses {
		inputs := slices.Clone(selectedStates[clauseIndex])
		inputs = append(inputs, fallthroughStates...)
		bodyStatements, fallsThrough := ps4008CaseBodyWithoutTerminalFallthrough(clause.Body)
		flows := ps4008DependencyFlowsForCaseBody(pass, bodyStatements, inner, inputs)
		fallthroughStates = nil
		if fallsThrough {
			for _, flow := range flows {
				if flow.exited {
					states = append(states, flow.deps)
					continue
				}
				fallthroughStates = append(fallthroughStates, flow.deps)
			}
			continue
		}
		for _, flow := range flows {
			states = append(states, flow.deps)
		}
	}
	states = append(states, remainingStates...)
	states = append(states, fallthroughStates...)
	return states
}

func ps4008SelectedDependencyStates(pass *analysis.Pass, clauses []*ast.CaseClause, inner types.Object, deps map[types.Object]bool, constantBools bool) (selected [][]map[types.Object]bool, remaining []map[types.Object]bool) {
	combined, unmatched := ps1006SelectedDependencyStates(pass, clauses, inner, ps1006DependencyState{
		deps: cloneObjectBoolMap(deps), strideDeps: make(map[types.Object]string),
	}, constantBools)
	selected = make([][]map[types.Object]bool, len(combined))
	for clauseIndex, states := range combined {
		for _, state := range states {
			selected[clauseIndex] = append(selected[clauseIndex], state.deps)
		}
	}
	for _, state := range unmatched {
		remaining = append(remaining, state.deps)
	}
	return selected, remaining
}

func ps4008CaseBodyWithoutTerminalFallthrough(statements []ast.Stmt) ([]ast.Stmt, bool) {
	if len(statements) == 0 {
		return statements, false
	}
	branch, ok := statements[len(statements)-1].(*ast.BranchStmt)
	if !ok || branch.Tok != token.FALLTHROUGH || branch.Label != nil {
		return statements, false
	}
	return statements[:len(statements)-1], true
}

type ps4008DependencyFlow struct {
	deps   map[types.Object]bool
	exited bool
}

func ps4008DependencyFlowsForCaseBody(pass *analysis.Pass, statements []ast.Stmt, inner types.Object, states []map[types.Object]bool) []ps4008DependencyFlow {
	flows := make([]ps4008DependencyFlow, 0, len(states))
	for _, state := range states {
		flows = append(flows, ps4008DependencyFlow{deps: cloneObjectBoolMap(state)})
	}
	for _, statement := range statements {
		if branch, ok := statement.(*ast.BranchStmt); ok && branch.Label == nil {
			switch branch.Tok {
			case token.BREAK:
				for index := range flows {
					if !flows[index].exited {
						flows[index].exited = true
					}
				}
				return flows
			case token.CONTINUE, token.GOTO:
				return nil
			}
		}
		var next []ps4008DependencyFlow
		for _, flow := range flows {
			if flow.exited {
				next = append(next, flow)
				continue
			}
			next = append(next, ps4008DependencyFlowsForCaseStatement(pass, statement, inner, flow.deps)...)
		}
		flows = next
		if len(flows) == 0 {
			return nil
		}
	}
	return flows
}

func ps4008DependencyFlowsForCaseStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool) []ps4008DependencyFlow {
	state := cloneObjectBoolMap(deps)
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, value, inner, state)
		return []ps4008DependencyFlow{{deps: state}}
	case *ast.ExprStmt:
		ps4008ApplyExpressionDependencyStatement(pass, value, inner, state)
		return []ps4008DependencyFlow{{deps: state}}
	case *ast.IfStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state)
		}
		whenTrue, whenFalse := ps4008ConditionDependencyStates(pass, value.Cond, inner, state)
		thenFlows := ps4008DependencyFlowsForCaseBody(pass, value.Body.List, inner, whenTrue)
		var elseFlows []ps4008DependencyFlow
		switch elseNode := value.Else.(type) {
		case *ast.BlockStmt:
			elseFlows = ps4008DependencyFlowsForCaseBody(pass, elseNode.List, inner, whenFalse)
		case *ast.IfStmt:
			for _, branch := range whenFalse {
				elseFlows = append(elseFlows, ps4008DependencyFlowsForCaseStatement(pass, elseNode, inner, branch)...)
			}
		default:
			for _, branch := range whenFalse {
				elseFlows = append(elseFlows, ps4008DependencyFlow{deps: branch})
			}
		}
		return append(thenFlows, elseFlows...)
	case *ast.BlockStmt:
		return ps4008DependencyFlowsForCaseBody(pass, value.List, inner, []map[types.Object]bool{state})
	case *ast.LabeledStmt:
		return ps4008DependencyFlowsForCaseStatement(pass, value.Stmt, inner, state)
	case *ast.BranchStmt:
		if value.Tok == token.BREAK && value.Label == nil {
			return []ps4008DependencyFlow{{deps: state, exited: true}}
		}
		return nil
	case *ast.ReturnStmt:
		return nil
	}
	exits := ps4008DependencyExitStatesForStatement(pass, statement, inner, state, false)
	flows := make([]ps4008DependencyFlow, 0, len(exits))
	for _, exit := range exits {
		flows = append(flows, ps4008DependencyFlow{deps: exit})
	}
	return flows
}

func ps4008DependencyExitStatesForCommClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, deps map[types.Object]bool) []map[types.Object]bool {
	if body == nil {
		return []map[types.Object]bool{cloneObjectBoolMap(deps)}
	}
	var states []map[types.Object]bool
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		clauseDeps := cloneObjectBoolMap(deps)
		ps4008ApplyDerivedDependencyStatement(pass, clause.Comm, inner, clauseDeps)
		states = append(states, ps4008DependencyExitStatesForBlock(pass, &ast.BlockStmt{List: clause.Body}, inner, []map[types.Object]bool{clauseDeps}, true)...)
	}
	if len(states) == 0 {
		return []map[types.Object]bool{cloneObjectBoolMap(deps)}
	}
	return states
}

func ps4008UnionDerivedDeps(states []map[types.Object]bool) map[types.Object]bool {
	union := make(map[types.Object]bool)
	for _, state := range states {
		for object, value := range state {
			if value {
				union[object] = true
			}
		}
	}
	return union
}

func ps4008ReplaceDeps(dst, src map[types.Object]bool) {
	clear(dst)
	for object, value := range src {
		dst[object] = value
	}
}

func ps4008MergeGotoDeps(dst, src map[string][]map[types.Object]bool) {
	for label, states := range src {
		for _, state := range states {
			dst[label] = append(dst[label], cloneObjectBoolMap(state))
		}
	}
}

func ps4008CollectGotoDepsForStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool) map[string][]map[types.Object]bool {
	gotos, _ := ps4008CollectGotoDepsStatement(pass, statement, inner, cloneObjectBoolMap(deps))
	return gotos
}

func ps4008CollectGotoDepsBlock(pass *analysis.Pass, statements []ast.Stmt, inner types.Object, states []map[types.Object]bool) (map[string][]map[types.Object]bool, []map[types.Object]bool) {
	collected := make(map[string][]map[types.Object]bool)
	for _, statement := range statements {
		var next []map[types.Object]bool
		for _, state := range states {
			gotos, exits := ps4008CollectGotoDepsStatement(pass, statement, inner, state)
			ps4008MergeGotoDeps(collected, gotos)
			next = append(next, exits...)
		}
		states = next
		if len(states) == 0 {
			break
		}
	}
	return collected, states
}

func ps4008CollectGotoDepsStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool) (map[string][]map[types.Object]bool, []map[types.Object]bool) {
	collected := make(map[string][]map[types.Object]bool)
	state := cloneObjectBoolMap(deps)
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, value, inner, state)
		return collected, []map[types.Object]bool{state}
	case *ast.ExprStmt:
		ps4008ApplyExpressionDependencyStatement(pass, value, inner, state)
		return collected, []map[types.Object]bool{state}
	case *ast.IfStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state)
		}
		whenTrue, whenFalse := ps4008ConditionDependencyStates(pass, value.Cond, inner, state)
		thenGotos, thenExits := ps4008CollectGotoDepsBlock(pass, value.Body.List, inner, whenTrue)
		ps4008MergeGotoDeps(collected, thenGotos)
		var elseExits []map[types.Object]bool
		switch elseNode := value.Else.(type) {
		case *ast.BlockStmt:
			elseGotos, exits := ps4008CollectGotoDepsBlock(pass, elseNode.List, inner, whenFalse)
			ps4008MergeGotoDeps(collected, elseGotos)
			elseExits = exits
		case *ast.IfStmt:
			for _, branch := range whenFalse {
				elseGotos, exits := ps4008CollectGotoDepsStatement(pass, elseNode, inner, branch)
				ps4008MergeGotoDeps(collected, elseGotos)
				elseExits = append(elseExits, exits...)
			}
		default:
			elseExits = whenFalse
		}
		return collected, append(thenExits, elseExits...)
	case *ast.BlockStmt:
		return ps4008CollectGotoDepsBlock(pass, value.List, inner, []map[types.Object]bool{state})
	case *ast.LabeledStmt:
		return ps4008CollectGotoDepsStatement(pass, value.Stmt, inner, state)
	case *ast.BranchStmt:
		if value.Tok == token.GOTO && value.Label != nil {
			collected[value.Label.Name] = append(collected[value.Label.Name], state)
		}
		return collected, nil
	case *ast.ReturnStmt:
		return collected, nil
	}
	return collected, []map[types.Object]bool{state}
}

func ps4008HasSerialAccumulatorInStatements(pass *analysis.Pass, statements []ast.Stmt, inner types.Object, deps map[types.Object]bool) bool {
	return ps4008HasSerialAccumulatorInBlock(pass, &ast.BlockStmt{List: statements}, inner, deps, make(map[types.Object]ps1006OrderedPointerAlias), nil)
}

func ps4008HasSerialAccumulatorInStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool) bool {
	if statement == nil {
		return false
	}
	return ps4008HasSerialAccumulatorInStatements(pass, []ast.Stmt{statement}, inner, deps)
}

func ps4008HasSerialAccumulatorInCaseClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, deps map[types.Object]bool, constantBools bool) bool {
	if body == nil {
		return false
	}
	clauses := ps1006CaseClauses(body)
	selectedStates, _ := ps4008SelectedDependencyStates(pass, clauses, inner, deps, constantBools)
	var fallthroughStates []map[types.Object]bool
	for clauseIndex, clause := range clauses {
		inputs := slices.Clone(selectedStates[clauseIndex])
		inputs = append(inputs, fallthroughStates...)
		statements, fallsThrough := ps4008CaseBodyWithoutTerminalFallthrough(clause.Body)
		for _, input := range inputs {
			if ps4008HasSerialAccumulatorInStatements(pass, statements, inner, cloneObjectBoolMap(input)) {
				return true
			}
		}
		fallthroughStates = nil
		if !fallsThrough {
			continue
		}
		for _, flow := range ps4008DependencyFlowsForCaseBody(pass, statements, inner, inputs) {
			if !flow.exited {
				fallthroughStates = append(fallthroughStates, flow.deps)
			}
		}
	}
	return false
}

type ps4008AccumulatorPath struct {
	deps           map[types.Object]bool
	pointerAliases map[types.Object]ps1006OrderedPointerAlias
	seen           map[types.Object]bool
	graph          psAccumulatorGraph
	tailGuard      ps4008TailGuard
	done           bool
	unsafe         bool
}

// psAccumulatorGraph tracks whether syntactically distinct accumulator names
// are genuinely independent loop-carried chains. A reset or indirect write
// makes a name ineligible; an RHS reference connects names into one dependency
// component. PS1006 and PS4008 share this model.
type psAccumulatorGraph struct {
	universe     map[types.Object]bool
	invalid      map[types.Object]bool
	edges        map[types.Object]map[types.Object]bool
	effectBefore token.Pos
}

func psNewAccumulatorGraph(pass *analysis.Pass, body *ast.BlockStmt) psAccumulatorGraph {
	graph := psAccumulatorGraph{
		universe: make(map[types.Object]bool),
		invalid:  make(map[types.Object]bool),
		edges:    make(map[types.Object]map[types.Object]bool),
	}
	if body == nil {
		return graph
	}
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		assign, ok := node.(*ast.AssignStmt)
		if !ok || assign.Tok != token.ADD_ASSIGN && assign.Tok != token.SUB_ASSIGN {
			return true
		}
		for _, lhs := range assign.Lhs {
			identifier, ok := ps2110Unparen(lhs).(*ast.Ident)
			if !ok || identifier.Name == "_" {
				continue
			}
			if object := identObject(pass, identifier); object != nil {
				graph.universe[object] = true
			}
		}
		return true
	})
	return graph
}

func (graph psAccumulatorGraph) clone() psAccumulatorGraph {
	cloned := psAccumulatorGraph{
		universe:     graph.universe,
		invalid:      cloneObjectBoolMap(graph.invalid),
		edges:        make(map[types.Object]map[types.Object]bool, len(graph.edges)),
		effectBefore: graph.effectBefore,
	}
	for object, dependencies := range graph.edges {
		cloned.edges[object] = cloneObjectBoolMap(dependencies)
	}
	return cloned
}

func (graph psAccumulatorGraph) key() string {
	parts := make([]string, 0, len(graph.edges))
	for object, dependencies := range graph.edges {
		parts = append(parts, fmt.Sprintf("%p=%s", object, ps4008BoolObjectSetKey(dependencies)))
	}
	slices.Sort(parts)
	return "invalid=" + ps4008BoolObjectSetKey(graph.invalid) + ";edges=" + strings.Join(parts, ",")
}

func (graph *psAccumulatorGraph) addEdge(left, right types.Object) {
	if left == nil || right == nil || left == right || !graph.universe[left] || !graph.universe[right] {
		return
	}
	if graph.edges[left] == nil {
		graph.edges[left] = make(map[types.Object]bool)
	}
	if graph.edges[right] == nil {
		graph.edges[right] = make(map[types.Object]bool)
	}
	graph.edges[left][right] = true
	graph.edges[right][left] = true
}

func (graph *psAccumulatorGraph) apply(pass *analysis.Pass, statement ast.Stmt, candidates map[types.Object]bool) {
	if graph == nil || len(graph.universe) == 0 || statement == nil {
		return
	}
	if expression, ok := statement.(*ast.ExprStmt); ok && expression.X == nil {
		return
	}
	if psAccumulatorStatementNeedsEffectScan(statement) {
		remaining := cloneObjectBoolMap(graph.universe)
		before := graph.effectBefore
		if !before.IsValid() {
			before = ps4008StatementPosition(statement)
		}
		if known := ps4008InvalidateMayWritesAt(pass, statement, remaining, before); !known {
			clear(remaining)
		}
		for object := range graph.universe {
			if !remaining[object] {
				graph.invalid[object] = true
			}
		}
	}
	for object := range psAccumulatorAddressedObjects(pass, statement) {
		if graph.universe[object] {
			graph.invalid[object] = true
		}
	}

	switch value := statement.(type) {
	case *ast.AssignStmt:
		for lhsIndex, lhs := range value.Lhs {
			identifier, ok := ps2110Unparen(lhs).(*ast.Ident)
			if !ok {
				continue
			}
			object := identObject(pass, identifier)
			if !graph.universe[object] {
				continue
			}
			var rhs ast.Expr
			if len(value.Lhs) == len(value.Rhs) {
				rhs = value.Rhs[lhsIndex]
			} else if len(value.Rhs) == 1 {
				rhs = value.Rhs[0]
			}
			for dependency := range ps1006MentionedObjects(pass, rhs) {
				graph.addEdge(object, dependency)
			}
			if !candidates[object] {
				graph.invalid[object] = true
			}
		}
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok {
			return
		}
		for _, spec := range declaration.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range valueSpec.Names {
				if object := identObject(pass, name); graph.universe[object] {
					graph.invalid[object] = true
				}
			}
		}
	case *ast.IncDecStmt:
		if identifier, ok := ps2110Unparen(value.X).(*ast.Ident); ok {
			if object := identObject(pass, identifier); graph.universe[object] {
				graph.invalid[object] = true
			}
		}
	}
}

func (graph *psAccumulatorGraph) applyWithDependencies(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]bool, candidates map[types.Object]bool) {
	if graph == nil {
		return
	}
	if len(deps) != 0 {
		remaining := cloneObjectBoolMap(deps)
		known := ps4008InvalidateMayWritesAt(pass, statement, remaining, ps4008StatementPosition(statement))
		if !known || len(remaining) != len(deps) {
			for object := range graph.universe {
				graph.invalid[object] = true
			}
		}
	}
	graph.apply(pass, statement, candidates)
}

// Direct identifier assignments are handled below without cloning and
// rechecking the whole accumulator universe. Only syntax that can execute an
// opaque call or write through an alias needs the derived-effect resolver.
// This keeps a wide straight-line accumulator block linear in its own size.
func psAccumulatorStatementNeedsEffectScan(statement ast.Stmt) bool {
	needed := false
	ast.Inspect(statement, func(node ast.Node) bool {
		if needed {
			return false
		}
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt:
			needed = true
			return false
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if _, ok := ps2110Unparen(lhs).(*ast.Ident); !ok {
					needed = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if _, ok := ps2110Unparen(value.X).(*ast.Ident); !ok {
				needed = true
				return false
			}
		}
		return true
	})
	return needed
}

func psAccumulatorAddressedObjects(pass *analysis.Pass, statement ast.Stmt) map[types.Object]bool {
	addressed := make(map[types.Object]bool)
	ast.Inspect(statement, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		unary, ok := node.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			return true
		}
		for object := range ps1006AssignmentRootObjects(pass, unary.X) {
			addressed[object] = true
		}
		return false
	})
	return addressed
}

func (graph psAccumulatorGraph) independentCount(objects map[types.Object]bool) int {
	visited := make(map[types.Object]bool, len(objects))
	count := 0
	for object := range objects {
		if visited[object] {
			continue
		}
		pending := []types.Object{object}
		eligible := false
		for len(pending) > 0 {
			current := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			if visited[current] || !objects[current] {
				continue
			}
			visited[current] = true
			eligible = eligible || !graph.invalid[current]
			for adjacent := range graph.edges[current] {
				pending = append(pending, adjacent)
			}
		}
		if eligible {
			count++
		}
	}
	return count
}

type ps4008TailGuard struct {
	output types.Object
	bound  types.Object
	width  int64
}

func ps4008EnclosingTailGuard(index *ps1006AnalysisIndex, pass *analysis.Pass, stack []ast.Node) ps4008TailGuard {
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		if !astutil.IsLoop(stack[stackIndex]) {
			continue
		}
		loop, ok := stack[stackIndex].(*ast.ForStmt)
		if !ok {
			return ps4008TailGuard{}
		}
		output := ps4008LoopObject(pass, loop)
		width := ps1006LoopStep(pass, loop)
		condition, ok := loop.Cond.(*ast.BinaryExpr)
		if output == nil || width < 4 || !ok || condition.Op != token.LSS && condition.Op != token.LEQ {
			return ps4008TailGuard{}
		}
		if _, ok := ps1006OuterPlusConst(pass, condition.X, output); !ok {
			return ps4008TailGuard{}
		}
		bound, ok := ps4008BoundIdent(pass, condition.Y)
		if !ok {
			return ps4008TailGuard{}
		}
		if !ps1006ObjectStableInNode(index, pass, loop.Body, output) || !ps1006ObjectStableInNode(index, pass, loop.Body, bound) {
			return ps4008TailGuard{}
		}
		return ps4008TailGuard{output: output, bound: bound, width: width}
	}
	return ps4008TailGuard{}
}

func ps4008ScanAccumulatorBlock(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, states []ps4008AccumulatorPath) []ps4008AccumulatorPath {
	if body == nil {
		return states
	}
	for _, statement := range body.List {
		var next []ps4008AccumulatorPath
		for _, state := range states {
			if state.done {
				next = append(next, state)
				continue
			}
			next = append(next, ps4008ScanAccumulatorStatement(pass, statement, inner, state)...)
		}
		states = ps4008DedupeAccumulatorPaths(next)
	}
	return states
}

const ps4008MaxAccumulatorPaths = 256

func ps4008DedupeAccumulatorPaths(states []ps4008AccumulatorPath) []ps4008AccumulatorPath {
	if len(states) <= 1 {
		return states
	}
	seen := make(map[string]bool, len(states))
	deduped := make([]ps4008AccumulatorPath, 0, len(states))
	for _, state := range states {
		key := ps4008AccumulatorPathKey(state)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, state)
		if len(deduped) > ps4008MaxAccumulatorPaths {
			return []ps4008AccumulatorPath{{done: true, unsafe: true}}
		}
	}
	return deduped
}

func ps4008AccumulatorPathKey(state ps4008AccumulatorPath) string {
	return fmt.Sprintf("done=%t;unsafe=%t;deps=%s;aliases=%s;seen=%s;graph=%s", state.done, state.unsafe, ps4008BoolObjectSetKey(state.deps), ps1006OrderedPointerAliasesKey(state.pointerAliases), ps4008BoolObjectSetKey(state.seen), state.graph.key())
}

func ps4008BoolObjectSetKey(objects map[types.Object]bool) string {
	if len(objects) == 0 {
		return ""
	}
	parts := make([]string, 0, len(objects))
	for object, value := range objects {
		if value {
			parts = append(parts, fmt.Sprintf("%p", object))
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func ps4008ConditionAccumulatorStates(pass *analysis.Pass, expression ast.Expr, inner types.Object, state ps4008AccumulatorPath) (whenTrue, whenFalse []ps4008AccumulatorPath) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			whenFalse, whenTrue = ps4008ConditionAccumulatorStates(pass, value.X, inner, state)
			return whenTrue, whenFalse
		}
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND:
			leftTrue, leftFalse := ps4008ConditionAccumulatorStates(pass, value.X, inner, state)
			whenFalse = append(whenFalse, leftFalse...)
			for _, left := range leftTrue {
				rightTrue, rightFalse := ps4008ConditionAccumulatorStates(pass, value.Y, inner, left)
				whenTrue = append(whenTrue, rightTrue...)
				whenFalse = append(whenFalse, rightFalse...)
			}
			return ps4008DedupeAccumulatorPaths(whenTrue), ps4008DedupeAccumulatorPaths(whenFalse)
		case token.LOR:
			leftTrue, leftFalse := ps4008ConditionAccumulatorStates(pass, value.X, inner, state)
			whenTrue = append(whenTrue, leftTrue...)
			for _, left := range leftFalse {
				rightTrue, rightFalse := ps4008ConditionAccumulatorStates(pass, value.Y, inner, left)
				whenTrue = append(whenTrue, rightTrue...)
				whenFalse = append(whenFalse, rightFalse...)
			}
			return ps4008DedupeAccumulatorPaths(whenTrue), ps4008DedupeAccumulatorPaths(whenFalse)
		}
	}
	next := state.clone()
	next.graph.applyWithDependencies(pass, &ast.ExprStmt{X: expression}, next.deps, nil)
	deps := ps1006DependencyState{deps: next.deps, strideDeps: make(map[types.Object]string), pointerAliases: next.pointerAliases}
	ps1006ApplyCaseExpression(pass, expression, inner, &deps)
	next.deps, next.pointerAliases = deps.deps, deps.pointerAliases
	if result, known := ps1006BoolConstant(pass, expression); known {
		if result {
			return []ps4008AccumulatorPath{next}, nil
		}
		return nil, []ps4008AccumulatorPath{next}
	}
	return []ps4008AccumulatorPath{next.clone()}, []ps4008AccumulatorPath{next}
}

func ps4008ScanAccumulatorStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, state ps4008AccumulatorPath) []ps4008AccumulatorPath {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		candidates := make(map[types.Object]bool)
		if object := ps4008AccumulatorObject(pass, value, inner, state.deps); object != nil {
			state.seen[object] = true
			candidates[object] = true
		}
		state.graph.applyWithDependencies(pass, value, state.deps, candidates)
		ps4008UpdateDerivedDeps(pass, value, inner, state.deps)
		return []ps4008AccumulatorPath{state}
	case *ast.DeclStmt:
		state.graph.applyWithDependencies(pass, value, state.deps, nil)
		ps4008UpdateDerivedDeps(pass, value, inner, state.deps)
		return []ps4008AccumulatorPath{state}
	case *ast.ExprStmt:
		state.graph.applyWithDependencies(pass, value, state.deps, nil)
		deps := ps1006DependencyState{deps: state.deps, strideDeps: make(map[types.Object]string), pointerAliases: state.pointerAliases}
		ps1006ApplyCaseExpression(pass, value.X, inner, &deps)
		state.deps, state.pointerAliases = deps.deps, deps.pointerAliases
		return []ps4008AccumulatorPath{state}
	case *ast.IfStmt:
		if value.Init != nil {
			state.graph.applyWithDependencies(pass, value.Init, state.deps, nil)
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state.deps)
		}
		whenTrue, whenFalse := ps4008ConditionAccumulatorStates(pass, value.Cond, inner, state)
		thenStates := ps4008ScanAccumulatorBlock(pass, value.Body, inner, whenTrue)
		var elseStates []ps4008AccumulatorPath
		switch elseNode := value.Else.(type) {
		case *ast.BlockStmt:
			elseStates = ps4008ScanAccumulatorBlock(pass, elseNode, inner, whenFalse)
		case *ast.IfStmt:
			for _, branch := range whenFalse {
				elseStates = append(elseStates, ps4008ScanAccumulatorStatement(pass, elseNode, inner, branch)...)
			}
		default:
			if ps4008CanMergeOptionalTailGuard(pass, value.Cond, inner, state.deps, state.tailGuard, thenStates) {
				return thenStates
			}
			elseStates = whenFalse
		}
		return ps4008DedupeAccumulatorPaths(append(thenStates, elseStates...))
	case *ast.BlockStmt:
		return ps4008ScanAccumulatorBlock(pass, value, inner, []ps4008AccumulatorPath{state})
	case *ast.LabeledStmt:
		return ps4008ScanAccumulatorStatement(pass, value.Stmt, inner, state)
	case *ast.SwitchStmt:
		if value.Init != nil {
			state.graph.applyWithDependencies(pass, value.Init, state.deps, nil)
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state.deps)
		}
		if value.Tag != nil {
			state.graph.applyWithDependencies(pass, &ast.ExprStmt{X: value.Tag}, state.deps, nil)
		}
		if ps4008ExprDependsOn(pass, value.Tag, inner, state.deps) {
			return []ps4008AccumulatorPath{state}
		}
		ps4008InvalidateDerivedDepsInExpr(pass, value.Tag, state.deps)
		return ps4008ScanAccumulatorCaseClauses(pass, value.Body, inner, state, value.Tag == nil)
	case *ast.TypeSwitchStmt:
		if value.Init != nil {
			state.graph.applyWithDependencies(pass, value.Init, state.deps, nil)
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state.deps)
		}
		state.graph.applyWithDependencies(pass, value.Assign, state.deps, nil)
		ps4008InvalidateDerivedDeps(pass, value.Assign, state.deps)
		return ps4008ScanAccumulatorCaseClauses(pass, value.Body, inner, state, false)
	case *ast.SelectStmt:
		return ps4008ScanAccumulatorCommClauses(pass, value.Body, inner, state)
	case *ast.IncDecStmt, *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt:
		state.graph.applyWithDependencies(pass, statement, state.deps, nil)
		return []ps4008AccumulatorPath{state}
	case *ast.ReturnStmt:
		state.graph.applyWithDependencies(pass, value, state.deps, nil)
		state.done = true
		return []ps4008AccumulatorPath{state}
	case *ast.BranchStmt:
		state.done = true
		state.unsafe = true
		return []ps4008AccumulatorPath{state}
	}
	return []ps4008AccumulatorPath{state}
}

func ps4008CanMergeOptionalTailGuard(pass *analysis.Pass, cond ast.Expr, inner types.Object, deps map[types.Object]bool, tailGuard ps4008TailGuard, states []ps4008AccumulatorPath) bool {
	if len(states) == 0 {
		return false
	}
	if !ps4008IsTailGuard(pass, cond, inner, deps, tailGuard) {
		return false
	}
	for _, state := range states {
		if state.done || state.unsafe {
			return false
		}
	}
	return true
}

func ps4008IsTailGuard(pass *analysis.Pass, cond ast.Expr, inner types.Object, deps map[types.Object]bool, tailGuard ps4008TailGuard) bool {
	if tailGuard.output == nil || tailGuard.bound == nil || tailGuard.width < 4 || ps4008ExprDependsOn(pass, cond, inner, deps) {
		return false
	}
	binary, ok := cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.LSS && binary.Op != token.LEQ {
		return false
	}
	bound, ok := ps4008BoundIdent(pass, binary.Y)
	if !ok || bound != tailGuard.bound {
		return false
	}
	offset, ok := ps1006OuterPlusConst(pass, binary.X, tailGuard.output)
	return ok && offset > 0 && offset < tailGuard.width
}

func ps4008BoundIdent(pass *analysis.Pass, expression ast.Expr) (types.Object, bool) {
	for {
		paren, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = paren.X
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := identObject(pass, identifier)
	return object, object != nil
}

func (state ps4008AccumulatorPath) clone() ps4008AccumulatorPath {
	return ps4008AccumulatorPath{
		deps:           cloneObjectBoolMap(state.deps),
		pointerAliases: clonePS1006OrderedPointerAliases(state.pointerAliases),
		seen:           cloneObjectBoolMap(state.seen),
		graph:          state.graph.clone(),
		tailGuard:      state.tailGuard,
		done:           state.done,
		unsafe:         state.unsafe,
	}
}

func ps4008ScanAccumulatorCaseClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, state ps4008AccumulatorPath, constantBools bool) []ps4008AccumulatorPath {
	if body == nil {
		return []ps4008AccumulatorPath{state}
	}
	clauses := ps1006CaseClauses(body)
	selectedStates, remainingStates := ps4008SelectedAccumulatorStates(pass, clauses, inner, state, constantBools)
	var states []ps4008AccumulatorPath
	var fallthroughStates []ps4008AccumulatorPath
	for clauseIndex, clause := range clauses {
		inputs := slices.Clone(selectedStates[clauseIndex])
		inputs = append(inputs, fallthroughStates...)
		statements, fallsThrough := ps4008CaseBodyWithoutTerminalFallthrough(clause.Body)
		outputs := ps4008ScanAccumulatorBlock(pass, &ast.BlockStmt{List: statements}, inner, inputs)
		fallthroughStates = nil
		if !fallsThrough {
			states = append(states, outputs...)
			continue
		}
		for _, output := range outputs {
			if output.done {
				states = append(states, output)
				continue
			}
			fallthroughStates = append(fallthroughStates, output)
		}
	}
	states = append(states, remainingStates...)
	states = append(states, fallthroughStates...)
	return states
}

func ps4008SelectedAccumulatorStates(pass *analysis.Pass, clauses []*ast.CaseClause, inner types.Object, state ps4008AccumulatorPath, constantBools bool) (selected [][]ps4008AccumulatorPath, remaining []ps4008AccumulatorPath) {
	selected = make([][]ps4008AccumulatorPath, len(clauses))
	current := state.clone()
	haveCurrent := true
	defaultIndex := -1
	for clauseIndex, clause := range clauses {
		if clause.List == nil {
			defaultIndex = clauseIndex
			continue
		}
		if !haveCurrent {
			continue
		}
		for _, expression := range clause.List {
			deps := ps1006DependencyState{deps: current.deps, strideDeps: make(map[types.Object]string), pointerAliases: current.pointerAliases}
			ps1006ApplyCaseExpression(pass, expression, inner, &deps)
			current.deps, current.pointerAliases = deps.deps, deps.pointerAliases
			current.graph.applyWithDependencies(pass, &ast.ExprStmt{X: expression}, current.deps, nil)
			value, known := false, false
			if constantBools {
				value, known = ps1006BoolConstant(pass, expression)
			}
			if !known || value {
				selected[clauseIndex] = append(selected[clauseIndex], current.clone())
			}
			if known && value {
				haveCurrent = false
				break
			}
		}
	}
	if haveCurrent && defaultIndex >= 0 {
		selected[defaultIndex] = append(selected[defaultIndex], current.clone())
		haveCurrent = false
	}
	if haveCurrent {
		remaining = []ps4008AccumulatorPath{current}
	}
	return selected, remaining
}

func ps4008ScanAccumulatorCommClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, state ps4008AccumulatorPath) []ps4008AccumulatorPath {
	if body == nil {
		return []ps4008AccumulatorPath{state}
	}
	var states []ps4008AccumulatorPath
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		clauseState := state.clone()
		ps4008ApplyDerivedDependencyStatement(pass, clause.Comm, inner, clauseState.deps)
		states = append(states, ps4008ScanAccumulatorBlock(pass, &ast.BlockStmt{List: clause.Body}, inner, []ps4008AccumulatorPath{clauseState})...)
	}
	if len(states) == 0 {
		return []ps4008AccumulatorPath{state}
	}
	return states
}

func cloneObjectBoolMap(input map[types.Object]bool) map[types.Object]bool {
	output := make(map[types.Object]bool, len(input))
	for object, value := range input {
		output[object] = value
	}
	return output
}

func ps4008AccumulatorObject(pass *analysis.Pass, assign *ast.AssignStmt, inner types.Object, deps map[types.Object]bool) types.Object {
	if assign == nil || assign.Tok != token.ADD_ASSIGN && assign.Tok != token.SUB_ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return nil
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name == "_" || ps4008ExprDependsOn(pass, lhs, inner, deps) {
		return nil
	}
	mul, ok := assign.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return nil
	}
	if !ps4008IndexedOperandDependsOn(pass, mul.X, inner, deps) || !ps4008IndexedOperandDependsOn(pass, mul.Y, inner, deps) {
		return nil
	}
	return identObject(pass, lhs)
}

func ps4008IndexedOperandDependsOn(pass *analysis.Pass, expression ast.Expr, inner types.Object, deps map[types.Object]bool) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		index, ok := node.(*ast.IndexExpr)
		if !ok {
			return true
		}
		if ps4008ExprDependsOn(pass, index.Index, inner, deps) {
			found = true
			return false
		}
		return true
	})
	return found
}

// ps4008AxpyFix builds the ikj/axpy rewrite for the exact canonical shape
//
//	for J := range B[0] {
//		SUM := 0.0
//		for K := range B {
//			SUM += A[I][K] * B[K][J]
//		}
//		C[I][J] = SUM
//	}
//
// where A, B, C are (underlying) fixed rectangular float64 arrays, C is not
// the same variable as A or B, and I is an identifier distinct from J and K.
// The replacement
//
//	for J := range B[0] {
//		C[I][J] = 0
//	}
//	for K := range B {
//		for J := range B[0] {
//			C[I][J] += A[I][K] * B[K][J]
//		}
//	}
//
// keeps every c[I][J] accumulating the same terms from the same +0.0 in the
// same ascending-k order, so each output element's IEEE rounding sequence is
// unchanged. Fixed dimensions prove all accesses in bounds, and distinct
// float arrays cannot share element storage. inner is the innermost (k) loop
// the diagnostic was reported on.
func ps4008AxpyFix(pass *analysis.Pass, stack []ast.Node, inner ast.Node) *analysis.SuggestedFix {
	innerLoop, ok := inner.(*ast.RangeStmt)
	if !ok {
		return nil
	}
	mid, ok := astutil.InLoop(stack)
	if !ok {
		return nil
	}
	middle, ok := mid.(*ast.RangeStmt)
	if !ok {
		return nil
	}
	// A labeled middle loop would leave its label attached to only the
	// zeroing loop after the rewrite; skip.
	for idx := len(stack) - 1; idx >= 1; idx-- {
		if stack[idx] == ast.Node(middle) {
			if _, isLab := stack[idx-1].(*ast.LabeledStmt); isLab {
				return nil
			}
			break
		}
	}
	info := pass.TypesInfo

	// Middle header: for J := range B[0], with J a used (non-blank) int var.
	if middle.Tok != token.DEFINE || middle.Value != nil {
		return nil
	}
	jIdent, ok := middle.Key.(*ast.Ident)
	if !ok || jIdent.Name == "_" {
		return nil
	}
	bRow, ok := middle.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	bIdent, ok := bRow.X.(*ast.Ident)
	if !ok {
		return nil
	}
	zero, ok := bRow.Index.(*ast.BasicLit)
	if !ok || zero.Kind != token.INT || zero.Value != "0" {
		return nil
	}
	if len(middle.Body.List) != 3 {
		return nil
	}

	// Statement 1: SUM := 0.0 (any float literal spelling of +0).
	initStmt, ok := middle.Body.List[0].(*ast.AssignStmt)
	if !ok || initStmt.Tok != token.DEFINE || len(initStmt.Lhs) != 1 || len(initStmt.Rhs) != 1 {
		return nil
	}
	sumIdent, ok := initStmt.Lhs[0].(*ast.Ident)
	if !ok || sumIdent.Name == "_" {
		return nil
	}
	sumLit, ok := initStmt.Rhs[0].(*ast.BasicLit)
	if !ok || sumLit.Kind != token.FLOAT {
		return nil
	}
	if v, err := strconv.ParseFloat(sumLit.Value, 64); err != nil || v != 0 {
		return nil
	}

	// Statement 2: the reported inner loop itself, for K := range B.
	if s, ok := middle.Body.List[1].(*ast.RangeStmt); !ok || s != innerLoop {
		return nil
	}
	if innerLoop.Tok != token.DEFINE || innerLoop.Value != nil {
		return nil
	}
	kIdent, ok := innerLoop.Key.(*ast.Ident)
	if !ok || kIdent.Name == "_" {
		return nil
	}
	bIdent2, ok := innerLoop.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if len(innerLoop.Body.List) != 1 {
		return nil
	}

	jObj := info.ObjectOf(jIdent)
	kObj := info.ObjectOf(kIdent)
	sumObj := info.ObjectOf(sumIdent)
	bObj := info.ObjectOf(bIdent)
	if jObj == nil || kObj == nil || sumObj == nil || bObj == nil {
		return nil
	}
	if info.ObjectOf(bIdent2) != bObj {
		return nil
	}

	// Inner body: SUM += A[I][K] * B[K][J].
	acc, ok := innerLoop.Body.List[0].(*ast.AssignStmt)
	if !ok || acc.Tok != token.ADD_ASSIGN || len(acc.Lhs) != 1 || len(acc.Rhs) != 1 {
		return nil
	}
	accLhs, ok := acc.Lhs[0].(*ast.Ident)
	if !ok || info.ObjectOf(accLhs) != sumObj {
		return nil
	}
	mul, ok := acc.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return nil
	}
	aIdent, iIdent, k1, ok := ps4008MatrixIndex(mul.X)
	if !ok {
		return nil
	}
	b3, k2, j1, ok := ps4008MatrixIndex(mul.Y)
	if !ok {
		return nil
	}
	if info.ObjectOf(k1) != kObj || info.ObjectOf(k2) != kObj || info.ObjectOf(j1) != jObj {
		return nil
	}
	if info.ObjectOf(b3) != bObj {
		return nil
	}
	iObj := info.ObjectOf(iIdent)
	if iObj == nil || iObj == jObj || iObj == kObj || iObj == sumObj {
		return nil
	}

	// Statement 3: C[I][J] = SUM.
	store, ok := middle.Body.List[2].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 {
		return nil
	}
	cIdent, i2, j2, ok := ps4008MatrixIndex(store.Lhs[0])
	if !ok {
		return nil
	}
	if info.ObjectOf(i2) != iObj || info.ObjectOf(j2) != jObj {
		return nil
	}
	storeRhs, ok := store.Rhs[0].(*ast.Ident)
	if !ok || info.ObjectOf(storeRhs) != sumObj {
		return nil
	}
	cObj := info.ObjectOf(cIdent)
	aObj := info.ObjectOf(aIdent)
	if cObj == nil || aObj == nil {
		return nil
	}
	// In-place matmul (c literally a or b) reads elements it already
	// overwrote; the rewrite would change which values are read.
	if cObj == aObj || cObj == bObj {
		return nil
	}
	// The rewrite moves C (and keeps A, B, I) inside the j/k loop bodies;
	// a name collision with the loop variables would capture them.
	for _, name := range []string{aIdent.Name, bIdent.Name, cIdent.Name, iIdent.Name} {
		if name == jIdent.Name || name == kIdent.Name {
			return nil
		}
	}

	// Moving output stores before all source reads changes ragged-slice panic
	// timing, and separately named [][]float64 values may still share rows.
	// Restrict the edit to fixed arrays whose dimensions prove every access
	// in both loop orders in bounds. Distinct float-only arrays cannot alias.
	aRows, aCols, aOK := ps4008Float64ArrayMatrix(info.TypeOf(aIdent))
	bRows, bCols, bOK := ps4008Float64ArrayMatrix(info.TypeOf(bIdent))
	cRows, cCols, cOK := ps4008Float64ArrayMatrix(info.TypeOf(cIdent))
	if !aOK || !bOK || !cOK || aCols < bRows || cRows < aRows || cCols < bCols {
		return nil
	}
	if !ps4008IndexBoundedByArrayRange(pass, stack, iObj, aObj) {
		return nil
	}

	col := pass.Fset.Position(middle.Pos()).Column
	if col < 1 {
		return nil
	}
	// Assume gofmt indentation (tabs), as ps2005 does.
	indent := strings.Repeat("\t", col-1)
	a, b, c, i, j, k := aIdent.Name, bIdent.Name, cIdent.Name, iIdent.Name, jIdent.Name, kIdent.Name
	var sb strings.Builder
	fmt.Fprintf(&sb, "for %s := range %s[0] {\n", j, b)
	fmt.Fprintf(&sb, "%s\t%s[%s][%s] = 0\n", indent, c, i, j)
	fmt.Fprintf(&sb, "%s}\n", indent)
	fmt.Fprintf(&sb, "%sfor %s := range %s {\n", indent, k, b)
	fmt.Fprintf(&sb, "%s\tfor %s := range %s[0] {\n", indent, j, b)
	fmt.Fprintf(&sb, "%s\t\t%s[%s][%s] += %s[%s][%s] * %s[%s][%s]\n", indent, c, i, j, a, i, k, b, k, j)
	fmt.Fprintf(&sb, "%s\t}\n", indent)
	fmt.Fprintf(&sb, "%s}", indent)
	return &analysis.SuggestedFix{
		Message: "restructure to ikj/axpy order: zero the output row, then accumulate rank-1 updates (per-element accumulation order preserved)",
		TextEdits: []analysis.TextEdit{
			{Pos: middle.Pos(), End: middle.End(), NewText: []byte(sb.String())},
		},
	}
}

// ps4008IndexBoundedByArrayRange requires the row index to be introduced by
// an enclosing `for i := range a`. Fixed dimensions do not prove a separately
// supplied index in bounds; zeroing C before reading A would then change the
// observable state of the original bounds panic.
func ps4008IndexBoundedByArrayRange(pass *analysis.Pass, stack []ast.Node, index, array types.Object) bool {
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		loop, ok := stack[stackIndex].(*ast.RangeStmt)
		if !ok || loop.Tok != token.DEFINE || loop.Value != nil {
			continue
		}
		key, ok := loop.Key.(*ast.Ident)
		if !ok || identObject(pass, key) != index {
			continue
		}
		root, ok := ps2110Unparen(loop.X).(*ast.Ident)
		return ok && identObject(pass, root) == array
	}
	return false
}

// ps4008MatrixIndex unpacks BASE[ROW][COL] where all three are plain
// identifiers.
func ps4008MatrixIndex(e ast.Expr) (base, row, col *ast.Ident, ok bool) {
	outer, ok2 := e.(*ast.IndexExpr)
	if !ok2 {
		return nil, nil, nil, false
	}
	col, ok2 = outer.Index.(*ast.Ident)
	if !ok2 {
		return nil, nil, nil, false
	}
	in, ok2 := outer.X.(*ast.IndexExpr)
	if !ok2 {
		return nil, nil, nil, false
	}
	row, ok2 = in.Index.(*ast.Ident)
	if !ok2 {
		return nil, nil, nil, false
	}
	base, ok2 = in.X.(*ast.Ident)
	if !ok2 {
		return nil, nil, nil, false
	}
	return base, row, col, true
}

// ps4008Float64ArrayMatrix returns the fixed rectangular dimensions when t is
// an array whose elements are arrays of exactly float64.
func ps4008Float64ArrayMatrix(t types.Type) (rows, columns int64, ok bool) {
	if t == nil {
		return 0, 0, false
	}
	outer, ok := t.Underlying().(*types.Array)
	if !ok {
		return 0, 0, false
	}
	in, ok := outer.Elem().Underlying().(*types.Array)
	if !ok {
		return 0, 0, false
	}
	bt, ok := in.Elem().(*types.Basic)
	return outer.Len(), in.Len(), ok && bt.Kind() == types.Float64
}
