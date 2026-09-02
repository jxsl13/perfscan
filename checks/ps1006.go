package checks

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/constant"
	"go/printer"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS1006 reports a reduction whose INNER loop variable is the high-stride
// (multiplied) part of a flat index while the outer variable is the
// contiguous (additive) part.
var PS1006 = register(&lint.Check{
	ID:       "PS1006",
	Category: "access",
	Slug:     "strided-inner-reduction",
	Level:    lint.LevelStructured,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "a reduction striding a flat array by the inner loop variable",
		Text: `ARR[inner*stride + outer] strides ARR by stride every inner
step — cache-thrashing when stride × working set exceeds L2. Interchange
the loops so ARR is walked contiguously when bounds and non-aliasing are
known; per output element the reduction then keeps its order.

The win SCALES WITH stride × working-set: big when it exceeds L2, ~noise
when L1-resident — rank candidates by the strided dimension's size. When
the strided access sits inside a fused scan where full interchange would
need O(output width) exact-accumulation scratch, prefer an allocation-free
outer output/channel register tile first: keep four independent ascending
tap accumulators live, read adjacent outputs per tap, and handle the
less-than-four tail with a paired scalar remainder. If a register tile is
not viable, use GATHER/SCATTER: copy the column into contiguous scratch
once, scan the scratch, scatter back once — same bit-exact remedy.

Local call summaries preserve this proof across declared function aliases,
generic instantiations, method expressions and method values. Callable
arguments contribute effects only when the callee can invoke them; definite
resets, conditional calls, deferred callback work and asynchronous go calls
retain their distinct execution semantics. Nested invoked closures and shared
package objects use the same path-sensitive summaries. Argument values are
snapshotted in Go's left-to-right evaluation order, read-modify writes retain
their prior dependency, and a write through a may-alias pointer is definite
only when every possible value names the same target. Pointer retargeting by an
earlier argument is carried into later arguments, while an opaque fallback
preserves the incoming value unless it proves an independent overwrite.
Saved method receivers, callable struct fields, fixed-array elements and
callable results retain their definition-time values even when the source
variable, field or element is reassigned. Multi-result call expansions
preserve the same value snapshots; ambiguous joins and escaped receivers
remain conservative.

The automatic edit is intentionally limited to fixed arrays with
compile-time non-negative bounds that prove every source and destination
index in range. Slice-shaped reductions stay advisory: distinct slice
variables may overlap, and source/output bounds panics expose the original
per-column store order. Reordering either case is not behavior-preserving.`,
		Before: `for c := 0; c < cols; c++ {
	s := 0.0
	for r := 0; r < rows; r++ {
		s += a[r*cols+c] // strides by cols every step
	}
	out[c] = s
}`,
		After: `sums := make([]float64, cols)
for r := 0; r < rows; r++ { // contiguous walk
	base := r * cols
	for c := 0; c < cols; c++ {
		sums[c] += a[base+c]
	}
}
for c := 0; c < cols; c++ {
	out[c] = sums[c]
}`,
		MeasuredWin: "reference corpus: MLA value-mix 1.13x/1.27x, spectral-norm power-iter 2.57x; WKV backward 2.2–2.9x via the gather/scatter form",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS1006",
		Doc:  "inner loop strides a flat array",
		Run:  runPS1006,
	},
})

const (
	ps1006MayStrideKey  = "$may-stride"
	ps1006ImpureCallKey = "$impure-call"
)

func runPS1006(pass *analysis.Pass) (any, error) {
	_, err := ps1006RunIndexed(pass)
	return nil, err
}

func ps1006RunIndexed(pass *analysis.Pass) (*ps1006AnalysisIndex, error) {
	index := ps1006BuildAnalysisIndex(pass)
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	defer ps1006ActiveAnalysisIndexes.Delete(pass)
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			index.analysisVisits++
			iv, body := loopVarAndBody(n)
			if iv == "" || body == nil || containsLoop(body) {
				return true
			}
			outerLoop, in := astutil.InLoop(stack)
			if !in {
				return true
			}
			ov := outermostLoopVar(outerLoop)
			if ov == "" || ov == iv {
				return true
			}
			ivObject := ps4008LoopObject(pass, n)
			ovObject := ps4008LoopObject(pass, outerLoop)
			haveObjects := ivObject != nil && ovObject != nil
			// A read ARR[iv*s + ov-part]: the inner variable multiplied,
			// the outer variable additive.
			var hit *ast.IndexExpr
			if haveObjects {
				hit = ps1006FindStridedReduction(pass, body, ivObject, ovObject)
			} else {
				ast.Inspect(body, func(m ast.Node) bool {
					if hit != nil {
						return false
					}
					ix, ok := m.(*ast.IndexExpr)
					if !ok {
						return true
					}
					if stridedByInner(ix.Index, iv, ov) {
						hit = ix
						return false
					}
					return true
				})
				if hit == nil {
					return true
				}
				// Must be a reduction: an accumulation into a target free of
				// the inner variable.
				reduces := false
				ast.Inspect(body, func(m ast.Node) bool {
					as, ok := m.(*ast.AssignStmt)
					if !ok || as.Tok != token.ADD_ASSIGN && as.Tok != token.SUB_ASSIGN {
						return true
					}
					for _, lhs := range as.Lhs {
						if !exprMentions(lhs, iv) {
							reduces = true
							return false
						}
					}
					return true
				})
				if !reduces {
					return true
				}
			}
			if hit == nil {
				return true
			}
			if ps1006ResolvedRegisterTile(index, pass, stack, outerLoop, n) {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "the inner loop variable is the multiplied (high-stride) part of this flat index — the array is strided every inner step; first prefer an allocation-free four-output register tile when exact interchange would need O(width) scratch, otherwise interchange or gather into contiguous scratch",
			}
			if fix := ps1006InterchangeFix(pass, f, outerLoop, n); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return index, nil
}

func ps1006FindStridedReduction(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object) *ast.IndexExpr {
	if body == nil || inner == nil || outer == nil {
		return nil
	}
	deps := make(map[types.Object]bool, len(body.List))
	strideDeps := make(map[types.Object]string, len(body.List))
	pointerAliases := make(map[types.Object]ps1006OrderedPointerAlias)
	return ps1006FindStridedReductionInBlock(pass, body, inner, outer, deps, strideDeps, pointerAliases)
}

func ps1006FindStridedReductionInBlock(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string, pointerAliases map[types.Object]ps1006OrderedPointerAlias) *ast.IndexExpr {
	gotos := make(map[string][]ps1006DependencyState)
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
		statementGotos := ps1006CollectGotoDepsForStatement(pass, statement, inner, deps, strideDeps)
		for label, states := range statementGotos {
			targetIndex, ok := labels[label]
			if !ok || targetIndex >= index {
				continue
			}
			for _, state := range states {
				if hit := ps1006FindStridedReductionInStatements(pass, body.List[targetIndex:index], inner, outer, cloneObjectBoolMap(state.deps), cloneObjectStringMap(state.strideDeps), clonePS1006OrderedPointerAliases(state.pointerAliases)); hit != nil {
					return hit
				}
			}
			delete(statementGotos, label)
		}
		ps1006MergeGotoDeps(gotos, statementGotos)
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if (value.Tok == token.ADD_ASSIGN || value.Tok == token.SUB_ASSIGN) && len(value.Lhs) == 1 && len(value.Rhs) == 1 && !ps4008ExprDependsOn(pass, value.Lhs[0], inner, deps) {
				var hit *ast.IndexExpr
				ast.Inspect(value.Rhs[0], func(node ast.Node) bool {
					if hit != nil {
						return false
					}
					index, ok := node.(*ast.IndexExpr)
					if !ok {
						return true
					}
					if ps1006FlatIndexDependsOn(pass, index.Index, inner, outer, strideDeps) {
						hit = index
						return false
					}
					return true
				})
				if hit != nil {
					return hit
				}
			}
			ps4008UpdateDerivedDeps(pass, value, inner, deps)
			ps1006UpdateDerivedStrideDeps(pass, value, inner, strideDeps)
			ps1006UpdateOrderedPointerAliasesForStatement(pass, value, pointerAliases)
		case *ast.DeclStmt:
			ps4008UpdateDerivedDeps(pass, value, inner, deps)
			ps1006UpdateDerivedStrideDeps(pass, value, inner, strideDeps)
			ps1006UpdateOrderedPointerAliasesForStatement(pass, value, pointerAliases)
		case *ast.ExprStmt:
			ps1006ApplyExpressionDependencyStatement(pass, value, inner, deps, strideDeps, pointerAliases)
		case *ast.IfStmt:
			preDeps := cloneObjectBoolMap(deps)
			preStrideDeps := cloneObjectStringMap(strideDeps)
			prePointerAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			if value.Init != nil {
				ps4008UpdateDerivedDeps(pass, value.Init, inner, deps)
				ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, strideDeps)
			}
			whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, value.Cond, inner, ps1006DependencyState{
				deps: cloneObjectBoolMap(deps), strideDeps: cloneObjectStringMap(strideDeps), pointerAliases: clonePS1006OrderedPointerAliases(pointerAliases),
			})
			for _, branch := range whenTrue {
				if hit := ps1006FindStridedReductionInBlock(pass, value.Body, inner, outer, cloneObjectBoolMap(branch.deps), cloneObjectStringMap(branch.strideDeps), clonePS1006OrderedPointerAliases(branch.pointerAliases)); hit != nil {
					return hit
				}
			}
			switch elseNode := value.Else.(type) {
			case *ast.BlockStmt:
				for _, branch := range whenFalse {
					if hit := ps1006FindStridedReductionInBlock(pass, elseNode, inner, outer, cloneObjectBoolMap(branch.deps), cloneObjectStringMap(branch.strideDeps), clonePS1006OrderedPointerAliases(branch.pointerAliases)); hit != nil {
						return hit
					}
				}
			case *ast.IfStmt:
				for _, branch := range whenFalse {
					if hit := ps1006FindStridedReductionInStatement(pass, elseNode, inner, outer, cloneObjectBoolMap(branch.deps), cloneObjectStringMap(branch.strideDeps), clonePS1006OrderedPointerAliases(branch.pointerAliases)); hit != nil {
						return hit
					}
				}
			}
			state := ps1006UnionDependencyState(ps1006DependencyExitStatesForIf(pass, value, inner, preDeps, preStrideDeps, prePointerAliases, false))
			ps4008ReplaceDeps(deps, state.deps)
			ps1006ReplaceStrideDeps(strideDeps, state.strideDeps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, state.pointerAliases)
		case *ast.ForStmt:
			state := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{
				deps: cloneObjectBoolMap(deps), strideDeps: cloneObjectStringMap(strideDeps), pointerAliases: clonePS1006OrderedPointerAliases(pointerAliases),
			}, false))
			ps4008ReplaceDeps(deps, state.deps)
			ps1006ReplaceStrideDeps(strideDeps, state.strideDeps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, state.pointerAliases)
		case *ast.BlockStmt:
			if hit := ps1006FindStridedReductionInBlock(pass, value, inner, outer, deps, strideDeps, pointerAliases); hit != nil {
				return hit
			}
		case *ast.LabeledStmt:
			labels[value.Label.Name] = index
			if labelStates := gotos[value.Label.Name]; len(labelStates) > 0 {
				inputs := []ps1006DependencyState{{deps: cloneObjectBoolMap(deps), strideDeps: cloneObjectStringMap(strideDeps), pointerAliases: clonePS1006OrderedPointerAliases(pointerAliases)}}
				inputs = append(inputs, labelStates...)
				state := ps1006UnionDependencyState(inputs)
				ps4008ReplaceDeps(deps, state.deps)
				ps1006ReplaceStrideDeps(strideDeps, state.strideDeps)
				ps1006ReplaceOrderedPointerAliases(pointerAliases, state.pointerAliases)
				delete(gotos, value.Label.Name)
			}
			if hit := ps1006FindStridedReductionInStatement(pass, value.Stmt, inner, outer, deps, strideDeps, pointerAliases); hit != nil {
				return hit
			}
		case *ast.SwitchStmt:
			preDeps := cloneObjectBoolMap(deps)
			preStrideDeps := cloneObjectStringMap(strideDeps)
			prePointerAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			if value.Init != nil {
				ps4008UpdateDerivedDeps(pass, value.Init, inner, deps)
				ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, strideDeps)
			}
			ps4008InvalidateDerivedDepsInExpr(pass, value.Tag, deps)
			ps1006InvalidateStrideDepsInExpr(pass, value.Tag, strideDeps)
			if hit := ps1006FindStridedReductionInCaseClauses(pass, value.Body, inner, outer, ps1006DependencyState{deps: deps, strideDeps: strideDeps, pointerAliases: pointerAliases}, value.Tag == nil); hit != nil {
				return hit
			}
			state := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{deps: preDeps, strideDeps: preStrideDeps, pointerAliases: prePointerAliases}, false))
			ps4008ReplaceDeps(deps, state.deps)
			ps1006ReplaceStrideDeps(strideDeps, state.strideDeps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, state.pointerAliases)
		case *ast.TypeSwitchStmt:
			preDeps := cloneObjectBoolMap(deps)
			preStrideDeps := cloneObjectStringMap(strideDeps)
			prePointerAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			if value.Init != nil {
				ps4008UpdateDerivedDeps(pass, value.Init, inner, deps)
				ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, strideDeps)
			}
			ps4008InvalidateDerivedDeps(pass, value.Assign, deps)
			ps1006InvalidateStrideDeps(pass, value.Assign, strideDeps)
			for _, clause := range value.Body.List {
				if caseClause, ok := clause.(*ast.CaseClause); ok {
					if hit := ps1006FindStridedReductionInStatements(pass, caseClause.Body, inner, outer, cloneObjectBoolMap(deps), cloneObjectStringMap(strideDeps), clonePS1006OrderedPointerAliases(pointerAliases)); hit != nil {
						return hit
					}
				}
			}
			state := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{deps: preDeps, strideDeps: preStrideDeps, pointerAliases: prePointerAliases}, false))
			ps4008ReplaceDeps(deps, state.deps)
			ps1006ReplaceStrideDeps(strideDeps, state.strideDeps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, state.pointerAliases)
		case *ast.SelectStmt:
			preDeps := cloneObjectBoolMap(deps)
			preStrideDeps := cloneObjectStringMap(strideDeps)
			prePointerAliases := clonePS1006OrderedPointerAliases(pointerAliases)
			for _, clause := range value.Body.List {
				if commClause, ok := clause.(*ast.CommClause); ok {
					clauseState := ps1006ApplyDependencyStatement(pass, commClause.Comm, inner, ps1006DependencyState{deps: deps, strideDeps: strideDeps, pointerAliases: pointerAliases})
					if hit := ps1006FindStridedReductionInStatements(pass, commClause.Body, inner, outer, clauseState.deps, clauseState.strideDeps, clauseState.pointerAliases); hit != nil {
						return hit
					}
				}
			}
			state := ps1006UnionDependencyState(ps1006DependencyExitStatesForStatement(pass, value, inner, ps1006DependencyState{deps: preDeps, strideDeps: preStrideDeps, pointerAliases: prePointerAliases}, false))
			ps4008ReplaceDeps(deps, state.deps)
			ps1006ReplaceStrideDeps(strideDeps, state.strideDeps)
			ps1006ReplaceOrderedPointerAliases(pointerAliases, state.pointerAliases)
		case *ast.BranchStmt:
			switch value.Tok {
			case token.GOTO:
				if value.Label == nil {
					return nil
				}
				targetIndex, ok := labels[value.Label.Name]
				if ok && targetIndex < index {
					if hit := ps1006FindStridedReductionInStatements(pass, body.List[targetIndex:index], inner, outer, cloneObjectBoolMap(deps), cloneObjectStringMap(strideDeps), clonePS1006OrderedPointerAliases(pointerAliases)); hit != nil {
						return hit
					}
					return nil
				}
				skipUntil = value.Label.Name
			case token.BREAK, token.CONTINUE:
				return nil
			}
		case *ast.ReturnStmt:
			return nil
		}
	}
	return nil
}

type ps1006DependencyState struct {
	deps           map[types.Object]bool
	strideDeps     map[types.Object]string
	pointerAliases map[types.Object]ps1006OrderedPointerAlias
}

type ps1006OrderedPointerAlias struct {
	targets []types.Object
	known   bool
}

func ps1006ApplyExpressionDependencyStatement(pass *analysis.Pass, statement *ast.ExprStmt, inner types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string, pointerAliases map[types.Object]ps1006OrderedPointerAlias) {
	if statement == nil || statement.X == nil {
		return
	}
	state := ps1006DependencyState{deps: deps, strideDeps: strideDeps, pointerAliases: pointerAliases}
	ps1006ApplyCaseExpression(pass, statement.X, inner, &state)
}

func ps1006CaseClauses(body *ast.BlockStmt) []*ast.CaseClause {
	if body == nil {
		return nil
	}
	clauses := make([]*ast.CaseClause, 0, len(body.List))
	for _, statement := range body.List {
		if clause, ok := statement.(*ast.CaseClause); ok {
			clauses = append(clauses, clause)
		}
	}
	return clauses
}

// ps1006ApplyCaseExpression models the effects of evaluating one case
// expression. Addressed outputs are first invalidated, then conservatively
// made dependent when a non-address input depends on the serial variable.
// This distinguishes miss(&base) from setAndMiss(&base, inner).
func ps1006ApplyCaseExpression(pass *analysis.Pass, expression ast.Expr, inner types.Object, state *ps1006DependencyState) {
	if expression == nil || state == nil {
		return
	}
	initial := state.clone()
	trackPointerAliases := state.pointerAliases != nil
	derivedInput := ps1006CaseExpressionInputDepends(pass, expression, inner, state.deps, nil)
	strideInput := ps1006CaseExpressionInputDepends(pass, expression, inner, nil, state.strideDeps)
	targets := ps1006CaseExpressionTargets(pass, expression, state)
	targetDerived, targetStride, targetKnown := ps1006CallArgumentOverwriteDependencies(pass, expression, inner, state)
	readModifyTargets := ps1006CallArgumentReadModifyTargets(pass, expression, state)
	ordered, orderedWrites, orderedKnown := ps1006OrderedCallDependencyState(pass, expression, inner, initial)
	ps4008InvalidateDerivedDepsInExpr(pass, expression, state.deps)
	ps1006InvalidateStrideDepsInExpr(pass, expression, state.strideDeps)
	ps1006RestoreDependencies(initial, state, ps1006AmbiguousPointerWriteTargets(pass, expression, state, initial.deps))
	if !ps1006ExpressionCallableKnown(pass, expression) {
		// An unresolved callable can read-modify-write any addressed or captured
		// dependency. A may-write invalidation is therefore not a proof of an
		// independent overwrite; keep the incoming value conservatively.
		ps1006RestoreDependencies(initial, state, initial.deps)
	} else if trackPointerAliases {
		// Static alias provenance predates control-flow retargets. Do not let a
		// later write through the retargeted pointer invalidate its displaced
		// static target unless that target is independently written by the call.
		displaced := ps1006DisplacedPointerTargets(pass, expression, state)
		for object := range targets {
			delete(displaced, object)
		}
		ps1006RestoreDependencies(initial, state, displaced)
	}
	preciseWrites := make(map[types.Object]bool, len(targets))
	for target := range targets {
		derived := derivedInput
		stride := strideInput
		if targetKnown[target] {
			derived = targetDerived[target]
			stride = targetStride[target]
			preciseWrites[target] = true
		}
		if preciseWrites[target] {
			if derived {
				state.deps[target] = true
			} else {
				delete(state.deps, target)
			}
			if stride {
				state.strideDeps[target] = ps1006MayStrideKey
			} else {
				delete(state.strideDeps, target)
			}
		} else {
			// A general effect summary can prove that storage is written without
			// proving that the new value is independent of the old one. Until an
			// exact overwrite or ordered transfer says otherwise, preserve every
			// incoming dependency; this is essential for compound assignments and
			// tuple/RHS snapshots hidden behind an unsupported statement.
			if readModifyTargets[target] && initial.deps[target] {
				state.deps[target] = true
			} else if derived {
				state.deps[target] = true
			}
			if key := initial.strideDeps[target]; readModifyTargets[target] && key != "" {
				state.strideDeps[target] = key
			} else if stride {
				state.strideDeps[target] = ps1006MayStrideKey
			}
		}
		if !orderedKnown || !orderedWrites[target] {
			continue
		}
		preciseWrites[target] = true
		if ordered.deps[target] {
			state.deps[target] = true
		} else {
			delete(state.deps, target)
		}
		if key := ordered.strideDeps[target]; key != "" {
			state.strideDeps[target] = key
		} else {
			delete(state.strideDeps, target)
		}
	}
	if orderedKnown {
		// The straight-line transfer starts from the complete incoming state and
		// evaluates receiver, arguments, and body in language order. Once it is
		// available it is authoritative for all objects, including a pointer
		// argument whose target was changed by an earlier argument expression.
		ps1006ReplaceObjectBoolMap(state.deps, ordered.deps)
		ps1006ReplaceStrideDeps(state.strideDeps, ordered.strideDeps)
		if trackPointerAliases {
			ps1006ReplaceOrderedPointerAliases(state.pointerAliases, ordered.pointerAliases)
		}
	} else if trackPointerAliases && ps1006ExpressionMayAffectPointerAliases(pass, expression) {
		ps1006InvalidateOrderedPointerAliases(pass, expression, state.pointerAliases)
	}
}

func ps1006ReplaceOrderedPointerAliases(dst, src map[types.Object]ps1006OrderedPointerAlias) {
	clear(dst)
	for object, alias := range src {
		dst[object] = clonePS1006OrderedPointerAlias(alias)
	}
}

func ps1006ExpressionCallableKnown(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || ps4008CallIsConversionOrBuiltin(pass, call) {
		return true
	}
	_, known := ps4008CallableSyntaxes(pass, call.Fun, call.Pos(), make(map[types.Object]bool))
	return known
}

func ps1006AmbiguousPointerWriteTargets(pass *analysis.Pass, expression ast.Expr, state *ps1006DependencyState, candidates map[types.Object]bool) map[types.Object]bool {
	result := make(map[types.Object]bool)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || state == nil || ps4008CallIsConversionOrBuiltin(pass, call) {
		return result
	}
	for argumentIndex, argument := range call.Args {
		if !ps4008IsPointerType(pass.TypesInfo.TypeOf(argument)) || !ps4008CallWritesArgument(pass, call, argumentIndex, call.Pos(), ps4008MayWrite) {
			continue
		}
		targets, known := ps1006PointerTargetsForState(pass, argument, call.Pos(), state)
		if !known {
			for object := range candidates {
				result[object] = true
			}
			continue
		}
		if len(targets) == 1 {
			continue
		}
		for _, target := range targets {
			if target != nil {
				result[target] = true
			}
		}
	}
	return result
}

func ps1006RestoreDependencies(initial ps1006DependencyState, state *ps1006DependencyState, objects map[types.Object]bool) {
	for object := range objects {
		if initial.deps[object] {
			state.deps[object] = true
		}
		if key := initial.strideDeps[object]; key != "" {
			state.strideDeps[object] = key
		}
	}
}

func ps1006DisplacedPointerTargets(pass *analysis.Pass, expression ast.Expr, state *ps1006DependencyState) map[types.Object]bool {
	result := make(map[types.Object]bool)
	if pass == nil || expression == nil || state == nil {
		return result
	}
	for aliasObject, current := range state.pointerAliases {
		if !current.known || !ps4008ExpressionMentionsObject(pass, expression, aliasObject) {
			continue
		}
		staticTargets, known := ps4008PointerObjectsFromAliases(pass, []types.Object{aliasObject}, expression.Pos(), make(map[types.Object]bool))
		if !known {
			continue
		}
		for _, target := range staticTargets {
			if target != nil && !slices.Contains(current.targets, target) {
				result[target] = true
			}
		}
	}
	return result
}

func ps1006ExpressionMayAffectPointerAliases(pass *analysis.Pass, expression ast.Expr) bool {
	mayAffect := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if mayAffect {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if function, typed := pass.TypesInfo.Types[call.Fun]; typed && function.IsType() {
			return true
		}
		mayAffect = true
		return false
	})
	return mayAffect
}

func ps1006CallArgumentOverwriteDependencies(pass *analysis.Pass, expression ast.Expr, inner types.Object, state *ps1006DependencyState) (derived, stride, known map[types.Object]bool) {
	derived = make(map[types.Object]bool)
	stride = make(map[types.Object]bool)
	known = make(map[types.Object]bool)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || state == nil || ps4008CallIsConversionOrBuiltin(pass, call) {
		return derived, stride, known
	}
	syntaxes, resolved := ps4008CallableSyntaxes(pass, call.Fun, call.Pos(), make(map[types.Object]bool))
	if !resolved || len(syntaxes) == 0 {
		return derived, stride, known
	}
	for argumentIndex, argument := range call.Args {
		targets, targetsKnown := ps1006PointerTargetsForState(pass, argument, call.Pos(), state)
		// A write through a pointer with multiple possible runtime targets is not
		// a definite overwrite of each member of that set. Leave it to the may
		// transfer unless all aliases collapse to one concrete object.
		if !targetsKnown || len(targets) != 1 {
			continue
		}
		argumentDerived, argumentStride := false, false
		overwriteKnown := true
		for _, syntax := range syntaxes {
			if !ps4008VariadicBindingIsUnique(pass, syntax, call, argumentIndex) {
				overwriteKnown = false
				break
			}
			rhs, ok := ps4008CallableOverwriteExpression(pass, syntax, argumentIndex)
			if !ok {
				overwriteKnown = false
				break
			}
			argumentDerived = argumentDerived || ps1006BoundOverwriteDepends(pass, rhs, syntax, call, inner, state.deps, nil)
			argumentStride = argumentStride || ps1006BoundOverwriteDepends(pass, rhs, syntax, call, inner, nil, state.strideDeps)
		}
		if !overwriteKnown {
			continue
		}
		for _, target := range targets {
			if target == nil {
				continue
			}
			known[target] = true
			derived[target] = derived[target] || argumentDerived
			stride[target] = stride[target] || argumentStride
		}
	}
	return derived, stride, known
}

// ps1006CallArgumentReadModifyTargets identifies storage whose incoming value
// participates in a callable's resulting write. Definite-write summaries alone
// cannot distinguish `*p = 0` from `*p += 0`; when ordered interpretation has
// to fall back, the latter must retain the caller's dependency.
func ps1006CallArgumentReadModifyTargets(pass *analysis.Pass, expression ast.Expr, state *ps1006DependencyState) map[types.Object]bool {
	result := make(map[types.Object]bool)
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || state == nil || ps4008CallIsConversionOrBuiltin(pass, call) {
		return result
	}
	syntaxes, resolved := ps4008CallableSyntaxes(pass, call.Fun, call.Pos(), make(map[types.Object]bool))
	if resolved {
		for argumentIndex, argument := range call.Args {
			targets, known := ps1006PointerTargetsForState(pass, argument, call.Pos(), state)
			if !known || len(targets) == 0 {
				continue
			}
			for _, syntax := range syntaxes {
				parameter := ps4008SyntaxParameterObject(pass, syntax, argumentIndex)
				if parameter == nil || !ps1006CallableParameterReadModify(pass, syntax.body, parameter, call.Pos(), make(map[types.Object]bool)) {
					continue
				}
				for _, target := range targets {
					if target != nil {
						result[target] = true
					}
				}
			}
		}
		if selector, selected := ps2110Unparen(call.Fun).(*ast.SelectorExpr); selected {
			selection := pass.TypesInfo.Selections[selector]
			if selection != nil && selection.Kind() == types.MethodVal {
				for _, syntax := range syntaxes {
					receiver := ps4008SyntaxParameterObject(pass, ps4008CallableSyntaxValue{
						functionType: syntax.functionType, receiver: syntax.receiver, receiverArgument: true, body: syntax.body,
					}, 0)
					if receiver == nil || !ps1006CallableParameterReadModify(pass, syntax.body, receiver, call.Pos(), make(map[types.Object]bool)) {
						continue
					}
					if targets, known := ps1006PointerTargetsForState(pass, selector.X, call.Pos(), state); known {
						for _, target := range targets {
							if target != nil {
								result[target] = true
							}
						}
					}
				}
			}
		}
	}
	ps1006CapturedReadModifyTargets(pass, expression, state, result)
	return result
}

func ps1006CallableParameterReadModify(pass *analysis.Pass, body *ast.BlockStmt, parameter types.Object, before token.Pos, seen map[types.Object]bool) bool {
	if pass == nil || body == nil || parameter == nil || seen[parameter] {
		return false
	}
	seen[parameter] = true
	readModify := false
	ast.Inspect(body, func(node ast.Node) bool {
		if readModify {
			return false
		}
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if ps4008DirectObjectAssignment(pass, lhs, parameter) || !ps4008ExpressionMentionsObject(pass, lhs, parameter) {
					continue
				}
				if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
					readModify = true
					return false
				}
				for _, rhs := range value.Rhs {
					if ps4008ExpressionMentionsObject(pass, rhs, parameter) {
						readModify = true
						return false
					}
				}
			}
		case *ast.IncDecStmt:
			if ps4008ExpressionMentionsObject(pass, value.X, parameter) && !ps4008DirectObjectAssignment(pass, value.X, parameter) {
				readModify = true
				return false
			}
		case *ast.CallExpr:
			syntaxes, known := ps4008CallableSyntaxes(pass, value.Fun, before, make(map[types.Object]bool))
			if !known {
				return true
			}
			for argumentIndex, argument := range value.Args {
				if !ps4008ExpressionMentionsObject(pass, argument, parameter) {
					continue
				}
				for _, syntax := range syntaxes {
					nested := ps4008SyntaxParameterObject(pass, syntax, argumentIndex)
					if nested != nil && ps1006CallableParameterReadModify(pass, syntax.body, nested, value.Pos(), maps.Clone(seen)) {
						readModify = true
						return false
					}
				}
			}
		}
		return true
	})
	return readModify
}

func ps1006CapturedReadModifyTargets(pass *analysis.Pass, expression ast.Expr, state *ps1006DependencyState, result map[types.Object]bool) {
	if pass == nil || expression == nil || state == nil {
		return
	}
	mark := func(target ast.Expr) {
		switch value := ps2110Unparen(target).(type) {
		case *ast.Ident:
			object := identObject(pass, value)
			if state.deps[object] || state.strideDeps[object] != "" {
				result[object] = true
			}
		case *ast.StarExpr:
			if targets, known := ps1006PointerTargetsForState(pass, value.X, expression.Pos(), state); known {
				for _, object := range targets {
					if state.deps[object] || state.strideDeps[object] != "" {
						result[object] = true
					}
				}
			}
		}
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok != token.ASSIGN && value.Tok != token.DEFINE {
				for _, lhs := range value.Lhs {
					mark(lhs)
				}
				return true
			}
			for _, lhs := range value.Lhs {
				for _, rhs := range value.Rhs {
					if ps1006ExpressionsShareDependencyObject(pass, lhs, rhs) {
						mark(lhs)
					}
				}
			}
		case *ast.IncDecStmt:
			mark(value.X)
		}
		return true
	})
}

func ps1006ExpressionsShareDependencyObject(pass *analysis.Pass, left, right ast.Expr) bool {
	objects := make(map[types.Object]bool)
	ast.Inspect(left, func(node ast.Node) bool {
		if identifier, ok := node.(*ast.Ident); ok {
			if object := identObject(pass, identifier); object != nil {
				objects[object] = true
			}
		}
		return true
	})
	shared := false
	ast.Inspect(right, func(node ast.Node) bool {
		if shared {
			return false
		}
		if identifier, ok := node.(*ast.Ident); ok && objects[identObject(pass, identifier)] {
			shared = true
			return false
		}
		return true
	})
	return shared
}

func ps1006PointerTargetsForState(pass *analysis.Pass, expression ast.Expr, before token.Pos, state *ps1006DependencyState) ([]types.Object, bool) {
	if state != nil && state.pointerAliases != nil {
		return ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{expression: expression}, before, state.pointerAliases, make(map[types.Object]bool))
	}
	return ps4008PointerObjectTargets(pass, expression, before, make(map[types.Object]bool))
}

func ps1006UpdateOrderedPointerAliasesForStatement(pass *analysis.Pass, statement ast.Stmt, aliases map[types.Object]ps1006OrderedPointerAlias) {
	if pass == nil || pass.TypesInfo == nil || statement == nil || aliases == nil {
		return
	}
	apply := func(lhs, rhs ast.Expr, before token.Pos) {
		target := ps1006OrderedAddressableObject(pass, lhs)
		if target == nil || !ps4008IsPointerType(target.Type()) || rhs == nil || !ps4008IsPointerType(pass.TypesInfo.TypeOf(rhs)) {
			return
		}
		targets, known := ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{expression: rhs}, before, aliases, make(map[types.Object]bool))
		aliases[target] = ps1006OrderedPointerAlias{targets: targets, known: known}
	}
	switch value := statement.(type) {
	case *ast.AssignStmt:
		if value.Tok != token.ASSIGN && value.Tok != token.DEFINE || len(value.Lhs) != len(value.Rhs) {
			return
		}
		for index := range value.Lhs {
			apply(value.Lhs[index], value.Rhs[index], value.Pos())
		}
	case *ast.DeclStmt:
		declaration, ok := value.Decl.(*ast.GenDecl)
		if !ok || declaration.Tok != token.VAR {
			return
		}
		for _, raw := range declaration.Specs {
			specification, ok := raw.(*ast.ValueSpec)
			if !ok || len(specification.Names) != len(specification.Values) {
				continue
			}
			for index := range specification.Names {
				apply(specification.Names[index], specification.Values[index], specification.Pos())
			}
		}
	}
}

const ps1006OrderedCallBudget = 512

type ps1006OrderedBinding struct {
	expression          ast.Expr
	environment         *ps1006OrderedEnvironment
	resultIndex         int
	resultSelection     bool
	valueSnapshot       bool
	dependency          ps1006OrderedDependency
	dependencySnapshot  bool
	pointerTargets      []types.Object
	pointerTargetsKnown bool
	pointerSnapshot     bool
	callableSyntaxes    []ps1006OrderedCallableSyntax
	callableKnown       bool
	callableSnapshot    bool
}

type ps1006OrderedEnvironment struct {
	bindings map[types.Object]ps1006OrderedBinding
}

type ps1006OrderedCallableSyntax struct {
	syntax           ps4008CallableSyntaxValue
	environment      *ps1006OrderedEnvironment
	receiver         ps1006OrderedBinding
	receiverSnapshot bool
}

type ps1006OrderedCallContext struct {
	active map[*ast.BlockStmt]bool
	budget int
}

type ps1006OrderedDependency struct {
	derived bool
	stride  bool
	known   bool
}

// ps1006OrderedCallDependencyState refines the conservative call transfer for
// straight-line local helpers and callbacks. It binds formal parameters to
// their actual expressions and applies writes in Go execution order, so the
// last definite write determines the value visible after the call. Any
// branch, early return, defer, go statement, unresolved alias, or unsupported
// expression abandons the refinement and retains the conservative transfer
// above. The fixed budget also makes recursive or exponentially shared
// callable graphs terminate without weakening the fallback.
func ps1006OrderedCallDependencyState(pass *analysis.Pass, expression ast.Expr, inner types.Object, initial ps1006DependencyState) (ps1006DependencyState, map[types.Object]bool, bool) {
	call, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || ps4008CallIsConversionOrBuiltin(pass, call) {
		return ps1006DependencyState{}, nil, false
	}
	initial = initial.clone()
	if initial.pointerAliases == nil {
		initial.pointerAliases = make(map[types.Object]ps1006OrderedPointerAlias)
	}
	context := &ps1006OrderedCallContext{active: make(map[*ast.BlockStmt]bool), budget: ps1006OrderedCallBudget}
	output, written, _, known := ps1006OrderedTransferCall(pass, ps1006OrderedBinding{expression: call.Fun}, call, inner, initial, context)
	return output, written, known
}

func ps1006OrderedTransferCall(pass *analysis.Pass, callable ps1006OrderedBinding, call *ast.CallExpr, inner types.Object, initial ps1006DependencyState, context *ps1006OrderedCallContext) (ps1006DependencyState, map[types.Object]bool, []ps1006OrderedBinding, bool) {
	if pass == nil || pass.TypesInfo == nil || call == nil || context == nil || context.budget <= 0 || call.Ellipsis.IsValid() {
		return ps1006DependencyState{}, nil, nil, false
	}
	context.budget--
	syntaxes, known := ps1006OrderedCallableSyntaxes(pass, callable, call.Pos(), make(map[types.Object]bool))
	if !known || len(syntaxes) == 0 {
		return ps1006DependencyState{}, nil, nil, false
	}
	receiverState := initial.clone()
	argumentState := initial.clone()
	if selector, ok := ps2110Unparen(callable.expression).(*ast.SelectorExpr); ok {
		if selection := pass.TypesInfo.Selections[selector]; selection != nil && selection.Kind() == types.MethodVal {
			// A method receiver is evaluated before the explicit arguments.
			ps1006ApplyCaseExpression(pass, selector.X, inner, &argumentState)
		}
	}
	var argumentBindings []ps1006OrderedBinding
	if len(call.Args) == 1 {
		actualType := pass.TypesInfo.TypeOf(call.Args[0])
		if actualType != nil {
			if tuple, _ := types.Unalias(actualType).Underlying().(*types.Tuple); tuple != nil {
				actual, ok := ps2110Unparen(call.Args[0]).(*ast.CallExpr)
				if !ok || ps4008CallIsConversionOrBuiltin(pass, actual) {
					return ps1006DependencyState{}, nil, nil, false
				}
				output, _, returned, ok := ps1006OrderedTransferCall(pass, ps1006OrderedBinding{
					expression: actual.Fun, environment: callable.environment,
				}, actual, inner, argumentState, context)
				if !ok || len(returned) != tuple.Len() {
					return ps1006DependencyState{}, nil, nil, false
				}
				argumentState = output
				argumentBindings = returned
			}
		}
	}
	if argumentBindings == nil {
		argumentBindings = make([]ps1006OrderedBinding, len(call.Args))
		for argumentIndex, argument := range call.Args {
			binding := ps1006OrderedBinding{expression: argument, environment: callable.environment}
			argumentBindings[argumentIndex] = ps1006OrderedSnapshotBinding(pass, binding, inner, argumentState, call.Pos())
			// Actual arguments are evaluated left to right before the selected body.
			// Snapshot the value first, then feed its effects into the entry state;
			// later argument effects must not retroactively change an earlier value.
			ps1006ApplyCaseExpression(pass, argument, inner, &argumentState)
		}
	}
	var outputs []ps1006DependencyState
	var returnSets [][]ps1006OrderedBinding
	written := make(map[types.Object]bool)
	for syntaxIndex := range syntaxes {
		resolved := &syntaxes[syntaxIndex]
		syntax := resolved.syntax
		if syntax.body == nil || context.active[syntax.body] {
			return ps1006DependencyState{}, nil, nil, false
		}
		environment := &ps1006OrderedEnvironment{bindings: make(map[types.Object]ps1006OrderedBinding)}
		for argumentIndex := range argumentBindings {
			binding := ps4008SyntaxParameterBinding(pass, syntax, argumentIndex)
			if binding.variadic {
				return ps1006DependencyState{}, nil, nil, false
			}
			if binding.object != nil {
				environment.bindings[binding.object] = argumentBindings[argumentIndex]
			}
		}
		if syntax.receiver != nil && !syntax.receiverArgument {
			receiver := ps4008SyntaxParameterObject(pass, ps4008CallableSyntaxValue{
				functionType: syntax.functionType, receiver: syntax.receiver, receiverArgument: true, body: syntax.body,
			}, 0)
			if receiver != nil {
				if resolved.receiverSnapshot {
					environment.bindings[receiver] = resolved.receiver
				} else {
					selector, ok := ps2110Unparen(callable.expression).(*ast.SelectorExpr)
					if !ok || pass.TypesInfo.Selections[selector] == nil || pass.TypesInfo.Selections[selector].Kind() != types.MethodVal {
						return ps1006DependencyState{}, nil, nil, false
					}
					receiverExpression := selector.X
					if len(syntax.receiver.List) == 1 {
						if _, pointerReceiver := ps2110Unparen(syntax.receiver.List[0].Type).(*ast.StarExpr); pointerReceiver {
							if _, pointerValue := types.Unalias(pass.TypesInfo.TypeOf(selector.X)).(*types.Pointer); !pointerValue {
								receiverExpression = &ast.UnaryExpr{Op: token.AND, X: selector.X}
							}
						}
					}
					environment.bindings[receiver] = ps1006OrderedSnapshotBinding(pass, ps1006OrderedBinding{
						expression: receiverExpression, environment: callable.environment,
					}, inner, receiverState, call.Pos())
				}
			}
		}
		context.active[syntax.body] = true
		output, syntaxWrites, returned, ok := ps1006OrderedTransferBlock(pass, syntax.body, environment, inner, argumentState.clone(), context)
		delete(context.active, syntax.body)
		if !ok {
			return ps1006DependencyState{}, nil, nil, false
		}
		outputs = append(outputs, output)
		returnSets = append(returnSets, returned)
		for object := range syntaxWrites {
			written[object] = true
		}
	}
	if len(outputs) == 0 {
		return ps1006DependencyState{}, nil, nil, false
	}
	returned, ok := ps1006MergeOrderedReturnBindings(returnSets)
	if !ok {
		return ps1006DependencyState{}, nil, nil, false
	}
	return ps1006UnionDependencyState(outputs), written, returned, true
}

func ps1006MergeOrderedReturnBindings(returnSets [][]ps1006OrderedBinding) ([]ps1006OrderedBinding, bool) {
	if len(returnSets) == 0 {
		return nil, false
	}
	count := len(returnSets[0])
	for _, set := range returnSets[1:] {
		if len(set) != count {
			return nil, false
		}
	}
	merged := make([]ps1006OrderedBinding, count)
	for resultIndex := 0; resultIndex < count; resultIndex++ {
		first := returnSets[0][resultIndex]
		result := first
		result.dependency = ps1006OrderedDependency{known: true}
		result.dependencySnapshot = true
		pointerSnapshot := first.pointerSnapshot
		pointerKnown := pointerSnapshot
		pointerTargets := make(map[types.Object]bool)
		callableSnapshot := false
		callableKnown := true
		callableMissing := false
		var callableSyntaxes []ps1006OrderedCallableSyntax
		for _, set := range returnSets {
			binding := set[resultIndex]
			result.dependency.known = result.dependency.known && binding.dependencySnapshot && binding.dependency.known
			result.dependency.derived = result.dependency.derived || binding.dependency.derived
			result.dependency.stride = result.dependency.stride || binding.dependency.stride
			if pointerSnapshot != binding.pointerSnapshot {
				pointerKnown = false
			}
			pointerKnown = pointerKnown && binding.pointerTargetsKnown
			for _, target := range binding.pointerTargets {
				if target != nil {
					pointerTargets[target] = true
				}
			}
			if binding.callableSnapshot {
				callableSnapshot = true
				callableKnown = callableKnown && binding.callableKnown
				callableSyntaxes = append(callableSyntaxes, binding.callableSyntaxes...)
			} else {
				callableMissing = true
			}
		}
		result.pointerSnapshot = pointerSnapshot
		result.pointerTargetsKnown = pointerKnown
		result.pointerTargets = nil
		for target := range pointerTargets {
			result.pointerTargets = append(result.pointerTargets, target)
		}
		result.callableSnapshot = callableSnapshot
		result.callableKnown = callableSnapshot && !callableMissing && callableKnown && len(callableSyntaxes) != 0
		result.callableSyntaxes = callableSyntaxes
		merged[resultIndex] = result
	}
	return merged, true
}

func ps1006OrderedSnapshotBinding(pass *analysis.Pass, binding ps1006OrderedBinding, inner types.Object, state ps1006DependencyState, before token.Pos) ps1006OrderedBinding {
	snapshot := binding
	snapshot.dependency = ps1006OrderedExpressionDependency(pass, binding, inner, state, make(map[types.Object]bool))
	snapshot.dependencySnapshot = true
	if binding.expression == nil || !ps4008IsPointerType(pass.TypesInfo.TypeOf(binding.expression)) {
		if ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(binding.expression), make(map[types.Type]bool)) {
			snapshot.callableSyntaxes, snapshot.callableKnown = ps1006OrderedCallableSyntaxes(pass, binding, before, make(map[types.Object]bool))
			if snapshot.callableKnown {
				snapshot.callableSyntaxes = ps1006OrderedSnapshotCallableReceivers(pass, snapshot.callableSyntaxes, binding, inner, state, before)
			}
			snapshot.callableSnapshot = true
		}
		return snapshot
	}
	snapshot.pointerTargets, snapshot.pointerTargetsKnown = ps1006OrderedPointerTargets(pass, binding, before, state.pointerAliases, make(map[types.Object]bool))
	snapshot.pointerSnapshot = true
	return snapshot
}

func ps1006OrderedSnapshotCallableReceivers(pass *analysis.Pass, syntaxes []ps1006OrderedCallableSyntax, callable ps1006OrderedBinding, inner types.Object, state ps1006DependencyState, before token.Pos) []ps1006OrderedCallableSyntax {
	selector, ok := ps2110Unparen(callable.expression).(*ast.SelectorExpr)
	if !ok {
		return syntaxes
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil || selection.Kind() != types.MethodVal {
		return syntaxes
	}
	result := slices.Clone(syntaxes)
	for index := range result {
		if result[index].receiverSnapshot {
			continue
		}
		receiverExpression := ast.Expr(selector.X)
		syntheticReceiver := false
		if result[index].syntax.receiver != nil && len(result[index].syntax.receiver.List) == 1 {
			if _, pointerReceiver := ps2110Unparen(result[index].syntax.receiver.List[0].Type).(*ast.StarExpr); pointerReceiver {
				if typ := pass.TypesInfo.TypeOf(selector.X); typ != nil {
					if _, pointerValue := types.Unalias(typ).(*types.Pointer); !pointerValue {
						receiverExpression = &ast.UnaryExpr{Op: token.AND, X: selector.X}
						syntheticReceiver = true
					}
				}
			}
		}
		binding := ps1006OrderedBinding{expression: receiverExpression, environment: callable.environment}
		if syntheticReceiver {
			object := ps1006OrderedAddressableObject(pass, selector.X)
			binding.pointerTargets = []types.Object{object}
			binding.pointerTargetsKnown = object != nil
			binding.pointerSnapshot = true
		} else {
			binding = ps1006OrderedSnapshotBinding(pass, binding, inner, state, before)
		}
		result[index].receiver = binding
		result[index].receiverSnapshot = true
	}
	return result
}

func ps1006OrderedCallableSyntaxes(pass *analysis.Pass, callable ps1006OrderedBinding, before token.Pos, seen map[types.Object]bool) ([]ps1006OrderedCallableSyntax, bool) {
	if callable.expression == nil {
		return nil, false
	}
	if callable.callableSnapshot {
		return slices.Clone(callable.callableSyntaxes), callable.callableKnown
	}
	switch value := ps2110Unparen(callable.expression).(type) {
	case *ast.FuncLit:
		return []ps1006OrderedCallableSyntax{{
			syntax: ps4008CallableSyntaxValue{functionType: value.Type, body: value.Body}, environment: callable.environment,
		}}, true
	case *ast.Ident:
		object := identObject(pass, value)
		if binding, ok := ps1006OrderedLookupBinding(callable.environment, object); ok {
			return ps1006OrderedCallableSyntaxes(pass, binding, before, seen)
		}
		if function, ok := object.(*types.Func); ok {
			index := ps1006AnalysisIndexForPass(pass)
			if index == nil || index.functionDeclarations[function] == nil {
				return nil, false
			}
			declaration := index.functionDeclarations[function]
			return []ps1006OrderedCallableSyntax{{
				syntax: ps4008CallableSyntaxValue{functionType: declaration.Type, receiver: declaration.Recv, body: declaration.Body},
			}}, true
		}
		if object == nil || seen[object] {
			return nil, false
		}
		seen[object] = true
		definitions := ps4008PossibleAliasDefinitions(pass, object, before)
		if len(definitions) == 0 {
			return nil, false
		}
		var result []ps1006OrderedCallableSyntax
		for _, definition := range definitions {
			nested, known := ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{
				expression: definition.value, environment: callable.environment,
				resultIndex: definition.resultIndex, resultSelection: definition.resultSelection,
				valueSnapshot: true,
			}, definition.position, maps.Clone(seen))
			if !known || len(nested) == 0 {
				return nil, false
			}
			result = append(result, nested...)
		}
		return result, len(result) != 0
	case *ast.SelectorExpr:
		function, _ := identObject(pass, value.Sel).(*types.Func)
		if index := ps1006AnalysisIndexForPass(pass); index != nil && index.functionDeclarations[function] != nil {
			declaration := index.functionDeclarations[function]
			selection := pass.TypesInfo.Selections[value]
			resolved := ps1006OrderedCallableSyntax{syntax: ps4008CallableSyntaxValue{
				functionType: declaration.Type, receiver: declaration.Recv,
				receiverArgument: selection != nil && selection.Kind() == types.MethodExpr,
				body:             declaration.Body,
			}, environment: callable.environment}
			if callable.valueSnapshot && selection != nil && selection.Kind() == types.MethodVal {
				receiverExpression := ast.Expr(value.X)
				if len(declaration.Recv.List) == 1 {
					if _, pointerReceiver := ps2110Unparen(declaration.Recv.List[0].Type).(*ast.StarExpr); pointerReceiver {
						if _, pointerValue := types.Unalias(pass.TypesInfo.TypeOf(value.X)).(*types.Pointer); !pointerValue {
							receiverExpression = &ast.UnaryExpr{Op: token.AND, X: value.X}
						}
					}
				}
				resolved.receiver = ps1006OrderedDefinitionSnapshotBinding(pass, ps1006OrderedBinding{
					expression: receiverExpression, environment: callable.environment,
				}, before)
				resolved.receiverSnapshot = true
			}
			return []ps1006OrderedCallableSyntax{resolved}, true
		}
		if key, ok := ps4008SelectorKey(pass, value); ok {
			values, dominating := ps4008PossibleSelectorValues(pass, key, before)
			if !dominating {
				return nil, false
			}
			return ps1006MergeOrderedCallableSyntaxes(pass, values, callable.environment, before, seen)
		}
	case *ast.IndexExpr:
		if typ := pass.TypesInfo.TypeOf(value.X); typ != nil {
			if _, signature := types.Unalias(typ).Underlying().(*types.Signature); signature {
				return ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{expression: value.X, environment: callable.environment}, before, seen)
			}
		}
		if values, dominating := ps4008PossibleConstantIndexValues(pass, value, before); dominating {
			return ps1006MergeOrderedCallableSyntaxes(pass, values, callable.environment, before, seen)
		}
		return nil, false
	case *ast.IndexListExpr:
		return ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{expression: value.X, environment: callable.environment}, before, seen)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{expression: value.Args[0], environment: callable.environment}, before, seen)
			}
		}
		resultIndex := 0
		if callable.resultSelection {
			resultIndex = callable.resultIndex
		} else if !ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(value), make(map[types.Type]bool)) {
			return nil, false
		}
		index := ps1006AnalysisIndexForPass(pass)
		if index == nil || index.activeCallableReturns[value] {
			return nil, false
		}
		index.activeCallableReturns[value] = true
		defer delete(index.activeCallableReturns, value)
		initial := ps1006DependencyState{
			deps: make(map[types.Object]bool), strideDeps: make(map[types.Object]string),
			pointerAliases: make(map[types.Object]ps1006OrderedPointerAlias),
		}
		context := &ps1006OrderedCallContext{active: make(map[*ast.BlockStmt]bool), budget: ps1006OrderedCallBudget}
		_, _, returned, known := ps1006OrderedTransferCall(pass, ps1006OrderedBinding{
			expression: value.Fun, environment: callable.environment,
		}, value, nil, initial, context)
		if !known || resultIndex < 0 || resultIndex >= len(returned) {
			return nil, false
		}
		return ps1006OrderedCallableSyntaxes(pass, returned[resultIndex], before, seen)
	case *ast.TypeAssertExpr:
		return ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{expression: value.X, environment: callable.environment}, before, seen)
	}
	return nil, false
}

func ps1006OrderedDefinitionSnapshotBinding(pass *analysis.Pass, binding ps1006OrderedBinding, before token.Pos) ps1006OrderedBinding {
	identifier, ok := ps2110Unparen(binding.expression).(*ast.Ident)
	provenanceKnown := true
	if ok {
		object := identObject(pass, identifier)
		index := ps1006AnalysisIndexForPass(pass)
		definition, found := ps4008BestAliasDefinition(pass, object, before)
		stable := index != nil && found && ps1006OrderedDefinitionSnapshotStable(index, object, definition.position, before)
		if stable && ps4008AliasDefinitionShadowsBackedge(index, definition.position, before) {
			binding.expression = definition.value
			before = definition.position
		} else if found && !stable {
			provenanceKnown = false
		}
	}
	if ps4008IsPointerType(pass.TypesInfo.TypeOf(binding.expression)) {
		if !provenanceKnown {
			binding.pointerSnapshot = true
			return binding
		}
		binding.pointerTargets, binding.pointerTargetsKnown = ps1006OrderedPointerTargets(pass, binding, before, nil, make(map[types.Object]bool))
		if binding.pointerTargetsKnown {
			binding.pointerSnapshot = true
		}
	}
	return binding
}

func ps1006OrderedDefinitionSnapshotStable(index *ps1006AnalysisIndex, object types.Object, definition, before token.Pos) bool {
	if index == nil || object == nil || !definition.IsValid() || !before.IsValid() || definition >= before {
		return false
	}
	for _, candidate := range index.aliasDefs[object] {
		if definition < candidate.position && candidate.position < before {
			return false
		}
	}
	facts := index.functionFactsAt[before]
	if facts == nil {
		return false
	}
	positions := facts.localUnsafe[object]
	position, _ := slices.BinarySearch(positions, definition+1)
	if position < len(positions) && positions[position] < before {
		return false
	}
	return true
}

func ps1006MergeOrderedCallableSyntaxes(pass *analysis.Pass, values []ast.Expr, environment *ps1006OrderedEnvironment, before token.Pos, seen map[types.Object]bool) ([]ps1006OrderedCallableSyntax, bool) {
	if len(values) == 0 {
		return nil, false
	}
	var result []ps1006OrderedCallableSyntax
	for _, value := range values {
		nested, known := ps1006OrderedCallableSyntaxes(pass, ps1006OrderedBinding{expression: value, environment: environment}, before, maps.Clone(seen))
		if !known || len(nested) == 0 {
			return nil, false
		}
		result = append(result, nested...)
	}
	return result, len(result) != 0
}

func ps1006OrderedTransferBlock(pass *analysis.Pass, body *ast.BlockStmt, environment *ps1006OrderedEnvironment, inner types.Object, initial ps1006DependencyState, context *ps1006OrderedCallContext) (ps1006DependencyState, map[types.Object]bool, []ps1006OrderedBinding, bool) {
	if body == nil || context == nil {
		return ps1006DependencyState{}, nil, nil, false
	}
	state := initial.clone()
	written := make(map[types.Object]bool)
	var returned []ps1006OrderedBinding
	for statementIndex, statement := range body.List {
		if context.budget <= 0 {
			return ps1006DependencyState{}, nil, nil, false
		}
		context.budget--
		switch value := statement.(type) {
		case *ast.AssignStmt:
			if !ps1006OrderedApplyAssignment(pass, value, environment, inner, &state, written) {
				return ps1006DependencyState{}, nil, nil, false
			}
		case *ast.DeclStmt:
			if !ps1006OrderedApplyDeclaration(pass, value, environment, inner, &state, written) {
				return ps1006DependencyState{}, nil, nil, false
			}
		case *ast.IncDecStmt:
			if !ps1006OrderedApplyIncDec(pass, value, environment, inner, &state, written) {
				return ps1006DependencyState{}, nil, nil, false
			}
		case *ast.ExprStmt:
			call, ok := ps2110Unparen(value.X).(*ast.CallExpr)
			if !ok {
				return ps1006DependencyState{}, nil, nil, false
			}
			if ps1006OrderedApplyPrintBuiltin(pass, call, inner, &state) {
				continue
			}
			if ps4008CallIsConversionOrBuiltin(pass, call) {
				return ps1006DependencyState{}, nil, nil, false
			}
			output, callWrites, _, ok := ps1006OrderedTransferCall(pass, ps1006OrderedBinding{expression: call.Fun, environment: environment}, call, inner, state, context)
			if !ok {
				return ps1006DependencyState{}, nil, nil, false
			}
			state = output
			for object := range callWrites {
				written[object] = true
			}
		case *ast.BlockStmt:
			output, blockWrites, _, ok := ps1006OrderedTransferBlock(pass, value, environment, inner, state, context)
			if !ok {
				return ps1006DependencyState{}, nil, nil, false
			}
			state = output
			for object := range blockWrites {
				written[object] = true
			}
		case *ast.ForStmt:
			output, loopWrites, ok := ps1006OrderedTransferLoop(pass, value, environment, inner, state, context)
			if !ok {
				return ps1006DependencyState{}, nil, nil, false
			}
			state = output
			for object := range loopWrites {
				written[object] = true
			}
		case *ast.ReturnStmt:
			if statementIndex != len(body.List)-1 || !ps1006OrderedExpressionsValueOnly(pass, value.Results, environment, state, inner) {
				return ps1006DependencyState{}, nil, nil, false
			}
			for _, result := range value.Results {
				binding := ps1006OrderedSnapshotBinding(pass, ps1006OrderedBinding{
					expression: result, environment: environment,
				}, inner, state, result.Pos())
				call, ok := ps2110Unparen(result).(*ast.CallExpr)
				if !ok || ps4008CallIsConversionOrBuiltin(pass, call) {
					if ps1006ExpressionMayAffectPointerAliases(pass, result) {
						return ps1006DependencyState{}, nil, nil, false
					}
					returned = append(returned, binding)
					continue
				}
				output, callWrites, nestedReturns, ok := ps1006OrderedTransferCall(pass, ps1006OrderedBinding{
					expression: call.Fun, environment: environment,
				}, call, inner, state, context)
				if !ok {
					return ps1006DependencyState{}, nil, nil, false
				}
				state = output
				if len(nestedReturns) == 0 {
					returned = append(returned, binding)
				} else {
					returned = append(returned, nestedReturns...)
				}
				for object := range callWrites {
					written[object] = true
				}
			}
		case *ast.EmptyStmt:
		default:
			return ps1006DependencyState{}, nil, nil, false
		}
	}
	return state, written, returned, true
}

func ps1006OrderedTransferLoop(pass *analysis.Pass, loop *ast.ForStmt, environment *ps1006OrderedEnvironment, inner types.Object, initial ps1006DependencyState, context *ps1006OrderedCallContext) (ps1006DependencyState, map[types.Object]bool, bool) {
	if loop == nil || loop.Body == nil || context == nil {
		return ps1006DependencyState{}, nil, false
	}
	entry := initial.clone()
	written := make(map[types.Object]bool)
	if !ps1006OrderedApplyLoopStatement(pass, loop.Init, environment, inner, &entry, written) {
		return ps1006DependencyState{}, nil, false
	}
	if loop.Cond != nil {
		dependency := ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: loop.Cond, environment: environment}, inner, entry, make(map[types.Object]bool))
		if !dependency.known || ps1006ExpressionMayAffectPointerAliases(pass, loop.Cond) {
			return ps1006DependencyState{}, nil, false
		}
		if value, known := ps1006BoolConstant(pass, loop.Cond); known && !value {
			return entry, written, true
		}
	}
	head := entry.clone()
	for range ps1006DependencyLoopIterations {
		output, bodyWrites, returned, ok := ps1006OrderedTransferBlock(pass, loop.Body, environment, inner, head, context)
		if !ok || len(returned) != 0 || !ps1006OrderedApplyLoopStatement(pass, loop.Post, environment, inner, &output, bodyWrites) {
			return ps1006DependencyState{}, nil, false
		}
		for object := range bodyWrites {
			written[object] = true
		}
		merged := ps1006UnionDependencyState([]ps1006DependencyState{entry, output})
		if ps1006DependencyStateKey(merged) == ps1006DependencyStateKey(head) {
			return merged, written, true
		}
		head = merged
	}
	return head, written, true
}

func ps1006OrderedApplyLoopStatement(pass *analysis.Pass, statement ast.Stmt, environment *ps1006OrderedEnvironment, inner types.Object, state *ps1006DependencyState, written map[types.Object]bool) bool {
	switch value := statement.(type) {
	case nil, *ast.EmptyStmt:
		return true
	case *ast.AssignStmt:
		return ps1006OrderedApplyAssignment(pass, value, environment, inner, state, written)
	case *ast.DeclStmt:
		return ps1006OrderedApplyDeclaration(pass, value, environment, inner, state, written)
	case *ast.IncDecStmt:
		return ps1006OrderedApplyIncDec(pass, value, environment, inner, state, written)
	}
	return false
}

func ps1006OrderedApplyPrintBuiltin(pass *analysis.Pass, call *ast.CallExpr, inner types.Object, state *ps1006DependencyState) bool {
	if pass == nil || pass.TypesInfo == nil || call == nil || state == nil {
		return false
	}
	identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := identObject(pass, identifier).(*types.Builtin)
	if !ok || builtin.Name() != "print" && builtin.Name() != "println" {
		return false
	}
	for _, argument := range call.Args {
		ps1006ApplyCaseExpression(pass, argument, inner, state)
	}
	return true
}

func ps1006OrderedApplyDeclaration(pass *analysis.Pass, statement *ast.DeclStmt, environment *ps1006OrderedEnvironment, inner types.Object, state *ps1006DependencyState, written map[types.Object]bool) bool {
	declaration, ok := statement.Decl.(*ast.GenDecl)
	if !ok || declaration.Tok != token.VAR {
		return false
	}
	for _, raw := range declaration.Specs {
		specification, ok := raw.(*ast.ValueSpec)
		if !ok || len(specification.Names) != len(specification.Values) {
			return false
		}
		lhs := make([]ast.Expr, len(specification.Names))
		for index, name := range specification.Names {
			lhs[index] = name
		}
		assignment := &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: specification.Values}
		if !ps1006OrderedApplyAssignment(pass, assignment, environment, inner, state, written) {
			return false
		}
	}
	return true
}

func ps1006OrderedApplyAssignment(pass *analysis.Pass, assignment *ast.AssignStmt, environment *ps1006OrderedEnvironment, inner types.Object, state *ps1006DependencyState, written map[types.Object]bool) bool {
	if assignment == nil || state == nil || len(assignment.Lhs) == 0 || len(assignment.Lhs) != len(assignment.Rhs) {
		return false
	}
	simple := assignment.Tok == token.ASSIGN || assignment.Tok == token.DEFINE
	compound := ps1006OrderedCompoundAssignment(assignment.Tok)
	if !simple && !compound || compound && len(assignment.Lhs) != 1 {
		return false
	}
	targets := make([]types.Object, len(assignment.Lhs))
	for index, lhs := range assignment.Lhs {
		target, ok := ps1006OrderedAssignmentTarget(pass, lhs, environment, state.pointerAliases, assignment.Pos())
		if !ok {
			return false
		}
		targets[index] = target
	}
	old := state.clone()
	rhsState := state.clone()
	dependencies := make([]ps1006OrderedDependency, len(assignment.Rhs))
	pointerValues := make([]ps1006OrderedPointerAlias, len(assignment.Rhs))
	pointerValue := make([]bool, len(assignment.Rhs))
	for index, rhs := range assignment.Rhs {
		dependencies[index] = ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{
			expression: rhs, environment: environment,
		}, inner, rhsState, make(map[types.Object]bool))
		if !dependencies[index].known {
			return false
		}
		if target := targets[index]; target != nil && ps4008IsPointerType(target.Type()) && ps4008IsPointerType(pass.TypesInfo.TypeOf(rhs)) {
			pointerValues[index].targets, pointerValues[index].known = ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{
				expression: rhs, environment: environment,
			}, assignment.Pos(), rhsState.pointerAliases, make(map[types.Object]bool))
			pointerValue[index] = true
		}
		ps1006ApplyCaseExpression(pass, rhs, inner, &rhsState)
	}
	*state = rhsState
	if compound {
		target := targets[0]
		dependencies[0].derived = dependencies[0].derived || target == inner || old.deps[target]
		dependencies[0].stride = dependencies[0].stride || target == inner || old.strideDeps[target] != ""
	}
	for index, target := range targets {
		if target == nil { // the blank identifier receives no value
			continue
		}
		written[target] = true
		ps1006OrderedSetDependency(state, target, dependencies[index])
		if pointerValue[index] {
			state.pointerAliases[target] = clonePS1006OrderedPointerAlias(pointerValues[index])
		}
	}
	return true
}

func ps1006OrderedAssignmentTarget(pass *analysis.Pass, expression ast.Expr, environment *ps1006OrderedEnvironment, aliases map[types.Object]ps1006OrderedPointerAlias, before token.Pos) (types.Object, bool) {
	switch lhs := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		if lhs.Name == "_" {
			return nil, true
		}
		object := identObject(pass, lhs)
		if object == nil {
			return nil, false
		}
		if _, bound := ps1006OrderedLookupBinding(environment, object); bound {
			// Reassigning a formal changes the callee-local binding rather than the
			// caller's value. Leave such bodies to the conservative transfer.
			return nil, false
		}
		return object, true
	case *ast.StarExpr:
		targets, known := ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{expression: lhs.X, environment: environment}, before, aliases, make(map[types.Object]bool))
		// A dereference definitely overwrites an object only when every possible
		// pointer value names that same object. Multiple targets are may-writes;
		// abandoning the refinement lets the conservative transfer retain all
		// incoming dependencies.
		if !known || len(targets) != 1 {
			return nil, false
		}
		return targets[0], targets[0] != nil
	default:
		return nil, false
	}
}

func ps1006OrderedApplyIncDec(pass *analysis.Pass, statement *ast.IncDecStmt, environment *ps1006OrderedEnvironment, inner types.Object, state *ps1006DependencyState, written map[types.Object]bool) bool {
	if statement == nil || state == nil {
		return false
	}
	target, ok := ps1006OrderedAssignmentTarget(pass, statement.X, environment, state.pointerAliases, statement.Pos())
	if !ok {
		return false
	}
	written[target] = true
	ps1006OrderedSetDependency(state, target, ps1006OrderedDependency{
		derived: target == inner || state.deps[target],
		stride:  target == inner || state.strideDeps[target] != "",
		known:   true,
	})
	return true
}

func ps1006OrderedCompoundAssignment(operation token.Token) bool {
	switch operation {
	case token.ADD_ASSIGN, token.SUB_ASSIGN, token.MUL_ASSIGN, token.QUO_ASSIGN,
		token.REM_ASSIGN, token.AND_ASSIGN, token.OR_ASSIGN, token.XOR_ASSIGN,
		token.SHL_ASSIGN, token.SHR_ASSIGN, token.AND_NOT_ASSIGN:
		return true
	}
	return false
}

func ps1006OrderedSetDependency(state *ps1006DependencyState, target types.Object, dependency ps1006OrderedDependency) {
	if dependency.derived {
		state.deps[target] = true
	} else {
		delete(state.deps, target)
	}
	if dependency.stride {
		state.strideDeps[target] = ps1006MayStrideKey
	} else {
		delete(state.strideDeps, target)
	}
}

func ps1006OrderedExpressionDependency(pass *analysis.Pass, expression ps1006OrderedBinding, inner types.Object, state ps1006DependencyState, seen map[types.Object]bool) ps1006OrderedDependency {
	if expression.expression == nil {
		return ps1006OrderedDependency{}
	}
	if expression.dependencySnapshot {
		return expression.dependency
	}
	switch value := ps2110Unparen(expression.expression).(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if binding, ok := ps1006OrderedLookupBinding(expression.environment, object); ok {
			if object == nil || seen[object] {
				return ps1006OrderedDependency{}
			}
			seen[object] = true
			return ps1006OrderedExpressionDependency(pass, binding, inner, state, seen)
		}
		return ps1006OrderedDependency{
			derived: object == inner || state.deps[object],
			stride:  object == inner || state.strideDeps[object] != "",
			known:   true,
		}
	case *ast.BasicLit, *ast.FuncLit:
		return ps1006OrderedDependency{known: true}
	case *ast.ParenExpr:
		return ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: value.X, environment: expression.environment}, inner, state, seen)
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps1006OrderedDependency{known: true}
		}
		if value.Op == token.MUL {
			targets, known := ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{
				expression: value.X, environment: expression.environment,
			}, value.Pos(), state.pointerAliases, make(map[types.Object]bool))
			if !known || len(targets) == 0 {
				return ps1006OrderedDependency{}
			}
			dependency := ps1006OrderedDependency{known: true}
			for _, target := range targets {
				dependency.derived = dependency.derived || target == inner || state.deps[target]
				dependency.stride = dependency.stride || target == inner || state.strideDeps[target] != ""
			}
			return dependency
		}
		return ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: value.X, environment: expression.environment}, inner, state, seen)
	case *ast.StarExpr:
		targets, known := ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{
			expression: value.X, environment: expression.environment,
		}, value.Pos(), state.pointerAliases, make(map[types.Object]bool))
		if !known || len(targets) == 0 {
			return ps1006OrderedDependency{}
		}
		dependency := ps1006OrderedDependency{known: true}
		for _, target := range targets {
			dependency.derived = dependency.derived || target == inner || state.deps[target]
			dependency.stride = dependency.stride || target == inner || state.strideDeps[target] != ""
		}
		return dependency
	case *ast.BinaryExpr:
		left := ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: value.X, environment: expression.environment}, inner, state, maps.Clone(seen))
		right := ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: value.Y, environment: expression.environment}, inner, state, maps.Clone(seen))
		return ps1006OrderedDependency{derived: left.derived || right.derived, stride: left.stride || right.stride, known: left.known && right.known}
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: value.Args[0], environment: expression.environment}, inner, state, seen)
			}
		}
		// A general call result can depend on hidden package or captured state.
		// Treat it as dependent, while still allowing a later definite write in
		// the same ordered body to replace it.
		return ps1006OrderedDependency{derived: true, stride: true, known: true}
	case *ast.SelectorExpr, *ast.IndexExpr, *ast.IndexListExpr:
		derived, stride, known := false, false, true
		ast.Inspect(value, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				if _, ok := node.(*ast.CallExpr); ok && node != value {
					known = false
					return false
				}
				return true
			}
			part := ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: identifier, environment: expression.environment}, inner, state, maps.Clone(seen))
			known = known && part.known
			derived = derived || part.derived
			stride = stride || part.stride
			return known
		})
		return ps1006OrderedDependency{derived: derived, stride: stride, known: known}
	}
	return ps1006OrderedDependency{}
}

func ps1006OrderedExpressionsValueOnly(pass *analysis.Pass, expressions []ast.Expr, environment *ps1006OrderedEnvironment, state ps1006DependencyState, inner types.Object) bool {
	for _, expression := range expressions {
		if !ps1006OrderedExpressionDependency(pass, ps1006OrderedBinding{expression: expression, environment: environment}, inner, state, make(map[types.Object]bool)).known {
			return false
		}
	}
	return true
}

func ps1006OrderedPointerTargets(pass *analysis.Pass, expression ps1006OrderedBinding, before token.Pos, aliases map[types.Object]ps1006OrderedPointerAlias, seen map[types.Object]bool) ([]types.Object, bool) {
	if expression.expression == nil {
		return nil, false
	}
	if expression.pointerSnapshot {
		return slices.Clone(expression.pointerTargets), expression.pointerTargetsKnown
	}
	switch value := ps2110Unparen(expression.expression).(type) {
	case *ast.UnaryExpr:
		if value.Op != token.AND {
			return nil, false
		}
		object := ps1006OrderedAddressableObject(pass, value.X)
		return []types.Object{object}, object != nil
	case *ast.Ident:
		object := identObject(pass, value)
		if binding, ok := ps1006OrderedLookupBinding(expression.environment, object); ok {
			if object == nil || seen[object] {
				return nil, false
			}
			seen[object] = true
			return ps1006OrderedPointerTargets(pass, binding, before, aliases, seen)
		}
		return ps1006OrderedPointerObjectTargets(pass, object, expression.environment, before, aliases, seen)
	case *ast.StarExpr:
		objects, known := ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{
			expression: value.X, environment: expression.environment,
		}, before, aliases, maps.Clone(seen))
		if !known || len(objects) == 0 {
			return nil, false
		}
		targets := make(map[types.Object]bool)
		for _, object := range objects {
			nested, resolved := ps1006OrderedPointerObjectTargets(pass, object, expression.environment, before, aliases, maps.Clone(seen))
			if !resolved {
				return nil, false
			}
			for _, target := range nested {
				targets[target] = true
			}
		}
		result := make([]types.Object, 0, len(targets))
		for target := range targets {
			result = append(result, target)
		}
		return result, len(result) != 0
	case *ast.SelectorExpr:
		if key, ok := ps4008SelectorKey(pass, value); ok {
			if root, bound := ps1006OrderedLookupBinding(expression.environment, key.root); bound {
				if selected, found := ps4008CompositeSelectorValue(pass, root.expression, key.path); found {
					return ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{
						expression: selected, environment: root.environment,
					}, before, aliases, maps.Clone(seen))
				}
			}
		}
		object := ps1006OrderedAddressableObject(pass, value)
		if _, tracked := aliases[object]; tracked {
			return ps1006OrderedPointerObjectTargets(pass, object, expression.environment, before, aliases, seen)
		}
		// Selector slots are synthetic objects used only by the ordered transfer.
		// Until such a slot is dynamically retargeted, retain the selector
		// provenance index (including composite-literal instance identity).
		return ps4008PointerObjectTargets(pass, value, before, seen)
	case *ast.CallExpr:
		if len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			if function, ok := pass.TypesInfo.Types[value.Fun]; ok && function.IsType() {
				return ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{expression: value.Args[0], environment: expression.environment}, before, aliases, seen)
			}
		}
	}
	return nil, false
}

func ps1006OrderedAddressableObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		return identObject(pass, value)
	case *ast.SelectorExpr:
		key, ok := ps4008SelectorKey(pass, value)
		index := ps1006AnalysisIndexForPass(pass)
		if !ok || index == nil {
			return nil
		}
		if object := index.orderedPointerSlots[key]; object != nil {
			return object
		}
		object := types.NewVar(token.NoPos, pass.Pkg, key.path, pass.TypesInfo.TypeOf(value))
		index.orderedPointerSlots[key] = object
		return object
	}
	return nil
}

func ps1006OrderedPointerObjectTargets(pass *analysis.Pass, object types.Object, environment *ps1006OrderedEnvironment, before token.Pos, aliases map[types.Object]ps1006OrderedPointerAlias, seen map[types.Object]bool) ([]types.Object, bool) {
	if object == nil || seen[object] {
		return nil, false
	}
	if alias, ok := aliases[object]; ok {
		return slices.Clone(alias.targets), alias.known
	}
	seen[object] = true
	values := ps4008PossibleAliasValues(pass, object, before)
	if len(values) == 0 {
		return nil, false
	}
	targets := make(map[types.Object]bool)
	for _, candidate := range values {
		nested, known := ps1006OrderedPointerTargets(pass, ps1006OrderedBinding{
			expression: candidate, environment: environment,
		}, before, aliases, maps.Clone(seen))
		if !known {
			return nil, false
		}
		for _, target := range nested {
			targets[target] = true
		}
	}
	result := make([]types.Object, 0, len(targets))
	for target := range targets {
		result = append(result, target)
	}
	return result, len(result) != 0
}

// ps1006InvalidateOrderedPointerAliases prevents a failed refinement from
// exposing an alias value captured before an opaque expression ran. It is
// intentionally conservative: any mentioned pointer variable may have been
// rebound directly, through **T, or by an invoked closure. A subsequent
// dereference therefore falls back instead of inventing a definite target.
func ps1006InvalidateOrderedPointerAliases(pass *analysis.Pass, expression ast.Expr, aliases map[types.Object]ps1006OrderedPointerAlias) {
	if pass == nil || pass.TypesInfo == nil || expression == nil || aliases == nil {
		return
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if object == nil || !ps4008IsPointerType(object.Type()) {
			return true
		}
		if _, variable := object.(*types.Var); variable {
			aliases[object] = ps1006OrderedPointerAlias{}
		}
		return true
	})
}

func ps1006OrderedLookupBinding(environment *ps1006OrderedEnvironment, object types.Object) (ps1006OrderedBinding, bool) {
	if environment == nil || object == nil {
		return ps1006OrderedBinding{}, false
	}
	binding, ok := environment.bindings[object]
	return binding, ok
}

func ps1006BoundOverwriteDepends(pass *analysis.Pass, expression ast.Expr, syntax ps4008CallableSyntaxValue, call *ast.CallExpr, inner types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string) bool {
	depends := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if depends {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		for argumentIndex, argument := range call.Args {
			if ps4008SyntaxParameterObject(pass, syntax, argumentIndex) != object {
				continue
			}
			depends = ps1006CaseExpressionInputDepends(pass, argument, inner, deps, strideDeps)
			return !depends
		}
		depends = object == inner || deps[object] || strideDeps[object] != ""
		return !depends
	})
	return depends
}

func ps1006CaseExpressionInputDepends(pass *analysis.Pass, expression ast.Expr, inner types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string) bool {
	depends := false
	for _, effectExpression := range ps1006ConditionEffectExpressions(pass, expression) {
		var inspect func(ast.Node) bool
		inspect = func(node ast.Node) bool {
			if depends {
				return false
			}
			if selector, ok := node.(*ast.SelectorExpr); ok {
				selection := pass.TypesInfo.Selections[selector]
				if selection != nil && selection.Kind() == types.MethodVal && ps4008ReceiverMayMutateReferencedStorage(pass, selector) {
					// A mutable method receiver is the destination transported into
					// the call. Its explicit arguments and body determine the value
					// written; merely selecting the method does not read that value.
					return false
				}
			}
			if literal, ok := node.(*ast.FuncLit); ok && literal != effectExpression {
				return false
			}
			if assignment, ok := node.(*ast.AssignStmt); ok {
				// Assignment destinations are outputs, not inputs. Traverse only
				// evaluated RHS expressions. Compound assignments also read their
				// destinations and therefore retain an existing dependency.
				if assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE {
					for _, lhs := range assignment.Lhs {
						ast.Inspect(lhs, inspect)
					}
				}
				for _, rhs := range assignment.Rhs {
					ast.Inspect(rhs, inspect)
				}
				return false
			}
			if unary, ok := node.(*ast.UnaryExpr); ok && unary.Op == token.AND {
				return false
			}
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			// Pointer/callable identities are transport for an output or callback,
			// not numeric inputs to the value written through them. The invoked
			// body is visited separately when the callee actually executes it.
			if typ := pass.TypesInfo.TypeOf(identifier); typ != nil && ps1006TypeMayAliasMutableStorage(typ, make(map[types.Type]bool)) {
				return true
			}
			object := identObject(pass, identifier)
			depends = object == inner || deps[object] || strideDeps[object] != ""
			return !depends
		}
		ast.Inspect(effectExpression, inspect)
		if depends {
			break
		}
	}
	return depends
}

func ps1006CaseExpressionTargets(pass *analysis.Pass, expression ast.Expr, state *ps1006DependencyState) map[types.Object]bool {
	targets := make(map[types.Object]bool)
	effectExpressions := ps1006ConditionEffectExpressions(pass, expression)
	candidates := make(map[types.Object]bool)
	if state != nil {
		for object := range state.deps {
			candidates[object] = true
		}
		for object := range state.strideDeps {
			candidates[object] = true
		}
	}
	// The first entry is the condition expression itself. Function literals
	// nested in a value-only conversion/comparison are dormant; only callable
	// values discovered at actual call sites below may execute their bodies.
	for _, effectExpression := range effectExpressions[1:] {
		for object := range ps1006MentionedObjects(pass, effectExpression) {
			if _, ok := object.(*types.Var); ok {
				candidates[object] = true
			}
		}
	}
	for _, effectExpression := range effectExpressions[1:] {
		ast.Inspect(effectExpression, func(node ast.Node) bool {
			if literal, ok := node.(*ast.FuncLit); ok && literal != effectExpression {
				return false
			}
			switch value := node.(type) {
			case *ast.AssignStmt:
				for _, lhs := range value.Lhs {
					for object := range ps1006AssignmentRootObjects(pass, lhs) {
						targets[object] = true
					}
				}
			case *ast.IncDecStmt:
				for object := range ps1006AssignmentRootObjects(pass, value.X) {
					targets[object] = true
				}
			}
			return true
		})
	}
	ast.Inspect(expression, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if ps4008CallIsConversionOrBuiltin(pass, call) {
				return true
			}
			for object := range ps1006CallablePackageObjects(pass, call.Fun, expression.Pos()) {
				candidates[object] = true
			}
			written, known := ps4008CallableWriteTargets(pass, call.Fun, candidates, expression.Pos(), make(map[types.Object]bool))
			if known {
				for object := range written {
					targets[object] = true
				}
			}
			for argumentIndex, argument := range call.Args {
				resolved, known := ps1006PointerTargetsForState(pass, argument, expression.Pos(), state)
				if known && ps4008CallWritesArgument(pass, call, argumentIndex, expression.Pos(), ps4008MayWrite) {
					for _, object := range resolved {
						if object != nil {
							targets[object] = true
						}
					}
				}
				if !ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
					continue
				}
				if !ps4008CallInvokesArgument(pass, call, argumentIndex, expression.Pos(), ps4008MayWrite) {
					continue
				}
				written, known := ps4008CallableWriteTargets(pass, argument, candidates, expression.Pos(), make(map[types.Object]bool))
				if !known {
					continue
				}
				for object := range written {
					targets[object] = true
				}
			}
		}
		return true
	})
	return targets
}

func ps1006CallablePackageObjects(pass *analysis.Pass, expression ast.Expr, before token.Pos) map[types.Object]bool {
	result := make(map[types.Object]bool)
	syntaxes, known := ps4008CallableSyntaxes(pass, expression, before, make(map[types.Object]bool))
	if !known {
		return result
	}
	for _, syntax := range syntaxes {
		for object := range ps1006CallableBodyPackageObjects(pass, syntax.body, make(map[*ast.BlockStmt]bool)) {
			result[object] = true
		}
	}
	return result
}

func ps1006CallableBodyPackageObjects(pass *analysis.Pass, body *ast.BlockStmt, seen map[*ast.BlockStmt]bool) map[types.Object]bool {
	result := make(map[types.Object]bool)
	if body == nil || seen[body] {
		return result
	}
	index := ps1006AnalysisIndexForPass(pass)
	if index != nil {
		if cached, ok := index.callablePackageObjects[body]; ok {
			return cloneObjectBoolMap(cached)
		}
	}
	seen[body] = true
	ast.Inspect(body, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if variable, ok := object.(*types.Var); ok && pass != nil && pass.Pkg != nil && variable.Parent() == pass.Pkg.Scope() {
			result[variable] = true
		}
		function, ok := object.(*types.Func)
		if !ok || index == nil {
			return true
		}
		if declaration := index.functionDeclarations[function]; declaration != nil {
			for nested := range ps1006CallableBodyPackageObjects(pass, declaration.Body, seen) {
				result[nested] = true
			}
		}
		return true
	})
	delete(seen, body)
	if index != nil {
		index.callablePackageObjects[body] = cloneObjectBoolMap(result)
	}
	return result
}

// ps1006ConditionEffectExpressions includes stored callable bodies which may
// execute while a condition is evaluated. This lets the dependency transfer
// relate a callback's captured writes to serial inputs used inside that body.
func ps1006ConditionEffectExpressions(pass *analysis.Pass, expression ast.Expr) []ast.Expr {
	if expression == nil {
		return nil
	}
	expressions := []ast.Expr{expression}
	seen := map[ast.Expr]bool{expression: true}
	ast.Inspect(expression, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var callables []ast.Expr
		if !ps4008CallIsConversionOrBuiltin(pass, call) {
			for argumentIndex, argument := range call.Args {
				if ps4008CallInvokesArgument(pass, call, argumentIndex, expression.Pos(), ps4008MayWrite) {
					callables = append(callables, argument)
				}
			}
		}
		if ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(call.Fun), make(map[types.Type]bool)) {
			callables = append(callables, call.Fun)
		}
		for _, argument := range callables {
			if !ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
				continue
			}
			for _, value := range ps4008AggregateCallableValues(pass, argument, expression.Pos()) {
				if value != nil && !seen[value] {
					seen[value] = true
					expressions = append(expressions, value)
				}
			}
			// Keep a direct alias-definition fallback for loop-local callbacks.
			// The general provenance resolver intentionally skips definitions
			// whose dominance is uncertain; condition transfer must retain such
			// values conservatively because the callback can execute here.
			identifier, ok := ps2110Unparen(argument).(*ast.Ident)
			index := ps1006AnalysisIndexForPass(pass)
			if !ok || index == nil {
				continue
			}
			for _, definition := range index.aliasDefs[identObject(pass, identifier)] {
				if definition.value == nil || definition.position >= expression.Pos() || seen[definition.value] {
					continue
				}
				seen[definition.value] = true
				expressions = append(expressions, definition.value)
			}
		}
		return true
	})
	return expressions
}

func ps1006BoolConstant(pass *analysis.Pass, expression ast.Expr) (bool, bool) {
	if pass == nil || pass.TypesInfo == nil || expression == nil {
		return false, false
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Bool {
		return false, false
	}
	return constant.BoolVal(value), true
}

// ps1006ConditionDependencyStates evaluates &&, || and ! with Go's exact
// short-circuit order. Leaf effects use the same multi-function transfer as
// switch case expressions: a callee may overwrite an addressed or captured
// local with a value derived from the serial loop variable.
func ps1006ConditionDependencyStates(pass *analysis.Pass, expression ast.Expr, inner types.Object, state ps1006DependencyState) (whenTrue, whenFalse []ps1006DependencyState) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			whenFalse, whenTrue = ps1006ConditionDependencyStates(pass, value.X, inner, state)
			return whenTrue, whenFalse
		}
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND:
			leftTrue, leftFalse := ps1006ConditionDependencyStates(pass, value.X, inner, state)
			whenFalse = append(whenFalse, leftFalse...)
			for _, left := range leftTrue {
				rightTrue, rightFalse := ps1006ConditionDependencyStates(pass, value.Y, inner, left)
				whenTrue = append(whenTrue, rightTrue...)
				whenFalse = append(whenFalse, rightFalse...)
			}
			return ps1006DedupeDependencyStates(whenTrue), ps1006DedupeDependencyStates(whenFalse)
		case token.LOR:
			leftTrue, leftFalse := ps1006ConditionDependencyStates(pass, value.X, inner, state)
			whenTrue = append(whenTrue, leftTrue...)
			for _, left := range leftFalse {
				rightTrue, rightFalse := ps1006ConditionDependencyStates(pass, value.Y, inner, left)
				whenTrue = append(whenTrue, rightTrue...)
				whenFalse = append(whenFalse, rightFalse...)
			}
			return ps1006DedupeDependencyStates(whenTrue), ps1006DedupeDependencyStates(whenFalse)
		}
	}
	next := state.clone()
	ps1006ApplyCaseExpression(pass, expression, inner, &next)
	if result, known := ps1006BoolConstant(pass, expression); known {
		if result {
			return []ps1006DependencyState{next}, nil
		}
		return nil, []ps1006DependencyState{next}
	}
	return []ps1006DependencyState{next.clone()}, []ps1006DependencyState{next}
}

func ps1006DedupeDependencyStates(states []ps1006DependencyState) []ps1006DependencyState {
	if len(states) <= 1 {
		return states
	}
	seen := make(map[string]bool, len(states))
	result := make([]ps1006DependencyState, 0, len(states))
	for _, state := range states {
		key := ps4008BoolObjectSetKey(state.deps) + ";" + ps1006StrideDepsKey(state.strideDeps) + ";" + ps1006OrderedPointerAliasesKey(state.pointerAliases)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, state)
		if len(result) > ps1006MaxAccumulatorPaths {
			return []ps1006DependencyState{ps1006UnionDependencyState(result)}
		}
	}
	return result
}

// ps1006SelectedDependencyStates evaluates case expressions in source order.
// selected[i] is the state after a possible match of clause i; remaining is
// the no-match state. Default is selected only after every expression misses.
func ps1006SelectedDependencyStates(pass *analysis.Pass, clauses []*ast.CaseClause, inner types.Object, state ps1006DependencyState, constantBools bool) (selected [][]ps1006DependencyState, remaining []ps1006DependencyState) {
	selected = make([][]ps1006DependencyState, len(clauses))
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
			ps1006ApplyCaseExpression(pass, expression, inner, &current)
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
		remaining = []ps1006DependencyState{current}
	}
	return selected, remaining
}

func ps1006DependencyExitStatesForIf(pass *analysis.Pass, statement *ast.IfStmt, inner types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string, pointerAliases map[types.Object]ps1006OrderedPointerAlias, breakExits bool) []ps1006DependencyState {
	if statement == nil {
		return []ps1006DependencyState{{deps: cloneObjectBoolMap(deps), strideDeps: cloneObjectStringMap(strideDeps), pointerAliases: clonePS1006OrderedPointerAliases(pointerAliases)}}
	}
	state := ps1006DependencyState{deps: cloneObjectBoolMap(deps), strideDeps: cloneObjectStringMap(strideDeps), pointerAliases: clonePS1006OrderedPointerAliases(pointerAliases)}
	if statement.Init != nil {
		ps4008UpdateDerivedDeps(pass, statement.Init, inner, state.deps)
		ps1006UpdateDerivedStrideDeps(pass, statement.Init, inner, state.strideDeps)
	}
	whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, statement.Cond, inner, state)
	thenStates := ps1006DependencyExitStatesForBlock(pass, statement.Body, inner, whenTrue, breakExits)
	var elseStates []ps1006DependencyState
	switch elseNode := statement.Else.(type) {
	case *ast.BlockStmt:
		elseStates = ps1006DependencyExitStatesForBlock(pass, elseNode, inner, whenFalse, breakExits)
	case *ast.IfStmt:
		for _, branch := range whenFalse {
			elseStates = append(elseStates, ps1006DependencyExitStatesForIf(pass, elseNode, inner, branch.deps, branch.strideDeps, branch.pointerAliases, breakExits)...)
		}
	default:
		elseStates = whenFalse
	}
	return ps1006DedupeDependencyStates(append(thenStates, elseStates...))
}

func ps1006DependencyExitStatesForBlock(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, states []ps1006DependencyState, breakExits bool) []ps1006DependencyState {
	if body == nil {
		return states
	}
	for _, statement := range body.List {
		var next []ps1006DependencyState
		for _, state := range states {
			next = append(next, ps1006DependencyExitStatesForStatement(pass, statement, inner, state, breakExits)...)
		}
		states = next
		if len(states) == 0 {
			return nil
		}
	}
	return states
}

func ps1006DependencyExitStatesForStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, state ps1006DependencyState, breakExits bool) []ps1006DependencyState {
	current := state.clone()
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, value, inner, current.deps)
		ps1006UpdateDerivedStrideDeps(pass, value, inner, current.strideDeps)
		ps1006UpdateOrderedPointerAliasesForStatement(pass, value, current.pointerAliases)
		return []ps1006DependencyState{current}
	case *ast.ExprStmt:
		ps1006ApplyExpressionDependencyStatement(pass, value, inner, current.deps, current.strideDeps, current.pointerAliases)
		return []ps1006DependencyState{current}
	case *ast.IfStmt:
		return ps1006DependencyExitStatesForIf(pass, value, inner, current.deps, current.strideDeps, current.pointerAliases, breakExits)
	case *ast.ForStmt:
		return ps1006DependencyExitStatesForLoop(pass, value.Init, value.Cond, value.Post, value.Body, inner, current)
	case *ast.BlockStmt:
		return ps1006DependencyExitStatesForBlock(pass, value, inner, []ps1006DependencyState{current}, breakExits)
	case *ast.LabeledStmt:
		return ps1006DependencyExitStatesForStatement(pass, value.Stmt, inner, current, breakExits)
	case *ast.SwitchStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, current.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, current.strideDeps)
		}
		ps4008InvalidateDerivedDepsInExpr(pass, value.Tag, current.deps)
		ps1006InvalidateStrideDepsInExpr(pass, value.Tag, current.strideDeps)
		return ps1006DependencyExitStatesForCaseClauses(pass, value.Body, inner, current, value.Tag == nil)
	case *ast.TypeSwitchStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, current.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, current.strideDeps)
		}
		ps4008InvalidateDerivedDeps(pass, value.Assign, current.deps)
		ps1006InvalidateStrideDeps(pass, value.Assign, current.strideDeps)
		return ps1006DependencyExitStatesForCaseClauses(pass, value.Body, inner, current, false)
	case *ast.SelectStmt:
		return ps1006DependencyExitStatesForCommClauses(pass, value.Body, inner, current)
	case *ast.ReturnStmt:
		return nil
	case *ast.BranchStmt:
		if value.Tok == token.BREAK && value.Label == nil && breakExits {
			return []ps1006DependencyState{current}
		}
		return nil
	}
	return []ps1006DependencyState{current}
}

const ps1006DependencyLoopIterations = 8

func ps1006DependencyExitStatesForLoop(pass *analysis.Pass, init ast.Stmt, condition ast.Expr, post ast.Stmt, body *ast.BlockStmt, inner types.Object, state ps1006DependencyState) []ps1006DependencyState {
	entry := ps1006ApplyDependencyStatement(pass, init, inner, state)
	head := entry.clone()
	for range ps1006DependencyLoopIterations {
		whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, condition, inner, head)
		iterations := ps1006DependencyExitStatesForBlock(pass, body, inner, whenTrue, true)
		next := make([]ps1006DependencyState, 0, len(iterations)+1)
		next = append(next, entry.clone()) // the loop may execute zero times
		for _, iteration := range iterations {
			next = append(next, ps1006ApplyDependencyStatement(pass, post, inner, iteration))
		}
		merged := ps1006UnionDependencyState(next)
		if ps1006DependencyStateKey(merged) == ps1006DependencyStateKey(head) {
			return ps1006DedupeDependencyStates(append(whenFalse, iterations...))
		}
		head = merged
	}
	whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, condition, inner, head)
	iterations := ps1006DependencyExitStatesForBlock(pass, body, inner, whenTrue, true)
	return ps1006DedupeDependencyStates(append(whenFalse, iterations...))
}

func ps1006DependencyStateKey(state ps1006DependencyState) string {
	return ps4008BoolObjectSetKey(state.deps) + ";" + ps1006StrideDepsKey(state.strideDeps) + ";" + ps1006OrderedPointerAliasesKey(state.pointerAliases)
}

func ps1006DependencyExitStatesForCaseClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, state ps1006DependencyState, constantBools bool) []ps1006DependencyState {
	if body == nil {
		return []ps1006DependencyState{state.clone()}
	}
	clauses := ps1006CaseClauses(body)
	selectedStates, remainingStates := ps1006SelectedDependencyStates(pass, clauses, inner, state, constantBools)
	var states []ps1006DependencyState
	var fallthroughStates []ps1006DependencyState
	for clauseIndex, clause := range clauses {
		inputs := slices.Clone(selectedStates[clauseIndex])
		inputs = append(inputs, fallthroughStates...)
		bodyStatements, fallsThrough := ps4008CaseBodyWithoutTerminalFallthrough(clause.Body)
		flows := ps1006DependencyFlowsForCaseBody(pass, bodyStatements, inner, inputs)
		fallthroughStates = nil
		if fallsThrough {
			for _, flow := range flows {
				if flow.exited {
					states = append(states, flow.state)
					continue
				}
				fallthroughStates = append(fallthroughStates, flow.state)
			}
			continue
		}
		for _, flow := range flows {
			states = append(states, flow.state)
		}
	}
	states = append(states, remainingStates...)
	states = append(states, fallthroughStates...)
	return states
}

type ps1006DependencyFlow struct {
	state  ps1006DependencyState
	exited bool
}

func ps1006DependencyFlowsForCaseBody(pass *analysis.Pass, statements []ast.Stmt, inner types.Object, states []ps1006DependencyState) []ps1006DependencyFlow {
	flows := make([]ps1006DependencyFlow, 0, len(states))
	for _, state := range states {
		flows = append(flows, ps1006DependencyFlow{state: state.clone()})
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
		var next []ps1006DependencyFlow
		for _, flow := range flows {
			if flow.exited {
				next = append(next, flow)
				continue
			}
			next = append(next, ps1006DependencyFlowsForCaseStatement(pass, statement, inner, flow.state)...)
		}
		flows = next
		if len(flows) == 0 {
			return nil
		}
	}
	return flows
}

func ps1006DependencyFlowsForCaseStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, state ps1006DependencyState) []ps1006DependencyFlow {
	current := state.clone()
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, value, inner, current.deps)
		ps1006UpdateDerivedStrideDeps(pass, value, inner, current.strideDeps)
		return []ps1006DependencyFlow{{state: current}}
	case *ast.ExprStmt:
		ps1006ApplyExpressionDependencyStatement(pass, value, inner, current.deps, current.strideDeps, current.pointerAliases)
		return []ps1006DependencyFlow{{state: current}}
	case *ast.IfStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, current.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, current.strideDeps)
		}
		whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, value.Cond, inner, current)
		thenFlows := ps1006DependencyFlowsForCaseBody(pass, value.Body.List, inner, whenTrue)
		var elseFlows []ps1006DependencyFlow
		switch elseNode := value.Else.(type) {
		case *ast.BlockStmt:
			elseFlows = ps1006DependencyFlowsForCaseBody(pass, elseNode.List, inner, whenFalse)
		case *ast.IfStmt:
			for _, branch := range whenFalse {
				elseFlows = append(elseFlows, ps1006DependencyFlowsForCaseStatement(pass, elseNode, inner, branch)...)
			}
		default:
			for _, branch := range whenFalse {
				elseFlows = append(elseFlows, ps1006DependencyFlow{state: branch})
			}
		}
		return append(thenFlows, elseFlows...)
	case *ast.BlockStmt:
		return ps1006DependencyFlowsForCaseBody(pass, value.List, inner, []ps1006DependencyState{current})
	case *ast.LabeledStmt:
		return ps1006DependencyFlowsForCaseStatement(pass, value.Stmt, inner, current)
	case *ast.BranchStmt:
		if value.Tok == token.BREAK && value.Label == nil {
			return []ps1006DependencyFlow{{state: current, exited: true}}
		}
		return nil
	case *ast.ReturnStmt:
		return nil
	}
	exits := ps1006DependencyExitStatesForStatement(pass, statement, inner, current, false)
	flows := make([]ps1006DependencyFlow, 0, len(exits))
	for _, exit := range exits {
		flows = append(flows, ps1006DependencyFlow{state: exit})
	}
	return flows
}

func ps1006DependencyExitStatesForCommClauses(pass *analysis.Pass, body *ast.BlockStmt, inner types.Object, state ps1006DependencyState) []ps1006DependencyState {
	if body == nil {
		return []ps1006DependencyState{state.clone()}
	}
	var states []ps1006DependencyState
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		clauseState := ps1006ApplyDependencyStatement(pass, clause.Comm, inner, state)
		states = append(states, ps1006DependencyExitStatesForBlock(pass, &ast.BlockStmt{List: clause.Body}, inner, []ps1006DependencyState{clauseState}, true)...)
	}
	if len(states) == 0 {
		return []ps1006DependencyState{state.clone()}
	}
	return states
}

func ps1006UnionDependencyState(states []ps1006DependencyState) ps1006DependencyState {
	union := ps1006DependencyState{deps: make(map[types.Object]bool), strideDeps: make(map[types.Object]string)}
	for _, state := range states {
		for object, value := range state.deps {
			if value {
				union.deps[object] = true
			}
		}
		for object, key := range state.strideDeps {
			if key == "" {
				continue
			}
			if existing, ok := union.strideDeps[object]; ok && existing != key {
				union.strideDeps[object] = ps1006MayStrideKey
				continue
			}
			union.strideDeps[object] = key
		}
	}
	aliasObjects := make(map[types.Object]bool)
	trackAliases := false
	for _, state := range states {
		trackAliases = trackAliases || state.pointerAliases != nil
		for object := range state.pointerAliases {
			aliasObjects[object] = true
		}
	}
	if trackAliases {
		union.pointerAliases = make(map[types.Object]ps1006OrderedPointerAlias, len(aliasObjects))
		for object := range aliasObjects {
			merged := ps1006OrderedPointerAlias{known: true}
			seenTargets := make(map[types.Object]bool)
			for _, state := range states {
				alias, present := state.pointerAliases[object]
				if !present || !alias.known {
					merged.known = false
					merged.targets = nil
					break
				}
				for _, target := range alias.targets {
					if target != nil && !seenTargets[target] {
						seenTargets[target] = true
						merged.targets = append(merged.targets, target)
					}
				}
			}
			union.pointerAliases[object] = merged
		}
	}
	return union
}

func (state ps1006DependencyState) clone() ps1006DependencyState {
	return ps1006DependencyState{
		deps:           cloneObjectBoolMap(state.deps),
		strideDeps:     cloneObjectStringMap(state.strideDeps),
		pointerAliases: clonePS1006OrderedPointerAliases(state.pointerAliases),
	}
}

func clonePS1006OrderedPointerAlias(alias ps1006OrderedPointerAlias) ps1006OrderedPointerAlias {
	return ps1006OrderedPointerAlias{targets: slices.Clone(alias.targets), known: alias.known}
}

func clonePS1006OrderedPointerAliases(input map[types.Object]ps1006OrderedPointerAlias) map[types.Object]ps1006OrderedPointerAlias {
	if input == nil {
		return nil
	}
	output := make(map[types.Object]ps1006OrderedPointerAlias, len(input))
	for object, alias := range input {
		output[object] = clonePS1006OrderedPointerAlias(alias)
	}
	return output
}

func ps1006ReplaceObjectBoolMap(dst, src map[types.Object]bool) {
	clear(dst)
	for object, value := range src {
		if value {
			dst[object] = true
		}
	}
}

func ps1006ReplaceStrideDeps(dst, src map[types.Object]string) {
	clear(dst)
	for object, value := range src {
		dst[object] = value
	}
}

func ps1006MergeGotoDeps(dst, src map[string][]ps1006DependencyState) {
	for label, states := range src {
		for _, state := range states {
			dst[label] = append(dst[label], state.clone())
		}
	}
}

func ps1006CollectGotoDepsForStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string) map[string][]ps1006DependencyState {
	gotos, _ := ps1006CollectGotoDepsStatement(pass, statement, inner, ps1006DependencyState{deps: cloneObjectBoolMap(deps), strideDeps: cloneObjectStringMap(strideDeps)})
	return gotos
}

func ps1006CollectGotoDepsBlock(pass *analysis.Pass, statements []ast.Stmt, inner types.Object, states []ps1006DependencyState) (map[string][]ps1006DependencyState, []ps1006DependencyState) {
	collected := make(map[string][]ps1006DependencyState)
	for _, statement := range statements {
		var next []ps1006DependencyState
		for _, state := range states {
			gotos, exits := ps1006CollectGotoDepsStatement(pass, statement, inner, state)
			ps1006MergeGotoDeps(collected, gotos)
			next = append(next, exits...)
		}
		states = next
		if len(states) == 0 {
			break
		}
	}
	return collected, states
}

func ps1006CollectGotoDepsStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, state ps1006DependencyState) (map[string][]ps1006DependencyState, []ps1006DependencyState) {
	collected := make(map[string][]ps1006DependencyState)
	current := state.clone()
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, value, inner, current.deps)
		ps1006UpdateDerivedStrideDeps(pass, value, inner, current.strideDeps)
		return collected, []ps1006DependencyState{current}
	case *ast.ExprStmt:
		ps1006ApplyExpressionDependencyStatement(pass, value, inner, current.deps, current.strideDeps, current.pointerAliases)
		return collected, []ps1006DependencyState{current}
	case *ast.IfStmt:
		if value.Init != nil {
			ps4008UpdateDerivedDeps(pass, value.Init, inner, current.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, current.strideDeps)
		}
		whenTrue, whenFalse := ps1006ConditionDependencyStates(pass, value.Cond, inner, current)
		thenGotos, thenExits := ps1006CollectGotoDepsBlock(pass, value.Body.List, inner, whenTrue)
		ps1006MergeGotoDeps(collected, thenGotos)
		var elseExits []ps1006DependencyState
		switch elseNode := value.Else.(type) {
		case *ast.BlockStmt:
			elseGotos, exits := ps1006CollectGotoDepsBlock(pass, elseNode.List, inner, whenFalse)
			ps1006MergeGotoDeps(collected, elseGotos)
			elseExits = exits
		case *ast.IfStmt:
			for _, branch := range whenFalse {
				elseGotos, exits := ps1006CollectGotoDepsStatement(pass, elseNode, inner, branch)
				ps1006MergeGotoDeps(collected, elseGotos)
				elseExits = append(elseExits, exits...)
			}
		default:
			elseExits = whenFalse
		}
		return collected, ps1006DedupeDependencyStates(append(thenExits, elseExits...))
	case *ast.BlockStmt:
		return ps1006CollectGotoDepsBlock(pass, value.List, inner, []ps1006DependencyState{current})
	case *ast.LabeledStmt:
		return ps1006CollectGotoDepsStatement(pass, value.Stmt, inner, current)
	case *ast.BranchStmt:
		if value.Tok == token.GOTO && value.Label != nil {
			collected[value.Label.Name] = append(collected[value.Label.Name], current)
		}
		return collected, nil
	case *ast.ReturnStmt:
		return collected, nil
	}
	return collected, []ps1006DependencyState{current}
}

func ps1006FindStridedReductionInStatements(pass *analysis.Pass, statements []ast.Stmt, inner, outer types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string, pointerAliases map[types.Object]ps1006OrderedPointerAlias) *ast.IndexExpr {
	return ps1006FindStridedReductionInBlock(pass, &ast.BlockStmt{List: statements}, inner, outer, deps, strideDeps, pointerAliases)
}

func ps1006FindStridedReductionInStatement(pass *analysis.Pass, statement ast.Stmt, inner, outer types.Object, deps map[types.Object]bool, strideDeps map[types.Object]string, pointerAliases map[types.Object]ps1006OrderedPointerAlias) *ast.IndexExpr {
	if statement == nil {
		return nil
	}
	return ps1006FindStridedReductionInStatements(pass, []ast.Stmt{statement}, inner, outer, deps, strideDeps, pointerAliases)
}

func ps1006FindStridedReductionInCaseClauses(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object, state ps1006DependencyState, constantBools bool) *ast.IndexExpr {
	if body == nil {
		return nil
	}
	clauses := ps1006CaseClauses(body)
	selectedStates, _ := ps1006SelectedDependencyStates(pass, clauses, inner, state, constantBools)
	var fallthroughStates []ps1006DependencyState
	for clauseIndex, clause := range clauses {
		inputs := slices.Clone(selectedStates[clauseIndex])
		inputs = append(inputs, fallthroughStates...)
		statements, fallsThrough := ps4008CaseBodyWithoutTerminalFallthrough(clause.Body)
		for _, input := range inputs {
			if hit := ps1006FindStridedReductionInStatements(pass, statements, inner, outer, cloneObjectBoolMap(input.deps), cloneObjectStringMap(input.strideDeps), clonePS1006OrderedPointerAliases(input.pointerAliases)); hit != nil {
				return hit
			}
		}
		fallthroughStates = nil
		if !fallsThrough {
			continue
		}
		for _, flow := range ps1006DependencyFlowsForCaseBody(pass, statements, inner, inputs) {
			if !flow.exited {
				fallthroughStates = append(fallthroughStates, flow.state)
			}
		}
	}
	return nil
}

// stridedByInner matches iv*s + <term with ov> / <term> + iv*s: the inner
// variable inside a product, an additive sibling mentioning the outer
// variable but not the inner one.
func stridedByInner(e ast.Expr, iv, ov string) bool {
	be, ok := e.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD {
		return false
	}
	isMulWithIv := func(x ast.Expr) bool {
		m, ok := x.(*ast.BinaryExpr)
		return ok && m.Op == token.MUL && exprMentions(m, iv)
	}
	additiveOK := func(x ast.Expr) bool {
		return exprMentions(x, ov) && !exprMentions(x, iv)
	}
	return (isMulWithIv(be.X) && additiveOK(be.Y)) || (isMulWithIv(be.Y) && additiveOK(be.X))
}

func ps1006UpdateDerivedStrideDeps(pass *analysis.Pass, statement ast.Stmt, inner types.Object, deps map[types.Object]string) {
	ps1006InvalidateStrideDeps(pass, statement, deps)
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
		prev := cloneObjectStringMap(deps)
		updates := make(map[types.Object]string, len(value.Lhs))
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
			if key, ok := ps1006StrideDependencyKey(pass, value.Rhs[index], inner, prev); ok {
				updates[object] = key
			}
		}
		for _, lhs := range value.Lhs {
			identifier, ok := lhs.(*ast.Ident)
			if !ok || identifier.Name == "_" {
				continue
			}
			object := identObject(pass, identifier)
			if object == nil {
				continue
			}
			if key := updates[object]; key != "" {
				deps[object] = key
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
			prev := cloneObjectStringMap(deps)
			updates := make(map[types.Object]string, len(specification.Names))
			for index, name := range specification.Names {
				object := identObject(pass, name)
				if object == nil {
					continue
				}
				if !ps4008CanTrackDerivedLocal(pass, name) {
					delete(deps, object)
					continue
				}
				if index < len(specification.Values) {
					if key, ok := ps1006StrideDependencyKey(pass, specification.Values[index], inner, prev); ok {
						updates[object] = key
					}
				}
			}
			for _, name := range specification.Names {
				object := identObject(pass, name)
				if object == nil {
					continue
				}
				if key := updates[object]; key != "" {
					deps[object] = key
					continue
				}
				delete(deps, object)
			}
		}
	}
}

func ps1006InvalidateStrideDeps(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]string) {
	ps1006InvalidateStrideDepsAt(pass, statement, deps, ps4008StatementPosition(statement))
}

func ps1006InvalidateStrideDepsAt(pass *analysis.Pass, statement ast.Stmt, deps map[types.Object]string, before token.Pos) {
	if len(deps) == 0 || statement == nil {
		return
	}
	clearAll := false
	kill := make(map[types.Object]bool)
	ast.Inspect(statement, func(node ast.Node) bool {
		if clearAll {
			return false
		}
		switch value := node.(type) {
		case *ast.FuncLit:
			// Defining a closure does not execute its body. A later call is
			// resolved by ps1006KillCallableWriteTargets.
			return false
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				switch target := ps2110Unparen(lhs).(type) {
				case *ast.StarExpr:
					if ps1006KillAggregateAliasTargets(pass, target.X, deps, kill, before) {
						continue
					}
					if ps1006KillPointerAliasTargets(pass, target.X, deps, kill, before) {
						continue
					}
					if ps1006ExprMentionsStrideDep(pass, target.X, deps) {
						clearAll = true
						return false
					}
				case *ast.IndexExpr:
					if ps1006ExprMentionsStrideDep(pass, target.X, deps) {
						clearAll = true
						return false
					}
				case *ast.SelectorExpr:
					if ps1006ExprMentionsStrideDep(pass, target.X, deps) {
						clearAll = true
						return false
					}
				}
			}
		case *ast.CallExpr:
			if ps1006KillCallableWriteTargetsMode(pass, value.Fun, deps, kill, before, ps4008DefiniteWrite) {
				// Known callable write provenance was handled precisely.
			} else if !ps4008CallIsConversionOrBuiltin(pass, value) && ps4008CallableMayCapture(pass, value.Fun) {
				// An unresolved callable may close over any live stride input. It
				// is unsound to keep an apparent tile merely because the call has
				// no explicit arguments mentioning those inputs.
				clearAll = true
				return false
			} else if ps1006ExprMentionsStrideDep(pass, value.Fun, deps) {
				clearAll = true
				return false
			}
			for argumentIndex, arg := range value.Args {
				if unary, ok := ps2110Unparen(arg).(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if ps4008CallWritesArgument(pass, value, argumentIndex, before, ps4008DefiniteWrite) {
						ps1006CollectMentionedStrideDeps(pass, unary.X, deps, kill)
					}
					continue
				}
				if ps4008TypeMayCarryCallable(pass.TypesInfo.TypeOf(arg), make(map[types.Type]bool)) {
					if ps4008CallInvokesArgument(pass, value, argumentIndex, before, ps4008DefiniteWrite) {
						if ps1006KillCallableWriteTargetsMode(pass, arg, deps, kill, before, ps4008DefiniteWrite) {
							continue
						}
						if ps4008CallableMayCapture(pass, arg) {
							clearAll = true
							return false
						}
					}
				}
				if typ := pass.TypesInfo.TypeOf(arg); typ != nil {
					if _, isInterface := typ.Underlying().(*types.Interface); isInterface && ps1006KillAggregateAliasTargets(pass, arg, deps, kill, before) {
						continue
					}
				}
				if ps4008IsPointerType(pass.TypesInfo.TypeOf(arg)) {
					if ps4008CallWritesArgument(pass, value, argumentIndex, before, ps4008DefiniteWrite) {
						ps1006KillPointerAliasTargets(pass, arg, deps, kill, before)
					}
					continue
				}
				if ps4008CallWritesArgument(pass, value, argumentIndex, before, ps4008DefiniteWrite) && ps1006KillAggregateAliasTargets(pass, arg, deps, kill, before) {
					continue
				}
				if ps1006ArgumentMayExposeStrideStorage(pass, arg, deps) {
					clearAll = true
					return false
				}
			}
			if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok && ps4008MethodValueReceiverWrites(pass, selector, before, ps4008DefiniteWrite) {
				if ps4008IsPointerType(pass.TypesInfo.TypeOf(selector.X)) && ps1006KillPointerAliasTargets(pass, selector.X, deps, kill, before) {
					return true
				}
				if ps1006KillAggregateAliasTargets(pass, selector.X, deps, kill, before) {
					return true
				}
				if ps1006ExprMentionsStrideDep(pass, selector.X, deps) {
					clearAll = true
					return false
				}
			}
		}
		return true
	})
	if clearAll {
		clear(deps)
		return
	}
	for object := range kill {
		delete(deps, object)
	}
}

func ps1006InvalidateStrideDepsInExpr(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]string) {
	if expression == nil {
		return
	}
	ps1006InvalidateStrideDeps(pass, &ast.ExprStmt{X: expression}, deps)
}

func ps1006ApplyDependencyStatement(pass *analysis.Pass, statement ast.Stmt, inner types.Object, state ps1006DependencyState) ps1006DependencyState {
	current := state.clone()
	if statement == nil {
		return current
	}
	switch value := statement.(type) {
	case *ast.AssignStmt, *ast.DeclStmt:
		ps4008UpdateDerivedDeps(pass, statement, inner, current.deps)
		ps1006UpdateDerivedStrideDeps(pass, statement, inner, current.strideDeps)
		ps1006UpdateOrderedPointerAliasesForStatement(pass, statement, current.pointerAliases)
	case *ast.ExprStmt:
		ps1006ApplyExpressionDependencyStatement(pass, value, inner, current.deps, current.strideDeps, current.pointerAliases)
	default:
		ps4008InvalidateDerivedDeps(pass, statement, current.deps)
		ps1006InvalidateStrideDeps(pass, statement, current.strideDeps)
	}
	return current
}

func ps1006KillAggregateAliasTargets(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]string, kill map[types.Object]bool, before token.Pos) bool {
	boolDeps := ps1006StrideDepsAsBool(deps)
	targets := make(map[types.Object]bool)
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		base := identObject(pass, identifier)
		for _, target := range ps4008AggregateAliasTargets(pass, base, boolDeps, before) {
			if deps[target] != "" {
				targets[target] = true
			}
		}
		return true
	})
	for target := range targets {
		kill[target] = true
	}
	return len(targets) > 0
}

func ps1006KillPointerAliasTargets(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]string, kill map[types.Object]bool, before token.Pos) bool {
	boolDeps := ps1006StrideDepsAsBool(deps)
	boolKill := make(map[types.Object]bool)
	if !ps4008KillPointerAliasTargets(pass, expression, boolDeps, boolKill, before, ps4008DefiniteWrite) {
		return false
	}
	for target := range boolKill {
		if deps[target] != "" {
			kill[target] = true
		}
	}
	return true
}

func ps1006KillCallableWriteTargetsMode(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]string, kill map[types.Object]bool, before token.Pos, mode ps4008CallableEffectMode) bool {
	boolDeps := ps1006StrideDepsAsBool(deps)
	boolKill := make(map[types.Object]bool)
	if !ps4008KillCallableWriteTargetsMode(pass, expression, boolDeps, boolKill, before, mode) {
		return false
	}
	for target := range boolKill {
		if deps[target] != "" {
			kill[target] = true
		}
	}
	return true
}

func ps1006StrideDepsAsBool(deps map[types.Object]string) map[types.Object]bool {
	result := make(map[types.Object]bool, len(deps))
	for object, key := range deps {
		if key != "" {
			result[object] = true
		}
	}
	return result
}

func ps1006CollectMentionedStrideDeps(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]string, dst map[types.Object]bool) {
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if deps[object] != "" {
			dst[object] = true
		}
		return true
	})
}

func ps1006ArgumentMayExposeStrideStorage(pass *analysis.Pass, expression ast.Expr, deps map[types.Object]string) bool {
	if expression == nil {
		return false
	}
	unparen := ps2110Unparen(expression)
	if unary, ok := unparen.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		return ps1006ExprMentionsStrideDep(pass, unary.X, deps)
	}
	if !ps1006ExprMentionsStrideDep(pass, expression, deps) {
		return false
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return true
	}
	return ps4008TypeMayExposeMutableStorage(typ)
}

func ps1006StrideDependencyKey(pass *analysis.Pass, expression ast.Expr, inner types.Object, strideDeps map[types.Object]string) (string, bool) {
	if expression == nil || inner == nil {
		return "", false
	}
	if key, ok := ps1006DirectStrideDependencyKey(pass, expression, inner, strideDeps); ok {
		return ps1006StrideKeyWithObjects(pass, expression, inner, strideDeps, key), true
	}
	var found string
	ast.Inspect(expression, func(node ast.Node) bool {
		if found != "" {
			return false
		}
		if key, ok := ps1006DirectStrideDependencyKey(pass, node, inner, strideDeps); ok {
			found = key
			return false
		}
		_, binary := node.(*ast.BinaryExpr)
		return !binary
	})
	if found == "" {
		return "", false
	}
	// Keep the complete term in the identity and stability proof. In
	// particular, neither next(t*stride) nor a narrowing conversion such as
	// int8(t*stride) has the same value identity as the nested multiplication.
	// The former is rejected as impure below; the latter remains eligible when
	// every lane uses the same conversion wrapper.
	key := ps1006CanonicalStrideExpr(pass, expression, inner, strideDeps)
	return ps1006StrideKeyWithObjects(pass, expression, inner, strideDeps, key), true
}

// ps1006StrideKeyWithObjects preserves every mutable object on which a
// canonical (or printer-fallback) stride expression depends. Text alone is
// insufficient for indexed expressions such as strides[axis]: both the slice
// and the selector object must participate in the later stability proof.
// Derived locals are omitted here because their stored key already contains
// the original inputs and their per-tap definition is expected to be a write.
func ps1006StrideKeyWithObjects(pass *analysis.Pass, expression ast.Expr, inner types.Object, strideDeps map[types.Object]string, key string) string {
	if expression == nil || key == "" {
		return key
	}
	if !ps1006StrideCallsValuePure(pass, expression) && !strings.Contains(key, ps1006ImpureCallKey) {
		key += ps1006ImpureCallKey
	}
	objects := make(map[string]bool)
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		object := identObject(pass, identifier)
		if object == nil || object == inner || strideDeps[object] != "" {
			return true
		}
		if _, ok := object.(*types.Var); ok {
			objects[ps1006ObjectKey(pass, object, identifier.Name)] = true
		}
		return true
	})
	if len(objects) == 0 {
		return key
	}
	parts := make([]string, 0, len(objects))
	for objectKey := range objects {
		parts = append(parts, objectKey)
	}
	slices.Sort(parts)
	return key + "$objects{" + strings.Join(parts, ",") + "}"
}

func ps1006StrideCallsValuePure(pass *analysis.Pass, expression ast.Expr) bool {
	pure := true
	ast.Inspect(expression, func(node ast.Node) bool {
		if !pure {
			return false
		}
		if call, ok := node.(*ast.CallExpr); ok && !ps1006ValuePureCall(pass, call) {
			pure = false
			return false
		}
		return true
	})
	return pure
}

func ps1006DirectStrideDependencyKey(pass *analysis.Pass, node ast.Node, inner types.Object, strideDeps map[types.Object]string) (string, bool) {
	switch value := node.(type) {
	case *ast.Ident:
		key, ok := strideDeps[identObject(pass, value)]
		return key, ok
	case *ast.BinaryExpr:
		if value.Op != token.MUL || !ps1006ExprMentionsInnerOrStrideDep(pass, value, inner, strideDeps) {
			return "", false
		}
		return ps1006CanonicalStrideExpr(pass, value, inner, strideDeps), true
	}
	return "", false
}

func ps1006ExprMentionsInnerOrStrideDep(pass *analysis.Pass, expression ast.Expr, inner types.Object, strideDeps map[types.Object]string) bool {
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
		if object == inner || strideDeps[object] != "" {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps1006CanonicalStrideExpr(pass *analysis.Pass, expression ast.Expr, inner types.Object, strideDeps map[types.Object]string) string {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return "(" + ps1006CanonicalStrideExpr(pass, value.X, inner, strideDeps) + ")"
	case *ast.Ident:
		object := identObject(pass, value)
		if object == inner {
			return "$inner"
		}
		if key := strideDeps[object]; key != "" {
			return key
		}
		return ps1006ObjectKey(pass, object, value.Name)
	case *ast.BasicLit:
		return value.Value
	case *ast.BinaryExpr:
		return ps1006CanonicalStrideExpr(pass, value.X, inner, strideDeps) + value.Op.String() + ps1006CanonicalStrideExpr(pass, value.Y, inner, strideDeps)
	case *ast.UnaryExpr:
		return value.Op.String() + ps1006CanonicalStrideExpr(pass, value.X, inner, strideDeps)
	case *ast.CallExpr:
		arguments := make([]string, len(value.Args))
		for index, argument := range value.Args {
			arguments[index] = ps1006CanonicalStrideExpr(pass, argument, inner, strideDeps)
		}
		if fun, ok := pass.TypesInfo.Types[value.Fun]; ok && fun.IsType() && len(value.Args) == 1 && !value.Ellipsis.IsValid() {
			target := pass.TypesInfo.TypeOf(value)
			source := pass.TypesInfo.TypeOf(value.Args[0])
			if target != nil && source != nil && types.Identical(target, source) {
				return arguments[0]
			}
			return "$convert{" + ps1006TypeIdentityKey(target) + "}(" + arguments[0] + ")"
		}
		ellipsis := ""
		if value.Ellipsis.IsValid() {
			ellipsis = "..."
		}
		return "$call{" + ps1006CanonicalStrideExpr(pass, value.Fun, inner, strideDeps) + "}(" + strings.Join(arguments, ",") + ellipsis + ")"
	case *ast.SelectorExpr:
		if _, sourceID, ok := ps1006SourceIdentity(pass, value); ok {
			return sourceID
		}
		return ps1006CanonicalStrideExpr(pass, value.X, inner, strideDeps) + "." + ps1006ObjectKey(pass, identObject(pass, value.Sel), value.Sel.Name)
	}
	return ps1006ExprText(pass, expression)
}

func ps1006TypeIdentityKey(typ types.Type) string {
	if typ == nil {
		return "$unknown-type"
	}
	typ = types.Unalias(typ)
	key := types.TypeString(typ, func(pkg *types.Package) string { return pkg.Path() })
	if named, ok := typ.(*types.Named); ok && named.Obj() != nil {
		key += "@" + strconv.Itoa(int(named.Obj().Pos()))
	}
	return key
}

func ps1006ObjectKey(pass *analysis.Pass, object types.Object, fallback string) string {
	if object == nil {
		return fallback
	}
	pkg := ""
	if object.Pkg() != nil {
		pkg = object.Pkg().Path()
	} else if pass.Pkg != nil {
		pkg = pass.Pkg.Path()
	}
	return pkg + "." + object.Name() + "@" + strconv.Itoa(int(object.Pos()))
}

func ps1006ExprMentionsStrideDep(pass *analysis.Pass, expression ast.Expr, strideDeps map[types.Object]string) bool {
	if expression == nil {
		return false
	}
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if ok && strideDeps[identObject(pass, identifier)] != "" {
			found = true
			return false
		}
		return true
	})
	return found
}

func ps1006AdditiveTerms(expression ast.Expr) []ast.Expr {
	binary, ok := expression.(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD {
		return []ast.Expr{expression}
	}
	terms := ps1006AdditiveTerms(binary.X)
	return append(terms, ps1006AdditiveTerms(binary.Y)...)
}

func ps1006ResolvedRegisterTile(index *ps1006AnalysisIndex, pass *analysis.Pass, stack []ast.Node, outerNode, innerNode ast.Node) bool {
	if ps1006OuterRegisterTileForInner(index, pass, outerNode, innerNode) {
		return true
	}
	parent := ps1006ParentBlock(stack, outerNode)
	if parent == nil {
		return false
	}
	for statementIndex, statement := range parent.List {
		if statement != outerNode {
			continue
		}
		if statementIndex == 0 {
			return false
		}
		tail, ok := outerNode.(*ast.ForStmt)
		if !ok {
			return false
		}
		tailBound, ok := ps1006ScalarTailBound(pass, tail)
		if !ok {
			return false
		}
		tailOuter := ps4008LoopObject(pass, outerNode)
		tailInner := ps4008LoopObject(pass, innerNode)
		tailBody := astutil.LoopBody(innerNode)
		if tailOuter == nil || tailInner == nil || !ps1006ObjectStableInNode(index, pass, tail.Body, tailBound) || !ps1006ObjectStableInNode(index, pass, tailBody, tailOuter) {
			return false
		}
		tile, ok := unwrapLabeled(parent.List[statementIndex-1]).(*ast.ForStmt)
		if !ok {
			return false
		}
		tileBound, ok := ps1006TileLoopBound(pass, tile)
		if !ok || tileBound != tailBound {
			return false
		}
		tileKeys := ps1006OuterRegisterTileKeys(index, pass, tile, tail)
		if len(tileKeys) == 0 {
			return false
		}
		tailKeys := ps1006InnerStridedAccumulatorKeys(pass, tailBody, tailInner, tailOuter)
		if len(tailKeys) != 1 || ps1006InnerStridedAccumulatorCount(pass, astutil.LoopBody(innerNode), ps4008LoopObject(pass, innerNode), ps4008LoopObject(pass, outerNode)) != 1 {
			return false
		}
		if !ps1006RegisterTileInputsStable(index, pass, tailBody, tailKeys) {
			return false
		}
		for key := range tailKeys {
			return tileKeys[key]
		}
		return false
	}
	return false
}

func ps1006ParentBlock(stack []ast.Node, child ast.Node) *ast.BlockStmt {
	for index := len(stack) - 1; index >= 0; index-- {
		block, ok := stack[index].(*ast.BlockStmt)
		if !ok {
			continue
		}
		for _, statement := range block.List {
			if statement == child {
				return block
			}
		}
	}
	return nil
}

func ps1006OuterRegisterTileForInner(index *ps1006AnalysisIndex, pass *analysis.Pass, outerNode, innerNode ast.Node) bool {
	outer, ok := outerNode.(*ast.ForStmt)
	if !ok {
		return false
	}
	tileBound, ok := ps1006TileLoopBound(pass, outer)
	if !ok {
		return false
	}
	outerObject := ps4008LoopObject(pass, outer)
	if outerObject == nil || outer.Body == nil {
		return false
	}
	innerObject := ps4008LoopObject(pass, innerNode)
	if innerObject == nil {
		return false
	}
	if !ps1006ObjectStableInNode(index, pass, outer.Body, tileBound) || !ps1006ObjectStableInNode(index, pass, astutil.LoopBody(innerNode), outerObject) {
		return false
	}
	return ps1006InnerRegisterTile(index, pass, astutil.LoopBody(innerNode), innerObject, outerObject, tileBound, ps1006LoopStep(pass, outer))
}

func ps1006OuterRegisterTileKeys(index *ps1006AnalysisIndex, pass *analysis.Pass, outerNode ast.Node, tail *ast.ForStmt) map[ps1006TileKey]bool {
	outer, ok := outerNode.(*ast.ForStmt)
	if !ok {
		return nil
	}
	tileBound, ok := ps1006TileLoopBound(pass, outer)
	if !ok {
		return nil
	}
	outerObject := ps4008LoopObject(pass, outer)
	if outerObject == nil || outer.Body == nil {
		return nil
	}
	if !ps1006ObjectStableInNode(index, pass, outer.Body, tileBound) {
		return nil
	}
	keys := make(map[ps1006TileKey]bool)
	for _, statement := range outer.Body.List {
		if !astutil.IsLoop(statement) {
			continue
		}
		innerObject := ps4008LoopObject(pass, statement)
		if innerObject == nil {
			continue
		}
		if !ps1006ObjectStableInNode(index, pass, astutil.LoopBody(statement), outerObject) {
			continue
		}
		innerKeys := ps1006InnerRegisterTileKeys(index, pass, astutil.LoopBody(statement), innerObject, outerObject, tileBound, ps1006LoopStep(pass, outer))
		if len(innerKeys) == 0 || !ps1006OuterIterationCompletesInnerTile(index, pass, outer, statement, tail) {
			continue
		}
		for key := range innerKeys {
			keys[key] = true
		}
	}
	return keys
}

// ps1006OuterIterationCompletesInnerTile proves that each entered outer
// iteration which can still reach the scalar tail completes the selected
// register-tile loop first. A continue after the tile is equivalent to the
// ordinary backedge and is safe; a continue before it is not. Any break (or
// goto) that leaves the outer loop for the tail is unsafe because it can skip
// later four-lane iterations. Return and no-return paths cannot reach the tail
// and therefore do not invalidate the proof. Feasible CFG edges fold constant
// if and switch conditions, avoiding dead-branch false negatives.
func ps1006OuterIterationCompletesInnerTile(index *ps1006AnalysisIndex, pass *analysis.Pass, outer *ast.ForStmt, inner ast.Node, tail *ast.ForStmt) bool {
	if index == nil || pass == nil || outer == nil || outer.Body == nil || inner == nil || tail == nil {
		return false
	}
	completionKey := ps1006TileCompletionKey{outer: outer, inner: inner, tail: tail}
	if result, ok := index.tileCompletions[completionKey]; ok {
		return result
	}
	result := false
	defer func() { index.tileCompletions[completionKey] = result }()
	facts := index.functionByNode[outer]
	if facts == nil || facts != index.functionByNode[inner] || facts != index.functionByNode[tail] {
		return false
	}
	body := ps4008FunctionBody(facts.root)
	if body == nil {
		return false
	}
	control := index.controlFlows[body]
	if control == nil {
		control = &ps1006ControlFlow{
			graph:   cfg.New(body, ps6080CallMayReturn(pass)),
			parents: ps6071Parents(body),
			blocks:  make(map[ps1006ControlFlowBlockKey]*cfg.Block),
		}
		for _, block := range control.graph.Blocks {
			if block.Stmt != nil {
				control.blocks[ps1006ControlFlowBlockKey{node: block.Stmt, kind: block.Kind}] = block
			}
		}
		index.controlFlows[body] = control
		index.controlFlowBuilds++
	}
	outerBody := control.blocks[ps1006ControlFlowBlockKey{node: outer, kind: cfg.KindForBody}]
	outerLoop := control.blocks[ps1006ControlFlowBlockKey{node: outer, kind: cfg.KindForLoop}]
	outerPost := control.blocks[ps1006ControlFlowBlockKey{node: outer, kind: cfg.KindForPost}]
	outerDone := control.blocks[ps1006ControlFlowBlockKey{node: outer, kind: cfg.KindForDone}]
	innerDone := control.blocks[ps1006ControlFlowBlockKey{node: inner, kind: cfg.KindForDone}]
	if innerDone == nil {
		innerDone = control.blocks[ps1006ControlFlowBlockKey{node: inner, kind: cfg.KindRangeDone}]
	}
	if outerBody == nil || outerLoop == nil || outerPost == nil || outerDone == nil || innerDone == nil {
		return false
	}
	type flowState struct {
		block     *cfg.Block
		completed bool
	}
	queue := []flowState{{block: outerBody}}
	seen := make(map[flowState]bool)
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		if state.block == nil || seen[state] {
			continue
		}
		seen[state] = true
		index.controlFlowVisits++
		if state.block == innerDone {
			state.completed = true
		}
		if state.block == outerPost || state.block == outerLoop {
			if !state.completed {
				return false
			}
			continue
		}
		if state.block == outerDone || state.block.Stmt == tail {
			return false
		}
		if state.block.Return() != nil {
			continue
		}
		for _, successor := range ps6080FeasibleSuccessors(pass, control.parents, state.block) {
			if successor != nil && successor.Live {
				queue = append(queue, flowState{block: successor, completed: state.completed})
			}
		}
	}
	result = true
	return result
}

func ps1006TileLoopBound(pass *analysis.Pass, loop *ast.ForStmt) (types.Object, bool) {
	if loop == nil || ps1006LoopStep(pass, loop) != 4 || !ps1006LoopStartsAtZero(loop) {
		return nil, false
	}
	outer := ps4008LoopObject(pass, loop)
	condition, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS {
		return nil, false
	}
	if offset, ok := ps1006OuterPlusConst(pass, condition.X, outer); !ok || offset != 3 {
		return nil, false
	}
	bound := ps1006BoundObject(pass, condition.Y)
	return bound, bound != nil
}

func ps1006ScalarTailBound(pass *analysis.Pass, loop *ast.ForStmt) (types.Object, bool) {
	if loop == nil || ps1006LoopStep(pass, loop) != 1 {
		return nil, false
	}
	outer := ps4008LoopObject(pass, loop)
	condition, ok := loop.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.LSS {
		return nil, false
	}
	if !ps1006ExprIsObject(pass, condition.X, outer) {
		return nil, false
	}
	bound := ps1006BoundObject(pass, condition.Y)
	if bound == nil || !ps1006LoopStartsAtTail(pass, loop, bound) {
		return nil, false
	}
	return bound, true
}

func ps1006LoopStartsAtZero(loop *ast.ForStmt) bool {
	assign, ok := loop.Init.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	literal, ok := assign.Rhs[0].(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == "0"
}

func ps1006LoopStartsAtTail(pass *analysis.Pass, loop *ast.ForStmt, bound types.Object) bool {
	assign, ok := loop.Init.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE && assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	sub, ok := assign.Rhs[0].(*ast.BinaryExpr)
	if !ok || sub.Op != token.SUB || !ps1006ExprIsObject(pass, sub.X, bound) {
		return false
	}
	rem, ok := sub.Y.(*ast.BinaryExpr)
	if !ok || rem.Op != token.REM || !ps1006ExprIsObject(pass, rem.X, bound) {
		return false
	}
	literal, ok := rem.Y.(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == "4"
}

func ps1006BoundObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	for {
		paren, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = paren.X
	}
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return nil
	}
	return identObject(pass, identifier)
}

func ps1006ExprIsObject(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	if object == nil {
		return false
	}
	for {
		paren, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = paren.X
	}
	identifier, ok := expression.(*ast.Ident)
	return ok && identObject(pass, identifier) == object
}

func ps1006OuterPlusConst(pass *analysis.Pass, expression ast.Expr, outer types.Object) (int64, bool) {
	if outer == nil {
		return 0, false
	}
	var offset int64
	seenOuter := false
	for _, term := range ps1006AdditiveTerms(expression) {
		if ps1006ExprIsObject(pass, term, outer) {
			if seenOuter {
				return 0, false
			}
			seenOuter = true
			continue
		}
		literal, ok := term.(*ast.BasicLit)
		if !ok || literal.Kind != token.INT {
			return 0, false
		}
		value, err := strconv.ParseInt(literal.Value, 0, 64)
		if err != nil {
			return 0, false
		}
		offset += value
	}
	return offset, seenOuter
}

func ps1006LoopStep(pass *analysis.Pass, loop *ast.ForStmt) int64 {
	if loop == nil {
		return 0
	}
	object := ps4008LoopObject(pass, loop)
	if object == nil {
		return 0
	}
	switch post := loop.Post.(type) {
	case *ast.IncDecStmt:
		if post.Tok == token.INC {
			if identifier, ok := post.X.(*ast.Ident); ok && identObject(pass, identifier) == object {
				return 1
			}
		}
	case *ast.AssignStmt:
		if post.Tok != token.ADD_ASSIGN || len(post.Lhs) != 1 || len(post.Rhs) != 1 {
			return 0
		}
		identifier, ok := post.Lhs[0].(*ast.Ident)
		if !ok || identObject(pass, identifier) != object {
			return 0
		}
		literal, ok := post.Rhs[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.INT {
			return 0
		}
		step, err := strconv.ParseInt(literal.Value, 0, 64)
		if err != nil {
			return 0
		}
		return step
	}
	return 0
}

func ps1006InnerStridedAccumulatorCount(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object) int {
	if body == nil || inner == nil || outer == nil {
		return 0
	}
	states := ps1006ScanAccumulatorBlock(pass, body, inner, outer, []ps1006AccumulatorPath{{
		deps:           make(map[types.Object]bool, len(body.List)),
		strideDeps:     make(map[types.Object]string, len(body.List)),
		pointerAliases: make(map[types.Object]ps1006OrderedPointerAlias),
		seen:           make(map[types.Object]bool, len(body.List)),
		keys:           make(map[ps1006TileKey]bool),
		tiles:          make(map[ps1006TileKey]ps1006TileSlots),
		tileSeen:       make(map[ps1006TileKey]map[types.Object]bool),
		graph:          psNewAccumulatorGraph(pass, body),
	}})
	maxCount := 0
	for _, state := range states {
		if count := state.graph.independentCount(state.seen); count > maxCount {
			maxCount = count
		}
	}
	return maxCount
}

func ps1006InnerStridedAccumulatorKeys(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object) map[ps1006TileKey]bool {
	if body == nil || inner == nil || outer == nil {
		return nil
	}
	states := ps1006ScanAccumulatorBlock(pass, body, inner, outer, []ps1006AccumulatorPath{{
		deps:           make(map[types.Object]bool, len(body.List)),
		strideDeps:     make(map[types.Object]string, len(body.List)),
		pointerAliases: make(map[types.Object]ps1006OrderedPointerAlias),
		seen:           make(map[types.Object]bool, len(body.List)),
		keys:           make(map[ps1006TileKey]bool),
		tiles:          make(map[ps1006TileKey]ps1006TileSlots),
		tileSeen:       make(map[ps1006TileKey]map[types.Object]bool),
		graph:          psNewAccumulatorGraph(pass, body),
	}})
	keys := make(map[ps1006TileKey]bool)
	for _, state := range states {
		if state.unsafe {
			return nil
		}
		for key := range state.keys {
			keys[key] = true
		}
	}
	return keys
}

func ps1006InnerRegisterTile(index *ps1006AnalysisIndex, pass *analysis.Pass, body *ast.BlockStmt, inner, outer, tileBound types.Object, tileWidth int64) bool {
	return len(ps1006InnerRegisterTileKeys(index, pass, body, inner, outer, tileBound, tileWidth)) > 0
}

func ps1006InnerRegisterTileKeys(index *ps1006AnalysisIndex, pass *analysis.Pass, body *ast.BlockStmt, inner, outer, tileBound types.Object, tileWidth int64) map[ps1006TileKey]bool {
	if body == nil || inner == nil || outer == nil {
		return nil
	}
	states := ps1006ScanAccumulatorBlock(pass, body, inner, outer, []ps1006AccumulatorPath{{
		deps:           make(map[types.Object]bool, len(body.List)),
		strideDeps:     make(map[types.Object]string, len(body.List)),
		pointerAliases: make(map[types.Object]ps1006OrderedPointerAlias),
		seen:           make(map[types.Object]bool, len(body.List)),
		keys:           make(map[ps1006TileKey]bool),
		tiles:          make(map[ps1006TileKey]ps1006TileSlots),
		tileSeen:       make(map[ps1006TileKey]map[types.Object]bool),
		graph:          psNewAccumulatorGraph(pass, body),
		tileBound:      tileBound,
		tileWidth:      tileWidth,
	}})
	common := ps1006CommonTiles(states)
	if !ps1006RegisterTileInputsStable(index, pass, body, common) {
		return nil
	}
	return common
}

func ps1006CommonTiles(states []ps1006AccumulatorPath) map[ps1006TileKey]bool {
	if len(states) == 0 {
		return nil
	}
	for _, state := range states {
		if state.unsafe {
			return nil
		}
		for key := range state.keys {
			if !ps1006CompleteTileForKey(state, key) {
				return nil
			}
		}
	}
	common := make(map[ps1006TileKey]bool, len(states[0].tiles))
	for key := range states[0].tiles {
		if ps1006CompleteTileForKey(states[0], key) {
			common[key] = true
		}
	}
	for _, state := range states[1:] {
		for key := range common {
			if !ps1006CompleteTileForKey(state, key) {
				delete(common, key)
			}
		}
	}
	return common
}

func ps1006CompleteTileForKey(state ps1006AccumulatorPath, key ps1006TileKey) bool {
	return len(state.tileSeen[key]) == 4 && ps1006CompleteDistinctTile(state.tiles[key]) && state.graph.independentCount(state.tileSeen[key]) == 4
}

type ps1006AnalysisIndex struct {
	objectByKey            map[string]types.Object
	functionDeclarations   map[*types.Func]*ast.FuncDecl
	functionByNode         map[ast.Node]*ps1006FunctionFacts
	functionFactsAt        map[token.Pos]*ps1006FunctionFacts
	aliasDefs              map[types.Object][]ps1006AliasDef
	selectorDefs           map[ps1006SelectorKey][]ps1006AliasDef
	orderedPointerSlots    map[ps1006SelectorKey]types.Object
	indirectAliasDefs      []ps1006IndirectAliasDef
	pureCalls              map[*ast.CallExpr]bool
	pureCallsKnown         map[*ast.CallExpr]bool
	activeCallableBodies   map[*ast.BlockStmt]bool
	activeCallableReturns  map[*ast.CallExpr]bool
	callableEffects        map[ps4008CallableEffectKey]map[types.Object]bool
	callableInvocations    map[ps4008CallableInvocationKey]bool
	callablePackageObjects map[*ast.BlockStmt]map[types.Object]bool
	branchAt               map[token.Pos]ps1006BranchRange
	branchPeers            map[ps1006BranchRange]ps1006BranchRange
	branchOwners           map[ps1006BranchRange]*ast.IfStmt
	loopsAt                map[token.Pos]*ps1006LoopRange
	controlFlows           map[*ast.BlockStmt]*ps1006ControlFlow
	tileCompletions        map[ps1006TileCompletionKey]bool
	buildVisits            int
	analysisVisits         int
	stabilityQueries       int
	purityVisits           int
	callableVisits         int
	controlFlowBuilds      int
	controlFlowVisits      int
}

type ps1006ControlFlow struct {
	graph   *cfg.CFG
	parents map[ast.Node]ast.Node
	blocks  map[ps1006ControlFlowBlockKey]*cfg.Block
}

type ps1006ControlFlowBlockKey struct {
	node ast.Node
	kind cfg.BlockKind
}

type ps1006TileCompletionKey struct {
	outer ast.Node
	inner ast.Node
	tail  ast.Node
}

type ps1006AliasDef struct {
	position        token.Pos
	value           ast.Expr
	branch          ps1006BranchRange
	resultIndex     int
	resultSelection bool
}

type ps1006SelectorKey struct {
	root types.Object
	path string
}

type ps1006IndirectAliasDef struct {
	position token.Pos
	target   ast.Expr
	value    ast.Expr
	branch   ps1006BranchRange
}

type ps1006BranchRange struct {
	start token.Pos
	end   token.Pos
}

type ps1006LoopRange struct {
	node           ast.Node
	iterationStart token.Pos
	iterationEnd   token.Pos
	bodyStart      token.Pos
	bodyEnd        token.Pos
	postStart      token.Pos
	postEnd        token.Pos
	parent         *ps1006LoopRange
}

var ps1006ActiveAnalysisIndexes sync.Map

func ps1006AnalysisIndexForPass(pass *analysis.Pass) *ps1006AnalysisIndex {
	if index, ok := ps1006ActiveAnalysisIndexes.Load(pass); ok {
		return index.(*ps1006AnalysisIndex)
	}
	return nil
}

type ps1006FunctionFacts struct {
	root        ast.Node
	priorUnsafe map[types.Object][]token.Pos
	localUnsafe map[types.Object][]token.Pos
	impureCalls []token.Pos
}

func ps1006BuildAnalysisIndex(pass *analysis.Pass) *ps1006AnalysisIndex {
	index := &ps1006AnalysisIndex{
		objectByKey:            make(map[string]types.Object),
		functionDeclarations:   make(map[*types.Func]*ast.FuncDecl),
		functionByNode:         make(map[ast.Node]*ps1006FunctionFacts),
		functionFactsAt:        make(map[token.Pos]*ps1006FunctionFacts),
		aliasDefs:              make(map[types.Object][]ps1006AliasDef),
		selectorDefs:           make(map[ps1006SelectorKey][]ps1006AliasDef),
		orderedPointerSlots:    make(map[ps1006SelectorKey]types.Object),
		pureCalls:              make(map[*ast.CallExpr]bool),
		pureCallsKnown:         make(map[*ast.CallExpr]bool),
		activeCallableBodies:   make(map[*ast.BlockStmt]bool),
		activeCallableReturns:  make(map[*ast.CallExpr]bool),
		callableEffects:        make(map[ps4008CallableEffectKey]map[types.Object]bool),
		callableInvocations:    make(map[ps4008CallableInvocationKey]bool),
		callablePackageObjects: make(map[*ast.BlockStmt]map[types.Object]bool),
		branchAt:               make(map[token.Pos]ps1006BranchRange),
		branchPeers:            make(map[ps1006BranchRange]ps1006BranchRange),
		branchOwners:           make(map[ps1006BranchRange]*ast.IfStmt),
		loopsAt:                make(map[token.Pos]*ps1006LoopRange),
		controlFlows:           make(map[*ast.BlockStmt]*ps1006ControlFlow),
		tileCompletions:        make(map[ps1006TileCompletionKey]bool),
	}
	loopByNode := make(map[ast.Node]*ps1006LoopRange)
	roots := make(map[ast.Node]*ps1006FunctionFacts)
	activeLiterals := ps1006ActiveFuncLiterals(pass)
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch node.(type) {
			case *ast.FuncDecl, *ast.FuncLit:
				roots[node] = &ps1006FunctionFacts{
					root: node, priorUnsafe: make(map[types.Object][]token.Pos),
					localUnsafe: make(map[types.Object][]token.Pos),
				}
			}
			if declaration, ok := node.(*ast.FuncDecl); ok && declaration.Name != nil {
				if function, ok := identObject(pass, declaration.Name).(*types.Func); ok {
					index.functionDeclarations[function] = declaration
				}
			}
			return true
		})
	}
	for _, file := range pass.Files {
		astutil.WithStack(file, func(node ast.Node, stack []ast.Node) bool {
			index.buildVisits++
			if statement, ok := node.(*ast.IfStmt); ok && statement.Body != nil {
				_, nestedConditional := ps1006BranchRangeForNode(node, stack)
				if alternative, ok := statement.Else.(*ast.BlockStmt); ok && !nestedConditional {
					left := ps1006BranchRange{start: statement.Body.Pos(), end: statement.Body.End()}
					right := ps1006BranchRange{start: alternative.Pos(), end: alternative.End()}
					index.branchPeers[left] = right
					index.branchPeers[right] = left
					index.branchOwners[left] = statement
					index.branchOwners[right] = statement
				}
			}
			facts := roots[node]
			if facts == nil {
				facts = ps1006NearestFunctionFacts(roots, stack)
			}
			if facts != nil {
				index.functionByNode[node] = facts
				index.functionFactsAt[node.Pos()] = facts
			}
			if branch, ok := ps1006BranchRangeForNode(node, stack); ok {
				index.branchAt[node.Pos()] = branch
			}
			parentLoop := ps1006NearestLoopRange(loopByNode, stack, node.Pos())
			if loop := ps1006LoopRangeForNode(node, parentLoop); loop != nil {
				loopByNode[node] = loop
			}
			if parentLoop != nil {
				index.loopsAt[node.Pos()] = parentLoop
			}
			if identifier, ok := node.(*ast.Ident); ok {
				if object := identObject(pass, identifier); object != nil {
					index.objectByKey[ps1006ObjectKey(pass, object, identifier.Name)] = object
				}
			}
			if function, ok := node.(*ast.FuncLit); ok && activeLiterals[function] {
				if parent := ps1006NearestFunctionFacts(roots, stack); parent != nil && parent != facts {
					for object := range ps1006MentionedObjects(pass, function.Body) {
						ps1006IndexUnsafe(parent, object, function.Pos(), true)
					}
				}
			}
			if facts != nil {
				ps1006IndexNode(pass, index, facts, node, stack)
			}
			ps1006IndexAliasDefinitions(pass, index, node)
			return true
		})
	}
	for _, facts := range roots {
		ps1006NormalizeFactPositions(facts.priorUnsafe)
		ps1006NormalizeFactPositions(facts.localUnsafe)
		slices.Sort(facts.impureCalls)
		facts.impureCalls = slices.Compact(facts.impureCalls)
	}
	for object := range index.aliasDefs {
		ps1006SortAliasDefs(index.aliasDefs[object])
	}
	for key := range index.selectorDefs {
		ps1006SortAliasDefs(index.selectorDefs[key])
	}
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	ps1006IndexIndirectAliasDefinitions(pass, index)
	ps1006ActiveAnalysisIndexes.Delete(pass)
	return index
}

// ps1006ActiveFuncLiterals separates creating a closure from invoking or
// exposing it. A literal bound only to blank (or to a local whose only use is
// a blank sink) is dormant and cannot make its captures unstable. Any other
// use is conservatively treated as an invocation/escape path.
func ps1006ActiveFuncLiterals(pass *analysis.Pass) map[*ast.FuncLit]bool {
	active := make(map[*ast.FuncLit]bool)
	bindings := make(map[types.Object][]*ast.FuncLit)
	parents := make(map[ast.Node]ast.Node)
	for _, file := range pass.Files {
		astutil.WithStack(file, func(node ast.Node, stack []ast.Node) bool {
			if len(stack) > 0 {
				parents[node] = stack[len(stack)-1]
			}
			literal, ok := node.(*ast.FuncLit)
			if !ok {
				return true
			}
			binding := ps1006FuncLiteralContainer(pass, literal, parents)
			parent := parents[binding]
			bound := false
			switch owner := parent.(type) {
			case *ast.AssignStmt:
				for index, rhs := range owner.Rhs {
					if rhs != binding || index >= len(owner.Lhs) {
						continue
					}
					identifier, ok := ps2110Unparen(owner.Lhs[index]).(*ast.Ident)
					if ok && identifier.Name != "_" {
						if object := identObject(pass, identifier); object != nil {
							bindings[object] = append(bindings[object], literal)
							if ps1006PackageObject(pass, object) {
								active[literal] = true
							}
							bound = true
						}
					} else {
						bound = true
					}
				}
			case *ast.ValueSpec:
				for index, rhs := range owner.Values {
					if rhs != binding || index >= len(owner.Names) {
						continue
					}
					name := owner.Names[index]
					if name.Name != "_" {
						if object := identObject(pass, name); object != nil {
							bindings[object] = append(bindings[object], literal)
							if ps1006PackageObject(pass, object) {
								active[literal] = true
							}
						}
					}
					bound = true
				}
			}
			if !bound {
				active[literal] = true
			}
			return true
		})
	}
	for identifier, object := range pass.TypesInfo.Uses {
		literals := bindings[object]
		if len(literals) == 0 || ps1006BlankCallableUse(identifier, parents) {
			continue
		}
		for _, literal := range literals {
			active[literal] = true
		}
	}
	return active
}

// ps1006FuncLiteralContainer follows value-only wrappers which neither call
// nor expose a literal by themselves. This lets `f := any(func(){...})` and a
// literal stored in an otherwise dormant local aggregate remain dormant,
// while a call/return/send still stops the walk and is conservatively active.
func ps1006FuncLiteralContainer(pass *analysis.Pass, literal *ast.FuncLit, parents map[ast.Node]ast.Node) ast.Expr {
	var current ast.Expr = literal
	for {
		switch parent := parents[current].(type) {
		case *ast.ParenExpr:
			current = parent
		case *ast.CallExpr:
			conversion := len(parent.Args) == 1 && parent.Args[0] == current
			if function, ok := pass.TypesInfo.Types[parent.Fun]; !ok || !function.IsType() || !conversion {
				return current
			}
			current = parent
		case *ast.TypeAssertExpr:
			if parent.X != current {
				return current
			}
			current = parent
		case *ast.KeyValueExpr:
			if parent.Value != current {
				return current
			}
			current = parent
		case *ast.CompositeLit:
			current = parent
		case *ast.UnaryExpr:
			if parent.Op != token.AND || parent.X != current {
				return current
			}
			current = parent
		default:
			return current
		}
	}
}

func ps1006BlankCallableUse(identifier *ast.Ident, parents map[ast.Node]ast.Node) bool {
	parent := parents[ast.Node(identifier)]
	for {
		paren, ok := parent.(*ast.ParenExpr)
		if !ok {
			break
		}
		parent = parents[paren]
	}
	switch owner := parent.(type) {
	case *ast.AssignStmt:
		for index, rhs := range owner.Rhs {
			if ps2110Unparen(rhs) != identifier || index >= len(owner.Lhs) {
				continue
			}
			blank, ok := ps2110Unparen(owner.Lhs[index]).(*ast.Ident)
			return ok && blank.Name == "_"
		}
	case *ast.ValueSpec:
		for index, rhs := range owner.Values {
			if ps2110Unparen(rhs) == identifier && index < len(owner.Names) {
				return owner.Names[index].Name == "_"
			}
		}
	}
	return false
}

func ps1006SortAliasDefs(definitions []ps1006AliasDef) {
	slices.SortFunc(definitions, func(left, right ps1006AliasDef) int {
		switch {
		case left.position < right.position:
			return -1
		case left.position > right.position:
			return 1
		default:
			return 0
		}
	})
}

func ps1006NearestLoopRange(loopByNode map[ast.Node]*ps1006LoopRange, stack []ast.Node, position token.Pos) *ps1006LoopRange {
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		loop := loopByNode[stack[stackIndex]]
		if loop != nil && (loop.bodyStart <= position && position <= loop.bodyEnd ||
			loop.postStart.IsValid() && loop.postStart <= position && position <= loop.postEnd ||
			loop.iterationStart.IsValid() && loop.iterationStart <= position && position <= loop.iterationEnd) {
			return loop
		}
	}
	return nil
}

func ps1006LoopRangeForNode(node ast.Node, parent *ps1006LoopRange) *ps1006LoopRange {
	switch loop := node.(type) {
	case *ast.ForStmt:
		if loop.Body == nil {
			return nil
		}
		result := &ps1006LoopRange{node: node, bodyStart: loop.Body.Pos(), bodyEnd: loop.Body.End(), parent: parent}
		if loop.Post != nil {
			result.postStart = loop.Post.Pos()
			result.postEnd = loop.Post.End()
		}
		return result
	case *ast.RangeStmt:
		if loop.Body != nil {
			result := &ps1006LoopRange{node: node, bodyStart: loop.Body.Pos(), bodyEnd: loop.Body.End(), parent: parent}
			if loop.Key != nil {
				result.iterationStart = loop.Key.Pos()
				result.iterationEnd = loop.Key.End()
			}
			if loop.Value != nil {
				if !result.iterationStart.IsValid() || loop.Value.Pos() < result.iterationStart {
					result.iterationStart = loop.Value.Pos()
				}
				if loop.Value.End() > result.iterationEnd {
					result.iterationEnd = loop.Value.End()
				}
			}
			return result
		}
	}
	return nil
}

func ps1006BranchRangeForNode(node ast.Node, stack []ast.Node) (ps1006BranchRange, bool) {
	if node == nil {
		return ps1006BranchRange{}, false
	}
	position := node.Pos()
	for stackIndex := len(stack) - 1; stackIndex >= 0; stackIndex-- {
		switch value := stack[stackIndex].(type) {
		case *ast.CaseClause, *ast.CommClause:
			return ps1006BranchRange{start: value.Pos(), end: value.End()}, true
		case *ast.IfStmt:
			if value.Body != nil && value.Body.Pos() <= position && position <= value.Body.End() {
				return ps1006BranchRange{start: value.Body.Pos(), end: value.Body.End()}, true
			}
			if value.Else != nil && value.Else.Pos() <= position && position <= value.Else.End() {
				return ps1006BranchRange{start: value.Else.Pos(), end: value.Else.End()}, true
			}
		}
	}
	return ps1006BranchRange{}, false
}

func ps1006IndexAliasDefinitions(pass *analysis.Pass, index *ps1006AnalysisIndex, node ast.Node) {
	branch := index.branchAt[node.Pos()]
	switch value := node.(type) {
	case *ast.AssignStmt:
		if value.Tok != token.DEFINE && value.Tok != token.ASSIGN {
			return
		}
		for rhsIndex, lhs := range value.Lhs {
			var rhs ast.Expr
			resultSelection := false
			if len(value.Lhs) == len(value.Rhs) {
				rhs = value.Rhs[rhsIndex]
			} else if len(value.Rhs) == 1 {
				if typ := pass.TypesInfo.TypeOf(value.Rhs[0]); typ != nil {
					if tuple, _ := types.Unalias(typ).Underlying().(*types.Tuple); tuple != nil && tuple.Len() == len(value.Lhs) {
						rhs = value.Rhs[0]
						resultSelection = true
					}
				}
			}
			identifier, ok := lhs.(*ast.Ident)
			if ok {
				if object := identObject(pass, identifier); object != nil {
					index.aliasDefs[object] = append(index.aliasDefs[object], ps1006AliasDef{
						position: identifier.Pos(), value: rhs, branch: branch,
						resultIndex: rhsIndex, resultSelection: resultSelection,
					})
				}
				continue
			}
			switch target := ps2110Unparen(lhs).(type) {
			case *ast.SelectorExpr:
				if key, ok := ps4008SelectorKey(pass, target); ok {
					index.selectorDefs[key] = append(index.selectorDefs[key], ps1006AliasDef{position: target.Pos(), value: rhs, branch: branch})
				}
			case *ast.IndexExpr:
				if key, ok := ps4008ConstantIndexKey(pass, target); ok {
					index.selectorDefs[key] = append(index.selectorDefs[key], ps1006AliasDef{position: target.Pos(), value: rhs, branch: branch})
				}
			case *ast.StarExpr:
				index.indirectAliasDefs = append(index.indirectAliasDefs, ps1006IndirectAliasDef{position: target.Pos(), target: target.X, value: rhs, branch: branch})
			}
		}
	case *ast.ValueSpec:
		for valueIndex, name := range value.Names {
			if object := identObject(pass, name); object != nil {
				var initial ast.Expr
				resultSelection := false
				if len(value.Names) == len(value.Values) {
					initial = value.Values[valueIndex]
				} else if len(value.Values) == 1 {
					if typ := pass.TypesInfo.TypeOf(value.Values[0]); typ != nil {
						if tuple, _ := types.Unalias(typ).Underlying().(*types.Tuple); tuple != nil && tuple.Len() == len(value.Names) {
							initial = value.Values[0]
							resultSelection = true
						}
					}
				}
				index.aliasDefs[object] = append(index.aliasDefs[object], ps1006AliasDef{
					position: name.Pos(), value: initial, branch: branch,
					resultIndex: valueIndex, resultSelection: resultSelection,
				})
			}
		}
	case *ast.RangeStmt:
		for _, expression := range []ast.Expr{value.Key, value.Value} {
			identifier, ok := expression.(*ast.Ident)
			if !ok || identifier.Name == "_" {
				continue
			}
			if object := identObject(pass, identifier); object != nil {
				index.aliasDefs[object] = append(index.aliasDefs[object], ps1006AliasDef{position: identifier.Pos(), branch: branch})
			}
		}
	}
}

func ps1006IndexIndirectAliasDefinitions(pass *analysis.Pass, index *ps1006AnalysisIndex) {
	if index == nil || len(index.indirectAliasDefs) == 0 {
		return
	}
	slices.SortFunc(index.indirectAliasDefs, func(left, right ps1006IndirectAliasDef) int {
		return cmp.Compare(left.position, right.position)
	})
	// An indirect retarget can itself make a later (or backedge-reachable)
	// indirect write resolvable. Iterate to a small structural fixed point;
	// each successful step adds one source-positioned alias definition.
	for range len(index.indirectAliasDefs) + 1 {
		changed := false
		for _, indirect := range index.indirectAliasDefs {
			if indirect.value == nil {
				continue
			}
			targets, resolved := ps4008PointerObjectTargets(pass, indirect.target, indirect.position, make(map[types.Object]bool))
			if !resolved {
				continue
			}
			for _, target := range targets {
				if !ps1006ObjectMayAliasMutableStorage(target) {
					continue
				}
				definition := ps1006AliasDef{position: indirect.position, value: indirect.value, branch: indirect.branch}
				if ps1006InsertAliasDef(index.aliasDefs, target, definition) {
					changed = true
				}
			}
		}
		if !changed {
			return
		}
	}
}

func ps1006InsertAliasDef(definitions map[types.Object][]ps1006AliasDef, object types.Object, definition ps1006AliasDef) bool {
	values := definitions[object]
	position, _ := slices.BinarySearchFunc(values, definition.position, func(value ps1006AliasDef, target token.Pos) int {
		return cmp.Compare(value.position, target)
	})
	for index := position; index < len(values) && values[index].position == definition.position; index++ {
		if values[index].value == definition.value {
			return false
		}
	}
	values = append(values, ps1006AliasDef{})
	copy(values[position+1:], values[position:])
	values[position] = definition
	definitions[object] = values
	return true
}

func ps1006NearestFunctionFacts(roots map[ast.Node]*ps1006FunctionFacts, stack []ast.Node) *ps1006FunctionFacts {
	for index := len(stack) - 1; index >= 0; index-- {
		if facts := roots[stack[index]]; facts != nil {
			return facts
		}
	}
	return nil
}

func ps1006IndexNode(pass *analysis.Pass, index *ps1006AnalysisIndex, facts *ps1006FunctionFacts, node ast.Node, stack []ast.Node) {
	if ps1006NodeUnreachableByConstantControl(pass, node, stack) {
		return
	}
	switch value := node.(type) {
	case *ast.AssignStmt:
		for _, lhs := range value.Lhs {
			for object := range ps1006AssignmentRootObjects(pass, lhs) {
				ps1006IndexUnsafe(facts, object, lhs.Pos(), false)
			}
		}
		if value.Tok == token.ASSIGN || value.Tok == token.DEFINE {
			for _, rhs := range value.Rhs {
				ps1006IndexMutableExposure(pass, facts, rhs)
			}
		}
	case *ast.ValueSpec:
		for _, rhs := range value.Values {
			ps1006IndexMutableExposure(pass, facts, rhs)
		}
	case *ast.IncDecStmt:
		for object := range ps1006MentionedObjects(pass, value.X) {
			ps1006IndexUnsafe(facts, object, value.Pos(), false)
		}
	case *ast.RangeStmt:
		if identifier, ok := value.Key.(*ast.Ident); ok {
			ps1006IndexUnsafe(facts, identObject(pass, identifier), identifier.Pos(), false)
		}
		if identifier, ok := value.Value.(*ast.Ident); ok {
			ps1006IndexUnsafe(facts, identObject(pass, identifier), identifier.Pos(), false)
		}
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			for object := range ps1006MentionedObjects(pass, value.X) {
				ps1006IndexUnsafe(facts, object, value.Pos(), true)
			}
		}
	case *ast.CallExpr:
		if !ps1006ValuePureCallIndexed(pass, index, value) {
			facts.impureCalls = append(facts.impureCalls, value.Pos())
		}
		if selector, ok := ps2110Unparen(value.Fun).(*ast.SelectorExpr); ok && ps4008ReceiverMayMutateReferencedStorage(pass, selector) {
			for object := range ps1006MentionedObjects(pass, selector.X) {
				ps1006IndexUnsafe(facts, object, value.Pos(), true)
			}
		}
		if !ps1006ValuePureCallIndexed(pass, index, value) {
			for _, argument := range value.Args {
				for object := range ps1006MentionedObjects(pass, argument) {
					if ps1006ObjectMayAliasMutableStorage(object) {
						ps1006IndexUnsafe(facts, object, argument.Pos(), true)
					}
				}
			}
		}
	case *ast.SendStmt:
		for object := range ps1006MentionedObjects(pass, value.Value) {
			if ps1006ObjectMayAliasMutableStorage(object) {
				ps1006IndexUnsafe(facts, object, value.Pos(), true)
			}
		}
	case *ast.ReturnStmt:
		for _, result := range value.Results {
			for object := range ps1006MentionedObjects(pass, result) {
				if ps1006ObjectMayAliasMutableStorage(object) {
					ps1006IndexUnsafe(facts, object, result.Pos(), false)
				}
			}
		}
	}
}

func ps1006NodeUnreachableByConstantControl(pass *analysis.Pass, node ast.Node, stack []ast.Node) bool {
	if node == nil {
		return false
	}
	for index := len(stack) - 1; index >= 0; index-- {
		switch parent := stack[index].(type) {
		case *ast.BinaryExpr:
			if node.Pos() < parent.Y.Pos() || node.End() > parent.Y.End() {
				continue
			}
			left, known := ps1006BoolConstant(pass, parent.X)
			if known && (parent.Op == token.LAND && !left || parent.Op == token.LOR && left) {
				return true
			}
		case *ast.IfStmt:
			condition, known := ps1006BoolConstant(pass, parent.Cond)
			if !known {
				continue
			}
			if parent.Body != nil && node.Pos() >= parent.Body.Pos() && node.End() <= parent.Body.End() && !condition {
				return true
			}
			if parent.Else != nil && node.Pos() >= parent.Else.Pos() && node.End() <= parent.Else.End() && condition {
				return true
			}
		}
	}
	return false
}

func ps1006AssignmentRootObjects(pass *analysis.Pass, expression ast.Expr) map[types.Object]bool {
	switch target := ps2110Unparen(expression).(type) {
	case *ast.Ident:
		object := identObject(pass, target)
		if object != nil {
			return map[types.Object]bool{object: true}
		}
	case *ast.IndexExpr:
		return ps1006MentionedObjects(pass, target.X)
	case *ast.SelectorExpr:
		// A package-qualified selector has no types.Selection: the selected
		// object itself is the writable package variable. Returning only the
		// PkgName root loses writes such as helper.Stride = n, so an imported
		// mutable stride can otherwise look stable across all tile lanes.
		if pass.TypesInfo.Selections[target] == nil {
			if object, ok := identObject(pass, target.Sel).(*types.Var); ok {
				return map[types.Object]bool{object: true}
			}
		}
		return ps1006MentionedObjects(pass, target.X)
	case *ast.StarExpr:
		return ps1006MentionedObjects(pass, target.X)
	}
	return nil
}

func ps1006IndexMutableExposure(pass *analysis.Pass, facts *ps1006FunctionFacts, expression ast.Expr) {
	// Creating a function value does not expose the storage it captures. Active
	// invocation/escape paths are indexed separately by
	// ps1006ActiveFuncLiterals; dormant literals must remain inert.
	if _, ok := ps2110Unparen(expression).(*ast.FuncLit); ok {
		return
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ != nil && !ps1006TypeMayAliasMutableStorage(typ, make(map[types.Type]bool)) {
		return
	}
	for object := range ps1006MentionedObjects(pass, expression) {
		if ps1006ObjectMayAliasMutableStorage(object) {
			ps1006IndexUnsafe(facts, object, expression.Pos(), true)
		}
	}
}

func ps1006ObjectMayAliasMutableStorage(object types.Object) bool {
	return object != nil && ps1006TypeMayAliasMutableStorage(object.Type(), make(map[types.Type]bool))
}

func ps1006MentionedObjects(pass *analysis.Pass, node ast.Node) map[types.Object]bool {
	objects := make(map[types.Object]bool)
	ast.Inspect(node, func(current ast.Node) bool {
		identifier, ok := current.(*ast.Ident)
		if ok {
			if object := identObject(pass, identifier); object != nil {
				objects[object] = true
			}
		}
		return true
	})
	return objects
}

func ps1006IndexUnsafe(facts *ps1006FunctionFacts, object types.Object, position token.Pos, prior bool) {
	if facts == nil || object == nil || !position.IsValid() {
		return
	}
	facts.localUnsafe[object] = append(facts.localUnsafe[object], position)
	if prior {
		facts.priorUnsafe[object] = append(facts.priorUnsafe[object], position)
	}
}

func ps1006NormalizeFactPositions(index map[types.Object][]token.Pos) {
	for object, positions := range index {
		slices.Sort(positions)
		index[object] = slices.Compact(positions)
	}
}

func ps1006ValuePureCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	index := ps1006AnalysisIndexForPass(pass)
	if index == nil {
		index = &ps1006AnalysisIndex{
			pureCalls:      make(map[*ast.CallExpr]bool),
			pureCallsKnown: make(map[*ast.CallExpr]bool),
		}
	}
	return ps1006ValuePureCallIndexed(pass, index, call)
}

func ps1006ValuePureCallIndexed(pass *analysis.Pass, index *ps1006AnalysisIndex, call *ast.CallExpr) bool {
	if call == nil || index == nil {
		return false
	}
	if index.pureCallsKnown[call] {
		return index.pureCalls[call]
	}
	index.pureCallsKnown[call] = true
	pure := false
	if value, ok := pass.TypesInfo.Types[call]; ok && value.Value != nil {
		pure = true
	} else if fun, ok := pass.TypesInfo.Types[call.Fun]; ok && fun.IsType() {
		pure = true
	} else if identifier, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		builtin, ok := identObject(pass, identifier).(*types.Builtin)
		pure = ok && (builtin.Name() == "len" || builtin.Name() == "cap")
	}
	if pure {
		pure = ps1006NestedCallsValuePure(pass, index, call)
	}
	index.pureCalls[call] = pure
	return pure
}

func ps1006NestedCallsValuePure(pass *analysis.Pass, index *ps1006AnalysisIndex, outer *ast.CallExpr) bool {
	pure := true
	for _, argument := range outer.Args {
		ast.Inspect(argument, func(node ast.Node) bool {
			if !pure {
				return false
			}
			if node != nil {
				index.purityVisits++
			}
			if call, ok := node.(*ast.CallExpr); ok {
				pure = ps1006ValuePureCallIndexed(pass, index, call)
				// The recursive result includes the call's arguments. Do not
				// descend into the same subtree independently as well.
				return false
			}
			return true
		})
	}
	return pure
}

// ps1006RegisterTileInputsStable rejects a syntactically complete tile when
// either its source or an input to its stride expression can change while the
// tap loop executes. The canonical stride key retains the object identities of
// direct and derived inputs, which lets this check cover both `t*stride` and a
// prior `base := t*stride` without treating the per-tap base definition itself
// as an unstable input.
func ps1006RegisterTileInputsStable(index *ps1006AnalysisIndex, pass *analysis.Pass, body *ast.BlockStmt, keys map[ps1006TileKey]bool) bool {
	if index == nil || body == nil || len(keys) == 0 {
		return false
	}
	inputs := make(map[types.Object]bool, len(keys)*2)
	for key := range keys {
		if key.source == nil || strings.Contains(key.stride, ps1006ImpureCallKey) {
			return false
		}
		inputs[key.source] = true
		for _, objectKey := range ps1006StrideObjectKeys(key.stride) {
			object := index.objectByKey[objectKey]
			if object == nil {
				return false
			}
			inputs[object] = true
		}
	}
	for object := range inputs {
		if !ps1006ObjectStableInNode(index, pass, body, object) {
			return false
		}
	}
	return true
}

func ps1006StrideObjectKeys(stride string) []string {
	var keys []string
	for {
		start := strings.Index(stride, "$objects{")
		if start < 0 {
			return keys
		}
		stride = stride[start+len("$objects{"):]
		end := strings.IndexByte(stride, '}')
		if end < 0 {
			return keys
		}
		if content := stride[:end]; content != "" {
			keys = append(keys, strings.Split(content, ",")...)
		}
		stride = stride[end+1:]
	}
}

// ps1006ObjectStableInNode is deliberately conservative. Besides direct
// writes it rejects taking the object's address, mutating storage rooted at
// the object, and exposing mutable storage through an assignment or call.
// That protects tile proofs from aliases without requiring whole-program
// escape analysis.
func ps1006ObjectStableInNode(index *ps1006AnalysisIndex, pass *analysis.Pass, node ast.Node, object types.Object) bool {
	if index == nil || node == nil || object == nil {
		return false
	}
	index.stabilityQueries++
	facts := index.functionByNode[node]
	if facts == nil {
		return false
	}
	if ps1006HasPositionBefore(facts.priorUnsafe[object], node.Pos()) || ps1006HasPositionInRange(facts.localUnsafe[object], node.Pos(), node.End()) {
		return false
	}
	// Any package-scope variable can be mutated by an opaque call into its
	// defining package. This includes imported vars: helper.ChangeStride()
	// may change helper.Stride without mentioning the variable at the call
	// site. Constants never enter the mutable-input set.
	if ps1006PackageScopeObject(object) && (ps1006HasPositionBefore(facts.impureCalls, node.Pos()) || ps1006HasPositionInRange(facts.impureCalls, node.Pos(), node.End())) {
		return false
	}
	return true
}

func ps1006HasPositionBefore(positions []token.Pos, before token.Pos) bool {
	index, _ := slices.BinarySearch(positions, before)
	return index > 0
}

func ps1006HasPositionInRange(positions []token.Pos, start, end token.Pos) bool {
	index, _ := slices.BinarySearch(positions, start)
	return index < len(positions) && positions[index] <= end
}

func ps1006PackageObject(pass *analysis.Pass, object types.Object) bool {
	return pass != nil && pass.Pkg != nil && object != nil && object.Parent() == pass.Pkg.Scope()
}

func ps1006PackageScopeObject(object types.Object) bool {
	return object != nil && object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}

func ps1006TypeMayAliasMutableStorage(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil || seen[typ] {
		return false
	}
	seen[typ] = true
	switch underlying := typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface, *types.Signature:
		return true
	case *types.Array:
		return ps1006TypeMayAliasMutableStorage(underlying.Elem(), seen)
	case *types.Struct:
		for index := 0; index < underlying.NumFields(); index++ {
			if ps1006TypeMayAliasMutableStorage(underlying.Field(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

type ps1006TileKey struct {
	source   types.Object
	sourceID string
	stride   string
}

type ps1006TileSlots [4]types.Object

type ps1006AccumulatorPath struct {
	deps           map[types.Object]bool
	strideDeps     map[types.Object]string
	pointerAliases map[types.Object]ps1006OrderedPointerAlias
	seen           map[types.Object]bool
	keys           map[ps1006TileKey]bool
	tiles          map[ps1006TileKey]ps1006TileSlots
	tileSeen       map[ps1006TileKey]map[types.Object]bool
	graph          psAccumulatorGraph
	tileBound      types.Object
	tileWidth      int64
	done           bool
	unsafe         bool
}

func ps1006ScanAccumulatorBlock(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object, states []ps1006AccumulatorPath) []ps1006AccumulatorPath {
	if body == nil {
		return states
	}
	for _, statement := range body.List {
		var next []ps1006AccumulatorPath
		for _, state := range states {
			if state.done {
				next = append(next, state)
				continue
			}
			next = append(next, ps1006ScanAccumulatorStatement(pass, statement, inner, outer, state)...)
		}
		states = ps1006DedupeAccumulatorPaths(next)
	}
	return states
}

const ps1006MaxAccumulatorPaths = 256

func ps1006DedupeAccumulatorPaths(states []ps1006AccumulatorPath) []ps1006AccumulatorPath {
	if len(states) <= 1 {
		return states
	}
	seen := make(map[string]bool, len(states))
	deduped := make([]ps1006AccumulatorPath, 0, len(states))
	for _, state := range states {
		key := ps1006AccumulatorPathKey(state)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, state)
		if len(deduped) > ps1006MaxAccumulatorPaths {
			return []ps1006AccumulatorPath{{done: true, unsafe: true}}
		}
	}
	return deduped
}

func ps1006AccumulatorPathKey(state ps1006AccumulatorPath) string {
	return fmt.Sprintf(
		"done=%t;unsafe=%t;deps=%s;stride=%s;aliases=%s;seen=%s;keys=%s;tiles=%s;tile-seen=%s",
		state.done,
		state.unsafe,
		ps4008BoolObjectSetKey(state.deps),
		ps1006StrideDepsKey(state.strideDeps),
		ps1006OrderedPointerAliasesKey(state.pointerAliases),
		ps4008BoolObjectSetKey(state.seen),
		ps1006TileKeySetKey(state.keys),
		ps1006TileSlotsMapKey(state.tiles),
		ps1006TileSeenMapKey(state.tileSeen)+";graph="+state.graph.key(),
	)
}

func ps1006OrderedPointerAliasesKey(aliases map[types.Object]ps1006OrderedPointerAlias) string {
	parts := make([]string, 0, len(aliases))
	for object, alias := range aliases {
		targets := make([]string, 0, len(alias.targets))
		for _, target := range alias.targets {
			targets = append(targets, fmt.Sprintf("%p", target))
		}
		slices.Sort(targets)
		parts = append(parts, fmt.Sprintf("%p=%t:%s", object, alias.known, strings.Join(targets, ",")))
	}
	slices.Sort(parts)
	return strings.Join(parts, ";")
}

func ps1006TileSeenMapKey(seen map[ps1006TileKey]map[types.Object]bool) string {
	if len(seen) == 0 {
		return ""
	}
	parts := make([]string, 0, len(seen))
	for key, accumulators := range seen {
		parts = append(parts, ps1006TileKeyString(key)+"="+ps4008BoolObjectSetKey(accumulators))
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func ps1006StrideDepsKey(deps map[types.Object]string) string {
	if len(deps) == 0 {
		return ""
	}
	parts := make([]string, 0, len(deps))
	for object, key := range deps {
		parts = append(parts, fmt.Sprintf("%p=%s", object, key))
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func ps1006TileKeySetKey(keys map[ps1006TileKey]bool) string {
	if len(keys) == 0 {
		return ""
	}
	parts := make([]string, 0, len(keys))
	for key, value := range keys {
		if value {
			parts = append(parts, ps1006TileKeyString(key))
		}
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func ps1006TileSlotsMapKey(tiles map[ps1006TileKey]ps1006TileSlots) string {
	if len(tiles) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tiles))
	for key, slots := range tiles {
		parts = append(parts, ps1006TileKeyString(key)+"="+ps1006TileSlotsKey(slots))
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func ps1006TileSlotsKey(slots ps1006TileSlots) string {
	parts := make([]string, 0, len(slots))
	for offset, object := range slots {
		if object == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%d:%p", offset, object))
	}
	slices.Sort(parts)
	return strings.Join(parts, ",")
}

func ps1006TileKeyString(key ps1006TileKey) string {
	return fmt.Sprintf("%p/%s/%s", key.source, key.sourceID, key.stride)
}

func ps1006ConditionAccumulatorStates(pass *analysis.Pass, expression ast.Expr, inner types.Object, state ps1006AccumulatorPath) (whenTrue, whenFalse []ps1006AccumulatorPath) {
	switch value := ps2110Unparen(expression).(type) {
	case *ast.UnaryExpr:
		if value.Op == token.NOT {
			whenFalse, whenTrue = ps1006ConditionAccumulatorStates(pass, value.X, inner, state)
			return whenTrue, whenFalse
		}
	case *ast.BinaryExpr:
		switch value.Op {
		case token.LAND:
			leftTrue, leftFalse := ps1006ConditionAccumulatorStates(pass, value.X, inner, state)
			whenFalse = append(whenFalse, leftFalse...)
			for _, left := range leftTrue {
				rightTrue, rightFalse := ps1006ConditionAccumulatorStates(pass, value.Y, inner, left)
				whenTrue = append(whenTrue, rightTrue...)
				whenFalse = append(whenFalse, rightFalse...)
			}
			return ps1006DedupeAccumulatorPaths(whenTrue), ps1006DedupeAccumulatorPaths(whenFalse)
		case token.LOR:
			leftTrue, leftFalse := ps1006ConditionAccumulatorStates(pass, value.X, inner, state)
			whenTrue = append(whenTrue, leftTrue...)
			for _, left := range leftFalse {
				rightTrue, rightFalse := ps1006ConditionAccumulatorStates(pass, value.Y, inner, left)
				whenTrue = append(whenTrue, rightTrue...)
				whenFalse = append(whenFalse, rightFalse...)
			}
			return ps1006DedupeAccumulatorPaths(whenTrue), ps1006DedupeAccumulatorPaths(whenFalse)
		}
	}
	next := state.clone()
	next.graph.applyWithDependencies(pass, &ast.ExprStmt{X: expression}, next.deps, nil)
	deps := ps1006DependencyState{deps: next.deps, strideDeps: next.strideDeps, pointerAliases: next.pointerAliases}
	ps1006ApplyCaseExpression(pass, expression, inner, &deps)
	next.deps, next.strideDeps, next.pointerAliases = deps.deps, deps.strideDeps, deps.pointerAliases
	if result, known := ps1006BoolConstant(pass, expression); known {
		if result {
			return []ps1006AccumulatorPath{next}, nil
		}
		return nil, []ps1006AccumulatorPath{next}
	}
	return []ps1006AccumulatorPath{next.clone()}, []ps1006AccumulatorPath{next}
}

func ps1006ScanAccumulatorStatement(pass *analysis.Pass, statement ast.Stmt, inner, outer types.Object, state ps1006AccumulatorPath) []ps1006AccumulatorPath {
	switch value := statement.(type) {
	case *ast.AssignStmt:
		reads := ps1006StridedAccumulatorReads(pass, value, inner, outer, state.strideDeps)
		candidates := make(map[types.Object]bool, len(reads))
		for _, read := range reads {
			candidates[read.accumulator] = true
			state.seen[read.accumulator] = true
			state.keys[read.key] = true
			if state.tileSeen[read.key] == nil {
				state.tileSeen[read.key] = make(map[types.Object]bool)
			}
			state.tileSeen[read.key][read.accumulator] = true
			if read.offset >= 0 && read.offset < 4 {
				slots := state.tiles[read.key]
				slots[read.offset] = read.accumulator
				state.tiles[read.key] = slots
			}
		}
		state.graph.applyWithDependencies(pass, value, state.deps, candidates)
		ps4008UpdateDerivedDeps(pass, value, inner, state.deps)
		ps1006UpdateDerivedStrideDeps(pass, value, inner, state.strideDeps)
		return []ps1006AccumulatorPath{state}
	case *ast.DeclStmt:
		state.graph.applyWithDependencies(pass, value, state.deps, nil)
		ps4008UpdateDerivedDeps(pass, value, inner, state.deps)
		ps1006UpdateDerivedStrideDeps(pass, value, inner, state.strideDeps)
		return []ps1006AccumulatorPath{state}
	case *ast.ExprStmt:
		state.graph.applyWithDependencies(pass, value, state.deps, nil)
		deps := ps1006DependencyState{deps: state.deps, strideDeps: state.strideDeps, pointerAliases: state.pointerAliases}
		ps1006ApplyCaseExpression(pass, value.X, inner, &deps)
		state.deps, state.strideDeps, state.pointerAliases = deps.deps, deps.strideDeps, deps.pointerAliases
		if ps1006IsPanicCall(pass, value.X) {
			state.done = true
		}
		return []ps1006AccumulatorPath{state}
	case *ast.IfStmt:
		if value.Init != nil {
			state.graph.applyWithDependencies(pass, value.Init, state.deps, nil)
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, state.strideDeps)
		}
		whenTrue, whenFalse := ps1006ConditionAccumulatorStates(pass, value.Cond, inner, state)
		thenStates := ps1006ScanAccumulatorBlock(pass, value.Body, inner, outer, whenTrue)
		var elseStates []ps1006AccumulatorPath
		switch elseNode := value.Else.(type) {
		case *ast.BlockStmt:
			elseStates = ps1006ScanAccumulatorBlock(pass, elseNode, inner, outer, whenFalse)
		case *ast.IfStmt:
			for _, branch := range whenFalse {
				elseStates = append(elseStates, ps1006ScanAccumulatorStatement(pass, elseNode, inner, outer, branch)...)
			}
		default:
			if ps1006CanMergeOptionalTailGuard(pass, value.Cond, outer, state.tileBound, state.tileWidth, thenStates) {
				return thenStates
			}
			elseStates = whenFalse
		}
		return ps1006DedupeAccumulatorPaths(append(thenStates, elseStates...))
	case *ast.BlockStmt:
		return ps1006ScanAccumulatorBlock(pass, value, inner, outer, []ps1006AccumulatorPath{state})
	case *ast.LabeledStmt:
		return ps1006ScanAccumulatorStatement(pass, value.Stmt, inner, outer, state)
	case *ast.SwitchStmt:
		if value.Init != nil {
			state.graph.applyWithDependencies(pass, value.Init, state.deps, nil)
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, state.strideDeps)
		}
		if value.Tag != nil {
			state.graph.applyWithDependencies(pass, &ast.ExprStmt{X: value.Tag}, state.deps, nil)
		}
		if ps4008ExprDependsOn(pass, value.Tag, inner, state.deps) || ps1006ExprMentionsStrideDep(pass, value.Tag, state.strideDeps) {
			return []ps1006AccumulatorPath{state}
		}
		ps4008InvalidateDerivedDepsInExpr(pass, value.Tag, state.deps)
		ps1006InvalidateStrideDepsInExpr(pass, value.Tag, state.strideDeps)
		return ps1006ScanAccumulatorCaseClauses(pass, value.Body, inner, outer, state, value.Tag == nil)
	case *ast.TypeSwitchStmt:
		if value.Init != nil {
			state.graph.applyWithDependencies(pass, value.Init, state.deps, nil)
			ps4008UpdateDerivedDeps(pass, value.Init, inner, state.deps)
			ps1006UpdateDerivedStrideDeps(pass, value.Init, inner, state.strideDeps)
		}
		state.graph.applyWithDependencies(pass, value.Assign, state.deps, nil)
		ps4008InvalidateDerivedDeps(pass, value.Assign, state.deps)
		ps1006InvalidateStrideDeps(pass, value.Assign, state.strideDeps)
		return ps1006ScanAccumulatorCaseClauses(pass, value.Body, inner, outer, state, false)
	case *ast.SelectStmt:
		return ps1006ScanAccumulatorCommClauses(pass, value.Body, inner, outer, state)
	case *ast.IncDecStmt, *ast.SendStmt, *ast.GoStmt, *ast.DeferStmt:
		state.graph.applyWithDependencies(pass, statement, state.deps, nil)
		return []ps1006AccumulatorPath{state}
	case *ast.ReturnStmt:
		state.graph.applyWithDependencies(pass, value, state.deps, nil)
		state.done = true
		return []ps1006AccumulatorPath{state}
	case *ast.BranchStmt:
		switch value.Tok {
		case token.BREAK, token.CONTINUE:
			state.done = true
			state.unsafe = value.Label != nil
			return []ps1006AccumulatorPath{state}
		case token.GOTO:
			state.done = true
			state.unsafe = true
			return []ps1006AccumulatorPath{state}
		}
	}
	if mayLeave, unsafe := ps1006MayLeaveTap(pass, statement); mayLeave {
		terminal := state.clone()
		terminal.done = true
		terminal.unsafe = unsafe
		return []ps1006AccumulatorPath{terminal, state}
	}
	return []ps1006AccumulatorPath{state}
}

func ps1006CanMergeOptionalTailGuard(pass *analysis.Pass, cond ast.Expr, outer, tileBound types.Object, tileWidth int64, states []ps1006AccumulatorPath) bool {
	if len(states) == 0 {
		return false
	}
	if !ps1006IsOuterTailGuard(pass, cond, outer, tileBound, tileWidth) {
		return false
	}
	for _, state := range states {
		if state.done || state.unsafe {
			return false
		}
	}
	return true
}

func ps1006IsOuterTailGuard(pass *analysis.Pass, cond ast.Expr, outer, tileBound types.Object, tileWidth int64) bool {
	if outer == nil || tileBound == nil || tileWidth < 4 {
		return false
	}
	binary, ok := cond.(*ast.BinaryExpr)
	if !ok || binary.Op != token.LSS && binary.Op != token.LEQ {
		return false
	}
	offset, ok := ps1006OuterPlusConst(pass, binary.X, outer)
	if !ok || offset <= 0 || offset >= tileWidth {
		return false
	}
	return ps1006BoundObject(pass, binary.Y) == tileBound
}

func ps1006ScanAccumulatorCaseClauses(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object, state ps1006AccumulatorPath, constantBools bool) []ps1006AccumulatorPath {
	if body == nil {
		return []ps1006AccumulatorPath{state}
	}
	clauses := ps1006CaseClauses(body)
	selectedStates, remainingStates := ps1006SelectedAccumulatorStates(pass, clauses, inner, state, constantBools)
	var states []ps1006AccumulatorPath
	var fallthroughStates []ps1006AccumulatorPath
	for clauseIndex, clause := range clauses {
		inputs := slices.Clone(selectedStates[clauseIndex])
		inputs = append(inputs, fallthroughStates...)
		statements, fallsThrough := ps4008CaseBodyWithoutTerminalFallthrough(clause.Body)
		outputs := ps1006ScanAccumulatorBlock(pass, &ast.BlockStmt{List: statements}, inner, outer, inputs)
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

func ps1006SelectedAccumulatorStates(pass *analysis.Pass, clauses []*ast.CaseClause, inner types.Object, state ps1006AccumulatorPath, constantBools bool) (selected [][]ps1006AccumulatorPath, remaining []ps1006AccumulatorPath) {
	selected = make([][]ps1006AccumulatorPath, len(clauses))
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
			deps := ps1006DependencyState{deps: current.deps, strideDeps: current.strideDeps, pointerAliases: current.pointerAliases}
			ps1006ApplyCaseExpression(pass, expression, inner, &deps)
			current.deps, current.strideDeps, current.pointerAliases = deps.deps, deps.strideDeps, deps.pointerAliases
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
		remaining = []ps1006AccumulatorPath{current}
	}
	return selected, remaining
}

func ps1006ScanAccumulatorCommClauses(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object, state ps1006AccumulatorPath) []ps1006AccumulatorPath {
	if body == nil {
		return []ps1006AccumulatorPath{state}
	}
	var states []ps1006AccumulatorPath
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		clauseState := state.clone()
		applied := ps1006ApplyDependencyStatement(pass, clause.Comm, inner, ps1006DependencyState{deps: clauseState.deps, strideDeps: clauseState.strideDeps, pointerAliases: clauseState.pointerAliases})
		clauseState.deps = applied.deps
		clauseState.strideDeps = applied.strideDeps
		clauseState.pointerAliases = applied.pointerAliases
		states = append(states, ps1006ScanAccumulatorBlock(pass, &ast.BlockStmt{List: clause.Body}, inner, outer, []ps1006AccumulatorPath{clauseState})...)
	}
	if len(states) == 0 {
		return []ps1006AccumulatorPath{state}
	}
	return states
}

func (state ps1006AccumulatorPath) clone() ps1006AccumulatorPath {
	return ps1006AccumulatorPath{
		deps:           cloneObjectBoolMap(state.deps),
		strideDeps:     cloneObjectStringMap(state.strideDeps),
		pointerAliases: clonePS1006OrderedPointerAliases(state.pointerAliases),
		seen:           cloneObjectBoolMap(state.seen),
		keys:           clonePS1006KeyMap(state.keys),
		tiles:          clonePS1006TileMap(state.tiles),
		tileSeen:       clonePS1006TileSeenMap(state.tileSeen),
		graph:          state.graph.clone(),
		tileBound:      state.tileBound,
		tileWidth:      state.tileWidth,
		done:           state.done,
		unsafe:         state.unsafe,
	}
}

func ps1006MayLeaveTap(pass *analysis.Pass, statement ast.Stmt) (bool, bool) {
	return ps1006NodeMayLeaveTap(pass, statement, 0, 0)
}

func ps1006NodeMayLeaveTap(pass *analysis.Pass, node ast.Node, loopDepth, switchDepth int) (bool, bool) {
	switch value := node.(type) {
	case nil:
		return false, false
	case *ast.ReturnStmt:
		return true, false
	case *ast.BranchStmt:
		switch value.Tok {
		case token.CONTINUE:
			if value.Label != nil {
				return true, true
			}
			return loopDepth == 0, false
		case token.BREAK:
			if value.Label != nil {
				return true, true
			}
			return loopDepth == 0 && switchDepth == 0, false
		case token.GOTO:
			return true, true
		}
		return false, false
	case *ast.ExprStmt:
		return ps1006IsPanicCall(pass, value.X), false
	case *ast.LabeledStmt:
		return true, true
	case *ast.BlockStmt:
		return ps1006StatementListMayLeaveTap(pass, value.List, loopDepth, switchDepth)
	case *ast.IfStmt:
		bodyMayLeave, bodyUnsafe := ps1006StatementListMayLeaveTap(pass, value.Body.List, loopDepth, switchDepth)
		elseMayLeave, elseUnsafe := ps1006NodeMayLeaveTap(pass, value.Else, loopDepth, switchDepth)
		return bodyMayLeave || elseMayLeave, bodyUnsafe || elseUnsafe
	case *ast.ForStmt:
		if value.Body == nil {
			return false, false
		}
		return ps1006StatementListMayLeaveTap(pass, value.Body.List, loopDepth+1, switchDepth)
	case *ast.RangeStmt:
		if value.Body == nil {
			return false, false
		}
		return ps1006StatementListMayLeaveTap(pass, value.Body.List, loopDepth+1, switchDepth)
	case *ast.SwitchStmt:
		return ps1006CaseClausesMayLeaveTap(pass, value.Body, loopDepth, switchDepth+1)
	case *ast.TypeSwitchStmt:
		return ps1006CaseClausesMayLeaveTap(pass, value.Body, loopDepth, switchDepth+1)
	case *ast.SelectStmt:
		return ps1006CommClausesMayLeaveTap(pass, value.Body, loopDepth, switchDepth+1)
	}
	return false, false
}

func ps1006StatementListMayLeaveTap(pass *analysis.Pass, statements []ast.Stmt, loopDepth, switchDepth int) (bool, bool) {
	var mayLeave, unsafe bool
	for _, statement := range statements {
		statementMayLeave, statementUnsafe := ps1006NodeMayLeaveTap(pass, statement, loopDepth, switchDepth)
		mayLeave = mayLeave || statementMayLeave
		unsafe = unsafe || statementUnsafe
	}
	return mayLeave, unsafe
}

func ps1006CaseClausesMayLeaveTap(pass *analysis.Pass, body *ast.BlockStmt, loopDepth, switchDepth int) (bool, bool) {
	if body == nil {
		return false, false
	}
	var mayLeave, unsafe bool
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CaseClause)
		if !ok {
			continue
		}
		clauseMayLeave, clauseUnsafe := ps1006StatementListMayLeaveTap(pass, clause.Body, loopDepth, switchDepth)
		mayLeave = mayLeave || clauseMayLeave
		unsafe = unsafe || clauseUnsafe
	}
	return mayLeave, unsafe
}

func ps1006CommClausesMayLeaveTap(pass *analysis.Pass, body *ast.BlockStmt, loopDepth, switchDepth int) (bool, bool) {
	if body == nil {
		return false, false
	}
	var mayLeave, unsafe bool
	for _, statement := range body.List {
		clause, ok := statement.(*ast.CommClause)
		if !ok {
			continue
		}
		clauseMayLeave, clauseUnsafe := ps1006StatementListMayLeaveTap(pass, clause.Body, loopDepth, switchDepth)
		mayLeave = mayLeave || clauseMayLeave
		unsafe = unsafe || clauseUnsafe
	}
	return mayLeave, unsafe
}

func ps1006IsPanicCall(pass *analysis.Pass, expression ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return false
	}
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok || identifier.Name != "panic" {
		return false
	}
	return identObject(pass, identifier) == types.Universe.Lookup("panic")
}

func clonePS1006KeyMap(input map[ps1006TileKey]bool) map[ps1006TileKey]bool {
	output := make(map[ps1006TileKey]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneObjectStringMap(input map[types.Object]string) map[types.Object]string {
	output := make(map[types.Object]string, len(input))
	for object, value := range input {
		output[object] = value
	}
	return output
}

func clonePS1006TileMap(input map[ps1006TileKey]ps1006TileSlots) map[ps1006TileKey]ps1006TileSlots {
	output := make(map[ps1006TileKey]ps1006TileSlots, len(input))
	for key, slots := range input {
		output[key] = slots
	}
	return output
}

func clonePS1006TileSeenMap(input map[ps1006TileKey]map[types.Object]bool) map[ps1006TileKey]map[types.Object]bool {
	output := make(map[ps1006TileKey]map[types.Object]bool, len(input))
	for key, seen := range input {
		output[key] = cloneObjectBoolMap(seen)
	}
	return output
}

func ps1006CompleteDistinctTile(slots ps1006TileSlots) bool {
	seen := make(map[types.Object]bool, len(slots))
	for _, accumulator := range slots {
		if accumulator == nil || seen[accumulator] {
			return false
		}
		seen[accumulator] = true
	}
	return true
}

type ps1006AccumulatorRead struct {
	accumulator types.Object
	key         ps1006TileKey
	offset      int64
}

func ps1006StridedAccumulatorReads(pass *analysis.Pass, assign *ast.AssignStmt, inner, outer types.Object, strideDeps map[types.Object]string) []ps1006AccumulatorRead {
	if assign == nil || assign.Tok != token.ADD_ASSIGN && assign.Tok != token.SUB_ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return nil
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name == "_" {
		return nil
	}
	accumulator := identObject(pass, lhs)
	if accumulator == nil {
		return nil
	}
	var reads []ps1006AccumulatorRead
	ast.Inspect(assign.Rhs[0], func(node ast.Node) bool {
		index, ok := node.(*ast.IndexExpr)
		if !ok {
			return true
		}
		key, offset, ok := ps1006IndexTileKey(pass, index, inner, outer, strideDeps)
		if !ok {
			return true
		}
		reads = append(reads, ps1006AccumulatorRead{accumulator: accumulator, key: key, offset: offset})
		return true
	})
	return reads
}

func ps1006IndexTileKey(pass *analysis.Pass, index *ast.IndexExpr, inner, outer types.Object, strideDeps map[types.Object]string) (ps1006TileKey, int64, bool) {
	source, sourceID, ok := ps1006SourceIdentity(pass, index.X)
	if !ok {
		return ps1006TileKey{}, 0, false
	}
	stride, offset, ok := ps1006StrideAndOuterOffset(pass, index.Index, inner, outer, strideDeps)
	if !ok {
		return ps1006TileKey{}, 0, false
	}
	return ps1006TileKey{source: source, sourceID: sourceID, stride: stride}, offset, true
}

func ps1006SourceIdentity(pass *analysis.Pass, expression ast.Expr) (types.Object, string, bool) {
	for {
		paren, ok := expression.(*ast.ParenExpr)
		if !ok {
			break
		}
		expression = paren.X
	}
	switch value := expression.(type) {
	case *ast.Ident:
		object := identObject(pass, value)
		if object == nil {
			return nil, "", false
		}
		return object, ps1006ObjectKey(pass, object, value.Name), true
	case *ast.SelectorExpr:
		baseObject, baseID, ok := ps1006SourceIdentity(pass, value.X)
		if !ok {
			return nil, "", false
		}
		field := pass.TypesInfo.Selections[value]
		if field == nil {
			return nil, "", false
		}
		fieldObject := field.Obj()
		return baseObject, baseID + "." + ps1006ObjectKey(pass, fieldObject, value.Sel.Name), true
	}
	return nil, "", false
}

func ps1006StrideAndOuterOffset(pass *analysis.Pass, index ast.Expr, inner, outer types.Object, strideDeps map[types.Object]string) (string, int64, bool) {
	if inner == nil || outer == nil {
		return "", 0, false
	}
	var stride string
	var offset int64
	seenOuter := false
	for _, term := range ps1006AdditiveTerms(index) {
		if key, ok := ps1006StrideDependencyKey(pass, term, inner, strideDeps); ok {
			if stride != "" {
				return "", 0, false
			}
			stride = key
			if stride == "" {
				return "", 0, false
			}
			continue
		}
		if ps1006ExprIsObject(pass, term, outer) {
			if seenOuter {
				return "", 0, false
			}
			seenOuter = true
			continue
		}
		literal, ok := term.(*ast.BasicLit)
		if !ok || literal.Kind != token.INT {
			return "", 0, false
		}
		value, err := strconv.ParseInt(literal.Value, 0, 64)
		if err != nil {
			return "", 0, false
		}
		offset += value
	}
	return stride, offset, stride != "" && seenOuter
}

func ps1006FlatIndexDependsOn(pass *analysis.Pass, index ast.Expr, inner, outer types.Object, strideDeps map[types.Object]string) bool {
	_, _, ok := ps1006StrideAndOuterOffset(pass, index, inner, outer, strideDeps)
	return ok
}

func ps1006ExprText(pass *analysis.Pass, expression ast.Expr) string {
	var buffer bytes.Buffer
	if err := printer.Fprint(&buffer, pass.Fset, expression); err != nil {
		return ""
	}
	return buffer.String()
}

// ps1006InterchangeFix builds the loop-interchange rewrite for the exact
// canonical column-reduction shape and nothing wider:
//
//	for C := 0; C < COLS; C++ {
//		S := 0.0 // or 0
//		for R := 0; R < ROWS; R++ {
//			S += A[R*COLS+C]
//		}
//		OUT[C] = S
//	}
//
// with A and OUT plain identifiers, fixed array types, and compile-time
// positive COLS and non-negative ROWS that prove every index in bounds. The
// replacement accumulates into scratch while walking A contiguously; per
// output element the accumulation order is unchanged, so the result is
// bit-identical:
//
//	psSumsN := make([]T, COLS)
//	for R := 0; R < ROWS; R++ {
//		psBase := R * COLS
//		for C := 0; C < COLS; C++ {
//			psSumsN[C] += A[psBase+C]
//		}
//	}
//	for C := 0; C < COLS; C++ {
//		OUT[C] = psSumsN[C]
//	}
//
// Slice inputs are deliberately advisory even with an indexed write-back:
// separate variables can overlap, and a source/output bounds panic observes
// the original per-column store timing. Extra statements, non-zero seeds,
// range loops, or non-ident arrays likewise keep the plain advisory report.
func ps1006InterchangeFix(pass *analysis.Pass, file *ast.File, outerNode, innerNode ast.Node) *analysis.SuggestedFix {
	outer, ok := outerNode.(*ast.ForStmt)
	if !ok {
		return nil
	}
	cVar, colsExpr := ps1006CountedLoop(outer)
	cols := simpleExprText(colsExpr)
	if cVar == "" || cols == "" || len(outer.Body.List) != 3 {
		return nil
	}
	inner, ok := outer.Body.List[1].(*ast.ForStmt)
	if !ok || ast.Node(inner) != innerNode {
		return nil
	}
	rVar, rowsExpr := ps1006CountedLoop(inner)
	rows := simpleExprText(rowsExpr)
	if rVar == "" || rows == "" || len(inner.Body.List) != 1 {
		return nil
	}
	// S := 0.0 (or S := 0): a zero seed, so make()'s zero value matches.
	seed, ok := outer.Body.List[0].(*ast.AssignStmt)
	if !ok || seed.Tok != token.DEFINE || len(seed.Lhs) != 1 || len(seed.Rhs) != 1 {
		return nil
	}
	sID, ok := seed.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}
	zero, ok := seed.Rhs[0].(*ast.BasicLit)
	if !ok || (zero.Value != "0" && zero.Value != "0.0") {
		return nil
	}
	// S += A[R*COLS+C]
	acc, ok := inner.Body.List[0].(*ast.AssignStmt)
	if !ok || acc.Tok != token.ADD_ASSIGN || len(acc.Lhs) != 1 || len(acc.Rhs) != 1 {
		return nil
	}
	if lhs, ok := acc.Lhs[0].(*ast.Ident); !ok || lhs.Name != sID.Name {
		return nil
	}
	ix, ok := acc.Rhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	arrID, ok := ix.X.(*ast.Ident)
	if !ok || !ps1006StrideIndex(ix.Index, rVar, cols, cVar) {
		return nil
	}
	// OUT[C] = S
	fin, ok := outer.Body.List[2].(*ast.AssignStmt)
	if !ok || fin.Tok != token.ASSIGN || len(fin.Lhs) != 1 || len(fin.Rhs) != 1 {
		return nil
	}
	outIx, ok := fin.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	outID, ok := outIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if idx, ok := outIx.Index.(*ast.Ident); !ok || idx.Name != cVar {
		return nil
	}
	if rhs, ok := fin.Rhs[0].(*ast.Ident); !ok || rhs.Name != sID.Name {
		return nil
	}
	// Reordering changes both the time at which source/output bounds panics
	// happen and the behavior of overlapping slices. Only fixed arrays with
	// compile-time positive-column and non-negative-row bounds let us prove
	// that every indexed access succeeds and that distinct variables cannot
	// share element storage.
	colsValue, colsOK := ps1006NonNegativeIntConstant(pass, colsExpr)
	rowsValue, rowsOK := ps1006NonNegativeIntConstant(pass, rowsExpr)
	sourceLength, sourceOK := ps1006ArrayLength(pass.TypesInfo.TypeOf(arrID))
	outputLength, outputOK := ps1006ArrayLength(pass.TypesInfo.TypeOf(outID))
	// With zero columns the original outer loop is a no-op, while the
	// interchanged form would still execute the row loop. Keep that degenerate
	// shape advisory so an arbitrarily large row bound cannot turn O(1) work
	// into O(rows).
	if !colsOK || !rowsOK || colsValue == 0 || !sourceOK || !outputOK || colsValue > outputLength {
		return nil
	}
	if rowsValue > sourceLength/colsValue || rowsValue*colsValue > sourceLength {
		return nil
	}
	// All five participating names must be pairwise distinct, and the bound
	// roots must not be any of the loop/accumulator variables (a triangular
	// bound like `r < c` would make the interchange wrong).
	names := []string{cVar, rVar, sID.Name, arrID.Name, outID.Name}
	for i := range names {
		for j := i + 1; j < len(names); j++ {
			if names[i] == names[j] {
				return nil
			}
		}
	}
	for _, bound := range []string{cols, rows} {
		root := bound
		if i := strings.IndexByte(root, '.'); i >= 0 {
			root = root[:i]
		}
		if root == cVar || root == rVar || root == sID.Name {
			return nil
		}
	}
	// Element type of the scratch slice: the accumulator's own basic type.
	tname := ""
	if obj := pass.TypesInfo.Defs[sID]; obj != nil {
		if b, ok := obj.Type().(*types.Basic); ok && b.Info()&types.IsNumeric != 0 {
			tname = b.Name()
		}
	}
	if tname == "" {
		if zero.Value != "0.0" {
			return nil
		}
		tname = "float64"
	}
	// Fresh names: line-derived scratch name, fixed psBase. Bail on any
	// user-written identifier with either name anywhere in the file.
	line := pass.Fset.Position(outer.Pos()).Line
	sums := fmt.Sprintf("psSums%d", line)
	collides := false
	ast.Inspect(file, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && (id.Name == sums || id.Name == "psBase") {
			collides = true
		}
		return !collides
	})
	if collides {
		return nil
	}
	// Assume gofmt indentation (tabs), matching the outer loop's column.
	ind := strings.Repeat("\t", pass.Fset.Position(outer.Pos()).Column-1)
	var b strings.Builder
	fmt.Fprintf(&b, "%s := make([]%s, %s)\n", sums, tname, cols)
	fmt.Fprintf(&b, "%sfor %s := 0; %s < %s; %s++ {\n", ind, rVar, rVar, rows, rVar)
	fmt.Fprintf(&b, "%s\tpsBase := %s * %s\n", ind, rVar, cols)
	fmt.Fprintf(&b, "%s\tfor %s := 0; %s < %s; %s++ {\n", ind, cVar, cVar, cols, cVar)
	fmt.Fprintf(&b, "%s\t\t%s[%s] += %s[psBase+%s]\n", ind, sums, cVar, arrID.Name, cVar)
	fmt.Fprintf(&b, "%s\t}\n", ind)
	fmt.Fprintf(&b, "%s}\n", ind)
	fmt.Fprintf(&b, "%sfor %s := 0; %s < %s; %s++ {\n", ind, cVar, cVar, cols, cVar)
	fmt.Fprintf(&b, "%s\t%s[%s] = %s[%s]\n", ind, outID.Name, cVar, sums, cVar)
	fmt.Fprintf(&b, "%s}", ind)
	return &analysis.SuggestedFix{
		Message: "interchange the loops so " + arrID.Name + " is walked contiguously",
		TextEdits: []analysis.TextEdit{
			{Pos: outer.Pos(), End: outer.End(), NewText: []byte(b.String())},
		},
	}
}

func ps1006ArrayLength(typ types.Type) (int64, bool) {
	if typ == nil {
		return 0, false
	}
	array, ok := typ.Underlying().(*types.Array)
	if !ok {
		return 0, false
	}
	return array.Len(), true
}

func ps1006NonNegativeIntConstant(pass *analysis.Pass, expression ast.Expr) (int64, bool) {
	if pass == nil || pass.TypesInfo == nil || expression == nil {
		return 0, false
	}
	value := pass.TypesInfo.Types[expression].Value
	if value == nil || value.Kind() != constant.Int {
		return 0, false
	}
	integer, exact := constant.Int64Val(value)
	return integer, exact && integer >= 0
}

// ps1006CountedLoop matches `for v := 0; v < bound; v++` and returns the
// loop variable name and the bound expression.
func ps1006CountedLoop(f *ast.ForStmt) (string, ast.Expr) {
	init, ok := f.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return "", nil
	}
	v, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil
	}
	lit, ok := init.Rhs[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.INT || lit.Value != "0" {
		return "", nil
	}
	cond, ok := f.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return "", nil
	}
	if x, ok := cond.X.(*ast.Ident); !ok || x.Name != v.Name {
		return "", nil
	}
	post, ok := f.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil
	}
	if x, ok := post.X.(*ast.Ident); !ok || x.Name != v.Name {
		return "", nil
	}
	return v.Name, cond.Y
}

// ps1006StrideIndex matches R*COLS + C (mul and add sides in either order,
// mul operands in either order): the inner variable times the outer bound,
// plus the outer variable.
func ps1006StrideIndex(e ast.Expr, rVar, cols, cVar string) bool {
	be, ok := e.(*ast.BinaryExpr)
	if !ok || be.Op != token.ADD {
		return false
	}
	mul, add := be.X, be.Y
	if _, ok := mul.(*ast.BinaryExpr); !ok {
		mul, add = be.Y, be.X
	}
	m, ok := mul.(*ast.BinaryExpr)
	if !ok || m.Op != token.MUL {
		return false
	}
	if id, ok := add.(*ast.Ident); !ok || id.Name != cVar {
		return false
	}
	x, y := m.X, m.Y
	if id, ok := x.(*ast.Ident); ok && id.Name == rVar {
		return simpleExprText(y) == cols
	}
	if id, ok := y.(*ast.Ident); ok && id.Name == rVar {
		return simpleExprText(x) == cols
	}
	return false
}
