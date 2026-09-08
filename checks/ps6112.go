package checks

import (
	"go/ast"
	"go/build"
	"go/constant"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6112 implements owner issue #797. Architecture tile heights and fallback
// semantics are project facts, so only complete configured chains are checked.
var PS6112 = register(&lint.Check{
	ID:          "PS6112",
	Category:    "vector",
	Slug:        "scheduler-grain-microkernel-tile",
	Level:       lint.LevelAggressive,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"schedulerTileGrainContracts"},
	Doc: lint.Documentation{
		Title: "a repeated scheduler grain is not a multiple of one target's microkernel tile height",
		Text: `A shared scheduler grain can align with one architecture's row tile
and force every full task through another architecture's scalar row fallback.
This is distinct from the unavoidable final task: when grain%tile is nonzero,
every complete scheduled band has a remainder; when the grain is aligned, a
dynamic total that leaves one partial band is deliberately not reported.

PS6112 is deliberately configured and narrow. Each contract names exact typed
package functions for the scheduler, band, and kernel entry; the source grain
constant; one-based row argument roles; and at least two architecture regimes.
Each regime fixes GOOS, GOARCH, build tags, tile height, an exact tile router
and row role, tiled kernel, and scalar fallback. The router may be the entry or
one explicit direct hop below it. The booleans are project promises that it
routes full tiles to the tiled kernel and the remainder to the scalar fallback.
Names alone prove nothing.

The analyzer resolves active and ignored package files under every configured
build regime, and requires at least two complete regimes. It requires one
positive integer-literal constant selection, the exact active package constant
object (not a same-named local), exact helper declarations, both configured
kernel calls, typed scheduler-to-band and band-to-kernel forwarding, and one of
the bounded native-int scheduling shapes. The compact form is exactly a
zero-based unit-step loop over the ceiling count; the owner form is an unbounded
task loop with a zero-initialized atomic.Int64 Add(1)-1 acquisition, terminal
task-count guard, and exact reverse-band mapping inside the contract's typed
synchronous-runner callback slot. The runner's executes-before-return flag is
a reviewed project promise; arbitrary callback arguments remain opaque. Both
forms require the exact index*grain start and min(grain, extent-start) clamp in
that loop. The extent must be an unchanged parameter or a dominating local
snapshot that is neither rebound nor address-exposed anywhere in the candidate
function or synchronous callback; this includes loop-carried writes after the
band call.
Narrow/custom/unsigned arithmetic, stored closures, methods, imported
helpers, aliases at the call boundaries, mutable grains, ambiguous build
selections, missing regimes or scalar/tiled implementations, unknown control
flow, zero-trip or statically partial-only extents, and incomplete contracts
stay silent. Version one requires a specific GOOS per regime so OS-specific
files cannot be accidentally combined.

There is NO automatic fix. A larger aligned grain can reduce parallel balance
or worsen cache behavior. Before changing it, prove the accepted extent range
keeps the native-int ceiling and task-count arithmetic in range. Choose each
architecture's grain from measured full workloads, keep the final partial task
bounded with min, and retain cross-build tests for the default, tagged SIMD,
and scalar-only configurations. Add
//perfscan:tile-grain-intentional or //perfscan:tile-grain-profiled with a
nonempty reason when a measured load-balance decision intentionally keeps a
misaligned grain. Generic //perfscan:ignore PS6112 remains available too.`,
		Before: `const attentionBandRows = 30

bands := (rows + attentionBandRows - 1) / attentionBandRows
start := band * attentionBandRows
count := min(attentionBandRows, rows-start)
attentionBand(input, start, count) // ARM64 4-row tile: 30%4 == 2 every full band`,
		After: `// Keep the amd64 six-row regime at 30 rows.
// In the measured arm64 SIMD build, use a separately selected 32-row grain.
const attentionBandRows = 32

// Preserve the same ceiling division and final partial-band clamp.`,
		MeasuredWin: `The merged GoAI owner change selected 32 rows for arm64
SIMD while retaining 30 for the amd64 six-row tile. On Apple M2 Pro, three
alternating campaigns (21 samples per arm) reduced five forward-attention
cells by 23.90%-40.88% (geomean 28.63%, p<.001), while decode was neutral
(p=.881). A matched profile removed gemmF32RowsScalar from the candidate hot
set; the control attributed 6.02 s flat there. Both exact binaries retained
9 allocs/op. Evidence: jxsl13/goai PR #1130, merge
06582e67fa4ce0445b8d0e49b02cb9ae72defea8. These results are one native
machine and workload matrix, not a universal grain-size speedup.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6112",
		Doc:  "repeated scheduler grains misaligned with configured architecture microkernel tiles",
		Run:  runPS6112,
	},
})

type ps6112Definition struct {
	expression ast.Expr
	writes     int
	declared   bool
}

type ps6112ResolvedVariant struct {
	variant config.SchedulerTileGrainVariant
	grain   int64
}

type ps6112LoopIndex struct {
	loop   ast.Node
	extent types.Object
}

func runPS6112(pass *analysis.Pass) (any, error) {
	return runPS6112WithContracts(pass, config.Current().SchedulerTileGrainContracts)
}

func runPS6112WithContracts(pass *analysis.Pass, contracts []config.SchedulerTileGrainContract) (any, error) {
	if len(contracts) == 0 {
		return nil, nil
	}
	sources := ps6074PackageSources(pass)
	functions := ps6112ActiveFunctions(pass)
	reported := make(map[string]bool)
	for index := range contracts {
		contract := &contracts[index]
		if !contract.Valid() || !strings.HasPrefix(contract.Scheduler, pass.Pkg.Path()+".") {
			continue
		}
		scheduler := functions[contract.Scheduler]
		band := functions[contract.Band]
		if scheduler == nil || band == nil || ps6112Suppressed(scheduler.file, scheduler.declaration) ||
			!ps6112SchedulerShape(pass, scheduler.declaration, contract) ||
			!ps6112BandForwardsRows(pass, band.declaration, contract) {
			continue
		}
		resolvedVariants := ps6112ResolveVariants(sources, contract)
		if len(resolvedVariants) < 2 || len(resolvedVariants) != len(contract.Variants) {
			continue
		}
		for resolvedIndex := range resolvedVariants {
			resolved := &resolvedVariants[resolvedIndex]
			remainder := resolved.grain % int64(resolved.variant.TileHeight)
			if remainder == 0 {
				continue
			}
			key := contract.Scheduler + "\x00" + resolved.variant.GOOS + "\x00" +
				resolved.variant.GOARCH + "\x00" + strings.Join(resolved.variant.BuildTags, ",")
			if reported[key] {
				continue
			}
			reported[key] = true
			tags := strings.Join(resolved.variant.BuildTags, ",")
			var target strings.Builder
			target.Grow(len(resolved.variant.GOOS) + len(resolved.variant.GOARCH) +
				len(tags) + 2)
			target.WriteString(resolved.variant.GOOS)
			target.WriteByte('/')
			target.WriteString(resolved.variant.GOARCH)
			if len(resolved.variant.BuildTags) > 0 {
				target.WriteByte('+')
				target.WriteString(tags)
			}
			pass.Reportf(scheduler.declaration.Name.Pos(), "%s selects grain %d for target %s and a %d-row microkernel tile (%d%%%d=%d); every repeated full band can enter %s. Select a measured architecture-specific multiple-of-%d grain, preserve the final partial band, native-int arithmetic range, and load-balancing behavior, and add cross-build preservation tests (PS6112 advisory, no automatic fix)",
				scheduler.declaration.Name.Name, resolved.grain, target.String(), resolved.variant.TileHeight,
				resolved.grain, resolved.variant.TileHeight, remainder, resolved.variant.ScalarFallback,
				resolved.variant.TileHeight)
		}
	}
	return nil, nil
}

type ps6112ActiveFunction struct {
	file        *ast.File
	declaration *ast.FuncDecl
}

func ps6112ActiveFunctions(pass *analysis.Pass) map[string]*ps6112ActiveFunction {
	result := make(map[string]*ps6112ActiveFunction)
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil || function.Recv != nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
			if ok {
				result[ps6090FunctionID(object)] = &ps6112ActiveFunction{file: file, declaration: function}
			}
		}
	}
	return result
}

func ps6112SchedulerShape(pass *analysis.Pass, function *ast.FuncDecl, contract *config.SchedulerTileGrainContract) bool {
	grainName := ps6112FinalName(contract.GrainConstant)
	grainObject := ps6112PackageObject(pass, grainName)
	if grainObject == nil {
		return false
	}
	definitions := ps6112Definitions(pass, function.Body)
	ceilingObjects := make(map[types.Object]types.Object)
	for object, definition := range definitions {
		if definition.writes != 1 {
			continue
		}
		extent, ok := ps6112CeilingExtent(pass, definition.expression, grainObject)
		if ok && ps6112StableExtent(pass, extent, definitions) {
			ceilingObjects[object] = extent
		}
	}
	parents := ps6090Parents(function.Body)
	loopIndexes := ps6112BoundedLoopIndexes(pass, function.Body, ceilingObjects, definitions)
	startObjects := make(map[types.Object]ps6112LoopIndex)
	rowObjects := make(map[types.Object]ps6112LoopIndex)
	for object, definition := range definitions {
		if definition.writes != 1 || definition.expression == nil {
			continue
		}
		if index, ok := ps6112GrainStart(pass, definition.expression, grainObject, loopIndexes); ok &&
			ps6112EnclosingLoop(definition.expression, parents) == index.loop {
			startObjects[object] = index
		}
	}
	for object, definition := range definitions {
		if definition.writes == 1 {
			if row, ok := ps6112MinRemainder(pass, definition.expression, grainObject, startObjects); ok &&
				ps6112EnclosingLoop(definition.expression, parents) == row.loop {
				rowObjects[object] = row
			}
		}
	}
	matchedBand := false
	dead := ps6112DeadNodes(pass, function.Body)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if dead[node] {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			if !ps6112DirectFuncLiteralUse(pass, value, parents, contract) {
				return false
			}
		case *ast.CallExpr:
			if ps6112CallID(pass, value) != contract.Band || len(value.Args) < contract.BandRowsArgument {
				break
			}
			identifier, ok := ps2110Unparen(value.Args[contract.BandRowsArgument-1]).(*ast.Ident)
			row, rowOK := rowObjects[pass.TypesInfo.Uses[identifier]]
			matchedBand = matchedBand || ok && rowOK && row.loop != nil &&
				ps6112EnclosingLoop(value, parents) == row.loop &&
				ps6112ExtentUnchanged(pass, function.Body, row.extent)
		}
		return true
	})
	return len(ceilingObjects) > 0 && len(startObjects) > 0 && len(rowObjects) > 0 && matchedBand
}

func ps6112DirectFuncLiteralUse(pass *analysis.Pass, literal *ast.FuncLit, parents map[ast.Node]ast.Node,
	contract *config.SchedulerTileGrainContract) bool {
	var expression ast.Expr = literal
	parent := parents[literal]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = paren
		parent = parents[paren]
	}
	call, ok := parent.(*ast.CallExpr)
	if !ok {
		return false
	}
	if ps2110Unparen(call.Fun) == ps2110Unparen(expression) {
		return true
	}
	if contract.SynchronousRunner == "" || contract.SynchronousRunnerWorkArgument <= 0 ||
		ps6112CallID(pass, call) != contract.SynchronousRunner ||
		len(call.Args) < contract.SynchronousRunnerWorkArgument {
		return false
	}
	return ps2110Unparen(call.Args[contract.SynchronousRunnerWorkArgument-1]) == ps2110Unparen(expression)
}

func ps6112ExtentUnchanged(pass *analysis.Pass, body *ast.BlockStmt, extent types.Object) bool {
	stable := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !stable || node == nil {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, left := range value.Lhs {
				identifier, ok := ps2110Unparen(left).(*ast.Ident)
				if ok && pass.TypesInfo.Uses[identifier] == extent {
					stable = false
					return false
				}
			}
		case *ast.IncDecStmt:
			identifier, ok := ps2110Unparen(value.X).(*ast.Ident)
			if ok && pass.TypesInfo.Uses[identifier] == extent {
				stable = false
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND && ps6112DirectObject(pass, value.X, extent) {
				stable = false
				return false
			}
		case *ast.RangeStmt:
			if value.Tok != token.ASSIGN {
				break
			}
			for _, target := range []ast.Expr{value.Key, value.Value} {
				identifier, ok := ps2110Unparen(target).(*ast.Ident)
				if ok && pass.TypesInfo.Uses[identifier] == extent {
					stable = false
					return false
				}
			}
		}
		return true
	})
	return stable
}

func ps6112CeilingExtent(pass *analysis.Pass, expression ast.Expr, grain types.Object) (types.Object, bool) {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.QUO || !ps6112DirectObject(pass, binary.Y, grain) ||
		!ps6112NativeIntType(pass.TypesInfo.TypeOf(binary)) {
		return nil, false
	}
	numerator, ok := ps2110Unparen(binary.X).(*ast.BinaryExpr)
	if !ok {
		return nil, false
	}
	var extent ast.Expr
	if numerator.Op == token.SUB && ps6112IntegerConstantEquals(numerator.Y, 1) {
		addition, ok := ps2110Unparen(numerator.X).(*ast.BinaryExpr)
		if ok && addition.Op == token.ADD {
			extent = ps6112OtherDirectObject(pass, addition, grain)
		}
	} else if numerator.Op == token.ADD {
		if ps6112GrainMinusOne(pass, numerator.X, grain) {
			extent = numerator.Y
		} else if ps6112GrainMinusOne(pass, numerator.Y, grain) {
			extent = numerator.X
		}
	}
	identifier, ok := ps2110Unparen(extent).(*ast.Ident)
	if !ok || !ps6112NativeIntType(pass.TypesInfo.TypeOf(identifier)) {
		return nil, false
	}
	object := pass.TypesInfo.Uses[identifier]
	return object, object != nil && object != grain
}

func ps6112StableExtent(pass *analysis.Pass, object types.Object, definitions map[types.Object]ps6112Definition) bool {
	variable, ok := object.(*types.Var)
	if !ok || variable.IsField() || variable.Pkg() == nil || variable.Parent() == variable.Pkg().Scope() ||
		!ps6112NativeIntType(variable.Type()) {
		return false
	}
	definition, local := definitions[object]
	return !local || !definition.declared || definition.writes == 1 &&
		ps6112StableExtentExpression(pass, definition.expression, definitions, make(map[types.Object]bool))
}

func ps6112StableExtentExpression(pass *analysis.Pass, expression ast.Expr,
	definitions map[types.Object]ps6112Definition, visiting map[types.Object]bool) bool {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object, ok := pass.TypesInfo.ObjectOf(value).(*types.Var)
		if !ok || object.IsField() || object.Pkg() == nil || object.Parent() == object.Pkg().Scope() ||
			!ps6112NativeIntType(object.Type()) || visiting[object] {
			return false
		}
		definition, local := definitions[object]
		if !local || !definition.declared {
			return true
		}
		if definition.writes != 1 {
			return false
		}
		visiting[object] = true
		stable := ps6112StableExtentExpression(pass, definition.expression, definitions, visiting)
		delete(visiting, object)
		return stable
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[value]
		root, ok := ps2110Unparen(value.X).(*ast.Ident)
		rootObject, rootOK := pass.TypesInfo.ObjectOf(root).(*types.Var)
		if selection == nil || !ok || !rootOK || rootObject.IsField() || rootObject.Pkg() == nil ||
			rootObject.Parent() == rootObject.Pkg().Scope() {
			return false
		}
		if definition, local := definitions[rootObject]; local && definition.writes != 1 {
			return false
		}
		_, pointer := types.Unalias(rootObject.Type()).Underlying().(*types.Pointer)
		return !pointer && ps6112NativeIntType(selection.Type())
	}
	return false
}

func ps6112OtherDirectObject(pass *analysis.Pass, binary *ast.BinaryExpr, object types.Object) ast.Expr {
	if ps6112DirectObject(pass, binary.X, object) && !ps6112ContainsObject(pass, binary.Y, object) {
		return binary.Y
	}
	if ps6112DirectObject(pass, binary.Y, object) && !ps6112ContainsObject(pass, binary.X, object) {
		return binary.X
	}
	return nil
}

func ps6112GrainMinusOne(pass *analysis.Pass, expression ast.Expr, grain types.Object) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	return ok && binary.Op == token.SUB && ps6112DirectObject(pass, binary.X, grain) &&
		ps6112IntegerConstantEquals(binary.Y, 1)
}

func ps6112IntegerConstantEquals(expression ast.Expr, want int64) bool {
	value, ok := ps6112IntegerConstant(expression)
	return ok && value == want
}

func ps6112DirectObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && pass.TypesInfo.Uses[identifier] == object
}

func ps6112ContainsObject(pass *analysis.Pass, expression ast.Expr, target types.Object) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && pass.TypesInfo.Uses[identifier] == target {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps6112BoundedLoopIndexes(pass *analysis.Pass, body *ast.BlockStmt, ceilings map[types.Object]types.Object, definitions map[types.Object]ps6112Definition) map[types.Object]ps6112LoopIndex {
	indexes := make(map[types.Object]ps6112LoopIndex)
	ast.Inspect(body, func(node ast.Node) bool {
		switch loop := node.(type) {
		case *ast.RangeStmt:
			ceiling, ok := ps6112DirectMappedObject(pass, loop.X, ceilings)
			identifier, keyOK := ps2110Unparen(loop.Key).(*ast.Ident)
			var object types.Object
			if keyOK {
				object = pass.TypesInfo.Defs[identifier]
			}
			if ok && keyOK && loop.Tok == token.DEFINE && loop.Value == nil && object != nil &&
				ps6112NativeIntType(object.Type()) {
				indexes[object] = ps6112LoopIndex{loop: loop, extent: ceiling}
			}
		case *ast.ForStmt:
			if object, extent, ok := ps6112CanonicalForIndex(pass, loop, ceilings); ok {
				indexes[object] = ps6112LoopIndex{loop: loop, extent: extent}
			}
			for object, extent := range ps6112GuardedTaskIndexes(pass, loop, ceilings, definitions) {
				indexes[object] = ps6112LoopIndex{loop: loop, extent: extent}
			}
		}
		return true
	})
	return indexes
}

func ps6112DirectMappedObject(pass *analysis.Pass, expression ast.Expr, values map[types.Object]types.Object) (types.Object, bool) {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok {
		return nil, false
	}
	extent, ok := values[pass.TypesInfo.Uses[identifier]]
	return extent, ok
}

func ps6112CanonicalForIndex(pass *analysis.Pass, loop *ast.ForStmt, ceilings map[types.Object]types.Object) (types.Object, types.Object, bool) {
	initializer, ok := loop.Init.(*ast.AssignStmt)
	if !ok || initializer.Tok != token.DEFINE || len(initializer.Lhs) != 1 || len(initializer.Rhs) != 1 ||
		!ps6112IntegerConstantEquals(initializer.Rhs[0], 0) {
		return nil, nil, false
	}
	identifier, ok := ps2110Unparen(initializer.Lhs[0]).(*ast.Ident)
	if !ok {
		return nil, nil, false
	}
	object := pass.TypesInfo.Defs[identifier]
	condition, ok := ps2110Unparen(loop.Cond).(*ast.BinaryExpr)
	if object == nil || !ps6112NativeIntType(object.Type()) || !ok || condition.Op != token.LSS ||
		!ps6112DirectObject(pass, condition.X, object) {
		return nil, nil, false
	}
	extent, ok := ps6112DirectMappedObject(pass, condition.Y, ceilings)
	post, postOK := loop.Post.(*ast.IncDecStmt)
	if !ok || !postOK || post.Tok != token.INC || !ps6112DirectObject(pass, post.X, object) {
		return nil, nil, false
	}
	return object, extent, true
}

func ps6112GuardedTaskIndexes(pass *analysis.Pass, loop *ast.ForStmt, ceilings map[types.Object]types.Object,
	definitions map[types.Object]ps6112Definition) map[types.Object]types.Object {
	result := make(map[types.Object]types.Object)
	if loop.Init != nil || loop.Cond != nil || loop.Post != nil {
		return result
	}
	for object, definition := range definitions {
		if definition.writes != 1 || definition.expression == nil || definition.expression.Pos() < loop.Body.Pos() ||
			definition.expression.End() > loop.Body.End() || !ps6112NativeIntType(object.Type()) {
			continue
		}
		ceiling, task, divisor, ok := ps6112ReverseBandIndex(pass, definition.expression, ceilings, definitions)
		if !ok || !ps6112TaskBounded(pass, loop.Body, task, divisor, ceiling, definitions, definition.expression.Pos()) {
			continue
		}
		result[object] = ceilings[ceiling]
	}
	return result
}

func ps6112ReverseBandIndex(pass *analysis.Pass, expression ast.Expr,
	ceilings map[types.Object]types.Object, definitions map[types.Object]ps6112Definition) (types.Object, types.Object, ast.Expr, bool) {
	outer, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || outer.Op != token.SUB {
		return nil, nil, nil, false
	}
	left, leftOK := ps2110Unparen(outer.X).(*ast.BinaryExpr)
	right, rightOK := ps2110Unparen(outer.Y).(*ast.BinaryExpr)
	if !leftOK || left.Op != token.SUB || !ps6112IntegerConstantEquals(left.Y, 1) ||
		!rightOK || right.Op != token.QUO {
		return nil, nil, nil, false
	}
	ceilingIdentifier, ceilingOK := ps2110Unparen(left.X).(*ast.Ident)
	taskIdentifier, taskOK := ps2110Unparen(right.X).(*ast.Ident)
	if !ceilingOK || !taskOK || !ps6112NativeIntType(pass.TypesInfo.TypeOf(right.Y)) {
		return nil, nil, nil, false
	}
	ceiling := pass.TypesInfo.Uses[ceilingIdentifier]
	task := pass.TypesInfo.Uses[taskIdentifier]
	_, configured := ceilings[ceiling]
	return ceiling, task, right.Y, configured && task != nil &&
		ps6112NativeIntType(task.Type()) && ps6112StableExpression(pass, right.Y, definitions)
}

func ps6112TaskBounded(pass *analysis.Pass, body *ast.BlockStmt, task types.Object, divisor ast.Expr, ceiling types.Object,
	definitions map[types.Object]ps6112Definition, before token.Pos) bool {
	taskDefinition, ok := definitions[task]
	if !ok || taskDefinition.writes != 1 || taskDefinition.expression == nil ||
		taskDefinition.expression.Pos() < body.Pos() || taskDefinition.expression.Pos() >= before ||
		!ps6112TaskAcquisition(pass, taskDefinition.expression, definitions) {
		return false
	}
	for _, statement := range body.List {
		if statement.Pos() >= before {
			break
		}
		guard, ok := statement.(*ast.IfStmt)
		if !ok || guard.Init != nil || guard.Else != nil {
			continue
		}
		condition, conditionOK := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
		if !conditionOK || condition.Op != token.GEQ ||
			!ps6112DirectObject(pass, condition.X, task) || len(guard.Body.List) != 1 {
			continue
		}
		returned, terminal := guard.Body.List[0].(*ast.ReturnStmt)
		if !terminal || len(returned.Results) != 0 {
			continue
		}
		tasksIdentifier, ok := ps2110Unparen(condition.Y).(*ast.Ident)
		if !ok {
			continue
		}
		tasks := pass.TypesInfo.Uses[tasksIdentifier]
		definition, ok := definitions[tasks]
		if ok && definition.writes == 1 && ps6112ExactProduct(pass, definition.expression, ceiling, divisor) {
			return true
		}
	}
	return false
}

func ps6112TaskAcquisition(pass *analysis.Pass, expression ast.Expr,
	definitions map[types.Object]ps6112Definition) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.SUB || !ps6112IntegerConstantEquals(binary.Y, 1) {
		return false
	}
	conversion, ok := ps2110Unparen(binary.X).(*ast.CallExpr)
	if !ok || conversion.Ellipsis.IsValid() || len(conversion.Args) != 1 ||
		!ps6112NativeIntType(pass.TypesInfo.TypeOf(conversion)) {
		return false
	}
	typeName, typeOK := ps2110Unparen(conversion.Fun).(*ast.Ident)
	if !typeOK || pass.TypesInfo.Uses[typeName] != types.Universe.Lookup("int") {
		return false
	}
	addition, ok := ps2110Unparen(conversion.Args[0]).(*ast.CallExpr)
	if !ok || addition.Ellipsis.IsValid() || len(addition.Args) != 1 ||
		!ps6112IntegerConstantEquals(addition.Args[0], 1) {
		return false
	}
	callee, signature, ok := typedCallee(pass, addition.Fun)
	if !ok || callee.Name() != "Add" || callee.Pkg() == nil || callee.Pkg().Path() != "sync/atomic" ||
		!typedReceiverNamed(signature, "sync/atomic", "Int64") {
		return false
	}
	selector, ok := ps2110Unparen(addition.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	receiver, receiverOK := ps2110Unparen(selector.X).(*ast.Ident)
	if !receiverOK {
		return false
	}
	definition, ok := definitions[pass.TypesInfo.ObjectOf(receiver)]
	return ok && definition.writes == 1 && definition.expression == nil
}

func ps6112ExactProduct(pass *analysis.Pass, expression ast.Expr, left types.Object, right ast.Expr) bool {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	return ok && binary.Op == token.MUL && ps6112NativeIntType(pass.TypesInfo.TypeOf(binary)) &&
		(ps6112DirectObject(pass, binary.X, left) && ps6112SameExpression(pass, binary.Y, right) ||
			ps6112DirectObject(pass, binary.Y, left) && ps6112SameExpression(pass, binary.X, right))
}

func ps6112SameExpression(pass *analysis.Pass, left, right ast.Expr) bool {
	left = ps2110Unparen(left)
	right = ps2110Unparen(right)
	switch leftValue := left.(type) {
	case *ast.Ident:
		rightValue, ok := right.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(leftValue) == pass.TypesInfo.ObjectOf(rightValue)
	case *ast.SelectorExpr:
		rightValue, ok := right.(*ast.SelectorExpr)
		return ok && pass.TypesInfo.Selections[leftValue] != nil && pass.TypesInfo.Selections[rightValue] != nil &&
			pass.TypesInfo.Selections[leftValue].Obj() == pass.TypesInfo.Selections[rightValue].Obj() &&
			ps6112SameExpression(pass, leftValue.X, rightValue.X)
	}
	return false
}

func ps6112StableExpression(pass *analysis.Pass, expression ast.Expr, definitions map[types.Object]ps6112Definition) bool {
	stable := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !stable {
			return false
		}
		switch value := node.(type) {
		case *ast.CallExpr, *ast.IndexExpr, *ast.IndexListExpr:
			stable = false
			return false
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			root, rootOK := ps2110Unparen(value.X).(*ast.Ident)
			if selection == nil || !rootOK || pass.TypesInfo.ObjectOf(root) == nil {
				stable = false
			}
			return false
		case *ast.Ident:
			object := pass.TypesInfo.ObjectOf(value)
			if definition, ok := definitions[object]; ok && definition.writes != 1 {
				stable = false
				return false
			}
		}
		return true
	})
	return stable
}

func ps6112EnclosingLoop(node ast.Node, parents map[ast.Node]ast.Node) ast.Node {
	for current := node; current != nil; current = parents[current] {
		switch current.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			return current
		}
	}
	return nil
}

func ps6112PackageObject(pass *analysis.Pass, name string) types.Object {
	object := pass.Pkg.Scope().Lookup(name)
	constantObject, ok := object.(*types.Const)
	if !ok || constantObject.Pkg() != pass.Pkg || !ps6112NativeIntOrUntyped(constantObject.Type()) {
		return nil
	}
	return object
}

func ps6112Definitions(pass *analysis.Pass, body *ast.BlockStmt) map[types.Object]ps6112Definition {
	definitions := make(map[types.Object]ps6112Definition)
	dead := ps6112DeadNodes(pass, body)
	ast.Inspect(body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if dead[node] {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			if len(value.Lhs) != len(value.Rhs) {
				for _, left := range value.Lhs {
					ps6112RecordWrite(pass, definitions, left, nil)
				}
				break
			}
			for index, left := range value.Lhs {
				expression := value.Rhs[index]
				if value.Tok != token.DEFINE && value.Tok != token.ASSIGN {
					expression = nil
				}
				ps6112RecordWrite(pass, definitions, left, expression)
			}
		case *ast.IncDecStmt:
			ps6112RecordWrite(pass, definitions, value.X, nil)
		case *ast.ValueSpec:
			for index, name := range value.Names {
				var expression ast.Expr
				if index < len(value.Values) {
					expression = value.Values[index]
				}
				object := pass.TypesInfo.Defs[name]
				definition := definitions[object]
				definition.writes++
				definition.declared = true
				definition.expression = expression
				definitions[object] = definition
			}
		}
		return true
	})
	return definitions
}

func ps6112RecordWrite(pass *analysis.Pass, definitions map[types.Object]ps6112Definition, expression ast.Expr, value ast.Expr) {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return
	}
	object := pass.TypesInfo.Defs[identifier]
	declared := object != nil
	if object == nil {
		object = pass.TypesInfo.Uses[identifier]
	}
	if object == nil {
		return
	}
	definition := definitions[object]
	definition.writes++
	definition.declared = definition.declared || declared
	if definition.writes == 1 {
		definition.expression = value
	} else {
		definition.expression = nil
	}
	definitions[object] = definition
}

func ps6112GrainStart(pass *analysis.Pass, expression ast.Expr, grain types.Object,
	loopIndexes map[types.Object]ps6112LoopIndex) (ps6112LoopIndex, bool) {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.MUL || !ps6112NativeIntType(pass.TypesInfo.TypeOf(binary)) {
		return ps6112LoopIndex{}, false
	}
	var indexExpression ast.Expr
	if ps6112DirectObject(pass, binary.X, grain) {
		indexExpression = binary.Y
	} else if ps6112DirectObject(pass, binary.Y, grain) {
		indexExpression = binary.X
	} else {
		return ps6112LoopIndex{}, false
	}
	identifier, ok := ps2110Unparen(indexExpression).(*ast.Ident)
	if !ok {
		return ps6112LoopIndex{}, false
	}
	index, ok := loopIndexes[pass.TypesInfo.Uses[identifier]]
	return index, ok
}

func ps6112MinRemainder(pass *analysis.Pass, expression ast.Expr, grain types.Object,
	starts map[types.Object]ps6112LoopIndex) (ps6112LoopIndex, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || !typedBuiltinName(pass, call.Fun, "min") || len(call.Args) != 2 || call.Ellipsis.IsValid() ||
		!ps6112NativeIntType(pass.TypesInfo.TypeOf(call)) {
		return ps6112LoopIndex{}, false
	}
	if ps6112DirectObject(pass, call.Args[0], grain) {
		return ps6112RemainingExtent(pass, call.Args[1], starts)
	}
	if ps6112DirectObject(pass, call.Args[1], grain) {
		return ps6112RemainingExtent(pass, call.Args[0], starts)
	}
	return ps6112LoopIndex{}, false
}

func ps6112RemainingExtent(pass *analysis.Pass, expression ast.Expr,
	starts map[types.Object]ps6112LoopIndex) (ps6112LoopIndex, bool) {
	binary, ok := ps2110Unparen(expression).(*ast.BinaryExpr)
	if !ok || binary.Op != token.SUB || !ps6112NativeIntType(pass.TypesInfo.TypeOf(binary)) {
		return ps6112LoopIndex{}, false
	}
	extentIdentifier, extentOK := ps2110Unparen(binary.X).(*ast.Ident)
	startIdentifier, startOK := ps2110Unparen(binary.Y).(*ast.Ident)
	if !extentOK || !startOK {
		return ps6112LoopIndex{}, false
	}
	start, ok := starts[pass.TypesInfo.Uses[startIdentifier]]
	return start, ok && pass.TypesInfo.Uses[extentIdentifier] == start.extent
}

func ps6112BandForwardsRows(pass *analysis.Pass, function *ast.FuncDecl, contract *config.SchedulerTileGrainContract) bool {
	row := ps6112ParameterObject(pass, function, contract.BandRowsArgument)
	if row == nil || !ps6112NativeIntType(row.Type()) {
		return false
	}
	matched := false
	dead := ps6112DeadNodes(pass, function.Body)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if node == nil {
			return true
		}
		if dead[node] {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok || ps6112CallID(pass, call) != contract.KernelEntry || len(call.Args) < contract.KernelRowsArgument {
			return true
		}
		identifier, ok := ps2110Unparen(call.Args[contract.KernelRowsArgument-1]).(*ast.Ident)
		matched = matched || ok && pass.TypesInfo.Uses[identifier] == row
		return true
	})
	return matched
}

func ps6112ParameterObject(pass *analysis.Pass, function *ast.FuncDecl, position int) *types.Var {
	object, ok := pass.TypesInfo.Defs[function.Name].(*types.Func)
	if !ok {
		return nil
	}
	signature, ok := object.Type().(*types.Signature)
	if !ok || position <= 0 || position > signature.Params().Len() {
		return nil
	}
	return signature.Params().At(position - 1)
}

func ps6112NativeIntType(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Int
}

func ps6112NativeIntOrUntyped(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	return ok && (basic.Kind() == types.Int || basic.Kind() == types.UntypedInt)
}

func ps6112CallID(pass *analysis.Pass, call *ast.CallExpr) string {
	function, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return ""
	}
	return ps6090FunctionID(function)
}

func ps6112ResolveVariants(sources []ps6074SourceFile, contract *config.SchedulerTileGrainContract) []ps6112ResolvedVariant {
	resolved := make([]ps6112ResolvedVariant, 0, len(contract.Variants))
	for index := range contract.Variants {
		variant := &contract.Variants[index]
		matching := ps6112MatchingSources(sources, variant)
		grain, grainFile, ok := ps6112ResolveGrain(matching, ps6112FinalName(contract.GrainConstant))
		if !ok || ps6112FileSuppressed(grainFile.file) ||
			!ps6112VariantKernel(matching, contract, variant) {
			continue
		}
		resolved = append(resolved, ps6112ResolvedVariant{variant: *variant, grain: grain})
	}
	return resolved
}

func ps6112MatchingSources(sources []ps6074SourceFile, variant *config.SchedulerTileGrainVariant) []ps6074SourceFile {
	context := build.Default
	context.GOOS = variant.GOOS
	context.GOARCH = variant.GOARCH
	context.BuildTags = slices.Clone(variant.BuildTags)
	context.CgoEnabled = slices.Contains(variant.BuildTags, "cgo")
	matching := make([]ps6074SourceFile, 0, len(sources))
	for _, source := range sources {
		if strings.HasSuffix(strings.TrimSuffix(source.filename, ".go"), "_test") {
			continue
		}
		ok, err := context.MatchFile(filepath.Dir(source.filename), filepath.Base(source.filename))
		if err == nil && ok {
			matching = append(matching, source)
		}
	}
	return matching
}

func ps6112ResolveGrain(sources []ps6074SourceFile, name string) (int64, ps6074SourceFile, bool) {
	var value int64
	var source ps6074SourceFile
	matches := 0
	shadowsInt := ps6112TopLevelName(sources, "int")
	for _, candidate := range sources {
		for _, declaration := range candidate.file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.CONST {
				continue
			}
			for _, spec := range general.Specs {
				values, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if values.Type != nil {
					identifier, ok := ps2110Unparen(values.Type).(*ast.Ident)
					if !ok || identifier.Name != "int" || shadowsInt {
						for _, nameIdentifier := range values.Names {
							if nameIdentifier.Name == name {
								return 0, ps6074SourceFile{}, false
							}
						}
						continue
					}
				}
				for index, identifier := range values.Names {
					if identifier.Name != name || index >= len(values.Values) {
						continue
					}
					integer, ok := ps6112IntegerConstant(values.Values[index])
					if !ok || integer <= 0 {
						return 0, ps6074SourceFile{}, false
					}
					matches++
					value, source = integer, candidate
				}
			}
		}
	}
	return value, source, matches == 1
}

func ps6112TopLevelName(sources []ps6074SourceFile, name string) bool {
	for _, source := range sources {
		for _, declaration := range source.file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Name.Name == name {
					return true
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch typed := spec.(type) {
					case *ast.TypeSpec:
						if typed.Name.Name == name {
							return true
						}
					case *ast.ValueSpec:
						for _, identifier := range typed.Names {
							if identifier.Name == name {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func ps6112IntegerConstant(expression ast.Expr) (int64, bool) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.BasicLit:
		if value.Kind != token.INT {
			return 0, false
		}
		constantValue := constant.MakeFromLiteral(value.Value, token.INT, 0)
		result, ok := constant.Int64Val(constantValue)
		return result, ok
	case *ast.UnaryExpr:
		operand, ok := ps6112IntegerConstant(value.X)
		if !ok {
			return 0, false
		}
		switch value.Op {
		case token.ADD:
			return operand, true
		case token.SUB:
			return -operand, operand != -1<<63
		}
	}
	return 0, false
}

func ps6112VariantKernel(sources []ps6074SourceFile, contract *config.SchedulerTileGrainContract, variant *config.SchedulerTileGrainVariant) bool {
	entry, entryCount := ps6112TargetFunction(sources, ps6112FinalName(contract.KernelEntry))
	router, routerCount := ps6112TargetFunction(sources, ps6112FinalName(variant.TileRouter))
	_, tiledCount := ps6112TargetFunction(sources, ps6112FinalName(variant.TiledKernel))
	_, scalarCount := ps6112TargetFunction(sources, ps6112FinalName(variant.ScalarFallback))
	if entryCount != 1 || routerCount != 1 || tiledCount != 1 || scalarCount != 1 ||
		entry.Body == nil || router.Body == nil ||
		ps6112FunctionParameterName(entry, contract.KernelRowsArgument) == "" ||
		ps6112FunctionParameterName(router, variant.TileRouterRowsArgument) == "" {
		return false
	}
	entryRows := ps6112FunctionParameterName(entry, contract.KernelRowsArgument)
	if entry != router && !ps6112DirectCallAtArgument(entry.Body, ps6112FinalName(variant.TileRouter), variant.TileRouterRowsArgument, entryRows) {
		return false
	}
	tiledName := ps6112FinalName(variant.TiledKernel)
	scalarName := ps6112FinalName(variant.ScalarFallback)
	if ps6112LocallyShadows(entry, ps6112FinalName(variant.TileRouter)) ||
		ps6112LocallyShadows(router, tiledName) || ps6112LocallyShadows(router, scalarName) {
		return false
	}
	return ps6112DirectCall(router.Body, tiledName) &&
		ps6112DirectCallAtArgument(router.Body, scalarName, variant.ScalarFallbackRowsArgument,
			ps6112FunctionParameterName(router, variant.TileRouterRowsArgument))
}

func ps6112TargetFunction(sources []ps6074SourceFile, name string) (*ast.FuncDecl, int) {
	var result *ast.FuncDecl
	count := 0
	for _, source := range sources {
		for _, declaration := range source.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Recv == nil && function.Name.Name == name {
				result = function
				count++
			}
		}
	}
	return result, count
}

func ps6112FunctionParameterName(function *ast.FuncDecl, position int) string {
	current := 0
	for _, field := range function.Type.Params.List {
		if len(field.Names) == 0 {
			current++
			if current == position {
				return ""
			}
			continue
		}
		for _, name := range field.Names {
			current++
			if current == position {
				return name.Name
			}
		}
	}
	return ""
}

func ps6112LocallyShadows(function *ast.FuncDecl, name string) bool {
	shadowed := false
	for _, field := range function.Type.Params.List {
		for _, identifier := range field.Names {
			shadowed = shadowed || identifier.Name == name
		}
	}
	if shadowed {
		return true
	}
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok == token.DEFINE {
				for _, left := range value.Lhs {
					if identifier, ok := left.(*ast.Ident); ok && identifier.Name == name {
						shadowed = true
					}
				}
			}
		case *ast.ValueSpec:
			for _, identifier := range value.Names {
				shadowed = shadowed || identifier.Name == name
			}
		}
		return !shadowed
	})
	return shadowed
}

func ps6112DirectCall(body *ast.BlockStmt, name string) bool {
	found := false
	dead := ps6112SyntacticDeadNodes(body)
	ast.Inspect(body, func(node ast.Node) bool {
		if node != nil && dead[node] {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
		if !ok || identifier.Name != name {
			return true
		}
		found = true
		return false
	})
	return found
}

func ps6112DirectCallAtArgument(body *ast.BlockStmt, name string, position int, requiredIdentifier string) bool {
	found := false
	dead := ps6112SyntacticDeadNodes(body)
	ast.Inspect(body, func(node ast.Node) bool {
		if node != nil && dead[node] {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
		if !ok || identifier.Name != name || position <= 0 || position > len(call.Args) {
			return true
		}
		argument, ok := ps2110Unparen(call.Args[position-1]).(*ast.Ident)
		found = ok && argument.Name == requiredIdentifier
		return !found
	})
	return found
}

func ps6112SyntacticDeadNodes(body *ast.BlockStmt) map[ast.Node]bool {
	dead := make(map[ast.Node]bool)
	var visitBlock func(*ast.BlockStmt) bool
	var visitStatement func(ast.Stmt) bool
	visitStatement = func(statement ast.Stmt) bool {
		switch value := statement.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.BlockStmt:
			return visitBlock(value)
		case *ast.IfStmt:
			identifier, known := ps2110Unparen(value.Cond).(*ast.Ident)
			if known && identifier.Name == "false" {
				dead[value.Body] = true
				if value.Else != nil {
					return visitStatement(value.Else)
				}
				return false
			}
			bodyTerminal := visitBlock(value.Body)
			if known && identifier.Name == "true" {
				if value.Else != nil {
					dead[value.Else] = true
				}
				return bodyTerminal
			}
			return value.Else != nil && bodyTerminal && visitStatement(value.Else)
		case *ast.ForStmt:
			if identifier, ok := ps2110Unparen(value.Cond).(*ast.Ident); ok && identifier.Name == "false" {
				dead[value.Body] = true
				return false
			}
			visitBlock(value.Body)
		case *ast.RangeStmt:
			visitBlock(value.Body)
		}
		return false
	}
	visitBlock = func(block *ast.BlockStmt) bool {
		terminal := false
		for _, statement := range block.List {
			if terminal {
				dead[statement] = true
				continue
			}
			terminal = visitStatement(statement)
		}
		return terminal
	}
	visitBlock(body)
	return dead
}

func ps6112FinalName(id string) string {
	index := strings.LastIndexByte(id, '.')
	if index < 0 {
		return id
	}
	return id[index+1:]
}

func ps6112Suppressed(file *ast.File, function *ast.FuncDecl) bool {
	for _, group := range file.Comments {
		if group.End() < function.Pos() || group.Pos() > function.End() {
			continue
		}
		if ps6112SuppressionGroup(group) {
			return true
		}
	}
	return function.Doc != nil && ps6112SuppressionGroup(function.Doc)
}

func ps6112FileSuppressed(file *ast.File) bool {
	for _, group := range file.Comments {
		if ps6112SuppressionGroup(group) {
			return true
		}
	}
	return false
}

func ps6112SuppressionGroup(group *ast.CommentGroup) bool {
	for _, comment := range group.List {
		text := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(comment.Text, "//"), "/*"))
		for _, marker := range []string{"perfscan:tile-grain-intentional", "perfscan:tile-grain-profiled"} {
			if reason, ok := strings.CutPrefix(text, marker); ok && strings.Trim(strings.TrimSpace(reason), "*/") != "" {
				return true
			}
		}
	}
	return false
}

func ps6112DeadNodes(pass *analysis.Pass, body *ast.BlockStmt) map[ast.Node]bool {
	dead := make(map[ast.Node]bool)
	var block func(*ast.BlockStmt) bool
	var statement func(ast.Stmt) bool
	statement = func(current ast.Stmt) bool {
		switch value := current.(type) {
		case *ast.ReturnStmt:
			return true
		case *ast.BlockStmt:
			return block(value)
		case *ast.IfStmt:
			condition, known := ps6112BoolConstant(pass, value.Cond)
			if known && !condition {
				dead[value.Body] = true
				if value.Else == nil {
					return false
				}
				return statement(value.Else)
			}
			bodyTerminal := block(value.Body)
			if known {
				if value.Else != nil {
					dead[value.Else] = true
				}
				return bodyTerminal
			}
			return value.Else != nil && bodyTerminal && statement(value.Else)
		case *ast.ForStmt:
			if condition, known := ps6112BoolConstant(pass, value.Cond); known && !condition {
				dead[value.Body] = true
				return false
			}
			block(value.Body)
		case *ast.RangeStmt:
			block(value.Body)
		case *ast.SwitchStmt:
			for _, clause := range value.Body.List {
				caseClause, ok := clause.(*ast.CaseClause)
				if ok {
					block(&ast.BlockStmt{List: caseClause.Body})
				}
			}
		}
		return false
	}
	block = func(current *ast.BlockStmt) bool {
		terminal := false
		for _, item := range current.List {
			if terminal {
				dead[item] = true
				continue
			}
			terminal = statement(item)
		}
		return terminal
	}
	block(body)
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.FuncLit)
		if ok && !dead[literal] {
			block(literal.Body)
		}
		return true
	})
	return dead
}

func ps6112BoolConstant(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if expression == nil {
		return false, false
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}
	result, err := strconv.ParseBool(value.ExactString())
	return result, err == nil
}
