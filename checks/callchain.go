package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/astutil"
)

// typedCallKind distinguishes package functions from methods. The distinction
// matters for stdlib rewrites: a package function with the right name is not a
// method with that name, even when both objects belong to the same package.
type typedCallKind uint8

const (
	typedPackageFunc typedCallKind = iota
	typedMethod
)

// typedCallStep describes one call in a nested call chain. NextArg is the
// argument containing the next call, or -1 for the terminal step.
//
// For example, the shape
//
//	binary.LittleEndian.Uint64(h.Sum(nil))
//
// is represented by two method steps: encoding/binary.Uint64 descending
// through argument 0, followed by hash/maphash.Sum with NextArg -1.
// Callees are resolved through go/types, so aliases work and shadowed package
// names or same-named user functions do not match.
type typedCallStep struct {
	PkgPath string
	Name    string
	Kind    typedCallKind
	Arity   int
	NextArg int
}

// repeatedTypedCall describes a maximal run of the same package function,
// nested through nextArg: Outer(...Outer(...Keep(x))...). The package binding
// is held constant so callers may delete redundant outer layers without
// orphaning a differently aliased import.
type repeatedTypedCall struct {
	outer  *ast.CallExpr
	keep   *ast.CallExpr
	fn     *types.Func
	calls  []*ast.CallExpr
	layers int
}

// typedUnaryCallChain describes one or more nested unary package calls whose
// concrete result type is identical to their argument type. base is the
// innermost argument; spans are the prefix/suffix scaffolding that can be
// deleted to splice base into the outer call's position byte-for-byte.
//
// Unlike repeatedTypedCall, adjacent layers may come from different packages.
// That supports heterogeneous chains such as
// bytes.Clone(slices.Clone(bytes.Clone(b))).
type typedUnaryCallChain struct {
	base  ast.Expr
	calls []*ast.CallExpr
	spans []tokenSpan
	paths []string
}

// typedCompoundNormalizerStage describes one logical normalization stage that
// is implemented by more than one typed call. root is the stage's complete
// result expression and input is the value it normalizes. bindings identifies
// the ordinary package imports used by each semantic role in the stage; the
// repeated-pipeline matcher holds those bindings stable across every layer.
// metadata is deliberately opaque so a rule can carry the constants or other
// postconditions needed by its adjacent-layer compatibility proof.
//
// For example, strings.Join(strings.Fields(x), " ") is one stage whose root
// is Join, input is x, bindings contains the shared strings import, and
// metadata contains the separator value.
type typedCompoundNormalizerStage struct {
	root     *ast.CallExpr
	input    ast.Expr
	bindings []*types.PkgName
	metadata any
}

// typedGuardedPackageTransformer describes an if whose package predicate is
// immediately followed, on the true path, by a package transformer over the
// same input. Some standard-library APIs provide a single multi-result call
// for exactly this composition: HasPrefix+TrimPrefix becomes CutPrefix, for
// example. predicateExpression and transformerExpression retain surrounding
// parentheses so a rule can preserve or deliberately replace their complete
// source spans.
type typedGuardedPackageTransformer struct {
	statement             ast.Stmt
	predicate             *ast.CallExpr
	predicateExpression   ast.Expr
	predicateSelector     *ast.SelectorExpr
	input                 *ast.Ident
	transformer           *ast.CallExpr
	transformerExpression ast.Expr
	predicateCompanion    ast.Expr
	transformerCompanion  ast.Expr
}

// guardedSingleFallback is the one-statement false path paired with a guarded
// transformer. For an explicit else block removalEnd consumes the complete if;
// otherwise the fallback must be the immediately following statement and
// removalEnd consumes that statement. This lets deletion-only rules retain the
// true-path statement byte-for-byte while removing equivalent control flow.
type guardedSingleFallback struct {
	statement  ast.Stmt
	removalEnd token.Pos
}

func matchGuardedSingleFallback(statement *ast.IfStmt, parents map[ast.Node]ast.Node) (guardedSingleFallback, bool) {
	if statement == nil {
		return guardedSingleFallback{}, false
	}
	if statement.Else != nil {
		block, ok := statement.Else.(*ast.BlockStmt)
		if !ok || len(block.List) != 1 {
			return guardedSingleFallback{}, false
		}
		return guardedSingleFallback{statement: block.List[0], removalEnd: statement.End()}, true
	}
	block, ok := parents[statement].(*ast.BlockStmt)
	if !ok {
		return guardedSingleFallback{}, false
	}
	for index, candidate := range block.List {
		if candidate == statement && index+1 < len(block.List) {
			fallback := block.List[index+1]
			return guardedSingleFallback{statement: fallback, removalEnd: fallback.End()}, true
		}
	}
	return guardedSingleFallback{}, false
}

// trailingDeletionComment reports a line comment immediately after the final
// token a deletion consumes. The comment lies outside the AST node's End, but
// deleting the node would silently reattach it to retained syntax.
func trailingDeletionComment(pass *analysis.Pass, file *ast.File, end token.Pos) bool {
	endLine := pass.Fset.Position(end).Line
	for _, group := range file.Comments {
		if group.Pos() >= end && pass.Fset.Position(group.Pos()).Line == endLine {
			return true
		}
	}
	return false
}

// typedGuardedFallbackTransformer is a guarded transformer whose false path
// assigns or returns a sentinel that the transformer itself already returns
// when its predicate is false. prefix and suffix are the control-flow spans a
// deletion-only fix removes while retaining the original transformer call and
// assignment/return spelling byte-for-byte.
type typedGuardedFallbackTransformer struct {
	composition typedGuardedPackageTransformer
	statement   *ast.IfStmt
	prefix      tokenSpan
	suffix      tokenSpan
}

// matchTypedGuardedFallbackTransformer recognizes the exact control-flow
// families shared by redundant predicate+observer compositions:
//
//   - transformed return plus an explicit else or immediate fallback return;
//   - transformed assignment plus a one-statement sentinel else; and
//   - a sentinel initializer immediately before a transformed assignment.
//
// The true branch must contain only the transformer statement. Assignment
// targets are resolved by object identity, and sentinel owns API-specific
// constant semantics such as -1 for Index or 0 for Count.
func matchTypedGuardedFallbackTransformer(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	parents map[ast.Node]ast.Node,
	composition typedGuardedPackageTransformer,
	sentinel func(ast.Expr) bool,
) (typedGuardedFallbackTransformer, bool) {
	if statement == nil || statement.Body == nil || len(statement.Body.List) != 1 || sentinel == nil ||
		composition.statement != statement.Body.List[0] {
		return typedGuardedFallbackTransformer{}, false
	}
	switch kept := composition.statement.(type) {
	case *ast.ReturnStmt:
		fallback, ok := matchGuardedSingleFallback(statement, parents)
		if !ok {
			return typedGuardedFallbackTransformer{}, false
		}
		returned, ok := fallback.statement.(*ast.ReturnStmt)
		if !ok || len(returned.Results) != 1 || !sentinel(returned.Results[0]) {
			return typedGuardedFallbackTransformer{}, false
		}
		return typedGuardedFallbackTransformer{
			composition: composition,
			statement:   statement,
			prefix:      tokenSpan{start: statement.Pos(), end: kept.Pos()},
			suffix:      tokenSpan{start: kept.End(), end: fallback.removalEnd},
		}, true
	case *ast.AssignStmt:
		if kept.Tok != token.ASSIGN {
			return typedGuardedFallbackTransformer{}, false
		}
		target, ok := plainAssignmentTarget(pass, kept)
		if !ok {
			return typedGuardedFallbackTransformer{}, false
		}
		if statement.Else != nil {
			fallback, ok := matchGuardedSingleFallback(statement, parents)
			if !ok {
				return typedGuardedFallbackTransformer{}, false
			}
			assignment, ok := fallback.statement.(*ast.AssignStmt)
			fallbackTarget, targetOK := plainAssignmentTarget(pass, assignment)
			if !ok || assignment.Tok != token.ASSIGN || !targetOK || fallbackTarget != target ||
				!sentinel(assignment.Rhs[0]) {
				return typedGuardedFallbackTransformer{}, false
			}
			return typedGuardedFallbackTransformer{
				composition: composition,
				statement:   statement,
				prefix:      tokenSpan{start: statement.Pos(), end: kept.Pos()},
				suffix:      tokenSpan{start: kept.End(), end: fallback.removalEnd},
			}, true
		}
		initializer, ok := previousSentinelAssignment(pass, statement, parents, target, sentinel)
		if !ok {
			return typedGuardedFallbackTransformer{}, false
		}
		return typedGuardedFallbackTransformer{
			composition: composition,
			statement:   statement,
			prefix: tokenSpan{
				start: initializer.Rhs[0].Pos(),
				end:   composition.transformerExpression.Pos(),
			},
			suffix: tokenSpan{start: kept.End(), end: statement.End()},
		}, true
	}
	return typedGuardedFallbackTransformer{}, false
}

// matchTypedGuardedIdentityTransformer handles transformers that return their
// input unchanged when the predicate is false. An explicit original-input
// return/assignment/initializer is delegated to the generic fallback matcher.
// A sole in-place assignment also has an implicit identity false path because
// skipping the branch leaves that same input object unchanged.
func matchTypedGuardedIdentityTransformer(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	parents map[ast.Node]ast.Node,
	composition typedGuardedPackageTransformer,
) (typedGuardedFallbackTransformer, bool) {
	inputObject := pass.TypesInfo.ObjectOf(composition.input)
	if inputObject == nil {
		return typedGuardedFallbackTransformer{}, false
	}
	if match, ok := matchTypedGuardedFallbackTransformer(pass, statement, parents, composition, func(expression ast.Expr) bool {
		return plainObjectExpression(pass, expression, inputObject)
	}); ok && identityFallbackPreservesDynamicType(pass, parents, composition) {
		return match, true
	}
	if statement == nil || statement.Else != nil || statement.Body == nil || len(statement.Body.List) != 1 ||
		composition.statement != statement.Body.List[0] {
		return typedGuardedFallbackTransformer{}, false
	}
	assignment, ok := composition.statement.(*ast.AssignStmt)
	if !ok || assignment.Tok != token.ASSIGN {
		return typedGuardedFallbackTransformer{}, false
	}
	target, ok := plainAssignmentTarget(pass, assignment)
	if !ok || target != inputObject {
		return typedGuardedFallbackTransformer{}, false
	}
	return typedGuardedFallbackTransformer{
		composition: composition,
		statement:   statement,
		prefix:      tokenSpan{start: statement.Pos(), end: assignment.Pos()},
		suffix:      tokenSpan{start: assignment.End(), end: statement.End()},
	}, true
}

// identityFallbackPreservesDynamicType rejects the only contextual conversion
// that can distinguish an original-input fallback from a transformer result:
// assigning or returning differently named string types through an interface.
// Concrete destinations convert both paths to the same static type.
func identityFallbackPreservesDynamicType(
	pass *analysis.Pass,
	parents map[ast.Node]ast.Node,
	composition typedGuardedPackageTransformer,
) bool {
	inputType := pass.TypesInfo.TypeOf(composition.input)
	transformedType := pass.TypesInfo.TypeOf(composition.transformerExpression)
	if inputType == nil || transformedType == nil || types.Identical(inputType, transformedType) {
		return inputType != nil && transformedType != nil
	}
	var destination types.Type
	switch statement := composition.statement.(type) {
	case *ast.AssignStmt:
		if len(statement.Lhs) == 1 {
			destination = pass.TypesInfo.TypeOf(statement.Lhs[0])
		}
	case *ast.ReturnStmt:
		destination = enclosingSingleResultType(pass, parents, statement)
	}
	if destination == nil {
		return false
	}
	_, isInterface := destination.Underlying().(*types.Interface)
	return !isInterface
}

func enclosingSingleResultType(pass *analysis.Pass, parents map[ast.Node]ast.Node, node ast.Node) types.Type {
	for current := parents[node]; current != nil; current = parents[current] {
		var signature *types.Signature
		switch function := current.(type) {
		case *ast.FuncDecl:
			object, _ := pass.TypesInfo.ObjectOf(function.Name).(*types.Func)
			if object != nil {
				signature, _ = object.Type().(*types.Signature)
			}
		case *ast.FuncLit:
			signature, _ = pass.TypesInfo.TypeOf(function.Type).(*types.Signature)
		default:
			continue
		}
		if signature != nil && signature.Results().Len() == 1 {
			return signature.Results().At(0).Type()
		}
		return nil
	}
	return nil
}

func plainObjectExpression(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	identifier, ok := ps2110Unparen(expression).(*ast.Ident)
	return ok && object != nil && pass.TypesInfo.ObjectOf(identifier) == object
}

func plainAssignmentTarget(pass *analysis.Pass, assignment *ast.AssignStmt) (types.Object, bool) {
	if assignment == nil || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return nil, false
	}
	identifier, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || identifier.Name == "_" {
		return nil, false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	return object, object != nil
}

func previousSentinelAssignment(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	parents map[ast.Node]ast.Node,
	target types.Object,
	sentinel func(ast.Expr) bool,
) (*ast.AssignStmt, bool) {
	block, ok := parents[statement].(*ast.BlockStmt)
	if !ok {
		return nil, false
	}
	for index, candidate := range block.List {
		if candidate != statement || index == 0 {
			continue
		}
		initializer, ok := block.List[index-1].(*ast.AssignStmt)
		if !ok || (initializer.Tok != token.ASSIGN && initializer.Tok != token.DEFINE) {
			return nil, false
		}
		initializerTarget, ok := plainAssignmentTarget(pass, initializer)
		return initializer, ok && initializerTarget == target && sentinel(initializer.Rhs[0])
	}
	return nil, false
}

func guardedFallbackDeletionFix(
	pass *analysis.Pass,
	file *ast.File,
	match *typedGuardedFallbackTransformer,
	message string,
) (analysis.SuggestedFix, bool) {
	return guardedFallbackDeletionFixPaths(pass, file, nil, match, message)
}

// guardedFallbackDeletionFixPaths is the cross-package form. It applies the
// same comment/local-liveness policy and may remove predicate-only imports
// that become orphaned when the retained transformer belongs to another
// standard-library package.
func guardedFallbackDeletionFixPaths(
	pass *analysis.Pass,
	file *ast.File,
	pkgPaths []string,
	match *typedGuardedFallbackTransformer,
	message string,
) (analysis.SuggestedFix, bool) {
	if match == nil || trailingDeletionComment(pass, file, match.suffix.end) ||
		!deletionsKeepRequiredLocalVariables(pass, file, match.prefix, match.suffix) {
		return analysis.SuggestedFix{}, false
	}
	return fixDeletedCallScaffoldingPaths(pass, file, pkgPaths, message, match.prefix, match.suffix)
}

// sameTypedStableCompanion accepts the same plain object, or equal compile-
// time constants when the API's companion is value-like. Slice companions
// must disable constants so two independently evaluated slice expressions are
// never conflated.
func sameTypedStableCompanion(pass *analysis.Pass, left, right ast.Expr, allowEqualConstants bool) bool {
	left = ps2110Unparen(left)
	right = ps2110Unparen(right)
	if allowEqualConstants {
		leftValue, leftOK := pass.TypesInfo.Types[left]
		rightValue, rightOK := pass.TypesInfo.Types[right]
		if leftOK && rightOK && leftValue.Value != nil && rightValue.Value != nil {
			return leftValue.Value.Kind() == rightValue.Value.Kind() &&
				constant.Compare(leftValue.Value, token.EQL, rightValue.Value)
		}
	}
	leftIdentifier, leftOK := left.(*ast.Ident)
	rightIdentifier, rightOK := right.(*ast.Ident)
	if !leftOK || !rightOK {
		return false
	}
	leftObject := pass.TypesInfo.ObjectOf(leftIdentifier)
	return leftObject != nil && leftObject == pass.TypesInfo.ObjectOf(rightIdentifier)
}

// stableTypedValue reports a compile-time constant or a plain variable/const
// object. Moving such an expression from a guarded branch to an unconditional
// package call cannot introduce a call, selector, index, or other observable
// evaluation on the former false path.
func stableTypedValue(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	if value, ok := pass.TypesInfo.Types[expression]; ok && value.Value != nil {
		return true
	}
	identifier, ok := expression.(*ast.Ident)
	return ok && identifier.Name != "_" && pass.TypesInfo.ObjectOf(identifier) != nil
}

// typedIntegerSentinel reports whether expression is the requested exact
// compile-time integer. Guarded stdlib transformers use it to share sentinel
// policy without accepting conversions or run-time expressions accidentally.
func typedIntegerSentinel(pass *analysis.Pass, expression ast.Expr, expected int64) bool {
	value, ok := pass.TypesInfo.Types[ps2110Unparen(expression)]
	if !ok || value.Value == nil || value.Value.Kind() != constant.Int {
		return false
	}
	integer, exact := constant.Int64Val(value.Value)
	return exact && integer == expected
}

// typedAssignedIndexedPackageProducer describes a one-result assignment whose
// right side directly indexes a package producer call. It is the reusable
// typed shape for replacing an allocated aggregate with a multi-result API,
// such as `head := strings.SplitN(s, sep, 2)[0]` becoming a strings.Cut
// assignment. resultExpression retains parentheses around the index so a rule
// can delete the complete consumer scaffolding without re-rendering operands.
type typedAssignedIndexedPackageProducer struct {
	assignment       *ast.AssignStmt
	resultExpression ast.Expr
	index            *ast.IndexExpr
	producer         *ast.CallExpr
	producerSelector *ast.SelectorExpr
}

// typedGuardedIndexedPackageProducer describes a package predicate guarding a
// first-branch expression that indexes a package aggregate producer over the
// same input. It supports assignment and return consumers. This is the shared
// shape for allocation-free multi-result rewrites such as
// Contains+SplitN(...)[1] -> Cut.
type typedGuardedIndexedPackageProducer struct {
	predicateExpression ast.Expr
	predicateSelector   *ast.SelectorExpr
	predicateCompanion  ast.Expr
	resultExpression    ast.Expr
	index               *ast.IndexExpr
	producer            *ast.CallExpr
	producerCompanion   ast.Expr
}

// matchTypedGuardedIndexedPackageProducer matches an exact package predicate
// condition and an immediately consumed indexed producer in the true branch.
// The input is required to be the same plain object, and an assignment target
// must be a plain identifier so moving producer work into the condition cannot
// cross an effectful left-hand-side evaluation. Companion equality, index,
// count, and result-identity policy remain rule-specific.
func matchTypedGuardedIndexedPackageProducer(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	pkgPath, predicateName, producerName string,
) (typedGuardedIndexedPackageProducer, bool) {
	if statement == nil || statement.Init != nil || statement.Body == nil || len(statement.Body.List) == 0 {
		return typedGuardedIndexedPackageProducer{}, false
	}
	predicateExpression := statement.Cond
	predicate, ok := ps2110Unparen(predicateExpression).(*ast.CallExpr)
	if !ok || len(predicate.Args) != 2 || predicate.Ellipsis.IsValid() {
		return typedGuardedIndexedPackageProducer{}, false
	}
	binding, ok := typedPackageBinding(pass, predicate.Fun)
	if !ok || !typedPackageCallWithBinding(pass, predicate, pkgPath, predicateName, binding) {
		return typedGuardedIndexedPackageProducer{}, false
	}
	predicateSelector, ok := ps2110Unparen(predicate.Fun).(*ast.SelectorExpr)
	if !ok {
		return typedGuardedIndexedPackageProducer{}, false
	}

	var resultExpression ast.Expr
	switch first := statement.Body.List[0].(type) {
	case *ast.AssignStmt:
		if (first.Tok != token.ASSIGN && first.Tok != token.DEFINE) || len(first.Lhs) != 1 || len(first.Rhs) != 1 {
			return typedGuardedIndexedPackageProducer{}, false
		}
		left, ok := ps2110Unparen(first.Lhs[0]).(*ast.Ident)
		if !ok || left.Name == "_" {
			return typedGuardedIndexedPackageProducer{}, false
		}
		resultExpression = first.Rhs[0]
	case *ast.ReturnStmt:
		if len(first.Results) != 1 {
			return typedGuardedIndexedPackageProducer{}, false
		}
		resultExpression = first.Results[0]
	default:
		return typedGuardedIndexedPackageProducer{}, false
	}
	index, ok := ps2110Unparen(resultExpression).(*ast.IndexExpr)
	if !ok {
		return typedGuardedIndexedPackageProducer{}, false
	}
	producer, ok := ps2110Unparen(index.X).(*ast.CallExpr)
	if !ok || len(producer.Args) < 2 || producer.Ellipsis.IsValid() ||
		!typedPackageCallWithBinding(pass, producer, pkgPath, producerName, binding) {
		return typedGuardedIndexedPackageProducer{}, false
	}
	predicateInput, ok := ps2110Unparen(predicate.Args[0]).(*ast.Ident)
	if !ok || predicateInput.Name == "_" {
		return typedGuardedIndexedPackageProducer{}, false
	}
	producerInput, ok := ps2110Unparen(producer.Args[0]).(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(predicateInput) == nil ||
		pass.TypesInfo.ObjectOf(predicateInput) != pass.TypesInfo.ObjectOf(producerInput) {
		return typedGuardedIndexedPackageProducer{}, false
	}
	return typedGuardedIndexedPackageProducer{
		predicateExpression: predicateExpression,
		predicateSelector:   predicateSelector,
		predicateCompanion:  predicate.Args[1],
		resultExpression:    resultExpression,
		index:               index,
		producer:            producer,
		producerCompanion:   producer.Args[1],
	}, true
}

// matchTypedAssignedIndexedPackageProducer matches a direct indexed package
// call in a single-value = or := assignment. acceptProducer owns the API-
// specific arity/semantic policy; package binding, callee kind, parentheses,
// and the plain assignment target are checked centrally.
func matchTypedAssignedIndexedPackageProducer(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
	pkgPath string,
	acceptProducer func(function *types.Func, signature *types.Signature, call *ast.CallExpr) bool,
) (typedAssignedIndexedPackageProducer, bool) {
	if assignment == nil || (assignment.Tok != token.ASSIGN && assignment.Tok != token.DEFINE) ||
		len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return typedAssignedIndexedPackageProducer{}, false
	}
	left, ok := ps2110Unparen(assignment.Lhs[0]).(*ast.Ident)
	if !ok || left.Name == "_" {
		return typedAssignedIndexedPackageProducer{}, false
	}
	resultExpression := assignment.Rhs[0]
	index, ok := ps2110Unparen(resultExpression).(*ast.IndexExpr)
	if !ok {
		return typedAssignedIndexedPackageProducer{}, false
	}
	producer, ok := ps2110Unparen(index.X).(*ast.CallExpr)
	if !ok || producer.Ellipsis.IsValid() {
		return typedAssignedIndexedPackageProducer{}, false
	}
	function, signature, ok := typedCallee(pass, producer.Fun)
	if !ok || signature.Recv() != nil || function.Pkg() == nil || function.Pkg().Path() != pkgPath ||
		acceptProducer == nil || !acceptProducer(function, signature, producer) {
		return typedAssignedIndexedPackageProducer{}, false
	}
	if _, ok := typedPackageBinding(pass, producer.Fun); !ok {
		return typedAssignedIndexedPackageProducer{}, false
	}
	selector, ok := ps2110Unparen(producer.Fun).(*ast.SelectorExpr)
	if !ok {
		return typedAssignedIndexedPackageProducer{}, false
	}
	return typedAssignedIndexedPackageProducer{
		assignment:       assignment,
		resultExpression: resultExpression,
		index:            index,
		producer:         producer,
		producerSelector: selector,
	}, true
}

// matchTypedGuardedPackageTransformer matches the control/data-flow shape
//
//	if pkg.Predicate(value, companion) {
//		result := pkg.Transformer(value, companion)
//	}
//
// and its simple-assignment/return variants. The calls must use one ordinary
// package binding, the input must be the same object, and the transformer must
// be the branch's first operation. The caller owns companion equivalence and
// the semantic proof that a combined API preserves evaluation and results.
func matchTypedGuardedPackageTransformer(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	pkgPath, predicateName, transformerName string,
	transformerArity int,
) (typedGuardedPackageTransformer, bool) {
	matched, ok := matchTypedGuardedTransformer(
		pass, statement,
		typedGuardedTransformerCallSpec{pkgPath: pkgPath, name: predicateName, arity: 2},
		typedGuardedTransformerCallSpec{pkgPath: pkgPath, name: transformerName, arity: transformerArity},
		false, true,
	)
	if !ok {
		return typedGuardedPackageTransformer{}, false
	}
	matched.predicateCompanion = matched.predicate.Args[1]
	matched.transformerCompanion = matched.transformer.Args[1]
	return matched, true
}

// matchTypedGuardedCrossPackageTransformer is the reusable cross-package
// counterpart used when one standard-library package proves the precondition
// for a transformer in another. predicateNegated accepts an exact !Predicate
// condition while preserving the complete condition span for deletion fixes.
func matchTypedGuardedCrossPackageTransformer(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	predicatePath, predicateName string,
	predicateArity int,
	transformerPath, transformerName string,
	transformerArity int,
	predicateNegated bool,
) (typedGuardedPackageTransformer, bool) {
	return matchTypedGuardedTransformer(
		pass, statement,
		typedGuardedTransformerCallSpec{pkgPath: predicatePath, name: predicateName, arity: predicateArity},
		typedGuardedTransformerCallSpec{pkgPath: transformerPath, name: transformerName, arity: transformerArity},
		predicateNegated, false,
	)
}

type typedGuardedTransformerCallSpec struct {
	pkgPath string
	name    string
	arity   int
}

func matchTypedGuardedTransformer(
	pass *analysis.Pass,
	statement *ast.IfStmt,
	predicateSpec, transformerSpec typedGuardedTransformerCallSpec,
	predicateNegated, requireSameBinding bool,
) (typedGuardedPackageTransformer, bool) {
	if statement == nil || statement.Init != nil || statement.Body == nil || len(statement.Body.List) == 0 {
		return typedGuardedPackageTransformer{}, false
	}
	predicateExpression := statement.Cond
	predicateCallExpression := predicateExpression
	if predicateNegated {
		negation, ok := ps2110Unparen(predicateExpression).(*ast.UnaryExpr)
		if !ok || negation.Op != token.NOT {
			return typedGuardedPackageTransformer{}, false
		}
		predicateCallExpression = negation.X
	}
	predicate, ok := ps2110Unparen(predicateCallExpression).(*ast.CallExpr)
	if !ok || len(predicate.Args) != predicateSpec.arity || predicate.Ellipsis.IsValid() {
		return typedGuardedPackageTransformer{}, false
	}
	predicateBinding, ok := typedPackageBinding(pass, predicate.Fun)
	if !ok || !typedPackageCallWithBinding(pass, predicate, predicateSpec.pkgPath, predicateSpec.name, predicateBinding) {
		return typedGuardedPackageTransformer{}, false
	}
	predicateSelector, ok := ps2110Unparen(predicate.Fun).(*ast.SelectorExpr)
	if !ok {
		return typedGuardedPackageTransformer{}, false
	}

	var firstStatement ast.Stmt
	var transformerExpression ast.Expr
	switch first := statement.Body.List[0].(type) {
	case *ast.AssignStmt:
		if (first.Tok != token.ASSIGN && first.Tok != token.DEFINE) || len(first.Lhs) != 1 || len(first.Rhs) != 1 {
			return typedGuardedPackageTransformer{}, false
		}
		left, ok := ps2110Unparen(first.Lhs[0]).(*ast.Ident)
		if !ok || left.Name == "_" {
			return typedGuardedPackageTransformer{}, false
		}
		firstStatement = first
		transformerExpression = first.Rhs[0]
	case *ast.ReturnStmt:
		if len(first.Results) != 1 {
			return typedGuardedPackageTransformer{}, false
		}
		firstStatement = first
		transformerExpression = first.Results[0]
	default:
		return typedGuardedPackageTransformer{}, false
	}
	transformer, ok := ps2110Unparen(transformerExpression).(*ast.CallExpr)
	if !ok || len(transformer.Args) != transformerSpec.arity || transformer.Ellipsis.IsValid() {
		return typedGuardedPackageTransformer{}, false
	}
	transformerBinding, ok := typedPackageBinding(pass, transformer.Fun)
	if !ok || requireSameBinding && transformerBinding != predicateBinding ||
		!typedPackageCallWithBinding(pass, transformer, transformerSpec.pkgPath, transformerSpec.name, transformerBinding) {
		return typedGuardedPackageTransformer{}, false
	}

	predicateInput, ok := ps2110Unparen(predicate.Args[0]).(*ast.Ident)
	if !ok || predicateInput.Name == "_" {
		return typedGuardedPackageTransformer{}, false
	}
	transformerInput, ok := ps2110Unparen(transformer.Args[0]).(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(predicateInput) == nil ||
		pass.TypesInfo.ObjectOf(predicateInput) != pass.TypesInfo.ObjectOf(transformerInput) {
		return typedGuardedPackageTransformer{}, false
	}

	return typedGuardedPackageTransformer{
		statement:             firstStatement,
		predicate:             predicate,
		predicateExpression:   predicateExpression,
		predicateSelector:     predicateSelector,
		input:                 predicateInput,
		transformer:           transformer,
		transformerExpression: transformerExpression,
	}, true
}

// repeatedTypedCompoundNormalizerPipeline is a maximal outer prefix of
// compound stages whose retained terminal postcondition makes every collected
// outer stage an identity. Keeping the last stage removes all earlier
// normalization work while preserving the original input evaluation.
type repeatedTypedCompoundNormalizerPipeline struct {
	outer  *ast.CallExpr
	keep   *ast.CallExpr
	stages []typedCompoundNormalizerStage
}

// typedNestedPackageCall identifies a direct nested package call and the full
// argument expression that encloses it (possibly with parentheses). Removing
// expression..Lparen and Rparen..expression splices the call's arguments into
// its parent variadic call without re-rendering any semantic operand.
type typedNestedPackageCall struct {
	expression ast.Expr
	call       *ast.CallExpr
}

// typedPackageWrapperProducerChain describes one or more identical unary
// package wrappers around a direct package producer whose result is already a
// fixed point for that wrapper. producerExpression includes any parentheses in
// the innermost wrapper argument so a deletion-only fix can retain it verbatim.
//
// For example:
//
//	path.Clean(path.Clean(path.Dir(name)))
//
// has two wrappers and path.Dir as its producer. The wrapper and producer must
// resolve through the same ordinary package binding; dot imports, function
// values, methods, and cross-package lookalikes are excluded centrally.
type typedPackageWrapperProducerChain struct {
	outer              *ast.CallExpr
	producer           *ast.CallExpr
	producerExpression ast.Expr
	wrappers           []*ast.CallExpr
	wrapper            *types.Func
	producerFunction   *types.Func
	binding            *types.PkgName
}

// typedPackageConsumerProducerComposition describes a package function whose
// configured argument is directly produced by another function from the same
// concrete package binding. It is the shared typed shape for inverse or
// absorbing compositions such as strings.Join(strings.Split(...), ...).
// producerExpression retains any parentheses around the nested producer so a
// caller can make a byte-preserving, deletion-only edit when its semantic
// identity permits that producer and consumer to disappear together.
type typedPackageConsumerProducerComposition struct {
	consumer           *ast.CallExpr
	producer           *ast.CallExpr
	producerExpression ast.Expr
	consumerFunction   *types.Func
	producerFunction   *types.Func
	consumerBinding    *types.PkgName
	producerBinding    *types.PkgName
}

// typedMethodConstructorCall describes an immediate method call on the result
// of a package constructor, for example strings.NewReader(s).Len(). Both
// callees are resolved objects; syntactic lookalikes and function values do
// not match.
type typedMethodConstructorCall struct {
	methodCall      *ast.CallExpr
	constructorCall *ast.CallExpr
	method          *types.Func
	methodSignature *types.Signature
	constructor     *types.Func
}

// matchTypedMethodOnPackageConstructor matches a method call whose selector
// receiver is immediately a package-function call. It deliberately rejects
// dot-imported constructors so a rewrite can reason about qualifier/import
// removal from an ordinary source selector.
func matchTypedMethodOnPackageConstructor(pass *analysis.Pass, outer *ast.CallExpr) (typedMethodConstructorCall, bool) {
	if outer == nil || outer.Ellipsis.IsValid() {
		return typedMethodConstructorCall{}, false
	}
	method, methodSignature, ok := typedCallee(pass, outer.Fun)
	if !ok || methodSignature.Recv() == nil || method.Pkg() == nil {
		return typedMethodConstructorCall{}, false
	}
	selector, ok := ps2110Unparen(outer.Fun).(*ast.SelectorExpr)
	if !ok {
		return typedMethodConstructorCall{}, false
	}
	constructorCall, ok := ps2110Unparen(selector.X).(*ast.CallExpr)
	if !ok || constructorCall.Ellipsis.IsValid() {
		return typedMethodConstructorCall{}, false
	}
	constructor, constructorSignature, ok := typedCallee(pass, constructorCall.Fun)
	if !ok || constructorSignature.Recv() != nil || constructor.Pkg() == nil {
		return typedMethodConstructorCall{}, false
	}
	if _, ok := typedPackageBinding(pass, constructorCall.Fun); !ok {
		return typedMethodConstructorCall{}, false
	}
	return typedMethodConstructorCall{
		methodCall: outer, constructorCall: constructorCall,
		method: method, methodSignature: methodSignature, constructor: constructor,
	}, true
}

// collectTypedNestedPackageCallTree walks direct arguments depth-first and
// records every non-empty, non-ellipsis call to pkgPath.name through binding.
// It is package-agnostic: io.MultiReader trees, errors.Join trees, and future
// variadic stdlib constructors all share this exact typed/comment-safe shape.
func collectTypedNestedPackageCallTree(
	pass *analysis.Pass,
	arguments []ast.Expr,
	pkgPath, name string,
	binding *types.PkgName,
	nested *[]typedNestedPackageCall,
) {
	for _, argument := range arguments {
		call, ok := ps2110Unparen(argument).(*ast.CallExpr)
		if !ok || len(call.Args) == 0 || call.Ellipsis.IsValid() ||
			!typedPackageCallWithBinding(pass, call, pkgPath, name, binding) {
			continue
		}
		*nested = append(*nested, typedNestedPackageCall{expression: argument, call: call})
		collectTypedNestedPackageCallTree(pass, call.Args, pkgPath, name, binding, nested)
	}
}

// collectTypedNestedPackageCallTreeMatching is the policy-aware form for
// associative variadic calls whose syntax alone is insufficient to prove that
// a nested result may be spliced into its parent. accept can enforce concrete
// generic types, snapshot/evaluation boundaries, or other operation-specific
// invariants while this helper owns typed callee/binding resolution and the
// depth-first tree walk.
func collectTypedNestedPackageCallTreeMatching(
	pass *analysis.Pass,
	parent *ast.CallExpr,
	pkgPath, name string,
	binding *types.PkgName,
	accept func(parent *ast.CallExpr, argument int, nested *ast.CallExpr) bool,
	nested *[]typedNestedPackageCall,
) {
	if parent == nil {
		return
	}
	for index, argument := range parent.Args {
		call, ok := ps2110Unparen(argument).(*ast.CallExpr)
		if !ok || len(call.Args) == 0 || call.Ellipsis.IsValid() ||
			!typedPackageCallWithBinding(pass, call, pkgPath, name, binding) ||
			accept != nil && !accept(parent, index, call) {
			continue
		}
		*nested = append(*nested, typedNestedPackageCall{expression: argument, call: call})
		collectTypedNestedPackageCallTreeMatching(pass, call, pkgPath, name, binding, accept, nested)
	}
}

// matchTypedPackageWrapperProducerChain matches a maximal run of unary
// pkgPath.wrapperName calls followed by a direct package-function producer.
// acceptProducer supplies the semantic fixed-point proof (for example, path.Dir
// always returns a cleaned nonempty path). Exact result types are required at
// every layer so deleting wrappers cannot change an interface's dynamic type.
func matchTypedPackageWrapperProducerChain(
	pass *analysis.Pass,
	root *ast.CallExpr,
	pkgPath, wrapperName string,
	acceptProducer func(function *types.Func, signature *types.Signature, call *ast.CallExpr) bool,
) (typedPackageWrapperProducerChain, bool) {
	if root == nil {
		return typedPackageWrapperProducerChain{}, false
	}
	wrapper, wrapperSignature, ok := typedCallee(pass, root.Fun)
	if !ok || wrapperSignature.Recv() != nil || wrapper.Pkg() == nil || wrapper.Pkg().Path() != pkgPath ||
		wrapper.Name() != wrapperName || len(root.Args) != 1 || root.Ellipsis.IsValid() {
		return typedPackageWrapperProducerChain{}, false
	}
	binding, ok := typedPackageBinding(pass, root.Fun)
	if !ok {
		return typedPackageWrapperProducerChain{}, false
	}
	resultType := pass.TypesInfo.TypeOf(root)
	if resultType == nil {
		return typedPackageWrapperProducerChain{}, false
	}

	current := root
	var wrappers []*ast.CallExpr
	for {
		currentFunction, currentSignature, ok := typedCallee(pass, current.Fun)
		if !ok || currentSignature.Recv() != nil || currentFunction != wrapper || len(current.Args) != 1 ||
			current.Ellipsis.IsValid() || !typedPackageCallWithBinding(pass, current, pkgPath, wrapperName, binding) ||
			!types.Identical(pass.TypesInfo.TypeOf(current), resultType) {
			return typedPackageWrapperProducerChain{}, false
		}
		wrappers = append(wrappers, current)
		expression := current.Args[0]
		next, ok := ps2110Unparen(expression).(*ast.CallExpr)
		if !ok {
			return typedPackageWrapperProducerChain{}, false
		}
		if typedPackageCallWithBinding(pass, next, pkgPath, wrapperName, binding) {
			current = next
			continue
		}
		producer, producerSignature, ok := typedCallee(pass, next.Fun)
		producerBinding, bindingOK := typedPackageBinding(pass, next.Fun)
		if !ok || producerSignature.Recv() != nil || producer.Pkg() == nil || producer.Pkg().Path() != pkgPath ||
			!bindingOK || producerBinding != binding || !types.Identical(pass.TypesInfo.TypeOf(next), resultType) ||
			acceptProducer == nil || !acceptProducer(producer, producerSignature, next) {
			return typedPackageWrapperProducerChain{}, false
		}
		return typedPackageWrapperProducerChain{
			outer: root, producer: next, producerExpression: expression,
			wrappers: wrappers, wrapper: wrapper, producerFunction: producer, binding: binding,
		}, true
	}
}

// matchTypedPackageConsumerProducerComposition matches a direct nested
// producer in consumerArgument of pkgPath.consumerName. Both calls must be
// ordinary package selectors through the exact same import object; dot
// imports, methods, function values, ellipsis calls, and cross-binding
// lookalikes are rejected centrally. acceptProducer owns operation-specific
// arity and semantic policy.
func matchTypedPackageConsumerProducerComposition(
	pass *analysis.Pass,
	root *ast.CallExpr,
	pkgPath, consumerName string,
	consumerArity, consumerArgument int,
	acceptProducer func(function *types.Func, signature *types.Signature, call *ast.CallExpr) bool,
) (typedPackageConsumerProducerComposition, bool) {
	composition, ok := matchTypedCrossPackageConsumerProducerComposition(
		pass, root, pkgPath, consumerName, consumerArity, consumerArgument, acceptProducer,
	)
	if !ok || composition.producerFunction.Pkg() == nil || composition.producerFunction.Pkg().Path() != pkgPath ||
		composition.producerBinding != composition.consumerBinding {
		return typedPackageConsumerProducerComposition{}, false
	}
	return composition, true
}

// matchTypedCrossPackageConsumerProducerComposition is the cross-package form
// of matchTypedPackageConsumerProducerComposition. It resolves an ordinary
// package consumer and a direct ordinary package producer independently, then
// delegates the producer package/name/arity policy to acceptProducer. This is
// the shared shape for contracts such as a unicode/utf8 observer consuming a
// strings or bytes sanitizer result.
func matchTypedCrossPackageConsumerProducerComposition(
	pass *analysis.Pass,
	root *ast.CallExpr,
	consumerPkgPath, consumerName string,
	consumerArity, consumerArgument int,
	acceptProducer func(function *types.Func, signature *types.Signature, call *ast.CallExpr) bool,
) (typedPackageConsumerProducerComposition, bool) {
	if root == nil || consumerArity < 1 || consumerArgument < 0 || consumerArgument >= consumerArity ||
		len(root.Args) != consumerArity || root.Ellipsis.IsValid() {
		return typedPackageConsumerProducerComposition{}, false
	}
	consumer, consumerSignature, ok := typedCallee(pass, root.Fun)
	if !ok || consumerSignature.Recv() != nil || consumer.Pkg() == nil ||
		consumer.Pkg().Path() != consumerPkgPath || consumer.Name() != consumerName {
		return typedPackageConsumerProducerComposition{}, false
	}
	consumerBinding, ok := typedPackageBinding(pass, root.Fun)
	if !ok {
		return typedPackageConsumerProducerComposition{}, false
	}
	expression := root.Args[consumerArgument]
	producerCall, ok := ps2110Unparen(expression).(*ast.CallExpr)
	if !ok || producerCall.Ellipsis.IsValid() {
		return typedPackageConsumerProducerComposition{}, false
	}
	producer, producerSignature, ok := typedCallee(pass, producerCall.Fun)
	producerBinding, bindingOK := typedPackageBinding(pass, producerCall.Fun)
	if !ok || producerSignature.Recv() != nil || producer.Pkg() == nil || !bindingOK ||
		acceptProducer == nil || !acceptProducer(producer, producerSignature, producerCall) {
		return typedPackageConsumerProducerComposition{}, false
	}
	return typedPackageConsumerProducerComposition{
		consumer: root, producer: producerCall, producerExpression: expression,
		consumerFunction: consumer, producerFunction: producer,
		consumerBinding: consumerBinding, producerBinding: producerBinding,
	}, true
}

// collectTypedNestedPackageCallSpine walks one argument position through
// nested calls to pkgPath.name. Ordinary non-empty calls are always syntactic
// splice candidates. An empty or ellipsis call is accepted only when it is the
// parent's sole argument: deleting its scaffolding then leaves a valid empty or
// spread call rather than an empty slot or a non-final ellipsis.
//
// Unlike collectTypedNestedPackageCallTree it deliberately ignores sibling
// arguments. That distinction supports operations which are associative only
// along one side, such as path.Join: a cleaned prefix may be expanded back
// into the next Join, while a Join nested in a later element can change rooted
// and leading-dotdot behavior if flattened.
func collectTypedNestedPackageCallSpine(
	pass *analysis.Pass,
	root *ast.CallExpr,
	nextArg int,
	pkgPath, name string,
	binding *types.PkgName,
) []typedNestedPackageCall {
	if root == nil || nextArg < 0 {
		return nil
	}
	current := root
	var nested []typedNestedPackageCall
	for nextArg < len(current.Args) {
		expression := current.Args[nextArg]
		call, ok := ps2110Unparen(expression).(*ast.CallExpr)
		if !ok || (len(call.Args) == 0 || call.Ellipsis.IsValid()) && len(current.Args) != 1 ||
			!typedPackageCallWithBinding(pass, call, pkgPath, name, binding) {
			break
		}
		nested = append(nested, typedNestedPackageCall{expression: expression, call: call})
		current = call
	}
	return nested
}

func typedPackageCallWithBinding(pass *analysis.Pass, call *ast.CallExpr, pkgPath, name string, binding *types.PkgName) bool {
	fn, signature, ok := typedCallee(pass, call.Fun)
	if !ok || signature.Recv() != nil || fn.Pkg() == nil || fn.Pkg().Path() != pkgPath || fn.Name() != name {
		return false
	}
	callBinding, ok := typedPackageBinding(pass, call.Fun)
	return ok && callBinding == binding
}

func flattenNestedPackageCallEdits(nested []typedNestedPackageCall) []analysis.TextEdit {
	edits := make([]analysis.TextEdit, 0, len(nested)*2)
	for _, call := range nested {
		edits = append(edits,
			analysis.TextEdit{Pos: call.expression.Pos(), End: call.call.Lparen + 1},
			analysis.TextEdit{Pos: call.call.Rparen, End: call.expression.End()},
		)
	}
	return edits
}

// matchTypedCallChain matches root against an outer-to-inner sequence of
// typed calls. It returns the calls in the same order as steps. Parentheses and
// explicit generic instantiation around a callee are ignored, while arguments
// remain untouched for a caller to splice byte-verbatim into a fix.
func matchTypedCallChain(pass *analysis.Pass, root ast.Expr, steps ...typedCallStep) ([]*ast.CallExpr, bool) {
	if len(steps) == 0 {
		return nil, false
	}

	expr := root
	calls := make([]*ast.CallExpr, 0, len(steps))
	for i, step := range steps {
		call, ok := ps2110Unparen(expr).(*ast.CallExpr)
		if !ok || call.Ellipsis.IsValid() || (step.Arity >= 0 && len(call.Args) != step.Arity) {
			return nil, false
		}
		fn, sig, ok := typedCallee(pass, call.Fun)
		if !ok || fn.Pkg() == nil || fn.Pkg().Path() != step.PkgPath || fn.Name() != step.Name {
			return nil, false
		}
		if (step.Kind == typedPackageFunc) != (sig.Recv() == nil) {
			return nil, false
		}
		calls = append(calls, call)

		last := i == len(steps)-1
		if last {
			if step.NextArg != -1 {
				return nil, false
			}
			continue
		}
		if step.NextArg < 0 || step.NextArg >= len(call.Args) {
			return nil, false
		}
		expr = call.Args[step.NextArg]
	}
	return calls, true
}

// typedCallee resolves fun to its *types.Func and signature. Parentheses and
// explicit generic instantiation are unwrapped before resolving the object.
func typedCallee(pass *analysis.Pass, fun ast.Expr) (*types.Func, *types.Signature, bool) {
	e := ps2110Unparen(fun)
	switch x := e.(type) {
	case *ast.IndexExpr:
		e = ps2110Unparen(x.X)
	case *ast.IndexListExpr:
		e = ps2110Unparen(x.X)
	}

	var obj types.Object
	switch x := e.(type) {
	case *ast.SelectorExpr:
		obj = pass.TypesInfo.Uses[x.Sel]
	case *ast.Ident:
		obj = pass.TypesInfo.Uses[x]
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return nil, nil, false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return nil, nil, false
	}
	return fn, sig, true
}

// typedReceiverNamed reports whether sig is a method declared on the exact
// named type pkgPath.typeName (or its pointer). Package path plus method name
// alone is insufficient for typed rewrite allowlists: net/http.Header.Get and
// net/http.Client.Get, for example, have radically different contracts.
func typedReceiverNamed(sig *types.Signature, pkgPath, typeName string) bool {
	if sig == nil || sig.Recv() == nil {
		return false
	}
	receiver := types.Unalias(sig.Recv().Type())
	if pointer, ok := receiver.(*types.Pointer); ok {
		receiver = types.Unalias(pointer.Elem())
	}
	named, ok := receiver.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return false
	}
	return named.Obj().Pkg().Path() == pkgPath && named.Obj().Name() == typeName
}

// typedBuiltinName reports whether expr resolves to the named predeclared
// builtin. A same-named local function is deliberately rejected.
func typedBuiltinName(pass *analysis.Pass, expr ast.Expr, name string) bool {
	id, ok := ps2110Unparen(expr).(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && builtin.Name() == name
}

// matchTypedUnaryPackageCallChain matches the maximal outer prefix of unary
// package calls accepted by allowed. Every layer must use an ordinary package
// selector (dot imports are rejected), have no ellipsis, and preserve the
// concrete type exactly. These constraints make removing the call scaffolding
// safe for callers that have independently established semantic identity.
func matchTypedUnaryPackageCallChain(
	pass *analysis.Pass,
	expr ast.Expr,
	allowed func(pkgPath, name string) bool,
) (typedUnaryCallChain, bool) {
	outer, ok := ps2110Unparen(expr).(*ast.CallExpr)
	if !ok {
		return typedUnaryCallChain{}, false
	}
	current := outer
	var calls []*ast.CallExpr
	var paths []string
	for {
		fn, sig, ok := typedCallee(pass, current.Fun)
		if !ok || sig.Recv() != nil || fn.Pkg() == nil ||
			!allowed(fn.Pkg().Path(), fn.Name()) || len(current.Args) != 1 || current.Ellipsis.IsValid() {
			break
		}
		callType := pass.TypesInfo.TypeOf(current)
		argType := pass.TypesInfo.TypeOf(current.Args[0])
		if _, ok := typedPackageBinding(pass, current.Fun); !ok || callType == nil || argType == nil ||
			!types.Identical(callType, argType) {
			break
		}
		calls = append(calls, current)
		paths = append(paths, fn.Pkg().Path())
		next, nested := ps2110Unparen(current.Args[0]).(*ast.CallExpr)
		if !nested {
			break
		}
		current = next
	}
	if len(calls) == 0 {
		return typedUnaryCallChain{}, false
	}
	base := calls[len(calls)-1].Args[0]
	return typedUnaryCallChain{
		base:  base,
		calls: calls,
		paths: paths,
		spans: []tokenSpan{
			{start: outer.Pos(), end: base.Pos()},
			{start: base.End(), end: outer.End()},
		},
	}, true
}

// matchRepeatedTypedCompoundNormalizerPipeline matches a repeated
// normalization pipeline even when one logical stage is composed from several
// functions rather than one recursively nested call. matchStage owns the
// typed shape and semantic metadata for a single stage; compatible proves that
// applying an outer stage to the retained terminal stage's result is an
// identity.
//
// The shared matcher enforces the invariants every such rewrite needs:
//
//   - every retained stage result has exactly the outer result's concrete type;
//   - every semantic package role uses the same ordinary import binding at
//     every layer; and
//   - the deepest terminal stage that makes every outer layer an identity is
//     retained, so contextual fixed points collapse in one pass even when an
//     intermediate stage would not be idempotent for arbitrary input.
func matchRepeatedTypedCompoundNormalizerPipeline(
	pass *analysis.Pass,
	expression ast.Expr,
	matchStage func(ast.Expr) (typedCompoundNormalizerStage, bool),
	compatible func(outer, inner typedCompoundNormalizerStage) bool,
) (repeatedTypedCompoundNormalizerPipeline, bool) {
	if pass == nil || matchStage == nil || compatible == nil {
		return repeatedTypedCompoundNormalizerPipeline{}, false
	}
	first, ok := matchStage(expression)
	if !ok || !validTypedCompoundNormalizerStage(pass, first) {
		return repeatedTypedCompoundNormalizerPipeline{}, false
	}
	stages := []typedCompoundNormalizerStage{first}
	current := first
	for {
		next, ok := matchStage(current.input)
		if !ok || !validTypedCompoundNormalizerStage(pass, next) ||
			!types.Identical(pass.TypesInfo.TypeOf(first.root), pass.TypesInfo.TypeOf(next.root)) ||
			!sameTypedPackageBindings(first.bindings, next.bindings) {
			break
		}
		stages = append(stages, next)
		current = next
	}
	if len(stages) < 2 {
		return repeatedTypedCompoundNormalizerPipeline{}, false
	}
	for keepIndex := len(stages) - 1; keepIndex >= 1; keepIndex-- {
		terminal := stages[keepIndex]
		accepted := true
		for outerIndex := 0; outerIndex < keepIndex; outerIndex++ {
			if !compatible(stages[outerIndex], terminal) {
				accepted = false
				break
			}
		}
		if accepted {
			return repeatedTypedCompoundNormalizerPipeline{
				outer:  first.root,
				keep:   terminal.root,
				stages: stages[:keepIndex+1],
			}, true
		}
	}
	return repeatedTypedCompoundNormalizerPipeline{}, false
}

func validTypedCompoundNormalizerStage(pass *analysis.Pass, stage typedCompoundNormalizerStage) bool {
	if stage.root == nil || stage.input == nil || len(stage.bindings) == 0 ||
		stage.input.Pos() < stage.root.Pos() || stage.input.End() > stage.root.End() {
		return false
	}
	return pass.TypesInfo.TypeOf(stage.root) != nil
}

func sameTypedPackageBindings(left, right []*types.PkgName) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] == nil || left[index] != right[index] {
			return false
		}
	}
	return true
}

func markRepeatedTypedCompoundNormalizerPipeline(covered map[*ast.CallExpr]bool, matched repeatedTypedCompoundNormalizerPipeline) {
	for _, stage := range matched.stages {
		covered[stage.root] = true
	}
}

func isTypedStdlibClone(pkgPath, name string) bool {
	return name == "Clone" && (pkgPath == "bytes" || pkgPath == "slices" || pkgPath == "maps" || pkgPath == "strings")
}

func isTypedContainerStdlibClone(pkgPath, name string) bool {
	return isTypedStdlibClone(pkgPath, name) && pkgPath != "strings"
}

func isTypedSliceStdlibClone(pkgPath, name string) bool {
	return name == "Clone" && (pkgPath == "bytes" || pkgPath == "slices")
}

func isTypedStringStdlibClone(pkgPath, name string) bool {
	return pkgPath == "strings" && name == "Clone"
}

// cloneRemovalLaterArgumentsStable reports whether evaluating the arguments
// after index cannot mutate a container whose snapshot is being removed. Go
// evaluates call arguments left-to-right; an arbitrary later call or channel
// receive can therefore make Clone semantically observable before the outer
// consumer starts. Conversions, len/cap, and recursively inspected stdlib
// Clone calls are the only calls known to be non-mutating here.
func cloneRemovalLaterArgumentsStable(pass *analysis.Pass, call *ast.CallExpr, index int) bool {
	for later := index + 1; later < len(call.Args); later++ {
		stable := true
		ast.Inspect(call.Args[later], func(node ast.Node) bool {
			if !stable {
				return false
			}
			switch value := node.(type) {
			case *ast.UnaryExpr:
				if value.Op == token.ARROW {
					stable = false
					return false
				}
			case *ast.CallExpr:
				// These calls only evaluate/read their operands. Keep
				// descending so a nested arbitrary call is still rejected.
				if typeValue, ok := pass.TypesInfo.Types[ps2110Unparen(value.Fun)]; ok && typeValue.IsType() {
					return true
				}
				if function, signature, ok := typedCallee(pass, value.Fun); ok && signature.Recv() == nil && function.Pkg() != nil && isTypedStdlibClone(function.Pkg().Path(), function.Name()) {
					return true
				}
				if typedBuiltinName(pass, value.Fun, "len") || typedBuiltinName(pass, value.Fun, "cap") {
					return true
				}
				stable = false
				return false
			}
			return true
		})
		if !stable {
			return false
		}
	}
	return true
}

// cloneCallsOwnedByTerminalConsumer marks Clone layers that a more specific
// terminal rewrite removes completely. Nested-clone PS5074 yields ownership to
// these rules so a single -fix pass lands on the allocation-free fixed point
// instead of emitting overlapping edits.
func cloneCallsOwnedByTerminalConsumer(pass *analysis.Pass, file *ast.File) map[*ast.CallExpr]bool {
	owned := make(map[*ast.CallExpr]bool)
	mark := func(chain typedUnaryCallChain, ok bool) {
		if !ok {
			return
		}
		for _, call := range chain.calls {
			owned[call] = true
		}
	}
	astutil.WithStack(file, func(node ast.Node, stack []ast.Node) bool {
		if index, ok := node.(*ast.IndexExpr); ok {
			chain, _, matched := ps5091IndexMatch(pass, index, stack)
			mark(chain, matched)
		}
		if comparison, ok := node.(*ast.BinaryExpr); ok {
			for _, chain := range ps5092ComparisonMatches(pass, comparison) {
				mark(chain, true)
			}
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if typedBuiltinName(pass, call.Fun, "len") && len(call.Args) == 1 && !call.Ellipsis.IsValid() {
			chain, matched := matchTypedUnaryPackageCallChain(pass, call.Args[0], isTypedStdlibClone)
			mark(chain, matched)
			if matched {
				if fixedPoint, ok := ps5083FixedPointMatch(pass, call, chain); ok && fixedPoint.nestedOK {
					mark(fixedPoint.nested, true)
				}
			}
		}
		if chain, _, matched := ps5084MatchCloneConsumer(pass, call); matched {
			mark(chain, true)
			if fixedPoint, ok := ps5084FixedPointMatch(pass, call, chain); ok && fixedPoint.nestedOK {
				mark(fixedPoint.nested, true)
			}
		}
		if chain, _, _, matched := ps5085Match(pass, call); matched {
			mark(chain, true)
		}
		if _, chains, matched := ps5086CloneChains(pass, call); matched {
			for _, chain := range chains {
				mark(chain, true)
			}
		}
		if chain, _, _, matched := ps5087Match(pass, call); matched {
			mark(chain, true)
		}
		if chain, _, matched := ps5088Match(pass, call); matched {
			mark(chain, true)
		}
		if chain, _, matched := ps5089Match(pass, call); matched {
			mark(chain, true)
		}
		if chain, _, matched := ps5090Match(pass, call); matched {
			mark(chain, true)
		}
		if chain, _, matched := ps5091DeleteMatch(pass, call); matched {
			mark(chain, true)
		}
		if match, matched := ps5093SizeMatch(pass, call); matched {
			mark(match.clone, match.cloneOK)
			mark(match.nested, match.nestedOK)
		}
		if match, matched := ps5094ExtractionMatch(pass, call, stack); matched {
			mark(match.clone, match.cloneOK)
		}

		fn, sig, typed := typedCallee(pass, call.Fun)
		if !typed || fn.Pkg() == nil || call.Ellipsis.IsValid() {
			return true
		}
		if observer, matched := ps5081Observers[fn.Pkg().Path()][fn.Name()]; matched &&
			(observer.kind == typedPackageFunc) == (sig.Recv() == nil) {
			for _, index := range observer.indices {
				if index >= 0 && index < len(call.Args) && cloneRemovalLaterArgumentsStable(pass, call, index) {
					chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[index], isTypedContainerStdlibClone)
					mark(chain, ok)
				}
			}
		}
		if observer, matched := ps5082Observers[fn.Pkg().Path()][fn.Name()]; matched &&
			(observer.kind == typedPackageFunc) == (sig.Recv() == nil) &&
			(observer.receiver == "" || typedReceiverNamed(sig, fn.Pkg().Path(), observer.receiver)) {
			for _, index := range observer.indices {
				if index >= 0 && index < len(call.Args) {
					chain, ok := matchTypedUnaryPackageCallChain(pass, call.Args[index], isTypedStringStdlibClone)
					mark(chain, ok)
				}
			}
		}
		return true
	})
	return owned
}

// matchRepeatedTypedUnaryPackageCall matches a maximal nested run of the same
// unary package function when allowed accepts its resolved package path and
// function name. It rejects dot imports, methods, ellipsis calls, and a change
// of import binding between layers.
func matchRepeatedTypedUnaryPackageCall(pass *analysis.Pass, outer *ast.CallExpr, allowed func(pkgPath, name string) bool) (repeatedTypedCall, bool) {
	return matchRepeatedTypedPackageCall(pass, outer, 1, 0, allowed, nil)
}

// matchRepeatedTypedPackageCall matches a maximal nested run of the same
// package function. Every call must have arity arguments and the next layer
// must occupy nextArg. compatible, when non-nil, validates the non-recursive
// arguments of each adjacent outer/inner pair (for example, equal constant
// cutsets). Callees, concrete import bindings, and result types must agree at
// every layer. The result-type condition is load-bearing for generic calls:
// slices.Clone[[]T](slices.Clone[NamedSlice](x)) compiles, but deleting the
// outer call would change the expression's static (and interface-dynamic)
// type from []T to NamedSlice.
func matchRepeatedTypedPackageCall(
	pass *analysis.Pass,
	outer *ast.CallExpr,
	arity, nextArg int,
	allowed func(pkgPath, name string) bool,
	compatible func(outer, inner *ast.CallExpr) bool,
) (repeatedTypedCall, bool) {
	if arity < 1 || nextArg < 0 || nextArg >= arity {
		return repeatedTypedCall{}, false
	}
	fn, sig, ok := typedCallee(pass, outer.Fun)
	if !ok || sig.Recv() != nil || fn.Pkg() == nil || !allowed(fn.Pkg().Path(), fn.Name()) ||
		len(outer.Args) != arity || outer.Ellipsis.IsValid() {
		return repeatedTypedCall{}, false
	}
	binding, ok := typedPackageBinding(pass, outer.Fun)
	if !ok {
		return repeatedTypedCall{}, false
	}
	resultType := pass.TypesInfo.TypeOf(outer)
	if resultType == nil {
		return repeatedTypedCall{}, false
	}

	keep := outer
	calls := []*ast.CallExpr{outer}
	layers := 1
	for {
		inner, ok := ps2110Unparen(keep.Args[nextArg]).(*ast.CallExpr)
		if !ok || len(inner.Args) != arity || inner.Ellipsis.IsValid() {
			break
		}
		innerFn, innerSig, ok := typedCallee(pass, inner.Fun)
		if !ok || innerSig.Recv() != nil || innerFn.Pkg() == nil ||
			innerFn.Pkg().Path() != fn.Pkg().Path() || innerFn.Name() != fn.Name() {
			break
		}
		innerBinding, ok := typedPackageBinding(pass, inner.Fun)
		innerResultType := pass.TypesInfo.TypeOf(inner)
		if !ok || innerBinding != binding || innerResultType == nil || !types.Identical(innerResultType, resultType) ||
			(compatible != nil && !compatible(keep, inner)) {
			break
		}
		keep = inner
		calls = append(calls, inner)
		layers++
	}
	if layers < 2 {
		return repeatedTypedCall{}, false
	}
	return repeatedTypedCall{outer: outer, keep: keep, fn: fn, calls: calls, layers: layers}, true
}

// matchRepeatedTypedPackageCallEndingIn is the terminal-postcondition form of
// matchRepeatedTypedPackageCall. It collects a typed repeated call run through
// value argument nextArg, then retains the deepest call for which terminal
// proves that every collected outer call is an identity. compatible owns the
// separate proof that deleting each outer call also deletes only safe companion
// argument evaluations.
//
// Searching from the deepest candidate outward matters for sanitizer chains:
// a still-deeper call may not establish the postcondition, while an enclosing
// call does. The returned prefix is therefore the maximal one-shot rewrite,
// rather than rejecting the whole site or requiring a second fix pass.
func matchRepeatedTypedPackageCallEndingIn(
	pass *analysis.Pass,
	outer *ast.CallExpr,
	arity, nextArg int,
	allowed func(pkgPath, name string) bool,
	compatible func(outer, inner *ast.CallExpr) bool,
	terminal func(call *ast.CallExpr) bool,
) (repeatedTypedCall, bool) {
	matched, ok := matchRepeatedTypedPackageCall(pass, outer, arity, nextArg, allowed, compatible)
	if !ok || terminal == nil {
		return repeatedTypedCall{}, false
	}
	for index := len(matched.calls) - 1; index >= 1; index-- {
		if !terminal(matched.calls[index]) {
			continue
		}
		matched.keep = matched.calls[index]
		matched.calls = matched.calls[:index+1]
		matched.layers = index + 1
		return matched, true
	}
	return repeatedTypedCall{}, false
}

// markRepeatedTypedCall records every call belonging to matched. A caller can
// keep walking the retained call's argument to find independent nested runs
// without emitting overlapping diagnostics for the same run.
func markRepeatedTypedCall(covered map[*ast.CallExpr]bool, matched repeatedTypedCall) {
	for _, call := range matched.calls {
		covered[call] = true
	}
}

// typedPackageBinding resolves the concrete import object used by a selector,
// unwrapping parentheses and explicit generic instantiation around the callee.
func typedPackageBinding(pass *analysis.Pass, fun ast.Expr) (*types.PkgName, bool) {
	e := ps2110Unparen(fun)
	switch x := e.(type) {
	case *ast.IndexExpr:
		e = ps2110Unparen(x.X)
	case *ast.IndexListExpr:
		e = ps2110Unparen(x.X)
	}
	selector, ok := e.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	id, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return nil, false
	}
	pkg, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
	return pkg, ok
}

// replacementPreservesStaticType reports whether retaining replacement in
// place of original preserves the expression's concrete type. Untyped
// constants are compared after defaulting because their interface dynamic type
// and ordinary inference default match the typed standard-library result.
func replacementPreservesStaticType(pass *analysis.Pass, original, replacement ast.Expr) bool {
	originalType := pass.TypesInfo.TypeOf(ps2110Unparen(original))
	replacementType := pass.TypesInfo.TypeOf(ps2110Unparen(replacement))
	return originalType != nil && replacementType != nil &&
		types.Identical(originalType, types.Default(replacementType))
}

// replacementIntroducesConstantInUniqueContext reports whether replacing node
// with a constant would make an enclosing switch case or composite-literal key
// a constant expression. Go permits duplicate runtime case/key expressions but
// rejects duplicate constants, so an otherwise value-identical rewrite can
// make valid source stop compiling in these contexts.
func replacementIntroducesConstantInUniqueContext(pass *analysis.Pass, node ast.Expr, parents map[ast.Node]ast.Node) bool {
	var current ast.Node = node
	for parent := parents[current]; parent != nil; parent = parents[current] {
		switch expression := parent.(type) {
		case *ast.ParenExpr, *ast.UnaryExpr:
			current = parent
		case *ast.BinaryExpr:
			other := expression.X
			if current == expression.X {
				other = expression.Y
			}
			if typed, ok := pass.TypesInfo.Types[other]; !ok || typed.Value == nil {
				return false
			}
			current = parent
		case *ast.CallExpr:
			if len(expression.Args) != 1 || current != expression.Args[0] {
				return false
			}
			typed, ok := pass.TypesInfo.Types[ps2110Unparen(expression.Fun)]
			if !ok || !typed.IsType() {
				return false
			}
			current = parent
		case *ast.CaseClause:
			for _, candidate := range expression.List {
				if candidate == current {
					return true
				}
			}
			return false
		case *ast.KeyValueExpr:
			return expression.Key == current
		default:
			return false
		}
	}
	return false
}
