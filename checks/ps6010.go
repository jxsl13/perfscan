package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS6010 reports an output loop whose inner accumulator re-reads an
// operand that does not vary with the output index.
var PS6010 = register(&lint.Check{
	ID:       "PS6010",
	Category: "verify",
	Slug:     "output-invariant-operand-reload",
	Level:    lint.LevelAggressive,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an accumulator loop re-reading an operand invariant in the output index",
		Text: `for o { for i { acc += A[i] * B[f(i,o)] } } re-streams all of A
once per output element: A[i] does not vary with o, so unrolling the OUTPUT
loop by 4 — four accumulators sharing one pass over A — amortizes that load
across four outputs (register blocking / unroll-and-jam).

L3 (aggressive): each accumulator keeps its own order, so the per-output
sums are bit-identical at any unroll factor; the tail loop handles the
remainder. This is the inline-accumulator sibling of the restreamed-row
shape (PS1007) — here the INPUT is re-read, there the OUTPUT is
re-streamed. Rank by the loop's share of runtime × the achievable factor.

For a row-owned parallel matrix product tmp[a,j] = sum_b inner[a,b] * V[j,b],
tile the j loop inside each row callback. Load inner[a,b] once per ascending b
step and feed four adjacent j accumulators. Keep b ascending independently in
every accumulator and run the original scalar body for the n%4 tail; reversing
b changes floating-point results and is not an equivalent optimization.
Body-local derived indexes are followed only when they are single-write,
not address-taken, independent of the output dimension, and not subject to a
control-transfer path that can bypass their definition. The complete indexed
container is part of the dependency proof: rows[o][i] and row[i] after
row := rows[o] vary with o even though the final index is i. Integer-only type
parameters (for example ~int) remain value-only index facts; constraints that
can contain reference storage remain unknown. Dominance queries use a per-file
statement-owner and ordinal index, so many derived values do not repeatedly
scan the enclosing statement list. A reference-backed container is not called
invariant when this function writes through a compatible parameter, an
explicit alias (including a[:], parenthesized forms, and reference-preserving
type conversions), stores it in a field, element, composite, interface, or
package global, transfers it through a channel, or passes it to an opaque call
that may mutate it. Reference-bearing descendants of otherwise incompatible
aggregate parameters remain possible aliases, and stored callbacks or method
values may mutate hidden captured storage. Inline reference-preserving
conversions and selections from struct, array, slice, or map literals retain
their recursively discovered descendants. Reference-bearing elements
transferred by append (including src...) or copy retain those descendants as
well; copying value-only elements does not create a storage alias. Slices,
pointers to compatible arrays, and pointers to their elements may address the
same backing subobjects, including through named types and aggregate fields.
Range values
and type-switch implicit bindings retain their source descendants; an opaque
range source or channel receive instead makes compatible parameter, captured
enclosing local, and package storage unknown. Opaque returns, map lookups,
receives, ranges, assertions, and package reference state carry a typed
external-storage wildcard; a write through that wildcard invalidates every
compatible non-fresh object in the enclosing function chain. Writes derived
from unsafe.Pointer (including uintptr conversions) use a universal wildcard.
Tuple-return calls and comma-ok map lookups, assertions, and receives bind
provenance to the corresponding result position. Package-local calls use
fixed-point may-effect summaries, and callable points-to facts flow through
separate declarations and assignments, aggregate fields, method values and
expressions, generic instantiations, and actual-to-formal callback edges.
Per-result callable-return slots preserve tuple position and propagate closures,
method values, aggregates, named results, transitive factories, and recursive
factory cycles without repeatedly rescanning the package.
When a sole multi-valued call supplies another call's arguments, each returned
callable slot is bound to its corresponding fixed or variadic parameter,
including a method-expression receiver.
Non-callable reference-bearing tuple results retain typed external-storage
provenance at the receiving call as well. This covers fixed and variadic
ordinary parameters and method-expression receivers while keeping incompatible
storage classes separate.
Hidden package, bodyless, and unsafe effects therefore cross higher-order,
transitive, and recursive helpers independent of declaration order. Interface
and type-parameter method calls remain dynamic even when the selected method
object belongs to an imported package, because the run-time implementation may
still be a package-local method with hidden state.
Static imported callees cannot name package-local state directly, but known
effects of callable-bearing arguments and the callable values they return are
still joined because the imported callee may invoke or retain either.
External storage types are preclassified into hashable compatibility classes;
candidate matching is linear in the indexed types and objects rather than the
product of both sets.
Local pointer-copy and
dereference provenance is propagated
over a strongly-connected-component condensation of the constraint graph;
dense acyclic component graphs are transitively reduced before fact
propagation, avoiding cubic repeated-edge scans. Channel sends escape reference
payloads; receives and channel ranges provide unknown storage without a
points-to proof. Builtins are
classified by effect: len, cap, min, and max are read-only; append, clear, and
delete mutate their first operand; copy mutates only its destination, with
possible overlap propagated through the may-alias relation.

The canonical automatic rewrite is offered only when type information proves
the destination is disjoint from both inputs: either storage is value-only or
the reference-backed destination is a non-escaped fresh local allocation whose
header address has never been exposed to an indirect rebind.
An addressable value array is not assumed disjoint from a slice or pointer that
can designate its whole storage, a field, or an element; mixed storage is safe
only when the types rule out that overlap.
Distinct compatible reference parameters are never assumed disjoint because
delayed stores could change later reads. Generated accumulators infer their
type from untyped zero literals, so shadowing a predeclared type name is safe.
The generated fast path first proves all array/slice indexes safe with len
checks, and is offered only when len resolves to the predeclared builtin at the
exact insertion point; any possible bounds panic uses the original scalar order
so defer, recover, and partial destination writes remain observable at the same
point.
Output-index names include the physical token offset (unaffected by //line
directives) and are made unique in the exact block, function-body, switch-case,
or select-case insertion scope and every visible outer, file-import, package,
and predeclared scope.
Labeled outer loops stay advisory because replacing one labeled statement with
several statements would not preserve label targets. A lexical function that
contains goto also stays advisory: hoisting the generated output index into the
surrounding statement list could otherwise turn a valid jump into an illegal
jump over a declaration.`,
		Before: `parallelForRows(n, func(a int) {
	row := a * n // this callback exclusively owns tmp[row:row+n]
	for j := 0; j < n; j++ {
		sum := 0.0
		for b := 0; b < n; b++ {
			sum += inner[row+b] * v[j*n+b] // inner row re-streamed per j
		}
		tmp[row+j] = sum
	}
})`,
		After: `parallelForRows(n, func(a int) {
	row := a * n
	j := 0
	for ; j+3 < n; j += 4 {
		var s0, s1, s2, s3 float64
		for b := 0; b < n; b++ { // ascending for every output
			innerAB := inner[row+b]
			s0 += innerAB * v[j*n+b]
			s1 += innerAB * v[(j+1)*n+b]
			s2 += innerAB * v[(j+2)*n+b]
			s3 += innerAB * v[(j+3)*n+b]
		}
		tmp[row+j], tmp[row+j+1] = s0, s1
		tmp[row+j+2], tmp[row+j+3] = s2, s3
	}
	for ; j < n; j++ { // unchanged scalar n%4 tail
		sum := 0.0
		for b := 0; b < n; b++ {
			sum += inner[row+b] * v[j*n+b]
		}
		tmp[row+j] = sum
	}
})`,
		MeasuredWin: `Issue #911, Apple M2 with GOMAXPROCS=12: the exact row-parallel
matrix product improved from 3.532182 ms to 3.024644 ms at n=128 (1.168x,
6/8 paired wins) and from 21.236037 ms to 17.336679 ms at n=256 (1.225x,
8/8 wins). Allocations were unchanged at 228/op and 356/op respectively.
Float64 raw-bit identity passed at n=5, 6, 7, 12, 48, and 64; a reversed-b
control failed the oracle.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6010",
		Doc:  "operand re-read per output element in an accumulator loop",
		Run:  runPS6010,
	},
})

func runPS6010(pass *analysis.Pass) (any, error) {
	return nil, ps6010Run(pass, nil)
}

type ps6010RunStats struct {
	indexedStatements           int
	dominanceQueries            int
	dominanceSteps              int
	pointerConstraints          int
	pointerPropagations         int
	pointerEdgeVisits           int
	externalTypesClassified     int
	externalCandidateClassScans int
	callableEdges               int
	callablePropagations        int
	callableCallTargets         int
}

// ps6010Run is also the production analyzer path exercised by the scaling
// regression. Optional stats charge index construction, bounded dominance
// lookups, unique pointer/callable constraints and propagations, and every
// constraint-graph edge scan; normal analyzer runs pass nil and carry no
// counters.
func ps6010Run(pass *analysis.Pass, stats *ps6010RunStats) error {
	definitionFacts := ps6010CollectDefinitionFacts(pass, nil, stats)
	for _, f := range pass.Files {
		astutil.WithStack(f, func(n ast.Node, stack []ast.Node) bool {
			iv, body := loopVarAndBody(n)
			if iv == "" || body == nil || containsLoop(body) {
				return true
			}
			outerLoop, in := astutil.InLoop(stack)
			if !in {
				return true
			}
			ov := outermostLoopVar(outerLoop)
			if ov == "" {
				return true
			}
			ivObject := ps6010LoopObject(pass, n)
			ovObject := ps6010LoopObject(pass, outerLoop)
			if ivObject != nil && ovObject != nil {
				if ivObject == ovObject {
					return true
				}
			} else if ov == iv {
				return true
			}
			hit := ps6010InvariantOperand(pass, body, ivObject, ovObject, iv, ov, definitionFacts)
			if hit == nil {
				return true
			}
			diag := analysis.Diagnostic{
				Pos:     hit.Pos(),
				End:     hit.End(),
				Message: "this operand does not vary with the output index " + ov + " but is re-streamed once per output element; unroll the output loop by 4 with independent accumulators to amortize the load (bit-identical per output)",
			}
			if fix := ps6010Fix(pass, outerLoop, n, definitionFacts); fix != nil {
				diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			}
			pass.Report(diag)
			return true
		})
	}
	return nil
}

func ps6010LoopObject(pass *analysis.Pass, loop ast.Node) types.Object {
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

func ps6010ExprDependsOn(pass *analysis.Pass, expression ast.Expr, object types.Object, fallback string) bool {
	found := false
	ast.Inspect(expression, func(node ast.Node) bool {
		if found {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		resolved := identObject(pass, identifier)
		if object != nil {
			if resolved == object {
				found = true
				return false
			}
			return true
		}
		if identifier.Name == fallback {
			found = true
			return false
		}
		return true
	})
	return found
}

type ps6010Dependency uint8

const (
	ps6010InnerDependency ps6010Dependency = 1 << iota
	ps6010OutputDependency
	ps6010UnknownDependency
)

// ps6010ExprDependencies returns every loop dimension carried by expression.
// State entries describe body-local derived values; a candidate referenced
// before its single definition is unknown rather than optimistically invariant.
func ps6010ExprDependencies(
	pass *analysis.Pass,
	expression ast.Expr,
	inner, outer types.Object,
	innerName, outerName string,
	writes map[types.Object]int,
	addressTaken map[types.Object]bool,
	state map[types.Object]ps6010Dependency,
	definitionFacts *ps6010DefinitionFacts,
) ps6010Dependency {
	return ps6010ExprDependenciesSeen(
		pass, expression, inner, outer, innerName, outerName, writes, addressTaken, state, definitionFacts,
		make(map[types.Object]bool),
	)
}

func ps6010ExprDependenciesSeen(
	pass *analysis.Pass,
	expression ast.Expr,
	inner, outer types.Object,
	innerName, outerName string,
	writes map[types.Object]int,
	addressTaken map[types.Object]bool,
	state map[types.Object]ps6010Dependency,
	definitionFacts *ps6010DefinitionFacts,
	resolving map[types.Object]bool,
) ps6010Dependency {
	return ps6010ExprDependenciesSeenMode(
		pass, expression, inner, outer, innerName, outerName, writes, addressTaken,
		state, definitionFacts, resolving, false,
	)
}

func ps6010ContainerDependencies(
	pass *analysis.Pass,
	expression ast.Expr,
	inner, outer types.Object,
	innerName, outerName string,
	writes map[types.Object]int,
	addressTaken map[types.Object]bool,
	state map[types.Object]ps6010Dependency,
	definitionFacts *ps6010DefinitionFacts,
) ps6010Dependency {
	return ps6010ExprDependenciesSeenMode(
		pass, expression, inner, outer, innerName, outerName, writes, addressTaken,
		state, definitionFacts, make(map[types.Object]bool), true,
	)
}

func ps6010ExprDependenciesSeenMode(
	pass *analysis.Pass,
	expression ast.Expr,
	inner, outer types.Object,
	innerName, outerName string,
	writes map[types.Object]int,
	addressTaken map[types.Object]bool,
	state map[types.Object]ps6010Dependency,
	definitionFacts *ps6010DefinitionFacts,
	resolving map[types.Object]bool,
	stableContainerRoots bool,
) ps6010Dependency {
	var dependencies ps6010Dependency
	ast.Inspect(expression, func(node ast.Node) bool {
		if unary, ok := node.(*ast.UnaryExpr); ok {
			switch unary.Op {
			case token.MUL:
				// A pointer value does not prove anything about the current pointee.
				// Without immutable points-to facts, dereferenced index data may
				// change with the output dimension through an alias. Still walk the
				// operand so explicit loop dependencies are not discarded.
				dependencies |= ps6010UnknownDependency
			case token.ARROW:
				// A receive may yield storage sent by any producer; the channel's
				// identity does not prove the received backing storage invariant.
				dependencies |= ps6010UnknownDependency
			}
			return true
		}
		if call, ok := node.(*ast.CallExpr); ok {
			if function, exists := pass.TypesInfo.Types[call.Fun]; exists && function.IsType() {
				return true
			}
			// An ordinary call can read package, receiver, or closure state that
			// is not represented by its arguments. Treat its result as unknown
			// until an explicit purity summary exists, while retaining explicit
			// dependencies carried by its receiver and arguments.
			dependencies |= ps6010UnknownDependency
			return true
		}
		identifier, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		resolved := identObject(pass, identifier)
		loopObject := false
		if inner != nil {
			if resolved == inner {
				dependencies |= ps6010InnerDependency
				loopObject = true
			}
		} else if identifier.Name == innerName {
			dependencies |= ps6010InnerDependency
			loopObject = true
		}
		if outer != nil {
			if resolved == outer {
				dependencies |= ps6010OutputDependency
				loopObject = true
			}
		} else if identifier.Name == outerName {
			dependencies |= ps6010OutputDependency
			loopObject = true
		}
		if resolved == nil {
			return true
		}
		if variable, ok := resolved.(*types.Var); ok && variable.Pkg() != nil &&
			variable.Parent() == variable.Pkg().Scope() {
			// A package variable may be mutated from another source file or
			// behind a helper call. Without an interprocedural immutable-global
			// proof it cannot establish an output-invariant index.
			dependencies |= ps6010UnknownDependency
		}
		if variable, ok := resolved.(*types.Var); ok &&
			ps6010TypeContainsReferenceStorage(variable.Type(), make(map[types.Type]bool)) &&
			(!stableContainerRoots || definitionFacts == nil || definitionFacts.containerUnstable[resolved]) {
			// A read through slice, map, pointer, interface, channel, function,
			// or a value containing one may observe writes through a distinct
			// alias. Retain explicit loop bits but do not use it as a stability
			// proof without points-to evidence.
			dependencies |= ps6010UnknownDependency
		}
		if addressTaken[resolved] || definitionFacts != nil && definitionFacts.addressTaken[resolved] {
			dependencies |= ps6010UnknownDependency
		}
		if derived, ok := state[resolved]; ok {
			dependencies |= derived
		} else if writes[resolved] > 0 {
			dependencies |= ps6010UnknownDependency
		} else if !loopObject && definitionFacts != nil {
			writeCount := definitionFacts.writes[resolved]
			definition := definitionFacts.definitions[resolved]
			if writeCount == 0 {
				return true
			}
			if writeCount != 1 || definition == nil || resolving[resolved] ||
				!definitionFacts.definitionDominates(resolved, identifier) {
				dependencies |= ps6010UnknownDependency
			} else {
				resolving[resolved] = true
				dependencies |= ps6010ExprDependenciesSeenMode(
					pass, definition, inner, outer, innerName, outerName,
					writes, addressTaken, state, definitionFacts, resolving, stableContainerRoots,
				)
				delete(resolving, resolved)
			}
		}
		return true
	})
	return dependencies
}

type ps6010DefinitionFacts struct {
	writes           map[types.Object]int
	bindingWrites    map[types.Object]int
	definitions      map[types.Object]ast.Expr
	definitionAt     map[types.Object]ast.Node
	addressTaken     map[types.Object]bool
	parents          map[ast.Node]ast.Node
	functionOwner    map[ast.Node]ast.Node
	statementOwner   map[ast.Node]ast.Stmt
	statementBlock   map[ast.Stmt]*ast.BlockStmt
	statementOrdinal map[ast.Stmt]int
	stats            *ps6010RunStats
	// A branch statement can bypass a syntactically earlier assignment. Until
	// this check carries a full CFG dominator tree, do not use enclosing
	// definition facts in a function containing goto/break/continue/fallthrough.
	controlTransfer map[ast.Node]bool
	functionHasGoto map[ast.Node]bool
	// Reference-backed roots become unstable when this function writes through
	// them or an explicit alias, or passes them to an opaque call.
	containerUnstable map[types.Object]bool
	containerEscaped  map[types.Object]bool
	freshStorage      map[types.Object]bool
	aliasClass        map[types.Object]types.Object
}

func (facts *ps6010DefinitionFacts) storageProvenDisjoint(left, right types.Object) bool {
	if facts == nil || left == nil || right == nil || left == right {
		return false
	}
	leftReference := ps6010ReferenceBackedObject(left)
	rightReference := ps6010ReferenceBackedObject(right)
	if !leftReference && !rightReference {
		// Safe Go copies distinct value-only objects instead of sharing them.
		return true
	}
	if !leftReference || !rightReference {
		// A slice or pointer may designate an addressable array, named array,
		// struct field, or array element. Mixed reference/value storage is disjoint
		// only when their types prove that no such target can be formed.
		return !ps6010TypesMayShareStorage(left.Type(), right.Type())
	}
	if leftClass, rightClass := facts.aliasClass[left], facts.aliasClass[right]; leftClass != nil && leftClass == rightClass {
		return false
	}
	safeFresh := func(object types.Object) bool {
		// Taking the address of a slice/map/interface header permits an indirect
		// rebind (possibly through multiple pointer aliases). Fresh backing storage
		// no longer proves what the object will reference at the rewritten loop.
		return facts.freshStorage[object] && !facts.containerEscaped[object] && !facts.addressTaken[object]
	}
	return safeFresh(left) || safeFresh(right)
}

func (facts *ps6010DefinitionFacts) definitionDominates(object types.Object, use ast.Node) bool {
	if facts == nil || object == nil || use == nil {
		return false
	}
	definition := facts.definitionAt[object]
	if definition == nil {
		return false
	}
	if facts.stats != nil {
		facts.stats.dominanceQueries++
	}
	function := facts.functionOwner[definition]
	if function == nil || function != facts.functionOwner[use] || facts.controlTransfer[function] {
		return false
	}
	definitionChild := facts.statementOwner[definition]
	block := facts.statementBlock[definitionChild]
	if definitionChild == nil || block == nil {
		return false
	}
	useChild := facts.statementOwner[use]
	for useChild != nil && facts.statementBlock[useChild] != block {
		if facts.stats != nil {
			facts.stats.dominanceSteps++
		}
		nestedBlock := facts.statementBlock[useChild]
		useChild = facts.statementOwner[nestedBlock]
	}
	if definitionChild == nil || useChild == nil || definitionChild == useChild {
		return false
	}
	definitionIndex, definitionKnown := facts.statementOrdinal[definitionChild]
	useIndex, useKnown := facts.statementOrdinal[useChild]
	return definitionKnown && useKnown && definitionIndex < useIndex
}

func ps6010EnclosingFunction(parents map[ast.Node]ast.Node, node ast.Node) ast.Node {
	for node != nil {
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			return node
		}
		node = parents[node]
	}
	return nil
}

func ps6010WrittenRootObject(pass *analysis.Pass, expression ast.Expr) types.Object {
	for {
		switch value := expression.(type) {
		case *ast.ParenExpr:
			expression = value.X
		case *ast.SelectorExpr:
			expression = value.X
		case *ast.IndexExpr:
			expression = value.X
		case *ast.IndexListExpr:
			expression = value.X
		case *ast.SliceExpr:
			expression = value.X
		case *ast.StarExpr:
			expression = value.X
		default:
			identifier, _ := expression.(*ast.Ident)
			if identifier == nil || identifier.Name == "_" {
				return nil
			}
			return identObject(pass, identifier)
		}
	}
}

func ps6010TypeContainsReferenceStorage(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil {
		return true
	}
	typ = types.Unalias(typ)
	if parameter, ok := typ.(*types.TypeParam); ok {
		// A type parameter is value-only only when a nonempty type restriction
		// proves every permitted value has an integer underlying type. Constraints
		// such as any, comparable, or ~int|~[]int remain reference-possible.
		return !ps6010IntegerTypeParameter(parameter, make(map[types.Type]bool))
	}
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch value := typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Chan, *types.Signature:
		return true
	case *types.Array:
		return ps6010TypeContainsReferenceStorage(value.Elem(), seen)
	case *types.Struct:
		for index := 0; index < value.NumFields(); index++ {
			if ps6010TypeContainsReferenceStorage(value.Field(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

// ps6010TypeContainsCallable reports whether a value can retain a function
// value whose hidden closure or receiver state is absent from its ordinary
// arguments. Interfaces and type parameters remain conservative because their
// dynamic value may itself be callable or contain callable descendants.
func ps6010TypeContainsCallable(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil {
		return true
	}
	typ = types.Unalias(typ)
	if _, parameter := typ.(*types.TypeParam); parameter {
		return true
	}
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch value := typ.Underlying().(type) {
	case *types.Signature, *types.Interface:
		return true
	case *types.Pointer, *types.Slice, *types.Array, *types.Chan:
		var element types.Type
		switch sequence := value.(type) {
		case *types.Pointer:
			element = sequence.Elem()
		case *types.Slice:
			element = sequence.Elem()
		case *types.Array:
			element = sequence.Elem()
		case *types.Chan:
			element = sequence.Elem()
		}
		return ps6010TypeContainsCallable(element, seen)
	case *types.Map:
		return ps6010TypeContainsCallable(value.Key(), seen) ||
			ps6010TypeContainsCallable(value.Elem(), seen)
	case *types.Struct:
		for index := 0; index < value.NumFields(); index++ {
			if ps6010TypeContainsCallable(value.Field(index).Type(), seen) {
				return true
			}
		}
	case *types.Tuple:
		for index := 0; index < value.Len(); index++ {
			if ps6010TypeContainsCallable(value.At(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

// ps6010TypeContainsNonCallableReferenceStorage separates ordinary aliasable
// storage from callable closure state. Exact functions already flow through
// the callable target graph; classifying a struct that contains only a pure
// callback as generic external storage would discard that precision and
// suppress valid findings. Interfaces and type parameters remain conservative
// because their dynamic representation may contain non-callable storage.
func ps6010TypeContainsNonCallableReferenceStorage(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil {
		return true
	}
	typ = types.Unalias(typ)
	if parameter, ok := typ.(*types.TypeParam); ok {
		return !ps6010IntegerTypeParameter(parameter, make(map[types.Type]bool))
	}
	if seen[typ] {
		return false
	}
	seen[typ] = true
	switch value := typ.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Interface, *types.Chan:
		return true
	case *types.Signature:
		return false
	case *types.Array:
		return ps6010TypeContainsNonCallableReferenceStorage(value.Elem(), seen)
	case *types.Struct:
		for index := 0; index < value.NumFields(); index++ {
			if ps6010TypeContainsNonCallableReferenceStorage(value.Field(index).Type(), seen) {
				return true
			}
		}
	}
	return false
}

func ps6010SequenceElementContainsReference(typ types.Type) bool {
	if typ == nil {
		return true
	}
	switch sequence := types.Unalias(typ).Underlying().(type) {
	case *types.Array:
		return ps6010TypeContainsReferenceStorage(sequence.Elem(), make(map[types.Type]bool))
	case *types.Slice:
		return ps6010TypeContainsReferenceStorage(sequence.Elem(), make(map[types.Type]bool))
	}
	return true
}

// ps6010IntegerTypeParameter recognizes constraints whose type set is narrowed
// to basic integer values. A single all-integer type element is sufficient:
// additional embedded constraints intersect (and therefore only narrow) it.
func ps6010IntegerTypeParameter(parameter *types.TypeParam, seen map[types.Type]bool) bool {
	if parameter == nil {
		return false
	}
	return ps6010IntegerConstraint(parameter.Constraint(), seen)
}

func ps6010IntegerConstraint(typ types.Type, seen map[types.Type]bool) bool {
	if typ == nil {
		return false
	}
	typ = types.Unalias(typ)
	if seen[typ] {
		return false
	}
	seen[typ] = true
	defer delete(seen, typ)
	if parameter, ok := typ.(*types.TypeParam); ok {
		return ps6010IntegerTypeParameter(parameter, seen)
	}
	if basic, ok := typ.Underlying().(*types.Basic); ok {
		return basic.Info()&types.IsInteger != 0
	}
	constraint, ok := typ.Underlying().(*types.Interface)
	if !ok {
		return false
	}
	constraint.Complete()
	for index := 0; index < constraint.NumEmbeddeds(); index++ {
		embedded := types.Unalias(constraint.EmbeddedType(index))
		if union, ok := embedded.(*types.Union); ok {
			if union.Len() == 0 {
				continue
			}
			allInteger := true
			for termIndex := 0; termIndex < union.Len(); termIndex++ {
				if !ps6010IntegerConstraint(union.Term(termIndex).Type(), seen) {
					allInteger = false
					break
				}
			}
			if allInteger {
				return true
			}
			continue
		}
		if ps6010IntegerConstraint(embedded, seen) {
			return true
		}
	}
	return false
}

type ps6010AliasSets struct {
	parent map[types.Object]types.Object
}

// ps6010StorageProvenance is a small may-point-to lattice. Concrete roots are
// joined through alias sets; external holds typed wildcards for storage that
// originates outside the current proof (for example a receive, map lookup, or
// opaque return). universal models unsafe.Pointer-derived writes.
type ps6010StorageProvenance struct {
	roots       map[types.Object]bool
	external    map[types.Type]bool
	classes     map[string]bool
	externalAll bool
	universal   bool
}

// ps6010StorageCompatibilityClasses converts an arbitrary storage-bearing type
// into a small set of hashable may-alias classes. Class intersection is a
// conservative replacement for repeatedly comparing every external type with
// every candidate object. Basic kinds remain distinct, while named scalars use
// their underlying kind so permitted pointer conversions such as
// *NamedFloat64 -> *float64 retain the alias edge.
func ps6010StorageCompatibilityClasses(typ types.Type) (map[string]bool, bool) {
	classes := make(map[string]bool)
	if typ == nil {
		return classes, true
	}
	qualifier := func(pkg *types.Package) string {
		if pkg == nil {
			return ""
		}
		return pkg.Path()
	}
	identity := func(current types.Type) string {
		if current == nil {
			return "unknown"
		}
		return types.TypeString(types.Unalias(current).Underlying(), qualifier)
	}
	seenAddressable := make(map[types.Type]bool)
	wildcard := false
	var addAddressable func(types.Type)
	addAddressable = func(current types.Type) {
		if current == nil {
			wildcard = true
			return
		}
		current = types.Unalias(current)
		if _, dynamic := current.(*types.TypeParam); dynamic {
			wildcard = true
			return
		}
		if seenAddressable[current] {
			return
		}
		seenAddressable[current] = true
		underlying := current.Underlying()
		switch value := underlying.(type) {
		case *types.Interface:
			wildcard = true
		case *types.Basic:
			if value.Kind() == types.UnsafePointer {
				wildcard = true
				return
			}
			classes["address:basic:"+value.Name()] = true
		case *types.Array:
			classes["address:array:"+identity(current)] = true
			addAddressable(value.Elem())
		case *types.Struct:
			classes["address:struct:"+identity(current)] = true
			if value.NumFields() == 0 {
				classes["address:empty-struct"] = true
			}
			for index := 0; index < value.NumFields(); index++ {
				addAddressable(value.Field(index).Type())
			}
		case *types.Pointer:
			classes["address:pointer:"+identity(value.Elem())] = true
			addAddressable(value.Elem())
		default:
			classes["address:"+identity(current)] = true
		}
	}
	seenReference := make(map[types.Type]bool)
	var collect func(types.Type)
	collect = func(current types.Type) {
		if current == nil {
			wildcard = true
			return
		}
		current = types.Unalias(current)
		if _, dynamic := current.(*types.TypeParam); dynamic {
			wildcard = true
			return
		}
		if seenReference[current] {
			return
		}
		seenReference[current] = true
		switch value := current.Underlying().(type) {
		case *types.Interface, *types.Signature:
			wildcard = true
		case *types.Basic:
			if value.Kind() == types.UnsafePointer {
				wildcard = true
				return
			}
			addAddressable(current)
		case *types.Pointer:
			addAddressable(value.Elem())
			collect(value.Elem())
		case *types.Slice:
			classes["slice:"+identity(value.Elem())] = true
			addAddressable(value.Elem())
			collect(value.Elem())
		case *types.Map:
			classes["map:"+identity(current)] = true
			if ps6010TypeContainsReferenceStorage(value.Key(), make(map[types.Type]bool)) {
				collect(value.Key())
			}
			if ps6010TypeContainsReferenceStorage(value.Elem(), make(map[types.Type]bool)) {
				collect(value.Elem())
			}
		case *types.Chan:
			classes["chan:"+identity(value.Elem())] = true
			if ps6010TypeContainsReferenceStorage(value.Elem(), make(map[types.Type]bool)) {
				collect(value.Elem())
			}
		case *types.Array:
			addAddressable(current)
			if ps6010TypeContainsReferenceStorage(value.Elem(), make(map[types.Type]bool)) {
				collect(value.Elem())
			}
		case *types.Struct:
			addAddressable(current)
			for index := 0; index < value.NumFields(); index++ {
				field := value.Field(index).Type()
				if ps6010TypeContainsReferenceStorage(field, make(map[types.Type]bool)) {
					collect(field)
				}
			}
		}
	}
	collect(typ)
	return classes, wildcard
}

func (sets *ps6010AliasSets) add(object types.Object) {
	if object != nil {
		if _, exists := sets.parent[object]; !exists {
			sets.parent[object] = object
		}
	}
}

func (sets *ps6010AliasSets) find(object types.Object) types.Object {
	parent, exists := sets.parent[object]
	if !exists {
		return nil
	}
	if parent != object {
		sets.parent[object] = sets.find(parent)
	}
	return sets.parent[object]
}

func (sets *ps6010AliasSets) union(left, right types.Object) {
	sets.add(left)
	sets.add(right)
	leftRoot, rightRoot := sets.find(left), sets.find(right)
	if leftRoot != nil && rightRoot != nil && leftRoot != rightRoot {
		sets.parent[rightRoot] = leftRoot
	}
}

func ps6010ReferenceBackedObject(object types.Object) bool {
	return object != nil && ps6010TypeContainsReferenceStorage(object.Type(), make(map[types.Type]bool))
}

// ps6010ReferenceMayPointIntoValue reports whether a reference-bearing value
// can designate storage owned by an addressable value-only object. Safe Go can
// form slices from array storage and pointers to a whole object, field, or array
// element. Interfaces and type parameters remain conservative. Maps, channels,
// and functions do not by themselves point into separately addressable value
// storage.
func ps6010ReferenceMayPointIntoValue(reference, value types.Type) bool {
	var valueContains func(types.Type, func(types.Type) bool, map[types.Type]bool) bool
	valueContains = func(current types.Type, matches func(types.Type) bool, seen map[types.Type]bool) bool {
		if current == nil {
			return true
		}
		current = types.Unalias(current)
		if seen[current] {
			return false
		}
		seen[current] = true
		defer delete(seen, current)
		if matches(current) {
			return true
		}
		switch underlying := current.Underlying().(type) {
		case *types.Array:
			return valueContains(underlying.Elem(), matches, seen)
		case *types.Struct:
			for index := 0; index < underlying.NumFields(); index++ {
				if valueContains(underlying.Field(index).Type(), matches, seen) {
					return true
				}
			}
		}
		return false
	}

	var referenceTargetsValue func(types.Type, map[types.Type]bool) bool
	referenceTargetsValue = func(current types.Type, seen map[types.Type]bool) bool {
		if current == nil {
			return true
		}
		current = types.Unalias(current)
		if _, parameter := current.(*types.TypeParam); parameter {
			return true
		}
		if seen[current] {
			return false
		}
		seen[current] = true
		defer delete(seen, current)
		switch underlying := current.Underlying().(type) {
		case *types.Interface:
			return true
		case *types.Slice:
			return valueContains(value, func(candidate types.Type) bool {
				array, ok := candidate.Underlying().(*types.Array)
				return ok && types.Identical(types.Unalias(array.Elem()), types.Unalias(underlying.Elem()))
			}, make(map[types.Type]bool))
		case *types.Pointer:
			return valueContains(value, func(candidate types.Type) bool {
				candidatePointer := types.NewPointer(candidate)
				return types.AssignableTo(candidatePointer, current) || types.ConvertibleTo(candidatePointer, current)
			}, make(map[types.Type]bool))
		case *types.Array:
			return referenceTargetsValue(underlying.Elem(), seen)
		case *types.Struct:
			for index := 0; index < underlying.NumFields(); index++ {
				if referenceTargetsValue(underlying.Field(index).Type(), seen) {
					return true
				}
			}
		}
		return false
	}
	return referenceTargetsValue(reference, make(map[types.Type]bool))
}

func ps6010TypesMayShareStorage(left, right types.Type) bool {
	leftReference := ps6010TypeContainsReferenceStorage(left, make(map[types.Type]bool))
	rightReference := ps6010TypeContainsReferenceStorage(right, make(map[types.Type]bool))
	switch {
	case !leftReference && !rightReference:
		return false
	case leftReference && !rightReference:
		return ps6010ReferenceMayPointIntoValue(left, right)
	case !leftReference && rightReference:
		return ps6010ReferenceMayPointIntoValue(right, left)
	default:
		return ps6010ReferenceTypesMayAlias(left, right)
	}
}

func ps6010ReferenceTypesMayAlias(left, right types.Type) bool {
	type typePair struct {
		left  types.Type
		right types.Type
	}
	seen := make(map[typePair]bool)
	var mayAlias func(types.Type, types.Type) bool
	mayAlias = func(left, right types.Type) bool {
		if left == nil || right == nil {
			return true
		}
		left, right = types.Unalias(left), types.Unalias(right)
		if _, parameter := left.(*types.TypeParam); parameter {
			return true
		}
		if _, parameter := right.(*types.TypeParam); parameter {
			return true
		}
		if !ps6010TypeContainsReferenceStorage(left, make(map[types.Type]bool)) ||
			!ps6010TypeContainsReferenceStorage(right, make(map[types.Type]bool)) {
			return false
		}
		pair := typePair{left: left, right: right}
		if seen[pair] {
			return false
		}
		seen[pair] = true
		leftUnderlying, rightUnderlying := left.Underlying(), right.Underlying()
		if _, dynamic := leftUnderlying.(*types.Interface); dynamic {
			return true
		}
		if _, dynamic := rightUnderlying.(*types.Interface); dynamic {
			return true
		}
		// A function value can retain arbitrary reference-bearing captures that
		// are absent from its signature. Calling a callback parameter can therefore
		// mutate any compatible reference parameter supplied by the same caller.
		if _, function := leftUnderlying.(*types.Signature); function {
			return true
		}
		if _, function := rightUnderlying.(*types.Signature); function {
			return true
		}
		// Slices, pointers to their elements, and pointers to compatible arrays may
		// all designate the same subobject. These types need not be assignable or
		// convertible to each other: backing[:], &backing, and &backing[0] form the
		// aliases directly. Check this before descending into value-only elements.
		if ps6010ReferenceSubobjectsMayAlias(left, right) {
			return true
		}
		compatible := types.AssignableTo(left, right) || types.AssignableTo(right, left) ||
			types.ConvertibleTo(left, right) || types.ConvertibleTo(right, left)
		switch leftUnderlying.(type) {
		case *types.Slice:
			_, sameKind := rightUnderlying.(*types.Slice)
			if sameKind && compatible {
				return true
			}
		case *types.Map:
			_, sameKind := rightUnderlying.(*types.Map)
			if sameKind && compatible {
				return true
			}
		case *types.Pointer:
			_, sameKind := rightUnderlying.(*types.Pointer)
			if sameKind && compatible {
				return true
			}
		case *types.Chan:
			_, sameKind := rightUnderlying.(*types.Chan)
			if sameKind && compatible {
				return true
			}
		}

		children := func(typ types.Type) []types.Type {
			switch value := typ.Underlying().(type) {
			case *types.Array:
				return []types.Type{value.Elem()}
			case *types.Struct:
				result := make([]types.Type, 0, value.NumFields())
				for index := 0; index < value.NumFields(); index++ {
					result = append(result, value.Field(index).Type())
				}
				return result
			case *types.Pointer:
				return []types.Type{value.Elem()}
			case *types.Slice:
				return []types.Type{value.Elem()}
			case *types.Map:
				return []types.Type{value.Key(), value.Elem()}
			case *types.Chan:
				return []types.Type{value.Elem()}
			}
			return nil
		}
		for _, child := range children(left) {
			if mayAlias(child, right) {
				return true
			}
		}
		for _, child := range children(right) {
			if mayAlias(left, child) {
				return true
			}
		}
		return false
	}
	return mayAlias(left, right)
}

func ps6010ReferenceSubobjectsMayAlias(left, right types.Type) bool {
	elementType := func(typ types.Type) (types.Type, bool) {
		if typ == nil {
			return nil, false
		}
		switch value := types.Unalias(typ).Underlying().(type) {
		case *types.Slice:
			return types.Unalias(value.Elem()), true
		case *types.Pointer:
			element := types.Unalias(value.Elem())
			if array, ok := element.Underlying().(*types.Array); ok {
				return types.Unalias(array.Elem()), true
			}
			return element, true
		}
		return nil, false
	}
	leftElement, leftOK := elementType(left)
	rightElement, rightOK := elementType(right)
	if !leftOK || !rightOK {
		return false
	}
	if types.Identical(leftElement, rightElement) {
		return true
	}
	leftPointer := types.NewPointer(leftElement)
	rightPointer := types.NewPointer(rightElement)
	return types.AssignableTo(leftPointer, rightPointer) || types.AssignableTo(rightPointer, leftPointer) ||
		types.ConvertibleTo(leftPointer, rightPointer) || types.ConvertibleTo(rightPointer, leftPointer)
}

func ps6010BoundedSequence(pass *analysis.Pass, expression ast.Expr) bool {
	if pass == nil || expression == nil {
		return false
	}
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return false
	}
	switch types.Unalias(typ).Underlying().(type) {
	case *types.Array, *types.Slice:
		return true
	}
	return false
}

func ps6010BuiltinName(pass *analysis.Pass, call *ast.CallExpr) string {
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok {
		return ""
	}
	builtin, ok := identObject(pass, identifier).(*types.Builtin)
	if !ok {
		return ""
	}
	return builtin.Name()
}

func ps6010FreshStorageExpression(pass *analysis.Pass, expression ast.Expr) bool {
	switch value := expression.(type) {
	case *ast.ParenExpr:
		return ps6010FreshStorageExpression(pass, value.X)
	case *ast.CompositeLit:
		return ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(value), make(map[types.Type]bool))
	case *ast.UnaryExpr:
		return value.Op == token.AND && ps6010FreshStorageExpression(pass, value.X)
	case *ast.CallExpr:
		name := ps6010BuiltinName(pass, value)
		return name == "make" || name == "new"
	}
	return false
}

func ps6010Unparen(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
	}
}

// ps6010ReferenceAliasSource returns the original storage root for expressions
// that preserve reference identity. In particular, converting []T to a named
// slice type does not copy the backing array. Value-only conversions are not
// alias edges.
func ps6010ReferenceAliasSource(pass *analysis.Pass, expression ast.Expr) types.Object {
	expression = ps6010Unparen(expression)
	if object := ps6010WrittenRootObject(pass, expression); object != nil {
		return object
	}
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		if value.Op == token.AND {
			return ps6010WrittenRootObject(pass, value.X)
		}
		return nil
	case *ast.TypeAssertExpr:
		return ps6010ReferenceAliasSource(pass, value.X)
	}
	call, ok := expression.(*ast.CallExpr)
	if !ok {
		return nil
	}
	if ps6010BuiltinName(pass, call) == "append" && len(call.Args) != 0 {
		return ps6010ReferenceAliasSource(pass, call.Args[0])
	}
	if len(call.Args) != 1 {
		return nil
	}
	function, exists := pass.TypesInfo.Types[call.Fun]
	if !exists || !function.IsType() ||
		!ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(call), make(map[types.Type]bool)) ||
		!ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(call.Args[0]), make(map[types.Type]bool)) {
		return nil
	}
	return ps6010ReferenceAliasSource(pass, call.Args[0])
}

// ps6010StoredRoots returns source objects whose storage identity is retained
// when expression is put in another variable, field, element, composite value,
// or interface. It deliberately does not treat ordinary call results
// as aliases; callers mark those destinations unknown instead.
func ps6010StoredRoots(pass *analysis.Pass, expression ast.Expr) map[types.Object]bool {
	roots := make(map[types.Object]bool)
	var collect func(ast.Expr)
	collect = func(current ast.Expr) {
		if current == nil {
			return
		}
		current = ps6010Unparen(current)
		referenceValue := ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(current), make(map[types.Type]bool))
		addRoot := func() {
			if object := ps6010WrittenRootObject(pass, current); object != nil && referenceValue {
				roots[object] = true
			}
		}
		switch value := current.(type) {
		case *ast.Ident:
			addRoot()
		case *ast.IndexExpr:
			addRoot()
			if referenceValue {
				// A lookup can return a reference-bearing value from an unnamed
				// aggregate, for example map[int]holder{0: {values: a}}[0].
				collect(value.X)
			}
		case *ast.IndexListExpr:
			addRoot()
			if referenceValue {
				collect(value.X)
			}
		case *ast.SliceExpr:
			addRoot()
			if referenceValue {
				collect(value.X)
			}
		case *ast.StarExpr:
			addRoot()
			if referenceValue {
				collect(value.X)
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if referenceValue {
				// Retain the concrete method identity as well as its receiver. This
				// lets the interprocedural effect graph follow method values stored in
				// locals or aggregate fields without confusing an interface method
				// declaration with its dynamic implementation.
				if selection != nil {
					if function, ok := selection.Obj().(*types.Func); ok &&
						!ps6010DynamicMethodDispatch(pass, value) {
						roots[function] = true
					}
				}
				// A method expression T.M retains the method but no receiver value;
				// treating the type name T as storage would merge every method on T
				// into one callable component.
				if selection != nil && selection.Kind() == types.MethodExpr {
					break
				}
				addRoot()
				// A reference field or method value retains its receiver even when
				// that receiver is an unnamed conversion/composite with no root.
				collect(value.X)
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if object := ps6010WrittenRootObject(pass, value.X); object != nil {
					roots[object] = true
				}
				// An address-of composite retains all reference descendants even
				// though the literal itself has no named root. This connects forms
				// such as p := &holder{values: a} to a before a stored pointer-
				// receiver method value is invoked through p.
				collect(value.X)
			}
		case *ast.TypeAssertExpr:
			collect(value.X)
		case *ast.CallExpr:
			if object := ps6010ReferenceAliasSource(pass, value); object != nil {
				roots[object] = true
			}
			// A reference-preserving conversion copies reference headers held in
			// its argument. Follow the complete argument expression rather than
			// asking only for a named root, because inline struct/array/map values
			// can retain descendants without having a root object themselves.
			if len(value.Args) == 1 {
				if function, exists := pass.TypesInfo.Types[value.Fun]; exists && function.IsType() &&
					referenceValue && ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(value.Args[0]), make(map[types.Type]bool)) {
					collect(value.Args[0])
				}
			}
			if ps6010BuiltinName(pass, value) == "append" {
				// append retains both its destination backing storage and every
				// reference-bearing element copied into that storage. This also
				// covers a variadic source: append(dst, src...) copies the source
				// elements, whose recursively retained roots are reachable through
				// src even though src itself is the sole AST argument.
				if len(value.Args) != 0 {
					collect(value.Args[0])
				}
				for index, argument := range value.Args[1:] {
					if value.Ellipsis.IsValid() && index == len(value.Args)-2 &&
						!ps6010SequenceElementContainsReference(pass.TypesInfo.TypeOf(value.Args[0])) {
						continue
					}
					collect(argument)
				}
			}
		case *ast.CompositeLit:
			_, mapLiteral := types.Unalias(pass.TypesInfo.TypeOf(value)).Underlying().(*types.Map)
			for _, element := range value.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					if mapLiteral {
						collect(pair.Key)
					}
					collect(pair.Value)
					continue
				}
				collect(element)
			}
		}
	}
	collect(expression)
	return roots
}

func ps6010ReceiveExpression(expression ast.Expr) bool {
	expression = ps6010Unparen(expression)
	receive, ok := expression.(*ast.UnaryExpr)
	return ok && receive.Op == token.ARROW
}

func ps6010ChannelType(typ types.Type) bool {
	if typ == nil {
		return false
	}
	_, ok := types.Unalias(typ).Underlying().(*types.Chan)
	return ok
}

// ps6010ExpressionResultType returns the type assigned to one position of a
// possibly multi-valued expression. Ordinary multi-result calls expose a
// *types.Tuple; comma-ok map indexes, assertions, and receives instead retain
// their value type and advertise the synthetic bool through HasOk.
func ps6010ExpressionResultType(pass *analysis.Pass, expression ast.Expr, index int) types.Type {
	if pass == nil || pass.TypesInfo == nil || expression == nil || index < 0 {
		return nil
	}
	value, exists := pass.TypesInfo.Types[expression]
	if !exists || value.Type == nil {
		return nil
	}
	if tuple, ok := types.Unalias(value.Type).(*types.Tuple); ok {
		if index < tuple.Len() {
			return tuple.At(index).Type()
		}
		return nil
	}
	if index == 0 {
		return value.Type
	}
	if index == 1 && value.HasOk() {
		return types.Typ[types.Bool]
	}
	return nil
}

func ps6010ResultComesFromExternalStorage(pass *analysis.Pass, expression ast.Expr, index int) bool {
	if index < 0 {
		return false
	}
	expression = ps6010Unparen(expression)
	switch value := expression.(type) {
	case *ast.UnaryExpr:
		return index == 0 && value.Op == token.ARROW
	case *ast.IndexExpr:
		_, ok := types.Unalias(pass.TypesInfo.TypeOf(value.X)).Underlying().(*types.Map)
		return index == 0 && ok
	case *ast.TypeAssertExpr:
		return index == 0
	case *ast.CallExpr:
		if function, exists := pass.TypesInfo.Types[value.Fun]; exists && function.IsType() {
			return false
		}
		switch ps6010BuiltinName(pass, value) {
		case "make", "new", "append", "len", "cap", "min", "max", "clear", "delete", "copy":
			return false
		}
		return true
	}
	return false
}

func ps6010CalledFunctionObject(pass *analysis.Pass, expression ast.Expr) *types.Func {
	expression = ps6010Unparen(expression)
	switch value := expression.(type) {
	case *ast.Ident:
		function, _ := identObject(pass, value).(*types.Func)
		return function
	case *ast.SelectorExpr:
		if selection := pass.TypesInfo.Selections[value]; selection != nil {
			function, _ := selection.Obj().(*types.Func)
			return function
		}
		function, _ := pass.TypesInfo.Uses[value.Sel].(*types.Func)
		return function
	case *ast.IndexExpr:
		return ps6010CalledFunctionObject(pass, value.X)
	case *ast.IndexListExpr:
		return ps6010CalledFunctionObject(pass, value.X)
	}
	return nil
}

// ps6010DynamicMethodDispatch distinguishes a statically selected concrete
// method from interface/type-parameter dispatch. The selected method object is
// declared by the interface (and may therefore belong to an imported package),
// but the implementation invoked at run time may be a method in this package
// with hidden package or unsafe effects.
func ps6010DynamicMethodDispatch(pass *analysis.Pass, expression ast.Expr) bool {
	if pass == nil || pass.TypesInfo == nil {
		return true
	}
	expression = ps6010Unparen(expression)
	for {
		switch value := expression.(type) {
		case *ast.IndexExpr:
			expression = ps6010Unparen(value.X)
		case *ast.IndexListExpr:
			expression = ps6010Unparen(value.X)
		default:
			goto unwrapped
		}
	}
unwrapped:
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	if selection == nil {
		return false
	}
	dynamicType := func(typ types.Type) bool {
		if typ == nil {
			return true
		}
		typ = types.Unalias(typ)
		if _, parameter := typ.(*types.TypeParam); parameter {
			return true
		}
		_, interfaceType := typ.Underlying().(*types.Interface)
		return interfaceType
	}
	if dynamicType(selection.Recv()) {
		return true
	}
	if signature, ok := selection.Obj().Type().(*types.Signature); ok && signature.Recv() != nil {
		return dynamicType(signature.Recv().Type())
	}
	return false
}

// ps6010CollectDefinitionFacts lets an inner-loop proof follow a single,
// enclosing local definition such as row := a*n or offset := o. Objects are
// type-resolved, so shadowed names cannot leak facts across scopes. Multiple
// writes, missing RHS mappings, and address escapes remain unknown.
func ps6010CollectDefinitionFacts(pass *analysis.Pass, root ast.Node, stats *ps6010RunStats) *ps6010DefinitionFacts {
	facts := &ps6010DefinitionFacts{
		writes:            make(map[types.Object]int),
		bindingWrites:     make(map[types.Object]int),
		definitions:       make(map[types.Object]ast.Expr),
		definitionAt:      make(map[types.Object]ast.Node),
		addressTaken:      make(map[types.Object]bool),
		parents:           make(map[ast.Node]ast.Node),
		functionOwner:     make(map[ast.Node]ast.Node),
		statementOwner:    make(map[ast.Node]ast.Stmt),
		statementBlock:    make(map[ast.Stmt]*ast.BlockStmt),
		statementOrdinal:  make(map[ast.Stmt]int),
		stats:             stats,
		controlTransfer:   make(map[ast.Node]bool),
		functionHasGoto:   make(map[ast.Node]bool),
		containerUnstable: make(map[types.Object]bool),
		containerEscaped:  make(map[types.Object]bool),
		freshStorage:      make(map[types.Object]bool),
		aliasClass:        make(map[types.Object]types.Object),
	}
	roots := make([]ast.Node, 0, len(pass.Files))
	for _, file := range pass.Files {
		roots = append(roots, file)
	}
	if len(roots) == 0 && root != nil {
		roots = append(roots, root)
	}
	inspectAll := func(visitor func(ast.Node) bool) {
		for _, candidate := range roots {
			ast.Inspect(candidate, visitor)
		}
	}
	var stack []ast.Node
	inspectAll(func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) != 0 {
			parent := stack[len(stack)-1]
			facts.parents[node] = parent
			facts.functionOwner[node] = facts.functionOwner[parent]
			facts.statementOwner[node] = facts.statementOwner[parent]
			if block, ok := parent.(*ast.BlockStmt); ok {
				if statement, ok := node.(ast.Stmt); ok {
					facts.statementOwner[node] = statement
					facts.statementBlock[statement] = block
				}
			}
		}
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			facts.functionOwner[node] = node
		}
		if block, ok := node.(*ast.BlockStmt); ok {
			for index, statement := range block.List {
				facts.statementOrdinal[statement] = index
			}
			if stats != nil {
				stats.indexedStatements += len(block.List)
			}
		}
		stack = append(stack, node)
		return true
	})
	writtenInFunctions := make(map[types.Object]map[ast.Node]bool)
	recordWriteFunction := func(object types.Object, site ast.Node) {
		if object == nil {
			return
		}
		function := facts.functionOwner[site]
		if function == nil {
			return
		}
		if writtenInFunctions[object] == nil {
			writtenInFunctions[object] = make(map[ast.Node]bool)
		}
		writtenInFunctions[object][function] = true
	}
	record := func(identifier *ast.Ident, expression ast.Expr, site ast.Node) {
		if identifier == nil || identifier.Name == "_" {
			return
		}
		object := identObject(pass, identifier)
		if object == nil {
			return
		}
		facts.writes[object]++
		facts.bindingWrites[object]++
		recordWriteFunction(object, site)
		if expression != nil {
			facts.definitions[object] = expression
			facts.definitionAt[object] = site
		}
	}
	recordMemberWrite := func(expression ast.Expr, site ast.Node) {
		if object := ps6010WrittenRootObject(pass, expression); object != nil {
			facts.writes[object]++
			recordWriteFunction(object, site)
		}
	}
	inspectAll(func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.BranchStmt:
			if function := ps6010EnclosingFunction(facts.parents, value); function != nil {
				facts.controlTransfer[function] = true
				if value.Tok == token.GOTO {
					facts.functionHasGoto[function] = true
				}
			}
		case *ast.AssignStmt:
			for index, lhs := range value.Lhs {
				identifier, _ := lhs.(*ast.Ident)
				if identifier == nil {
					recordMemberWrite(lhs, value)
					continue
				}
				var expression ast.Expr
				if (value.Tok == token.DEFINE || value.Tok == token.ASSIGN) && len(value.Lhs) == len(value.Rhs) {
					expression = value.Rhs[index]
				}
				record(identifier, expression, value)
			}
		case *ast.ValueSpec:
			for index, identifier := range value.Names {
				var expression ast.Expr
				if len(value.Names) == len(value.Values) {
					expression = value.Values[index]
				}
				record(identifier, expression, value)
			}
		case *ast.RangeStmt:
			identifier, _ := value.Key.(*ast.Ident)
			record(identifier, nil, value)
			identifier, _ = value.Value.(*ast.Ident)
			record(identifier, nil, value)
		case *ast.IncDecStmt:
			identifier, _ := value.X.(*ast.Ident)
			if identifier != nil {
				record(identifier, nil, value)
			} else {
				recordMemberWrite(value.X, value)
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if object := ps6010WrittenRootObject(pass, value.X); object != nil {
					facts.addressTaken[object] = true
				}
			}
		case *ast.SliceExpr:
			// Slicing an array takes its address implicitly and exposes the
			// original backing storage to writes through the resulting slice.
			if ps6010SliceExposesArrayStorage(pass, value.X) {
				if object := ps6010WrittenRootObject(pass, value.X); object != nil {
					facts.addressTaken[object] = true
				}
			}
		case *ast.SelectorExpr:
			selection := pass.TypesInfo.Selections[value]
			if selection == nil {
				break
			}
			signature, ok := selection.Obj().Type().(*types.Signature)
			if !ok || signature.Recv() == nil {
				break
			}
			if _, ok := types.Unalias(signature.Recv().Type()).(*types.Pointer); !ok {
				break
			}
			if object := ps6010WrittenRootObject(pass, value.X); object != nil {
				facts.addressTaken[object] = true
			}
		}
		return true
	})

	// Track compatible reference parameters and explicit reference copies before
	// classifying writes. Parameters with compatible storage types may overlap at
	// the call site; local fresh allocations start in distinct components.
	aliases := &ps6010AliasSets{parent: make(map[types.Object]types.Object)}
	allParameterObjects := func(fields ...*ast.FieldList) []types.Object {
		var objects []types.Object
		for _, list := range fields {
			if list == nil {
				continue
			}
			for _, field := range list.List {
				for _, name := range field.Names {
					object := identObject(pass, name)
					if object != nil {
						objects = append(objects, object)
					}
				}
			}
		}
		return objects
	}
	referenceObjects := func(objects []types.Object) []types.Object {
		references := make([]types.Object, 0, len(objects))
		for _, object := range objects {
			if ps6010ReferenceBackedObject(object) {
				aliases.add(object)
				references = append(references, object)
			}
		}
		return references
	}
	unionParameters := func(objects []types.Object) {
		for left := range objects {
			for right := 0; right < left; right++ {
				if ps6010ReferenceTypesMayAlias(objects[left].Type(), objects[right].Type()) {
					aliases.union(objects[left], objects[right])
				}
			}
		}
	}
	parametersByFunction := make(map[ast.Node][]types.Object)
	allParametersByFunction := make(map[ast.Node][]types.Object)
	inspectAll(func(node ast.Node) bool {
		switch function := node.(type) {
		case *ast.FuncDecl:
			allParameters := allParameterObjects(function.Recv, function.Type.Params)
			parameters := referenceObjects(allParameters)
			parametersByFunction[function] = parameters
			allParametersByFunction[function] = allParameters
			unionParameters(parameters)
		case *ast.FuncLit:
			allParameters := allParameterObjects(function.Type.Params)
			parameters := referenceObjects(allParameters)
			parametersByFunction[function] = parameters
			allParametersByFunction[function] = allParameters
			unionParameters(parameters)
		}
		return true
	})
	// Index reference-bearing objects captured by each function literal. A
	// receive inside a closure may yield storage backed by an enclosing parameter
	// or local even though that object is not one of the literal's own params.
	definitionFunction := make(map[types.Object]ast.Node)
	objectsByFunction := make(map[ast.Node][]types.Object)
	for identifier, object := range pass.TypesInfo.Defs {
		if object != nil {
			function := facts.functionOwner[identifier]
			definitionFunction[object] = function
			if function != nil {
				objectsByFunction[function] = append(objectsByFunction[function], object)
			}
		}
	}
	capturedByFunction := make(map[ast.Node]map[types.Object]bool)
	for identifier, object := range pass.TypesInfo.Uses {
		function := facts.functionOwner[identifier]
		owner := definitionFunction[object]
		if function == nil || owner == nil || function == owner || object == nil {
			continue
		}
		captured := capturedByFunction[function]
		if captured == nil {
			captured = make(map[types.Object]bool)
			capturedByFunction[function] = captured
		}
		captured[object] = true
	}
	escapedByStore := make(map[types.Object]bool)
	unknownFromReceive := make(map[types.Object]bool)
	externalByObject := make(map[types.Object]map[types.Type]bool)
	universalByObject := make(map[types.Object]bool)
	type unknownReceive struct {
		function ast.Node
		typ      types.Type
	}
	var unknownReceives []unknownReceive
	isPackageObject := func(object types.Object) bool {
		variable, ok := object.(*types.Var)
		return ok && variable.Pkg() != nil && variable.Parent() == variable.Pkg().Scope()
	}
	addExternal := func(object types.Object, typ types.Type) {
		if object == nil || typ == nil ||
			!ps6010TypeContainsReferenceStorage(typ, make(map[types.Type]bool)) {
			return
		}
		typesByObject := externalByObject[object]
		if typesByObject == nil {
			typesByObject = make(map[types.Type]bool)
			externalByObject[object] = typesByObject
		}
		typesByObject[types.Unalias(typ)] = true
	}
	unsafePointer := func(typ types.Type) bool {
		if typ == nil {
			return false
		}
		basic, ok := types.Unalias(typ).Underlying().(*types.Basic)
		return ok && basic.Kind() == types.UnsafePointer
	}
	// provenanceOf deliberately records both named roots and storage whose
	// identity cannot be represented by a source object. The latter is a typed
	// wildcard: an opaque []T result may overlap any compatible non-fresh []T,
	// *T, or *[N]T storage, without poisoning unrelated element types.
	provenanceOf := func(expression ast.Expr) ps6010StorageProvenance {
		provenance := ps6010StorageProvenance{
			roots:    ps6010StoredRoots(pass, expression),
			external: make(map[types.Type]bool),
		}
		joinObject := func(object types.Object) {
			if object == nil {
				return
			}
			for typ := range externalByObject[object] {
				provenance.external[typ] = true
			}
			provenance.universal = provenance.universal || universalByObject[object]
			if isPackageObject(object) && ps6010ReferenceBackedObject(object) {
				provenance.external[types.Unalias(object.Type())] = true
			}
		}
		for object := range provenance.roots {
			joinObject(object)
		}
		ast.Inspect(expression, func(node ast.Node) bool {
			current, ok := node.(ast.Expr)
			if !ok {
				return true
			}
			typ := pass.TypesInfo.TypeOf(current)
			if unsafePointer(typ) {
				provenance.universal = true
			}
			switch value := current.(type) {
			case *ast.Ident:
				joinObject(identObject(pass, value))
			case *ast.SelectorExpr:
				// A package-qualified variable is represented by Sel, whereas the
				// syntactic root is the package name and has no storage identity.
				if pass.TypesInfo.Selections[value] == nil {
					joinObject(pass.TypesInfo.Uses[value.Sel])
				}
			case *ast.UnaryExpr:
				if value.Op == token.ARROW &&
					ps6010TypeContainsReferenceStorage(typ, make(map[types.Type]bool)) {
					provenance.external[types.Unalias(typ)] = true
				}
			case *ast.IndexExpr:
				if _, ok := types.Unalias(pass.TypesInfo.TypeOf(value.X)).Underlying().(*types.Map); ok &&
					ps6010TypeContainsReferenceStorage(typ, make(map[types.Type]bool)) {
					provenance.external[types.Unalias(typ)] = true
				}
			case *ast.TypeAssertExpr:
				if ps6010TypeContainsReferenceStorage(typ, make(map[types.Type]bool)) {
					provenance.external[types.Unalias(typ)] = true
				}
			case *ast.CallExpr:
				if function, exists := pass.TypesInfo.Types[value.Fun]; exists && function.IsType() {
					return true
				}
				switch ps6010BuiltinName(pass, value) {
				case "make", "new", "append", "len", "cap", "min", "max", "clear", "delete", "copy":
					return true
				}
				// A sole multi-valued call can supply every argument of another
				// call, including a method-expression receiver. The tuple itself is
				// not reference-backed according to go/types, but each result slot
				// retains its own storage provenance. Record every reference-bearing
				// slot as typed external storage; the enclosing call's mutation pass
				// then invalidates only compatible non-fresh roots. This covers
				// ordinary, receiver, fixed, and variadic slots without collapsing an
				// unrelated []int result into []float64 storage.
				if tuple, ok := types.Unalias(typ).(*types.Tuple); ok {
					for index := 0; index < tuple.Len(); index++ {
						resultType := tuple.At(index).Type()
						if unsafePointer(resultType) {
							provenance.universal = true
						}
						if ps6010TypeContainsNonCallableReferenceStorage(resultType, make(map[types.Type]bool)) &&
							ps6010ResultComesFromExternalStorage(pass, value, index) {
							provenance.external[types.Unalias(resultType)] = true
						}
					}
				}
				exactCallable := false
				if typ != nil {
					_, exactCallable = types.Unalias(typ).Underlying().(*types.Signature)
				}
				if !exactCallable && ps6010TypeContainsReferenceStorage(typ, make(map[types.Type]bool)) {
					provenance.external[types.Unalias(typ)] = true
				}
			}
			return true
		})
		return provenance
	}
	aliasObject := func(left types.Object, expression ast.Expr, resultIndex ...int) bool {
		if !ps6010ReferenceBackedObject(left) || expression == nil {
			return false
		}
		if ps6010ReceiveExpression(expression) &&
			ps6010ReferenceBackedObject(left) {
			unknownFromReceive[left] = true
		}
		provenance := provenanceOf(expression)
		if len(resultIndex) != 0 {
			index := resultIndex[0]
			resultType := ps6010ExpressionResultType(pass, expression, index)
			if resultType != nil && unsafePointer(resultType) {
				provenance.universal = true
			}
			exactCallable := false
			if resultType != nil {
				_, exactCallable = types.Unalias(resultType).Underlying().(*types.Signature)
			}
			if resultType != nil && !exactCallable &&
				ps6010TypeContainsReferenceStorage(resultType, make(map[types.Type]bool)) &&
				ps6010ResultComesFromExternalStorage(pass, expression, index) {
				provenance.external[types.Unalias(resultType)] = true
			}
		}
		connected := false
		for right := range provenance.roots {
			aliases.union(left, right)
			connected = true
			if isPackageObject(left) {
				escapedByStore[right] = true
			}
		}
		for typ := range provenance.external {
			addExternal(left, typ)
		}
		if provenance.universal {
			universalByObject[left] = true
		}
		return connected
	}
	aliasStore := func(destination, expression ast.Expr, resultIndex ...int) {
		if destination == nil {
			return
		}
		aliasObject(ps6010WrittenRootObject(pass, destination), expression, resultIndex...)
	}
	recordUnknownTransfer := func(object types.Object, node ast.Node) {
		if !ps6010ReferenceBackedObject(object) {
			return
		}
		unknownFromReceive[object] = true
		unknownReceives = append(unknownReceives, unknownReceive{
			function: ps6010EnclosingFunction(facts.parents, node),
			typ:      object.Type(),
		})
	}
	aliasTransfer := func(destination ast.Expr, sources ...ast.Expr) {
		leftRoots := ps6010StoredRoots(pass, destination)
		for _, source := range sources {
			for right := range ps6010StoredRoots(pass, source) {
				for left := range leftRoots {
					aliases.union(left, right)
					if isPackageObject(left) {
						escapedByStore[right] = true
					}
				}
			}
		}
	}
	inspectAll(func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if (value.Tok == token.DEFINE || value.Tok == token.ASSIGN) && len(value.Lhs) == len(value.Rhs) {
				for index := range value.Lhs {
					aliasStore(value.Lhs[index], value.Rhs[index], 0)
				}
			} else if (value.Tok == token.DEFINE || value.Tok == token.ASSIGN) && len(value.Rhs) == 1 {
				for index := range value.Lhs {
					aliasStore(value.Lhs[index], value.Rhs[0], index)
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, identifier := range value.Names {
					aliasStore(identifier, value.Values[index], 0)
				}
			} else if len(value.Values) == 1 {
				for index, identifier := range value.Names {
					aliasStore(identifier, value.Values[0], index)
				}
			}
		case *ast.RangeStmt:
			if ps6010ChannelType(pass.TypesInfo.TypeOf(value.X)) {
				if identifier, ok := value.Key.(*ast.Ident); ok && identifier.Name != "_" {
					object := identObject(pass, identifier)
					recordUnknownTransfer(object, value)
					if object != nil {
						addExternal(object, object.Type())
					}
				}
				channelType := pass.TypesInfo.TypeOf(value.X)
				if channelType != nil {
					channel, ok := types.Unalias(channelType).Underlying().(*types.Chan)
					if ok &&
						ps6010TypeContainsReferenceStorage(channel.Elem(), make(map[types.Type]bool)) {
						unknownReceives = append(unknownReceives, unknownReceive{
							function: ps6010EnclosingFunction(facts.parents, value),
							typ:      channel.Elem(),
						})
					}
				}
				break
			}
			_, iteratorFunction := types.Unalias(pass.TypesInfo.TypeOf(value.X)).Underlying().(*types.Signature)
			// A range value is copied from an array, slice, or map element. Any
			// reference-bearing descendants remain shared with the ranged source.
			// Map keys may carry references too; slice/array keys are value-only.
			if identifier, ok := value.Key.(*ast.Ident); ok && identifier.Name != "_" {
				object := identObject(pass, identifier)
				if object != nil {
					addExternal(object, object.Type())
				}
				if ps6010ReferenceBackedObject(object) && (iteratorFunction || !aliasObject(object, value.X)) {
					recordUnknownTransfer(object, value)
				}
			}
			if identifier, ok := value.Value.(*ast.Ident); ok && identifier.Name != "_" {
				object := identObject(pass, identifier)
				if object != nil {
					addExternal(object, object.Type())
				}
				if ps6010ReferenceBackedObject(object) && (iteratorFunction || !aliasObject(object, value.X)) {
					recordUnknownTransfer(object, value)
				}
			}
		case *ast.TypeSwitchStmt:
			assignment, ok := value.Assign.(*ast.AssignStmt)
			if !ok || len(assignment.Rhs) != 1 {
				break
			}
			assertion, ok := assignment.Rhs[0].(*ast.TypeAssertExpr)
			if !ok || assertion.Type != nil {
				break
			}
			for _, statement := range value.Body.List {
				clause, ok := statement.(*ast.CaseClause)
				if !ok {
					continue
				}
				object := pass.TypesInfo.Implicits[clause]
				if object != nil {
					addExternal(object, object.Type())
				}
				if ps6010ReferenceBackedObject(object) && !aliasObject(object, assertion.X) {
					recordUnknownTransfer(object, clause)
				}
			}
		case *ast.UnaryExpr:
			if value.Op == token.ARROW &&
				ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(value), make(map[types.Type]bool)) {
				unknownReceives = append(unknownReceives, unknownReceive{
					function: ps6010EnclosingFunction(facts.parents, value),
					typ:      pass.TypesInfo.TypeOf(value),
				})
			}
		case *ast.SendStmt:
			for object := range ps6010StoredRoots(pass, value.Value) {
				escapedByStore[object] = true
			}
		case *ast.CallExpr:
			switch ps6010BuiltinName(pass, value) {
			case "append":
				if len(value.Args) > 1 {
					sources := value.Args[1:]
					if !value.Ellipsis.IsValid() || ps6010SequenceElementContainsReference(pass.TypesInfo.TypeOf(value.Args[0])) {
						aliasTransfer(value.Args[0], sources...)
					}
				}
			case "copy":
				if len(value.Args) == 2 && ps6010SequenceElementContainsReference(pass.TypesInfo.TypeOf(value.Args[0])) {
					aliasTransfer(value.Args[0], value.Args[1])
				}
			}
		}
		return true
	})
	// A received reference value can originate outside the analyzed function,
	// so even an inline receive with no assignable root may retain any compatible
	// reference parameter or package object visible to that function.
	for _, receive := range unknownReceives {
		for _, parameter := range parametersByFunction[receive.function] {
			if ps6010ReferenceTypesMayAlias(receive.typ, parameter.Type()) {
				unknownFromReceive[parameter] = true
			}
		}
		for captured := range capturedByFunction[receive.function] {
			if ps6010ReferenceTypesMayAlias(receive.typ, captured.Type()) {
				unknownFromReceive[captured] = true
			}
		}
		if pass.Pkg != nil {
			scope := pass.Pkg.Scope()
			for _, name := range scope.Names() {
				object := scope.Lookup(name)
				if _, variable := object.(*types.Var); variable && ps6010ReferenceBackedObject(object) &&
					ps6010ReferenceTypesMayAlias(receive.typ, object.Type()) {
					unknownFromReceive[object] = true
				}
			}
		}
	}
	// A written value aggregate and a compatible reference parameter can share
	// the same addressable storage (for example globalDst and globalDst[:]).
	// Connect them before classifying writes so both diagnostics and fixes see
	// the possible overlap. Distinct value-only objects remain separate.
	for object, functions := range writtenInFunctions {
		if !isPackageObject(object) {
			continue
		}
		for function := range functions {
			for _, parameter := range parametersByFunction[function] {
				if ps6010ReferenceMayPointIntoValue(parameter.Type(), object.Type()) {
					aliases.union(object, parameter)
				}
			}
		}
	}
	// Once a local reference header's address is exposed, an indirect rebind can
	// make it refer to any compatible input visible to the function. This is a
	// may-alias relation; exact local pointer assignments below only add edges.
	for object := range facts.addressTaken {
		if !ps6010ReferenceBackedObject(object) {
			continue
		}
		function := ps6010EnclosingFunction(facts.parents, facts.definitionAt[object])
		for _, parameter := range parametersByFunction[function] {
			if ps6010ReferenceTypesMayAlias(object.Type(), parameter.Type()) {
				aliases.union(object, parameter)
			}
		}
	}

	// Resolve local pointer copies far enough to recognize indirect header
	// rebinds such as p := &dst; q := p; *q = a and pp := &p; **pp = a.
	// Unknown pointer-producing calls remain conservative via addressTaken and
	// escape handling below.
	type pointerDefinition struct {
		object     types.Object
		expression ast.Expr
	}
	var pointerDefinitions []pointerDefinition
	directPointer := func(object types.Object) bool {
		if object == nil || object.Type() == nil {
			return false
		}
		_, ok := types.Unalias(object.Type()).Underlying().(*types.Pointer)
		return ok
	}
	inspectAll(func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if (value.Tok == token.DEFINE || value.Tok == token.ASSIGN) && len(value.Lhs) == len(value.Rhs) {
				for index, lhs := range value.Lhs {
					identifier, _ := lhs.(*ast.Ident)
					object := identObject(pass, identifier)
					if directPointer(object) {
						pointerDefinitions = append(pointerDefinitions, pointerDefinition{object, value.Rhs[index]})
					}
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, identifier := range value.Names {
					object := identObject(pass, identifier)
					if directPointer(object) {
						pointerDefinitions = append(pointerDefinitions, pointerDefinition{object, value.Values[index]})
					}
				}
			}
		}
		return true
	})
	// Build inclusion constraints once and propagate new points-to facts through
	// a worklist. Re-scanning every definition until a backward chain settles is
	// quadratic for p0=p1; ...; pN=&x. Load constraints model *pp values by adding
	// a copy edge from each newly discovered holder to the dereference result.
	type pointerNode int
	objectNodes := make(map[types.Object]pointerNode)
	expressionNodes := make(map[ast.Expr]pointerNode)
	initialPoints := make([]map[types.Object]bool, 1)
	copySuccessors := make([]map[pointerNode]bool, 1)
	loadSuccessors := make([]map[pointerNode]bool, 1)
	newPointerNode := func() pointerNode {
		node := pointerNode(len(initialPoints))
		initialPoints = append(initialPoints, nil)
		copySuccessors = append(copySuccessors, nil)
		loadSuccessors = append(loadSuccessors, nil)
		return node
	}
	nodeForObject := func(object types.Object) pointerNode {
		if object == nil {
			return 0
		}
		if node := objectNodes[object]; node != 0 {
			return node
		}
		node := newPointerNode()
		objectNodes[object] = node
		return node
	}
	newExpressionNode := func(expression ast.Expr) pointerNode {
		if node := expressionNodes[expression]; node != 0 {
			return node
		}
		node := newPointerNode()
		expressionNodes[expression] = node
		return node
	}
	addPoint := func(node pointerNode, target types.Object) bool {
		if node == 0 || target == nil {
			return false
		}
		targets := initialPoints[node]
		if targets == nil {
			targets = make(map[types.Object]bool)
			initialPoints[node] = targets
		}
		if targets[target] {
			return false
		}
		targets[target] = true
		return true
	}
	addCopyConstraint := func(source, destination pointerNode) {
		if source == 0 || destination == 0 || source == destination {
			return
		}
		successors := copySuccessors[source]
		if successors == nil {
			successors = make(map[pointerNode]bool)
			copySuccessors[source] = successors
		}
		if successors[destination] {
			return
		}
		successors[destination] = true
		if stats != nil {
			stats.pointerConstraints++
		}
	}
	addLoadConstraint := func(source, destination pointerNode) {
		if source == 0 || destination == 0 {
			return
		}
		successors := loadSuccessors[source]
		if successors == nil {
			successors = make(map[pointerNode]bool)
			loadSuccessors[source] = successors
		}
		if successors[destination] {
			return
		}
		successors[destination] = true
		if stats != nil {
			stats.pointerConstraints++
		}
	}
	var nodeForPointerValue func(ast.Expr) pointerNode
	nodeForPointerValue = func(expression ast.Expr) pointerNode {
		expression = ps6010Unparen(expression)
		switch value := expression.(type) {
		case *ast.Ident:
			return nodeForObject(identObject(pass, value))
		case *ast.UnaryExpr:
			if value.Op != token.AND {
				return 0
			}
			node := newExpressionNode(expression)
			addPoint(node, ps6010WrittenRootObject(pass, value.X))
			return node
		case *ast.StarExpr:
			if node := expressionNodes[expression]; node != 0 {
				return node
			}
			source := nodeForPointerValue(value.X)
			if source == 0 {
				return 0
			}
			node := newExpressionNode(expression)
			addLoadConstraint(source, node)
			return node
		case *ast.CallExpr:
			if len(value.Args) == 1 {
				if function, exists := pass.TypesInfo.Types[value.Fun]; exists && function.IsType() {
					return nodeForPointerValue(value.Args[0])
				}
			}
		}
		return 0
	}
	for _, definition := range pointerDefinitions {
		addCopyConstraint(nodeForPointerValue(definition.expression), nodeForObject(definition.object))
	}
	type indirectPointerStore struct {
		pointer pointerNode
		source  types.Object
	}
	var indirectPointerStores []indirectPointerStore
	inspectAll(func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != len(assignment.Rhs) {
			return true
		}
		for index, lhs := range assignment.Lhs {
			indirect, ok := ps6010Unparen(lhs).(*ast.StarExpr)
			if !ok {
				continue
			}
			right := ps6010ReferenceAliasSource(pass, assignment.Rhs[index])
			if !ps6010ReferenceBackedObject(right) {
				continue
			}
			if pointer := nodeForPointerValue(indirect.X); pointer != 0 {
				indirectPointerStores = append(indirectPointerStores, indirectPointerStore{pointer: pointer, source: right})
			}
		}
		return true
	})
	// Every address-taken object needs a node before the static copy graph is
	// condensed. Load constraints later refer to these nodes by object without
	// growing the graph during propagation.
	for node := pointerNode(1); int(node) < len(initialPoints); node++ {
		for target := range initialPoints[node] {
			nodeForObject(target)
		}
	}

	// Collapse strongly connected copy constraints once. A dense set of pointer
	// copies has quadratically many syntax edges and quadratically many logical
	// node/target facts, but all nodes in a cycle have the same points-to set.
	// Propagating each fact through every dense edge would add an unnecessary
	// cubic factor; the component graph retains exactly the same inclusion
	// semantics while processing each distinct component fact once.
	nodeCount := len(initialPoints)
	indices := make([]int, nodeCount)
	lowlinks := make([]int, nodeCount)
	onStack := make([]bool, nodeCount)
	componentFor := make([]int, nodeCount)
	for index := range indices {
		indices[index] = -1
		componentFor[index] = -1
	}
	var pointerStack []pointerNode
	nextIndex := 0
	componentCount := 0
	var visit func(pointerNode)
	visit = func(node pointerNode) {
		indices[node] = nextIndex
		lowlinks[node] = nextIndex
		nextIndex++
		pointerStack = append(pointerStack, node)
		onStack[node] = true
		for successor := range copySuccessors[node] {
			if stats != nil {
				stats.pointerEdgeVisits++
			}
			if indices[successor] == -1 {
				visit(successor)
				lowlinks[node] = min(lowlinks[node], lowlinks[successor])
			} else if onStack[successor] {
				lowlinks[node] = min(lowlinks[node], indices[successor])
			}
		}
		if lowlinks[node] != indices[node] {
			return
		}
		for {
			last := len(pointerStack) - 1
			member := pointerStack[last]
			pointerStack = pointerStack[:last]
			onStack[member] = false
			componentFor[member] = componentCount
			if member == node {
				break
			}
		}
		componentCount++
	}
	for node := pointerNode(1); int(node) < nodeCount; node++ {
		if indices[node] == -1 {
			visit(node)
		}
	}

	componentPoints := make([]map[types.Object]bool, componentCount)
	type componentEdge struct {
		source      int
		destination int
	}
	componentCopies := make([][]int, componentCount)
	copyEdges := make(map[componentEdge]bool)
	componentLoads := make([]map[int]bool, componentCount)
	componentCopyEdges := 0
	for source := pointerNode(1); int(source) < nodeCount; source++ {
		sourceComponent := componentFor[source]
		for destination := range copySuccessors[source] {
			if stats != nil {
				stats.pointerEdgeVisits++
			}
			destinationComponent := componentFor[destination]
			if sourceComponent == destinationComponent {
				continue
			}
			edge := componentEdge{source: sourceComponent, destination: destinationComponent}
			if !copyEdges[edge] {
				copyEdges[edge] = true
				componentCopies[sourceComponent] = append(componentCopies[sourceComponent], destinationComponent)
				componentCopyEdges++
			}
		}
		for destination := range loadSuccessors[source] {
			destinationComponent := componentFor[destination]
			if componentLoads[sourceComponent] == nil {
				componentLoads[sourceComponent] = make(map[int]bool)
			}
			componentLoads[sourceComponent][destinationComponent] = true
		}
	}
	// Dense acyclic copy graphs contain many transitively redundant edges. Even
	// after SCC condensation, forwarding each distinct target along every such
	// edge is cubic for the complete forward DAG. Reduce only dense component
	// graphs: sparse chains already propagate linearly and should not pay for a
	// quadratic reachability table. Processing successors in topological order
	// makes each retained edge add reachability not covered by an earlier edge.
	if componentCount != 0 && componentCount <= 8192 && componentCopyEdges > 4*componentCount {
		indegree := make([]int, componentCount)
		for _, successors := range componentCopies {
			for _, destination := range successors {
				indegree[destination]++
				if stats != nil {
					stats.pointerEdgeVisits++
				}
			}
		}
		queue := make([]int, 0, componentCount)
		for component, degree := range indegree {
			if degree == 0 {
				queue = append(queue, component)
			}
		}
		topological := make([]int, 0, componentCount)
		for cursor := 0; cursor < len(queue); cursor++ {
			component := queue[cursor]
			topological = append(topological, component)
			for _, destination := range componentCopies[component] {
				if stats != nil {
					stats.pointerEdgeVisits++
				}
				indegree[destination]--
				if indegree[destination] == 0 {
					queue = append(queue, destination)
				}
			}
		}
		// SCC condensation is acyclic by construction. Retain the guard so a
		// future constraint kind cannot accidentally apply a partial reduction.
		if len(topological) == componentCount {
			wordCount := (componentCount + 63) / 64
			reachable := make([][]uint64, componentCount)
			reachableStorage := make([]uint64, componentCount*wordCount)
			for component := range reachable {
				start := component * wordCount
				reachable[component] = reachableStorage[start : start+wordCount : start+wordCount]
			}
			originalStamp := make([]int, componentCount)
			generation := 1
			for index := len(topological) - 1; index >= 0; index-- {
				source := topological[index]
				original := componentCopies[source]
				for _, destination := range original {
					originalStamp[destination] = generation
				}
				reduced := make([]int, 0, len(original))
				for _, destination := range topological {
					// Scanning one topological row per source is deliberately O(V^2)
					// in this dense-only path and avoids per-row comparison sorting.
					if stats != nil {
						stats.pointerEdgeVisits++
					}
					if originalStamp[destination] != generation {
						continue
					}
					word, bit := destination/64, uint(destination%64)
					if reachable[source][word]&(uint64(1)<<bit) != 0 {
						continue
					}
					reduced = append(reduced, destination)
					reachable[source][word] |= uint64(1) << bit
					for word, bits := range reachable[destination] {
						if stats != nil {
							stats.pointerEdgeVisits++
						}
						reachable[source][word] |= bits
					}
				}
				componentCopies[source] = reduced
				generation++
			}
		}
	}
	copyEdges = make(map[componentEdge]bool, componentCopyEdges)
	for source, successors := range componentCopies {
		for _, destination := range successors {
			copyEdges[componentEdge{source: source, destination: destination}] = true
		}
	}
	type componentFact struct {
		component int
		target    types.Object
	}
	var worklist []componentFact
	addComponentPoint := func(component int, target types.Object) bool {
		if component < 0 || target == nil {
			return false
		}
		targets := componentPoints[component]
		if targets == nil {
			targets = make(map[types.Object]bool)
			componentPoints[component] = targets
		}
		if targets[target] {
			return false
		}
		targets[target] = true
		worklist = append(worklist, componentFact{component: component, target: target})
		if stats != nil {
			stats.pointerPropagations++
		}
		return true
	}
	for node := pointerNode(1); int(node) < nodeCount; node++ {
		for target := range initialPoints[node] {
			addComponentPoint(componentFor[node], target)
		}
	}
	addComponentCopy := func(source, destination int) {
		if source < 0 || destination < 0 || source == destination {
			return
		}
		edge := componentEdge{source: source, destination: destination}
		if copyEdges[edge] {
			return
		}
		copyEdges[edge] = true
		componentCopies[source] = append(componentCopies[source], destination)
		if stats != nil {
			stats.pointerConstraints++
		}
		for target := range componentPoints[source] {
			if stats != nil {
				stats.pointerEdgeVisits++
			}
			addComponentPoint(destination, target)
		}
	}
	for cursor := 0; cursor < len(worklist); cursor++ {
		fact := worklist[cursor]
		for _, destination := range componentCopies[fact.component] {
			if stats != nil {
				stats.pointerEdgeVisits++
			}
			addComponentPoint(destination, fact.target)
		}
		for destination := range componentLoads[fact.component] {
			if stats != nil {
				stats.pointerEdgeVisits++
			}
			holderNode := objectNodes[fact.target]
			if holderNode != 0 {
				addComponentCopy(componentFor[holderNode], destination)
			}
		}
	}
	for _, store := range indirectPointerStores {
		for target := range componentPoints[componentFor[store.pointer]] {
			if ps6010ReferenceBackedObject(target) {
				aliases.union(target, store.source)
			}
		}
	}
	for object, expression := range facts.definitions {
		if facts.bindingWrites[object] == 1 && ps6010EnclosingFunction(facts.parents, facts.definitionAt[object]) != nil &&
			ps6010FreshStorageExpression(pass, expression) {
			facts.freshStorage[object] = true
		}
	}
	// External facts follow the same equivalence classes as concrete roots. Do
	// this after pointer constraints have added their final alias edges so a
	// wildcard introduced on either end of a copy is visible at every later
	// mutation of that storage class.
	externalByClass := make(map[types.Object]map[types.Type]bool)
	universalByClass := make(map[types.Object]bool)
	for object := range aliases.parent {
		class := aliases.find(object)
		for typ := range externalByObject[object] {
			typesByClass := externalByClass[class]
			if typesByClass == nil {
				typesByClass = make(map[types.Type]bool)
				externalByClass[class] = typesByClass
			}
			typesByClass[typ] = true
		}
		if universalByObject[object] {
			universalByClass[class] = true
		}
	}
	for object := range aliases.parent {
		class := aliases.find(object)
		if typesByClass := externalByClass[class]; len(typesByClass) != 0 {
			externalByObject[object] = typesByClass
		}
		if universalByClass[class] {
			universalByObject[object] = true
		}
	}

	unstableComponents := make(map[types.Object]bool)
	escapedComponents := make(map[types.Object]bool)
	markUnstable := func(object types.Object, escaped bool) {
		if object == nil {
			return
		}
		aliases.add(object)
		root := aliases.find(object)
		unstableComponents[root] = true
		if escaped {
			escapedComponents[root] = true
		}
	}
	// An addressable reference header can be rebound indirectly, including via
	// copied pointers, pointer-to-pointer chains, and helpers or closures. Treat
	// its complete storage component as escaped instead of trying to infer a
	// local points-to graph from syntax alone.
	for object := range facts.addressTaken {
		markUnstable(object, true)
	}
	for object := range escapedByStore {
		markUnstable(object, true)
	}
	for object := range unknownFromReceive {
		markUnstable(object, true)
	}
	type externalMutation struct {
		function   ast.Node
		provenance ps6010StorageProvenance
	}
	var externalMutations []externalMutation
	recordExternalMutation := func(expression ast.Expr, site ast.Node) {
		provenance := provenanceOf(expression)
		if len(provenance.external) == 0 && !provenance.externalAll && !provenance.universal {
			return
		}
		externalMutations = append(externalMutations, externalMutation{
			function:   facts.functionOwner[site],
			provenance: provenance,
		})
	}
	inspectAll(func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if _, identifier := lhs.(*ast.Ident); !identifier {
					markUnstable(ps6010WrittenRootObject(pass, lhs), false)
					recordExternalMutation(lhs, value)
				}
			}
		case *ast.IncDecStmt:
			if _, identifier := value.X.(*ast.Ident); !identifier {
				markUnstable(ps6010WrittenRootObject(pass, value.X), false)
				recordExternalMutation(value.X, value)
			}
		case *ast.CallExpr:
			if function, exists := pass.TypesInfo.Types[value.Fun]; exists && function.IsType() {
				break
			}
			markArgument := func(index int, escaped bool) {
				if index >= 0 && index < len(value.Args) {
					for object := range ps6010StoredRoots(pass, value.Args[index]) {
						markUnstable(object, escaped)
					}
					recordExternalMutation(value.Args[index], value)
				}
			}
			switch ps6010BuiltinName(pass, value) {
			case "len", "cap", "min", "max":
				// Read-only builtins neither mutate nor retain their operands.
			case "append", "clear", "delete":
				markArgument(0, false)
			case "copy":
				// copy may overlap, but only its destination is written. Parameter
				// and explicit-alias components propagate that write to the source.
				markArgument(0, false)
			default:
				for index := range value.Args {
					markArgument(index, true)
				}
				// Invoking a stored function or method value can mutate storage held
				// only in its hidden closure/receiver environment. Alias stores connect
				// method values to their receiver; callback parameters are conservatively
				// connected to the caller's other reference-bearing parameters.
				for object := range ps6010StoredRoots(pass, value.Fun) {
					markUnstable(object, true)
				}
				recordExternalMutation(value.Fun, value)
				if selector, ok := value.Fun.(*ast.SelectorExpr); ok &&
					ps6010TypeContainsReferenceStorage(pass.TypesInfo.TypeOf(selector.X), make(map[types.Type]bool)) {
					markUnstable(ps6010WrittenRootObject(pass, selector.X), true)
					recordExternalMutation(selector.X, value)
				}
			}
		case *ast.SendStmt:
			for object := range ps6010StoredRoots(pass, value.Value) {
				markUnstable(object, true)
			}
		}
		return true
	})
	// A mutation through an external wildcard may hit any compatible storage in
	// the lexical function chain. Include enclosing parameters even when the
	// nested closure does not mention them: an opaque/global channel can still
	// deliver their backing storage. Universal unsafe writes additionally cover
	// value-only addressable objects whose address may have been encoded in a
	// uintptr.
	type storageClassInfo struct {
		classes  map[string]bool
		wildcard bool
	}
	classCache := make(map[types.Type]storageClassInfo)
	storageClasses := func(typ types.Type) storageClassInfo {
		if typ == nil {
			return storageClassInfo{wildcard: true}
		}
		typ = types.Unalias(typ)
		if cached, ok := classCache[typ]; ok {
			return cached
		}
		classes, wildcard := ps6010StorageCompatibilityClasses(typ)
		result := storageClassInfo{classes: classes, wildcard: wildcard}
		classCache[typ] = result
		if stats != nil {
			stats.externalTypesClassified++
		}
		return result
	}
	indexProvenance := func(provenance *ps6010StorageProvenance) {
		if provenance.classes == nil {
			provenance.classes = make(map[string]bool)
		}
		for typ := range provenance.external {
			info := storageClasses(typ)
			for class := range info.classes {
				provenance.classes[class] = true
			}
			provenance.externalAll = provenance.externalAll || info.wildcard
		}
	}
	mutationMayHit := func(provenance ps6010StorageProvenance, object types.Object) bool {
		if object == nil || facts.freshStorage[object] {
			return false
		}
		if provenance.universal {
			return true
		}
		if !ps6010ReferenceBackedObject(object) {
			return false
		}
		if provenance.externalAll {
			return true
		}
		candidate := storageClasses(object.Type())
		if candidate.wildcard && len(provenance.classes) != 0 {
			return true
		}
		for class := range candidate.classes {
			if stats != nil {
				stats.externalCandidateClassScans++
			}
			if provenance.classes[class] {
				return true
			}
		}
		return false
	}
	joinProvenance := func(destination *ps6010StorageProvenance, source ps6010StorageProvenance) bool {
		changed := false
		if destination.external == nil {
			destination.external = make(map[types.Type]bool)
		}
		for typ := range source.external {
			if !destination.external[typ] {
				destination.external[typ] = true
				changed = true
			}
		}
		if source.externalAll && !destination.externalAll {
			destination.externalAll = true
			changed = true
		}
		if source.universal && !destination.universal {
			destination.universal = true
			changed = true
		}
		return changed
	}
	// Summarize hidden storage effects over the complete package. Callable
	// points-to facts use a separate monotone worklist: assignments, aggregate
	// storage, method values, and actual-to-formal callback flow all contribute
	// edges. Each newly reachable target is attached to each call site once, so
	// recursive SCCs and reverse declaration order do not require global rescans.
	functionByObject := make(map[*types.Func]ast.Node)
	functionNodes := make(map[ast.Node]bool)
	formalParameters := make(map[ast.Node][]types.Object)
	formalReceiver := make(map[ast.Node]types.Object)
	formalResults := make(map[ast.Node][]types.Object)
	callableReturns := make(map[ast.Node][]types.Object)
	variadicFunction := make(map[ast.Node]bool)
	effects := make(map[ast.Node]ps6010StorageProvenance)
	parameterObjects := func(list *ast.FieldList) []types.Object {
		if list == nil {
			return nil
		}
		var objects []types.Object
		for _, field := range list.List {
			if len(field.Names) == 0 {
				objects = append(objects, nil)
				continue
			}
			for _, name := range field.Names {
				objects = append(objects, identObject(pass, name))
			}
		}
		return objects
	}
	registerCallableReturns := func(function ast.Node, results *ast.FieldList, signature *types.Signature) {
		formalResults[function] = parameterObjects(results)
		if signature == nil || signature.Results() == nil {
			return
		}
		slots := make([]types.Object, signature.Results().Len())
		for index := 0; index < signature.Results().Len(); index++ {
			result := signature.Results().At(index)
			if !ps6010TypeContainsCallable(result.Type(), make(map[types.Type]bool)) {
				continue
			}
			slots[index] = types.NewVar(
				token.NoPos, pass.Pkg,
				"ps6010$return@"+strconv.Itoa(int(function.Pos()))+":"+strconv.Itoa(index),
				result.Type(),
			)
		}
		callableReturns[function] = slots
	}
	inspectAll(func(node ast.Node) bool {
		switch function := node.(type) {
		case *ast.FuncDecl:
			functionNodes[function] = true
			formalParameters[function] = parameterObjects(function.Type.Params)
			variadicFunction[function] = function.Type.Params != nil && len(function.Type.Params.List) != 0 &&
				func() bool {
					_, variadic := function.Type.Params.List[len(function.Type.Params.List)-1].Type.(*ast.Ellipsis)
					return variadic
				}()
			if receivers := parameterObjects(function.Recv); len(receivers) != 0 {
				formalReceiver[function] = receivers[0]
			}
			if object, ok := identObject(pass, function.Name).(*types.Func); ok {
				functionByObject[object] = function
				signature, _ := types.Unalias(object.Type()).Underlying().(*types.Signature)
				registerCallableReturns(function, function.Type.Results, signature)
			}
			// A bodyless declaration is implemented outside the inspected Go
			// package (normally in assembly). Its hidden effects are unknowable.
			if function.Body == nil {
				effect := effects[function]
				effect.externalAll = true
				effects[function] = effect
			}
		case *ast.FuncLit:
			functionNodes[function] = true
			formalParameters[function] = parameterObjects(function.Type.Params)
			signature, _ := types.Unalias(pass.TypesInfo.TypeOf(function.Type)).Underlying().(*types.Signature)
			registerCallableReturns(function, function.Type.Results, signature)
			variadicFunction[function] = function.Type.Params != nil && len(function.Type.Params.List) != 0 &&
				func() bool {
					_, variadic := function.Type.Params.List[len(function.Type.Params.List)-1].Type.(*ast.Ellipsis)
					return variadic
				}()
		}
		return true
	})
	for _, mutation := range externalMutations {
		if mutation.function == nil {
			continue
		}
		effect := effects[mutation.function]
		joinProvenance(&effect, mutation.provenance)
		effects[mutation.function] = effect
	}
	callableKey := func(object types.Object) types.Object {
		if object == nil {
			return nil
		}
		if root := aliases.find(object); root != nil {
			return root
		}
		return object
	}
	callableCallResults := make(map[*ast.CallExpr][]types.Object)
	inspectAll(func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if function, exists := pass.TypesInfo.Types[call.Fun]; exists && function.IsType() {
			return true
		}
		switch ps6010BuiltinName(pass, call) {
		case "make", "new", "append":
			return true
		}
		resultType := pass.TypesInfo.TypeOf(call)
		if resultType == nil {
			return true
		}
		resultTypes := []types.Type{resultType}
		if tuple, ok := types.Unalias(resultType).(*types.Tuple); ok {
			resultTypes = make([]types.Type, tuple.Len())
			for index := 0; index < tuple.Len(); index++ {
				resultTypes[index] = tuple.At(index).Type()
			}
		}
		slots := make([]types.Object, len(resultTypes))
		for index, typ := range resultTypes {
			if !ps6010TypeContainsCallable(typ, make(map[types.Type]bool)) {
				continue
			}
			slots[index] = types.NewVar(
				token.NoPos, pass.Pkg,
				"ps6010$call@"+strconv.Itoa(int(call.Pos()))+":"+strconv.Itoa(index),
				typ,
			)
		}
		callableCallResults[call] = slots
		return true
	})
	type callableExpressionFacts struct {
		targets map[ast.Node]bool
		sources map[types.Object]bool
		unknown bool
	}
	newCallableFacts := func() callableExpressionFacts {
		return callableExpressionFacts{
			targets: make(map[ast.Node]bool),
			sources: make(map[types.Object]bool),
		}
	}
	mergeCallableFacts := func(destination *callableExpressionFacts, source callableExpressionFacts) {
		for target := range source.targets {
			destination.targets[target] = true
		}
		for object := range source.sources {
			destination.sources[object] = true
		}
		destination.unknown = destination.unknown || source.unknown
	}
	var callableFacts func(ast.Expr) callableExpressionFacts
	callableFacts = func(expression ast.Expr) callableExpressionFacts {
		result := newCallableFacts()
		if expression == nil {
			return result
		}
		expression = ps6010Unparen(expression)
		if literal, ok := expression.(*ast.FuncLit); ok {
			result.targets[literal] = true
			return result
		}
		if ps6010DynamicMethodDispatch(pass, expression) {
			result.unknown = true
			return result
		}
		if object := ps6010CalledFunctionObject(pass, expression); object != nil {
			if target := functionByObject[object]; target != nil {
				result.targets[target] = true
			} else if object.Pkg() == pass.Pkg {
				// A same-package function object without an inspectable declaration
				// is equivalent to an opaque bodyless implementation.
				result.unknown = true
			}
			return result
		}
		if call, ok := expression.(*ast.CallExpr); ok {
			if function, exists := pass.TypesInfo.Types[call.Fun]; exists && function.IsType() {
				if len(call.Args) == 1 && ps6010TypeContainsCallable(pass.TypesInfo.TypeOf(call), make(map[types.Type]bool)) {
					return callableFacts(call.Args[0])
				}
				return result
			}
			switch ps6010BuiltinName(pass, call) {
			case "make", "new":
				return result
			case "append":
				for _, argument := range call.Args {
					mergeCallableFacts(&result, callableFacts(argument))
				}
				return result
			}
			for _, slot := range callableCallResults[call] {
				if key := callableKey(slot); key != nil {
					result.sources[key] = true
				}
			}
			return result
		}
		if composite, ok := expression.(*ast.CompositeLit); ok {
			for _, element := range composite.Elts {
				if pair, ok := element.(*ast.KeyValueExpr); ok {
					mergeCallableFacts(&result, callableFacts(pair.Key))
					mergeCallableFacts(&result, callableFacts(pair.Value))
					continue
				}
				mergeCallableFacts(&result, callableFacts(element))
			}
			return result
		}
		for object := range ps6010StoredRoots(pass, expression) {
			if function, ok := object.(*types.Func); ok {
				if target := functionByObject[function]; target != nil {
					result.targets[target] = true
					continue
				}
			}
			if key := callableKey(object); key != nil {
				result.sources[key] = true
			}
		}
		if receive, ok := expression.(*ast.UnaryExpr); ok && receive.Op == token.ARROW &&
			ps6010TypeContainsCallable(pass.TypesInfo.TypeOf(expression), make(map[types.Type]bool)) {
			result.unknown = true
		}
		return result
	}
	callableFactsAt := func(expression ast.Expr, resultIndex int) callableExpressionFacts {
		expression = ps6010Unparen(expression)
		call, ok := expression.(*ast.CallExpr)
		if !ok {
			return callableFacts(expression)
		}
		slots, ordinaryCall := callableCallResults[call]
		if !ordinaryCall {
			return callableFacts(expression)
		}
		result := newCallableFacts()
		if resultIndex < 0 || resultIndex >= len(slots) {
			result.unknown = true
			return result
		}
		if key := callableKey(slots[resultIndex]); key != nil {
			result.sources[key] = true
		}
		return result
	}

	callableTargets := make(map[types.Object]map[ast.Node]bool)
	callableUnknown := make(map[types.Object]bool)
	callableSuccessors := make(map[types.Object]map[types.Object]bool)
	var callableQueue []types.Object
	callableQueued := make(map[types.Object]bool)
	enqueueCallable := func(object types.Object) {
		if object != nil && !callableQueued[object] {
			callableQueued[object] = true
			callableQueue = append(callableQueue, object)
		}
	}
	addCallableTarget := func(object types.Object, target ast.Node) bool {
		object = callableKey(object)
		if object == nil || target == nil {
			return false
		}
		targets := callableTargets[object]
		if targets == nil {
			targets = make(map[ast.Node]bool)
			callableTargets[object] = targets
		}
		if targets[target] {
			return false
		}
		targets[target] = true
		if stats != nil {
			stats.callablePropagations++
		}
		enqueueCallable(object)
		return true
	}
	addCallableUnknown := func(object types.Object) bool {
		object = callableKey(object)
		if object == nil || callableUnknown[object] {
			return false
		}
		callableUnknown[object] = true
		enqueueCallable(object)
		return true
	}
	addCallableEdge := func(source, destination types.Object) {
		source, destination = callableKey(source), callableKey(destination)
		if source == nil || destination == nil || source == destination {
			return
		}
		successors := callableSuccessors[source]
		if successors == nil {
			successors = make(map[types.Object]bool)
			callableSuccessors[source] = successors
		}
		if successors[destination] {
			return
		}
		successors[destination] = true
		if stats != nil {
			stats.callableEdges++
		}
		for target := range callableTargets[source] {
			addCallableTarget(destination, target)
		}
		if callableUnknown[source] {
			addCallableUnknown(destination)
		}
	}
	bindCallableExpression := func(destination types.Object, expression ast.Expr, resultIndex ...int) {
		destination = callableKey(destination)
		if destination == nil || expression == nil {
			return
		}
		resolvedFacts := callableFacts(expression)
		if len(resultIndex) != 0 {
			resolvedFacts = callableFactsAt(expression, resultIndex[0])
		}
		for target := range resolvedFacts.targets {
			addCallableTarget(destination, target)
		}
		for source := range resolvedFacts.sources {
			addCallableEdge(source, destination)
		}
		if resolvedFacts.unknown {
			addCallableUnknown(destination)
		}
		if len(resultIndex) != 0 {
			resultType := ps6010ExpressionResultType(pass, expression, resultIndex[0])
			if resultType != nil && ps6010TypeContainsCallable(resultType, make(map[types.Type]bool)) &&
				ps6010ResultComesFromExternalStorage(pass, expression, resultIndex[0]) {
				expression := ps6010Unparen(expression)
				call, ordinaryCall := expression.(*ast.CallExpr)
				_, summarizedCall := callableCallResults[call]
				if !ordinaryCall || !summarizedCall {
					addCallableUnknown(destination)
				}
			}
		}
	}
	for object, target := range functionByObject {
		addCallableTarget(object, target)
	}
	for function, slots := range callableReturns {
		declaration, bodyless := function.(*ast.FuncDecl)
		if !bodyless || declaration.Body != nil {
			continue
		}
		for _, slot := range slots {
			addCallableUnknown(slot)
		}
	}
	for object, externalTypes := range externalByObject {
		unknown := universalByObject[object]
		for typ := range externalTypes {
			unknown = unknown || ps6010TypeContainsCallable(typ, make(map[types.Type]bool))
		}
		if unknown {
			addCallableUnknown(object)
		}
	}
	for object, universal := range universalByObject {
		if universal && ps6010TypeContainsCallable(object.Type(), make(map[types.Type]bool)) {
			addCallableUnknown(object)
		}
	}
	inspectAll(func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok != token.DEFINE && value.Tok != token.ASSIGN {
				return true
			}
			if len(value.Lhs) == len(value.Rhs) {
				for index, lhs := range value.Lhs {
					destination := ps6010WrittenRootObject(pass, lhs)
					if destination != nil && ps6010TypeContainsCallable(destination.Type(), make(map[types.Type]bool)) {
						bindCallableExpression(destination, value.Rhs[index], 0)
					}
				}
			} else if len(value.Rhs) == 1 {
				for index, lhs := range value.Lhs {
					destination := ps6010WrittenRootObject(pass, lhs)
					if destination != nil && ps6010TypeContainsCallable(destination.Type(), make(map[types.Type]bool)) {
						bindCallableExpression(destination, value.Rhs[0], index)
					}
				}
			}
		case *ast.ValueSpec:
			if len(value.Names) == len(value.Values) {
				for index, identifier := range value.Names {
					destination := identObject(pass, identifier)
					if destination != nil && ps6010TypeContainsCallable(destination.Type(), make(map[types.Type]bool)) {
						bindCallableExpression(destination, value.Values[index], 0)
					}
				}
			} else if len(value.Values) == 1 {
				for index, identifier := range value.Names {
					destination := identObject(pass, identifier)
					if destination != nil && ps6010TypeContainsCallable(destination.Type(), make(map[types.Type]bool)) {
						bindCallableExpression(destination, value.Values[0], index)
					}
				}
			}
		}
		return true
	})
	inspectAll(func(node ast.Node) bool {
		statement, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		function := facts.functionOwner[statement]
		slots := callableReturns[function]
		if len(slots) == 0 {
			return true
		}
		if len(statement.Results) == 0 {
			named := formalResults[function]
			for index, slot := range slots {
				if slot == nil {
					continue
				}
				if index >= len(named) || named[index] == nil {
					addCallableUnknown(slot)
					continue
				}
				addCallableEdge(named[index], slot)
			}
			return true
		}
		if len(statement.Results) == len(slots) {
			for index, slot := range slots {
				if slot != nil {
					bindCallableExpression(slot, statement.Results[index], 0)
				}
			}
			return true
		}
		if len(statement.Results) == 1 {
			for index, slot := range slots {
				if slot != nil {
					bindCallableExpression(slot, statement.Results[0], index)
				}
			}
			return true
		}
		for _, slot := range slots {
			addCallableUnknown(slot)
		}
		return true
	})

	callersByCallee := make(map[ast.Node]map[ast.Node]bool)
	type callableCall struct {
		call          *ast.CallExpr
		caller        ast.Node
		expression    ast.Expr
		bindArguments bool
		invokeReturns bool
	}
	callsBySource := make(map[types.Object][]callableCall)
	var calls []callableCall
	inspectAll(func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if function, exists := pass.TypesInfo.Types[call.Fun]; exists && function.IsType() {
			return true
		}
		if ps6010BuiltinName(pass, call) != "" {
			return true
		}
		caller := facts.functionOwner[call]
		if caller == nil {
			return true
		}
		markResultsUnknown := func() {
			for _, slot := range callableCallResults[call] {
				addCallableUnknown(slot)
			}
		}
		recordCall := func(expression ast.Expr, bindArguments, invokeReturns bool) {
			record := callableCall{
				call:          call,
				caller:        caller,
				expression:    expression,
				bindArguments: bindArguments,
				invokeReturns: invokeReturns,
			}
			calls = append(calls, record)
			resolution := callableFacts(expression)
			for source := range resolution.sources {
				source = callableKey(source)
				callsBySource[source] = append(callsBySource[source], record)
			}
			if resolution.unknown {
				effect := effects[caller]
				effect.externalAll = true
				effects[caller] = effect
				if bindArguments {
					markResultsUnknown()
				}
			}
		}
		recordCall(call.Fun, true, false)
		// A statically imported function cannot directly name this package's
		// state, but it may invoke a callback supplied in an argument and then
		// invoke callable values returned by that callback. Attach every known
		// target directly to the caller; unknown contents remain external-all.
		object := ps6010CalledFunctionObject(pass, call.Fun)
		if object != nil && object.Pkg() != nil && object.Pkg() != pass.Pkg &&
			!ps6010DynamicMethodDispatch(pass, call.Fun) {
			markResultsUnknown()
			for _, argument := range call.Args {
				if ps6010TypeContainsCallable(pass.TypesInfo.TypeOf(argument), make(map[types.Type]bool)) {
					recordCall(argument, false, true)
				}
			}
		}
		return true
	})
	processedCallTargets := make(map[*ast.CallExpr]map[ast.Node]bool)
	processedArgumentTargets := make(map[*ast.CallExpr]map[ast.Node]bool)
	processedReturnedTargets := make(map[*ast.CallExpr]map[ast.Node]bool)
	processedUnknownCalls := make(map[*ast.CallExpr]bool)
	baseSelector := func(expression ast.Expr) (*ast.SelectorExpr, *types.Selection) {
		expression = ps6010Unparen(expression)
		for {
			switch value := expression.(type) {
			case *ast.IndexExpr:
				expression = ps6010Unparen(value.X)
			case *ast.IndexListExpr:
				expression = ps6010Unparen(value.X)
			default:
				selector, _ := expression.(*ast.SelectorExpr)
				if selector == nil {
					return nil, nil
				}
				return selector, pass.TypesInfo.Selections[selector]
			}
		}
	}
	var processCallTarget func(callableCall, ast.Node)
	processCallTarget = func(record callableCall, target ast.Node) {
		if target == nil {
			return
		}
		processed := processedCallTargets[record.call]
		if processed == nil {
			processed = make(map[ast.Node]bool)
			processedCallTargets[record.call] = processed
		}
		if !processed[target] {
			processed[target] = true
			if stats != nil {
				stats.callableCallTargets++
			}
			callers := callersByCallee[target]
			if callers == nil {
				callers = make(map[ast.Node]bool)
				callersByCallee[target] = callers
			}
			callers[record.caller] = true
		}
		if record.invokeReturns {
			returned := processedReturnedTargets[record.call]
			if returned == nil {
				returned = make(map[ast.Node]bool)
				processedReturnedTargets[record.call] = returned
			}
			if !returned[target] {
				returned[target] = true
				for _, slot := range callableReturns[target] {
					source := callableKey(slot)
					if source == nil {
						continue
					}
					callsBySource[source] = append(callsBySource[source], record)
					for returnedTarget := range callableTargets[source] {
						processCallTarget(record, returnedTarget)
					}
					if callableUnknown[source] {
						effect := effects[record.caller]
						effect.externalAll = true
						effects[record.caller] = effect
					}
				}
			}
		}
		if !record.bindArguments {
			return
		}
		arguments := processedArgumentTargets[record.call]
		if arguments == nil {
			arguments = make(map[ast.Node]bool)
			processedArgumentTargets[record.call] = arguments
		}
		if arguments[target] {
			return
		}
		arguments[target] = true
		resultSlots := callableCallResults[record.call]
		returnSlots := callableReturns[target]
		for index, destination := range resultSlots {
			if destination == nil {
				continue
			}
			if index >= len(returnSlots) || returnSlots[index] == nil {
				addCallableUnknown(destination)
				continue
			}
			addCallableEdge(returnSlots[index], destination)
		}

		parameters := formalParameters[target]
		offset := 0
		var expandedExpression ast.Expr
		expandedCount := 0
		if len(record.call.Args) == 1 && !record.call.Ellipsis.IsValid() {
			if tuple, ok := types.Unalias(pass.TypesInfo.TypeOf(record.call.Args[0])).(*types.Tuple); ok {
				expandedExpression = record.call.Args[0]
				expandedCount = tuple.Len()
			}
		}
		var receiverExpression ast.Expr
		if receiver := formalReceiver[target]; receiver != nil {
			selector, selection := baseSelector(record.call.Fun)
			switch {
			case selection != nil && selection.Kind() == types.MethodVal:
				receiverExpression = selector.X
			case selection != nil && selection.Kind() == types.MethodExpr && len(record.call.Args) != 0:
				receiverExpression = record.call.Args[0]
				offset = 1
			default:
				if signature, ok := types.Unalias(pass.TypesInfo.TypeOf(record.call.Fun)).Underlying().(*types.Signature); ok &&
					signature.Params().Len() == len(parameters)+1 && len(record.call.Args) != 0 {
					receiverExpression = record.call.Args[0]
					offset = 1
				}
			}
			if receiverExpression != nil && ps6010TypeContainsCallable(receiver.Type(), make(map[types.Type]bool)) {
				bindCallableExpression(receiver, receiverExpression, 0)
			}
		}
		if expandedExpression != nil {
			// In f(g()), a multi-result g supplies one actual per fixed
			// parameter and every remaining result supplies one variadic
			// element. Keep those result slots separate: binding only slot zero
			// loses callbacks after the first, while merging every slot into an
			// ordinary argument invents targets that the call cannot receive.
			resultIndex := offset
			for index, parameter := range parameters {
				if parameter == nil || !ps6010TypeContainsCallable(parameter.Type(), make(map[types.Type]bool)) {
					resultIndex++
					continue
				}
				if variadicFunction[target] && index == len(parameters)-1 {
					for resultIndex < expandedCount {
						bindCallableExpression(parameter, expandedExpression, resultIndex)
						resultIndex++
					}
					continue
				}
				if resultIndex < expandedCount {
					bindCallableExpression(parameter, expandedExpression, resultIndex)
				}
				resultIndex++
			}
			return
		}
		for index, parameter := range parameters {
			if parameter == nil || !ps6010TypeContainsCallable(parameter.Type(), make(map[types.Type]bool)) {
				continue
			}
			actual := offset + index
			if variadicFunction[target] && index == len(parameters)-1 {
				for ; actual < len(record.call.Args); actual++ {
					bindCallableExpression(parameter, record.call.Args[actual], 0)
					if record.call.Ellipsis.IsValid() {
						break
					}
				}
				continue
			}
			if actual < len(record.call.Args) {
				bindCallableExpression(parameter, record.call.Args[actual], 0)
			}
		}
	}
	// Concrete method values retain their receiver before the eventual call.
	// Bind callable descendants of that receiver to the method's formal receiver
	// independently of declaration and invocation order.
	inspectAll(func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok || ps6010DynamicMethodDispatch(pass, selector) {
			return true
		}
		selection := pass.TypesInfo.Selections[selector]
		if selection == nil || selection.Kind() != types.MethodVal {
			return true
		}
		function, ok := selection.Obj().(*types.Func)
		if !ok {
			return true
		}
		target := functionByObject[function]
		receiver := formalReceiver[target]
		if receiver != nil && ps6010TypeContainsCallable(receiver.Type(), make(map[types.Type]bool)) {
			bindCallableExpression(receiver, selector.X, 0)
		}
		return true
	})
	for _, record := range calls {
		resolution := callableFacts(record.expression)
		for target := range resolution.targets {
			processCallTarget(record, target)
		}
		for source := range resolution.sources {
			for target := range callableTargets[callableKey(source)] {
				processCallTarget(record, target)
			}
		}
	}
	for cursor := 0; cursor < len(callableQueue); cursor++ {
		object := callableQueue[cursor]
		callableQueued[object] = false
		for destination := range callableSuccessors[object] {
			for target := range callableTargets[object] {
				addCallableTarget(destination, target)
			}
			if callableUnknown[object] {
				addCallableUnknown(destination)
			}
		}
		for _, record := range callsBySource[object] {
			for target := range callableTargets[object] {
				processCallTarget(record, target)
			}
			if callableUnknown[object] {
				if record.bindArguments {
					for _, slot := range callableCallResults[record.call] {
						addCallableUnknown(slot)
					}
				}
				if !processedUnknownCalls[record.call] {
					processedUnknownCalls[record.call] = true
					effect := effects[record.caller]
					effect.externalAll = true
					effects[record.caller] = effect
				}
			}
		}
	}
	queue := make([]ast.Node, 0, len(functionNodes))
	queued := make(map[ast.Node]bool, len(functionNodes))
	enqueue := func(function ast.Node) {
		if function != nil && !queued[function] {
			queued[function] = true
			queue = append(queue, function)
		}
	}
	for function, effect := range effects {
		if len(effect.external) != 0 || effect.externalAll || effect.universal {
			enqueue(function)
		}
	}
	for cursor := 0; cursor < len(queue); cursor++ {
		callee := queue[cursor]
		queued[callee] = false
		for caller := range callersByCallee[callee] {
			effect := effects[caller]
			if joinProvenance(&effect, effects[callee]) {
				effects[caller] = effect
				enqueue(caller)
			}
		}
	}
	summarizedMutations := make([]externalMutation, 0, len(effects))
	for _, mutation := range externalMutations {
		if mutation.function == nil {
			summarizedMutations = append(summarizedMutations, mutation)
		}
	}
	for function, effect := range effects {
		if len(effect.external) != 0 || effect.externalAll || effect.universal {
			summarizedMutations = append(summarizedMutations, externalMutation{
				function:   function,
				provenance: effect,
			})
		}
	}
	externalMutations = summarizedMutations
	// Collapse all effects in one lexical scope before visiting its objects.
	// This avoids rescanning O(objects) candidates once per mutation in large
	// generated functions while retaining the same union-of-wildcards result.
	mutationsByFunction := make(map[ast.Node]ps6010StorageProvenance)
	packageMutations := ps6010StorageProvenance{external: make(map[types.Type]bool)}
	for _, mutation := range externalMutations {
		joinProvenance(&packageMutations, mutation.provenance)
		for function := mutation.function; function != nil; function = ps6010EnclosingFunction(facts.parents, facts.parents[function]) {
			provenance := mutationsByFunction[function]
			joinProvenance(&provenance, mutation.provenance)
			mutationsByFunction[function] = provenance
		}
	}
	indexProvenance(&packageMutations)
	for function, provenance := range mutationsByFunction {
		indexProvenance(&provenance)
		mutationsByFunction[function] = provenance
	}
	for function, provenance := range mutationsByFunction {
		parameters := parametersByFunction[function]
		if provenance.universal {
			parameters = allParametersByFunction[function]
		}
		candidates := make(map[types.Object]bool,
			len(parameters)+len(objectsByFunction[function])+len(capturedByFunction[function]))
		for _, parameter := range parameters {
			candidates[parameter] = true
		}
		for _, object := range objectsByFunction[function] {
			if provenance.universal || ps6010ReferenceBackedObject(object) {
				candidates[object] = true
			}
		}
		for captured := range capturedByFunction[function] {
			candidates[captured] = true
		}
		for candidate := range candidates {
			if mutationMayHit(provenance, candidate) {
				markUnstable(candidate, true)
			}
		}
	}
	if pass.Pkg != nil {
		scope := pass.Pkg.Scope()
		for _, name := range scope.Names() {
			if object, variable := scope.Lookup(name).(*types.Var); variable && mutationMayHit(packageMutations, object) {
				markUnstable(object, true)
			}
		}
	}
	for object := range aliases.parent {
		root := aliases.find(object)
		facts.aliasClass[object] = root
		facts.containerUnstable[object] = unstableComponents[root]
		facts.containerEscaped[object] = escapedComponents[root]
	}
	return facts
}

// ps6010WriteFacts records every direct write and address escape in this loop
// body, including those in branches and nested closures. A derived index is
// trusted only when its object has exactly one write, is not address-taken, and
// its definition is a direct, dominating body statement.
func ps6010WriteFacts(pass *analysis.Pass, body *ast.BlockStmt) (map[types.Object]int, map[types.Object]bool) {
	counts := make(map[types.Object]int)
	addressTaken := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if object := ps6010WrittenRootObject(pass, lhs); object != nil {
					counts[object]++
				}
			}
		case *ast.ValueSpec:
			for _, identifier := range value.Names {
				if identifier.Name != "_" {
					counts[identObject(pass, identifier)]++
				}
			}
		case *ast.IncDecStmt:
			if object := ps6010WrittenRootObject(pass, value.X); object != nil {
				counts[object]++
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				if object := ps6010WrittenRootObject(pass, value.X); object != nil {
					addressTaken[object] = true
				}
			}
		case *ast.SliceExpr:
			if ps6010SliceExposesArrayStorage(pass, value.X) {
				if object := ps6010WrittenRootObject(pass, value.X); object != nil {
					addressTaken[object] = true
				}
			}
		case *ast.SelectorExpr:
			// Selecting a pointer-receiver method on an addressable value takes
			// its address implicitly, including when forming a method value that
			// is invoked later.
			selection := pass.TypesInfo.Selections[value]
			if selection == nil {
				break
			}
			signature, ok := selection.Obj().Type().(*types.Signature)
			if !ok || signature.Recv() == nil {
				break
			}
			if _, ok := types.Unalias(signature.Recv().Type()).(*types.Pointer); !ok {
				break
			}
			if object := ps6010WrittenRootObject(pass, value.X); object != nil {
				addressTaken[object] = true
			}
		}
		return true
	})
	return counts, addressTaken
}

func ps6010SliceExposesArrayStorage(pass *analysis.Pass, expression ast.Expr) bool {
	typ := pass.TypesInfo.TypeOf(expression)
	if typ == nil {
		return true
	}
	typ = types.Unalias(typ)
	if _, ok := typ.(*types.TypeParam); ok {
		return true
	}
	_, ok := typ.Underlying().(*types.Array)
	return ok
}

type ps6010DerivedUpdate struct {
	object       types.Object
	dependencies ps6010Dependency
}

func ps6010RecordDerived(
	pass *analysis.Pass,
	statement ast.Stmt,
	inner, outer types.Object,
	innerName, outerName string,
	counts map[types.Object]int,
	addressTaken map[types.Object]bool,
	derived map[types.Object]ps6010Dependency,
	definitionFacts *ps6010DefinitionFacts,
) {
	recordAll := func(identifiers []*ast.Ident, expressions []ast.Expr) {
		// Go evaluates every RHS before assigning any LHS. Build this complete
		// update list against the pre-statement state, then commit it together.
		updates := make([]ps6010DerivedUpdate, 0, len(identifiers))
		for index, identifier := range identifiers {
			if identifier == nil || identifier.Name == "_" {
				continue
			}
			object := identObject(pass, identifier)
			if object == nil {
				continue
			}
			dependencies := ps6010ExprDependencies(
				pass, expressions[index], inner, outer, innerName, outerName, counts, addressTaken, derived, definitionFacts,
			)
			if counts[object] != 1 || addressTaken[object] {
				dependencies |= ps6010UnknownDependency
			}
			// Copies of slices, maps, pointers, interfaces, or value aggregates
			// containing them may share mutable storage with another root. Value-
			// only structs and arrays remain safe when their own root is stable.
			if ps6010TypeContainsReferenceStorage(object.Type(), make(map[types.Type]bool)) {
				dependencies |= ps6010UnknownDependency
			}
			updates = append(updates, ps6010DerivedUpdate{object: object, dependencies: dependencies})
		}
		for _, update := range updates {
			derived[update.object] = update.dependencies
		}
	}
	switch value := statement.(type) {
	case *ast.AssignStmt:
		if value.Tok != token.DEFINE && value.Tok != token.ASSIGN || len(value.Lhs) != len(value.Rhs) {
			return
		}
		identifiers := make([]*ast.Ident, len(value.Lhs))
		for index, lhs := range value.Lhs {
			identifiers[index], _ = lhs.(*ast.Ident)
		}
		recordAll(identifiers, value.Rhs)
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
			recordAll(specification.Names, specification.Values)
		}
	}
}

func ps6010InvariantOperand(pass *analysis.Pass, body *ast.BlockStmt, inner, outer types.Object, innerName, outerName string, definitionFacts *ps6010DefinitionFacts) ast.Expr {
	counts, addressTaken := ps6010WriteFacts(pass, body)
	derived := make(map[types.Object]ps6010Dependency)
	unsafeDerivedFlow := ps6010HasControlTransfer(body)
	for _, statement := range body.List {
		ps6010RecordDerived(pass, statement, inner, outer, innerName, outerName, counts, addressTaken, derived, definitionFacts)
		if unsafeDerivedFlow {
			for object, dependencies := range derived {
				derived[object] = dependencies | ps6010UnknownDependency
			}
		}
		var hit ast.Expr
		ast.Inspect(statement, func(node ast.Node) bool {
			if hit != nil {
				return false
			}
			if _, nested := node.(*ast.FuncLit); nested {
				return false
			}
			assign, ok := node.(*ast.AssignStmt)
			if !ok || assign.Tok != token.ADD_ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				return true
			}
			if _, ok := assign.Lhs[0].(*ast.Ident); !ok {
				return true
			}
			multiply, ok := assign.Rhs[0].(*ast.BinaryExpr)
			if !ok || multiply.Op != token.MUL {
				return true
			}
			check := func(invariant, varying ast.Expr) bool {
				index, ok := invariant.(*ast.IndexExpr)
				if !ok {
					return false
				}
				indexDependencies := ps6010ExprDependencies(
					pass, index.Index, inner, outer, innerName, outerName, counts, addressTaken, derived, definitionFacts,
				)
				// The selected container is part of the operand identity. In
				// rows[o][i], or row[i] after row := rows[o], an inner-only final
				// index does not make the accessed operand output-invariant.
				containerDependencies := ps6010ContainerDependencies(
					pass, index.X, inner, outer, innerName, outerName, counts, addressTaken, derived, definitionFacts,
				)
				varyingDependencies := ps6010ExprDependencies(
					pass, varying, inner, outer, innerName, outerName, counts, addressTaken, derived, definitionFacts,
				)
				return indexDependencies&ps6010InnerDependency != 0 &&
					indexDependencies&(ps6010OutputDependency|ps6010UnknownDependency) == 0 &&
					containerDependencies&(ps6010OutputDependency|ps6010UnknownDependency) == 0 &&
					varyingDependencies&ps6010OutputDependency != 0
			}
			if check(multiply.X, multiply.Y) {
				hit = multiply.X
			} else if check(multiply.Y, multiply.X) {
				hit = multiply.Y
			}
			return hit == nil
		})
		if hit != nil {
			return hit
		}
	}
	return nil
}

func ps6010HasControlTransfer(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		if _, branch := node.(*ast.BranchStmt); branch {
			found = true
			return false
		}
		return true
	})
	return found
}

// ps6010CountedHeader matches the counted loop header `for v := 0; v < BOUND;
// v++` and returns the loop variable name and the bound expression. PS1007
// shares this syntax-only helper; PS6010 itself uses the object-aware form
// below before issuing or applying a finding.
func ps6010CountedHeader(l *ast.ForStmt) (string, ast.Expr, bool) {
	init, ok := l.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return "", nil, false
	}
	iv, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, false
	}
	zero, ok := init.Rhs[0].(*ast.BasicLit)
	if !ok || zero.Kind != token.INT || zero.Value != "0" {
		return "", nil, false
	}
	cond, ok := l.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return "", nil, false
	}
	if lhs, ok := cond.X.(*ast.Ident); !ok || lhs.Name != iv.Name {
		return "", nil, false
	}
	post, ok := l.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil, false
	}
	if pv, ok := post.X.(*ast.Ident); !ok || pv.Name != iv.Name {
		return "", nil, false
	}
	return iv.Name, cond.Y, true
}

func ps6010CountedHeaderObject(pass *analysis.Pass, l *ast.ForStmt) (string, types.Object, ast.Expr, bool) {
	init, ok := l.Init.(*ast.AssignStmt)
	if !ok || init.Tok != token.DEFINE || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return "", nil, nil, false
	}
	iv, ok := init.Lhs[0].(*ast.Ident)
	if !ok {
		return "", nil, nil, false
	}
	object := identObject(pass, iv)
	zero, ok := init.Rhs[0].(*ast.BasicLit)
	if !ok || zero.Kind != token.INT || zero.Value != "0" {
		return "", nil, nil, false
	}
	cond, ok := l.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return "", nil, nil, false
	}
	if lhs, ok := cond.X.(*ast.Ident); !ok || !ps6010SameIdent(pass, lhs, object, iv.Name) {
		return "", nil, nil, false
	}
	post, ok := l.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return "", nil, nil, false
	}
	if pv, ok := post.X.(*ast.Ident); !ok || !ps6010SameIdent(pass, pv, object, iv.Name) {
		return "", nil, nil, false
	}
	return iv.Name, object, cond.Y, true
}

func ps6010SameIdent(pass *analysis.Pass, identifier *ast.Ident, object types.Object, fallback string) bool {
	if identifier == nil {
		return false
	}
	if object != nil {
		return identObject(pass, identifier) == object
	}
	return identifier.Name == fallback
}

func ps6010SameSimpleExpr(pass *analysis.Pass, left, right ast.Expr) bool {
	switch l := left.(type) {
	case *ast.Ident:
		r, ok := right.(*ast.Ident)
		if !ok {
			return false
		}
		leftObject, rightObject := identObject(pass, l), identObject(pass, r)
		if leftObject != nil || rightObject != nil {
			return leftObject != nil && leftObject == rightObject
		}
		return l.Name == r.Name
	case *ast.SelectorExpr:
		r, ok := right.(*ast.SelectorExpr)
		if !ok || l.Sel.Name != r.Sel.Name {
			return false
		}
		return ps6010SameSimpleExpr(pass, l.X, r.X)
	}
	return false
}

// ps6010Fix builds the unroll-by-4 rewrite for the exact canonical matvec
// shape and nothing else:
//
//	for o := 0; o < out; o++ {
//		acc := 0.0
//		for i := 0; i < n; i++ {
//			acc += a[i] * w[i*out+o]
//		}
//		dst[o] = acc
//	}
//
// with every name a plain identifier and out/n side-effect-free invariant
// expressions. The replacement keeps each output's accumulation order, so the
// per-output sums are bit-identical; a serial tail loop (sharing the hoisted
// index variable) handles the remainder. Any deviation from the canonical
// shape returns nil and the diagnostic stays advisory.
func ps6010Fix(pass *analysis.Pass, outer, inner ast.Node, definitionFacts *ps6010DefinitionFacts) *analysis.SuggestedFix {
	ol, ok := outer.(*ast.ForStmt)
	if !ok {
		return nil
	}
	il, ok := inner.(*ast.ForStmt)
	if !ok {
		return nil
	}
	o, oObject, outBound, ok := ps6010CountedHeaderObject(pass, ol)
	if !ok {
		return nil
	}
	i, iObject, nBound, ok := ps6010CountedHeaderObject(pass, il)
	if !ok || (iObject != nil && iObject == oObject) || (iObject == nil && oObject == nil && i == o) {
		return nil
	}
	out := simpleExprText(outBound)
	n := simpleExprText(nBound)
	if out == "" || n == "" ||
		ps6010ExprDependsOn(pass, outBound, oObject, o) || ps6010ExprDependsOn(pass, outBound, iObject, i) ||
		ps6010ExprDependsOn(pass, nBound, oObject, o) || ps6010ExprDependsOn(pass, nBound, iObject, i) {
		return nil
	}
	// Outer body: exactly { acc := 0.0; <inner loop>; dst[o] = acc }.
	if len(ol.Body.List) != 3 || ol.Body.List[1] != ast.Stmt(il) {
		return nil
	}
	accInit, ok := ol.Body.List[0].(*ast.AssignStmt)
	if !ok || accInit.Tok != token.DEFINE || len(accInit.Lhs) != 1 || len(accInit.Rhs) != 1 {
		return nil
	}
	accID, ok := accInit.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}
	seed, ok := accInit.Rhs[0].(*ast.BasicLit)
	if !ok || seed.Kind != token.FLOAT || seed.Value != "0.0" {
		return nil
	}
	acc := accID.Name
	accObject := identObject(pass, accID)
	store, ok := ol.Body.List[2].(*ast.AssignStmt)
	if !ok || store.Tok != token.ASSIGN || len(store.Lhs) != 1 || len(store.Rhs) != 1 {
		return nil
	}
	dstIx, ok := store.Lhs[0].(*ast.IndexExpr)
	if !ok {
		return nil
	}
	dstID, ok := dstIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if ix, ok := dstIx.Index.(*ast.Ident); !ok || !ps6010SameIdent(pass, ix, oObject, o) {
		return nil
	}
	if rhs, ok := store.Rhs[0].(*ast.Ident); !ok || !ps6010SameIdent(pass, rhs, accObject, acc) {
		return nil
	}
	dst := dstID.Name
	dstObject := identObject(pass, dstID)
	// Inner body: exactly { acc += a[i] * w[i*out+o] }.
	if len(il.Body.List) != 1 {
		return nil
	}
	as, ok := il.Body.List[0].(*ast.AssignStmt)
	if !ok || as.Tok != token.ADD_ASSIGN || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
		return nil
	}
	if lhs, ok := as.Lhs[0].(*ast.Ident); !ok || !ps6010SameIdent(pass, lhs, accObject, acc) {
		return nil
	}
	mul, ok := as.Rhs[0].(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return nil
	}
	aIx, ok := mul.X.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	aID, ok := aIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	if ix, ok := aIx.Index.(*ast.Ident); !ok || !ps6010SameIdent(pass, ix, iObject, i) {
		return nil
	}
	wIx, ok := mul.Y.(*ast.IndexExpr)
	if !ok {
		return nil
	}
	wID, ok := wIx.X.(*ast.Ident)
	if !ok {
		return nil
	}
	sum, ok := wIx.Index.(*ast.BinaryExpr)
	if !ok || sum.Op != token.ADD {
		return nil
	}
	stride, ok := sum.X.(*ast.BinaryExpr)
	if !ok || stride.Op != token.MUL {
		return nil
	}
	if sx, ok := stride.X.(*ast.Ident); !ok || !ps6010SameIdent(pass, sx, iObject, i) {
		return nil
	}
	if !ps6010SameSimpleExpr(pass, stride.Y, outBound) {
		return nil
	}
	if oy, ok := sum.Y.(*ast.Ident); !ok || !ps6010SameIdent(pass, oy, oObject, o) {
		return nil
	}
	a, w := aID.Name, wID.Name
	aObject, wObject := identObject(pass, aID), identObject(pass, wID)
	// The unrolled version delays each block's dst stores past later reads of a
	// and w. Distinct slices, maps, pointers, interfaces, or aggregates containing
	// them may still alias, so identifier inequality is not a proof. Value-only
	// objects and non-escaped fresh allocations provide the required separation.
	if dst == a || dst == w || !definitionFacts.storageProvenDisjoint(dstObject, aObject) ||
		!definitionFacts.storageProvenDisjoint(dstObject, wObject) {
		return nil
	}
	// The tile changes when individual outputs are stored. Bounds failures are
	// observable through recover/defer and through the already-written dst
	// prefix, so use the fast path only after non-panicking length checks prove
	// every generated index safe. Otherwise retain the exact scalar order.
	// Maps, pointer-to-array values, and type-parameter containers need different
	// panic proofs and therefore remain advisory.
	if !ps6010BoundedSequence(pass, dstIx.X) || !ps6010BoundedSequence(pass, aIx.X) ||
		!ps6010BoundedSequence(pass, wIx.X) {
		return nil
	}
	insertionScope := ps6010InsertionScope(pass, definitionFacts.parents, ol)
	if insertionScope == nil {
		// Replacing a labeled loop with three statements would leave the label
		// attached only to the generated declaration (and can make it unused),
		// changing both syntax and continue/break targets. Other non-list parents
		// are equally unsuitable for a multi-statement replacement.
		return nil
	}
	// The replacement hoists a new output-index declaration into the
	// surrounding statement list. A valid goto that previously skipped the
	// for statement can thereby become an illegal jump over that declaration.
	// Determining the exact target/source scope relation buys little here and is
	// easy to get wrong around nested blocks, so keep every diagnostic in a
	// goto-containing lexical function advisory-only. Gotos in nested function
	// literals have independent label scopes and do not affect this edit.
	if ps6010FunctionHasGoto(definitionFacts, ol) {
		return nil
	}
	// The bounds guard emitted below calls the predeclared len builtin. A local,
	// parameter, import, or package declaration may shadow it even though the
	// original scalar loop never mentions len. Keep the diagnostic advisory
	// unless name resolution at the exact edit position proves the builtin.
	if !ps6010BuiltinVisibleAt(pass, insertionScope, ol.Pos(), "len") {
		return nil
	}
	physical := pass.Fset.PositionFor(ol.Pos(), false)
	file := pass.Fset.File(ol.Pos())
	if file == nil || !physical.IsValid() {
		return nil
	}
	po, ok := ps6010FreshName(pass, insertionScope, ol.Pos(), fmt.Sprintf("psO%d", file.Offset(ol.Pos())))
	if !ok {
		return nil
	}
	// Names introduced by the rewrite must not capture anything the
	// templated body references.
	fresh := map[string]bool{po: true, "a0": true, "a1": true, "a2": true, "a3": true, "ai": true}
	for _, name := range []string{o, i, acc, a, w, dst, rootIdentName(outBound), rootIdentName(nBound)} {
		if fresh[name] {
			return nil
		}
	}
	// Use the physical, unadjusted column: //line directives must not affect the
	// inserted indentation or the deterministic temporary name above.
	ind := strings.Repeat("\t", physical.Column-1)
	var b strings.Builder
	wf := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	wf("%s := 0\n", po)
	wf("%sif %s <= 0 || (%s <= len(%s) && (%s <= 0 || (%s <= len(%s) && %s <= len(%s)/%s))) {\n", ind, out, out, dst, n, n, a, n, w, out)
	// Check the remaining trip count instead of po+3 so an extremely large
	// positive bound cannot wrap the tile condition near MaxInt. The leading
	// po < out also makes out-po safe when the no-iteration out<=0 case enters
	// this branch through the guard's first disjunct.
	wf("%s\tfor ; %s < %s && %s-%s >= 4; %s += 4 {\n", ind, po, out, out, po, po)
	wf("%s\t\ta0, a1, a2, a3 := 0.0, 0.0, 0.0, 0.0\n", ind)
	wf("%s\t\tfor %s := 0; %s < %s; %s++ {\n", ind, i, i, n, i)
	wf("%s\t\t\tai := %s[%s]\n", ind, a, i)
	wf("%s\t\t\ta0 += ai * %s[%s*%s+%s]\n", ind, w, i, out, po)
	wf("%s\t\t\ta1 += ai * %s[%s*%s+%s+1]\n", ind, w, i, out, po)
	wf("%s\t\t\ta2 += ai * %s[%s*%s+%s+2]\n", ind, w, i, out, po)
	wf("%s\t\t\ta3 += ai * %s[%s*%s+%s+3]\n", ind, w, i, out, po)
	wf("%s\t\t}\n", ind)
	wf("%s\t\t%s[%s], %s[%s+1], %s[%s+2], %s[%s+3] = a0, a1, a2, a3\n", ind, dst, po, dst, po, dst, po, dst, po)
	wf("%s\t}\n", ind)
	wf("%s\tfor ; %s < %s; %s++ {\n", ind, po, out, po)
	wf("%s\t\t%s := 0.0\n", ind, acc)
	wf("%s\t\tfor %s := 0; %s < %s; %s++ {\n", ind, i, i, n, i)
	wf("%s\t\t\t%s += %s[%s] * %s[%s*%s+%s]\n", ind, acc, a, i, w, i, out, po)
	wf("%s\t\t}\n", ind)
	wf("%s\t\t%s[%s] = %s\n", ind, dst, po, acc)
	wf("%s\t}\n", ind)
	wf("%s} else {\n", ind)
	wf("%s\tfor ; %s < %s; %s++ {\n", ind, po, out, po)
	wf("%s\t\t%s := 0.0\n", ind, acc)
	wf("%s\t\tfor %s := 0; %s < %s; %s++ {\n", ind, i, i, n, i)
	wf("%s\t\t\t%s += %s[%s] * %s[%s*%s+%s]\n", ind, acc, a, i, w, i, out, po)
	wf("%s\t\t}\n", ind)
	wf("%s\t\t%s[%s] = %s\n", ind, dst, po, acc)
	wf("%s\t}\n", ind)
	wf("%s}", ind)
	return &analysis.SuggestedFix{
		Message: "unroll the output loop by 4 with independent accumulators (serial tail)",
		TextEdits: []analysis.TextEdit{
			{Pos: ol.Pos(), End: ol.End(), NewText: []byte(b.String())},
		},
	}
}

func ps6010FunctionHasGoto(facts *ps6010DefinitionFacts, node ast.Node) bool {
	if facts == nil || node == nil {
		return true
	}
	function := facts.functionOwner[node]
	if function == nil {
		return true
	}
	return facts.functionHasGoto[function]
}

func ps6010InsertionScope(pass *analysis.Pass, parents map[ast.Node]ast.Node, loop *ast.ForStmt) ast.Node {
	parent := parents[loop]
	switch value := parent.(type) {
	case *ast.BlockStmt:
		if pass.TypesInfo.Scopes[value] != nil {
			return value
		}
		// go/types associates a function body's lexical scope with its FuncType,
		// not with the body BlockStmt itself.
		switch function := parents[value].(type) {
		case *ast.FuncDecl:
			return function.Type
		case *ast.FuncLit:
			return function.Type
		}
	case *ast.CaseClause, *ast.CommClause:
		return parent
	}
	return nil
}

func ps6010BuiltinVisibleAt(pass *analysis.Pass, insertionScope ast.Node, position token.Pos, name string) bool {
	if pass == nil || pass.TypesInfo == nil || insertionScope == nil || !position.IsValid() {
		return false
	}
	typeScope := pass.TypesInfo.Scopes[insertionScope]
	if typeScope == nil {
		return false
	}
	_, object := typeScope.LookupParent(name, position)
	builtin, ok := object.(*types.Builtin)
	return ok && builtin.Name() == name
}

func ps6010FreshName(pass *analysis.Pass, insertionScope ast.Node, position token.Pos, base string) (string, bool) {
	typeScope := pass.TypesInfo.Scopes[insertionScope]
	if typeScope == nil {
		// A missing lexical scope means uniqueness cannot be proven without a
		// second AST scan. Keep the diagnostic advisory instead.
		return "", false
	}
	available := func(name string) bool {
		// Lookup catches declarations later in the exact insertion scope too:
		// introducing the same name earlier would make a later short declaration
		// invalid. LookupParent catches every name already visible at the edit
		// position, including outer locals, imports, package declarations, and
		// predeclared identifiers.
		if typeScope.Lookup(name) != nil {
			return false
		}
		_, object := typeScope.LookupParent(name, position)
		return object == nil
	}
	if available(base) {
		return base, true
	}
	for suffix := 2; ; suffix++ {
		candidate := base + "_" + strconv.Itoa(suffix)
		if available(candidate) {
			return candidate, true
		}
	}
}
