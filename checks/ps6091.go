package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
)

// PS6091 implements owner issue #885. It identifies generic Top-K calls with
// k=1 whose ranked-index slice is observed only at rank zero.
var PS6091 = register(&lint.Check{
	ID:          "PS6091",
	Category:    "alloc",
	Slug:        "topk-one-result-needs-scalar-argmax",
	Level:       lint.LevelStructured,
	AutoFix:     false,
	NeedsConfig: true,
	Vocab:       []string{"topKOneContracts"},
	Doc: lint.Documentation{
		Title: "a generic Top-K(k=1) result is allocated only to read its first index",
		Text: `A generic Top-K implementation often allocates result slices and
retains heap-selection machinery even when greedy decoding asks for k=1 and
observes only the first ranked index. A scalar, allocation-free ArgmaxN-style
capability can remove that overhead at a per-token hot boundary.

This check implements owner issue #885. It requires an exact configured
topKOneContracts entry, a typed call whose configured k argument is
the compile-time integer one, and a fresh ranked-index result introduced by a
local short or var declaration and used exclusively through reads of index
zero. Every other non-error result must be discarded. It stays silent when k
is dynamic, values are retained, another rank is read, the slice is returned,
stored, sliced, passed to another call, captured by a closure, or otherwise
escapes. Writes through index zero are also excluded, including assignment,
compound assignment, increment/decrement, range or receive targets, implicit
method addressing, and explicit address-taking.

Contracts distinguish package functions from methods. The kind may be omitted
only when the documented ID has exactly one valid parse; dotted IDs that can be
both an import-path function and a receiver method require kind: function or
kind: method. The ranked-index element may be a concrete named or alias integer
type, or a type parameter only when its complete, nonempty constraint type set
contains exclusively integer-underlying terms.

There is NO automatic fix. Go types and local use-def facts cannot prove that a
different API preserves the Top-K implementation's first-n prefix boundary,
first-index tie behavior, finite-logit assumptions or NaN semantics, and
backend error handling. Introduce or use a scalar ArgmaxN-style capability only
after those contracts are explicit, retain the established fallback where they
are not, and gate the replacement with equivalence and end-to-end benchmarks.`,
		Before: `indices, _, err := logits.TopKN(vocab, 1)
if err != nil {
	return 0, err
}
return indices[0], nil`,
		After: `index, err := logits.ArgmaxN(vocab)
if err != nil {
	return 0, err
}
return index, nil // after tie/NaN/prefix equivalence is proven`,
		MeasuredWin: `In the Apple-M2 GPT-2-small reproduction behind issue
#885, resident TopKN(vocab, 1) reached 67.80 tok/s versus 68.65 tok/s for the
established host fallback and added 62 allocations per generation. The generic
path avoided about 13.1 MB of full-vocabulary materialization, so a scalar
resident capability is the candidate that retains the byte saving without the
O(k) result containers and generic selector overhead.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6091",
		Doc:  "Top-K called with k=1 allocates ranked results whose only observation is index zero",
		Run:  runPS6091,
	},
})

func runPS6091(pass *analysis.Pass) (any, error) {
	return runPS6091WithContracts(pass, config.Current().TopKOneContracts)
}

func runPS6091WithContracts(pass *analysis.Pass, contracts []config.TopKOneContract) (any, error) {
	configured := make(map[string][]ps6091ConfiguredContract)
	for _, contract := range contracts {
		kind, ok := contract.ResolvedKind()
		if ok {
			configured[contract.Function] = append(configured[contract.Function], ps6091ConfiguredContract{
				contract: contract,
				kind:     kind,
			})
		}
	}
	if len(configured) == 0 {
		return nil, nil
	}

	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			switch function := node.(type) {
			case *ast.FuncDecl:
				ps6091FunctionBody(pass, function.Body, configured)
			case *ast.FuncLit:
				ps6091FunctionBody(pass, function.Body, configured)
			}
			return true
		})
	}
	return nil, nil
}

type ps6091ConfiguredContract struct {
	contract config.TopKOneContract
	kind     config.TopKOneContractKind
}

func ps6091FunctionBody(pass *analysis.Pass, body *ast.BlockStmt, configured map[string][]ps6091ConfiguredContract) {
	if body == nil {
		return
	}
	// Unlike candidate discovery below, the parent map intentionally includes
	// nested function literals so uses captured by a closure are classified as
	// escapes rather than as ordinary scalar reads.
	parents := ps6087Parents(body)
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		var bindings ps6091Bindings
		var rhs ast.Expr
		switch value := node.(type) {
		case *ast.AssignStmt:
			if value.Tok != token.DEFINE || len(value.Rhs) != 1 {
				return true
			}
			bindings.assignment = value
			rhs = value.Rhs[0]
		case *ast.ValueSpec:
			if len(value.Values) != 1 {
				return true
			}
			bindings.declaration = value
			rhs = value.Values[0]
		default:
			return true
		}
		call, ok := ps2110Unparen(rhs).(*ast.CallExpr)
		if !ok {
			return true
		}
		kind, ok := ps6091CalleeKind(pass, call)
		if !ok {
			return true
		}
		for _, configuredContract := range configured[ps6087FunctionID(pass, call)] {
			if configuredContract.kind != kind {
				continue
			}
			contract := configuredContract.contract
			indices, ok := ps6091Candidate(pass, bindings, call, contract)
			if !ok || !ps6091OnlyFirstIndexReads(pass, body, parents, indices) {
				continue
			}
			label := contract.Name
			if label == "" {
				label = contract.Function
			}
			pass.Reportf(call.Pos(), "%s: configured Top-K call uses compile-time k=1, discards every non-error result except ranked indices, and reads that fresh slice only at index zero; benchmark an allocation-free scalar ArgmaxN-style capability while preserving the first-n prefix, first-index tie, NaN/finite-logit, and backend-error contracts (advisory, no automatic fix)", label)
			break
		}
		return true
	})
}

func ps6091CalleeKind(pass *analysis.Pass, call *ast.CallExpr) (config.TopKOneContractKind, bool) {
	_, signature, ok := typedCallee(pass, call.Fun)
	if !ok {
		return "", false
	}
	if signature.Recv() != nil {
		return config.TopKOneContractMethod, true
	}
	return config.TopKOneContractFunction, true
}

type ps6091Bindings struct {
	assignment  *ast.AssignStmt
	declaration *ast.ValueSpec
}

func (b ps6091Bindings) Len() int {
	if b.assignment != nil {
		return len(b.assignment.Lhs)
	}
	if b.declaration != nil {
		return len(b.declaration.Names)
	}
	return 0
}

func (b ps6091Bindings) Ident(index int) (*ast.Ident, bool) {
	if b.assignment != nil {
		identifier, ok := ps2110Unparen(b.assignment.Lhs[index]).(*ast.Ident)
		return identifier, ok
	}
	if b.declaration != nil {
		return b.declaration.Names[index], true
	}
	return nil, false
}

func ps6091Candidate(pass *analysis.Pass, bindings ps6091Bindings, call *ast.CallExpr, contract config.TopKOneContract) (types.Object, bool) {
	kArg := contract.KArgPosition - 1
	if ps6091MethodExpression(pass, call.Fun) {
		kArg++
	}
	indicesResult := contract.IndicesResultPosition - 1
	if kArg >= len(call.Args) || !ps6091ConstantInteger(pass, call.Args[kArg], 1) {
		return nil, false
	}
	signature, ok := pass.TypesInfo.TypeOf(call.Fun).(*types.Signature)
	if !ok || signature.Results() == nil || signature.Results().Len() != bindings.Len() ||
		indicesResult >= signature.Results().Len() {
		return nil, false
	}
	if !ps6091IntegerSequence(signature.Results().At(indicesResult).Type()) {
		return nil, false
	}

	var indices types.Object
	for index := range bindings.Len() {
		identifier, direct := bindings.Ident(index)
		if !direct {
			return nil, false
		}
		if index == indicesResult {
			if identifier.Name == "_" {
				return nil, false
			}
			indices = pass.TypesInfo.Defs[identifier]
			continue
		}
		if identifier.Name != "_" && !ps6091ErrorType(signature.Results().At(index).Type()) {
			return nil, false
		}
	}
	return indices, indices != nil
}

func ps6091MethodExpression(pass *analysis.Pass, expression ast.Expr) bool {
	expression = ps2110Unparen(expression)
	switch value := expression.(type) {
	case *ast.IndexExpr:
		expression = ps2110Unparen(value.X)
	case *ast.IndexListExpr:
		expression = ps2110Unparen(value.X)
	}
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	selection := pass.TypesInfo.Selections[selector]
	return selection != nil && selection.Kind() == types.MethodExpr
}

func ps6091ConstantInteger(pass *analysis.Pass, expression ast.Expr, want int64) bool {
	value := pass.TypesInfo.Types[ps2110Unparen(expression)].Value
	if value == nil || value.Kind() != constant.Int {
		return false
	}
	got, exact := constant.Int64Val(value)
	return exact && got == want
}

func ps6091IntegerSequence(value types.Type) bool {
	sequence, ok := types.Unalias(value).Underlying().(*types.Slice)
	return ok && ps6091Integer(sequence.Elem())
}

func ps6091Integer(value types.Type) bool {
	basic, ok := types.Unalias(value).Underlying().(*types.Basic)
	if ok {
		return basic.Info()&types.IsInteger != 0
	}
	typeParameter, ok := types.Unalias(value).(*types.TypeParam)
	if !ok {
		return false
	}
	constraint, ok := types.Unalias(typeParameter.Constraint()).Underlying().(*types.Interface)
	if !ok {
		return false
	}
	set, ok := ps6091ConstraintTypeSet(constraint, make(map[types.Type]bool))
	if !ok || set.unrestricted || len(set.terms) == 0 {
		return false
	}
	feasible := 0
	for _, term := range set.terms {
		termFeasible, known := ps6091TypeTermFeasible(term, constraint, typeParameter)
		if !known {
			return false
		}
		if !termFeasible {
			continue
		}
		feasible++
		basic, ok := types.Unalias(term.value).Underlying().(*types.Basic)
		if !ok || basic.Info()&types.IsInteger == 0 {
			return false
		}
	}
	return feasible > 0
}

func ps6091TypeTermFeasible(term ps6091TypeTerm, constraint *types.Interface, self *types.TypeParam) (bool, bool) {
	if !term.tilde {
		// An exact self-referential term would require a finite type T that is
		// identical to a type expression containing T. Go can express recursive
		// types only through a distinct named type, so such a fixed point cannot
		// inhabit an exact term.
		if ps6091UsesTypeParam(term.value, self) {
			return false, true
		}
		if constraint.IsComparable() && !types.Comparable(term.value) {
			return false, true
		}
		if constraint.NumMethods() == 0 {
			return true, true
		}
		methods, known := ps6091ConstraintMethods(constraint, self, term.value)
		if !known {
			return false, false
		}
		return types.Satisfies(term.value, methods), true
	}
	underlying := types.Unalias(term.value).Underlying()
	if !ps6091TildeUnderlying(underlying) {
		return false, true
	}
	if constraint.IsComparable() && !types.Comparable(underlying) {
		return false, true
	}
	if constraint.NumMethods() == 0 {
		return true, true
	}
	selfUnderlying := ps6091UsesTypeParam(underlying, self)
	if !selfUnderlying {
		methods, known := ps6091ConstraintMethods(constraint, self, underlying)
		if !known {
			// An unknown method shape cannot soundly prove either feasibility or
			// impossibility, so abandon the enclosing all-integer proof.
			return false, false
		}
		// Some otherwise method-hostile underlying types can already have a
		// method set. In particular, *T has the methods declared on T or *T, so
		// an unnamed pointer such as *TokenID can itself witness ~*TokenID.
		if types.Satisfies(underlying, methods) {
			return true, true
		}
	}
	if !ps6091MethodReceiverUnderlying(underlying) {
		return false, true
	}
	if ps6091UnderlyingMethodConflict(underlying, constraint) {
		return false, true
	}

	// A tilde term denotes every defined type with this underlying type. Build
	// one hypothetical defined type with the exact required method set and ask
	// go/types whether it satisfies the whole constraint. Unexported method
	// identity includes its declaring package, so all unexported requirements
	// must originate in one package for any declared type to implement them.
	witnessPackage := ps6091WitnessPackage(constraint)
	if witnessPackage == nil {
		return false, true
	}
	witnessName := types.NewTypeName(token.NoPos, witnessPackage, "ps6091Witness", nil)
	var witness *types.Named
	if selfUnderlying {
		witness = types.NewNamed(witnessName, nil, nil)
		witnessUnderlying, known := ps6091SubstituteSelf(underlying, self, witness)
		if !known {
			return false, false
		}
		witness.SetUnderlying(witnessUnderlying)
		if !ps6091ValidRecursiveWitness(witness, witnessUnderlying) {
			return false, true
		}
	} else {
		witness = types.NewNamed(witnessName, underlying, nil)
	}
	witnessMethods, known := ps6091ConstraintMethods(constraint, self, witness)
	if !known {
		return false, false
	}
	for index := range witnessMethods.NumMethods() {
		method := witnessMethods.Method(index)
		signature, ok := method.Type().(*types.Signature)
		if !ok {
			return false, true
		}
		receiver := types.NewVar(token.NoPos, witnessPackage, "", witness)
		witnessSignature := types.NewSignatureType(receiver, nil, nil, signature.Params(), signature.Results(), signature.Variadic())
		methodPackage := witnessPackage
		if !method.Exported() {
			methodPackage = method.Pkg()
		}
		witness.AddMethod(types.NewFunc(token.NoPos, methodPackage, method.Name(), witnessSignature))
	}
	return types.Satisfies(witness, witnessMethods), true
}

func ps6091ValidRecursiveWitness(witness *types.Named, value types.Type) bool {
	value = types.Unalias(value)
	switch typed := value.(type) {
	case *types.Named:
		if typed == witness {
			return false
		}
		return ps6091ValidRecursiveWitness(witness, typed.Underlying())
	case *types.Array:
		return ps6091ValidRecursiveWitness(witness, typed.Elem())
	case *types.Struct:
		for index := range typed.NumFields() {
			if !ps6091ValidRecursiveWitness(witness, typed.Field(index).Type()) {
				return false
			}
		}
	case *types.Interface:
		for index := range typed.NumEmbeddeds() {
			if !ps6091ValidRecursiveWitness(witness, typed.EmbeddedType(index)) {
				return false
			}
		}
	case *types.Union:
		for index := range typed.Len() {
			if !ps6091ValidRecursiveWitness(witness, typed.Term(index).Type()) {
				return false
			}
		}
	}
	// Pointers, slices, maps, channels, and signatures break the in-memory
	// expansion cycle and therefore make recursive occurrences legal.
	return true
}

func ps6091UsesTypeParam(value types.Type, target *types.TypeParam) bool {
	value = types.Unalias(value)
	switch typed := value.(type) {
	case *types.TypeParam:
		return typed == target
	case *types.Array:
		return ps6091UsesTypeParam(typed.Elem(), target)
	case *types.Slice:
		return ps6091UsesTypeParam(typed.Elem(), target)
	case *types.Pointer:
		return ps6091UsesTypeParam(typed.Elem(), target)
	case *types.Map:
		return ps6091UsesTypeParam(typed.Key(), target) || ps6091UsesTypeParam(typed.Elem(), target)
	case *types.Chan:
		return ps6091UsesTypeParam(typed.Elem(), target)
	case *types.Signature:
		return ps6091TupleUsesTypeParam(typed.Params(), target) || ps6091TupleUsesTypeParam(typed.Results(), target)
	case *types.Struct:
		for index := range typed.NumFields() {
			if ps6091UsesTypeParam(typed.Field(index).Type(), target) {
				return true
			}
		}
	case *types.Interface:
		for index := range typed.NumExplicitMethods() {
			if ps6091UsesTypeParam(typed.ExplicitMethod(index).Type(), target) {
				return true
			}
		}
		for index := range typed.NumEmbeddeds() {
			if ps6091UsesTypeParam(typed.EmbeddedType(index), target) {
				return true
			}
		}
	case *types.Union:
		for index := range typed.Len() {
			if ps6091UsesTypeParam(typed.Term(index).Type(), target) {
				return true
			}
		}
	case *types.Named:
		for index := range typed.TypeArgs().Len() {
			if ps6091UsesTypeParam(typed.TypeArgs().At(index), target) {
				return true
			}
		}
	}
	return false
}

func ps6091TupleUsesTypeParam(tuple *types.Tuple, target *types.TypeParam) bool {
	for index := range tuple.Len() {
		if ps6091UsesTypeParam(tuple.At(index).Type(), target) {
			return true
		}
	}
	return false
}

func ps6091ConstraintMethods(constraint *types.Interface, self *types.TypeParam, replacement types.Type) (*types.Interface, bool) {
	methods := make([]*types.Func, constraint.NumMethods())
	for index := range constraint.NumMethods() {
		method := constraint.Method(index)
		signature, ok := method.Type().(*types.Signature)
		if !ok {
			return nil, false
		}
		substituted, ok := ps6091SubstituteSelfSignature(signature, self, replacement)
		if !ok {
			return nil, false
		}
		methods[index] = types.NewFunc(method.Pos(), method.Pkg(), method.Name(), substituted)
	}
	return types.NewInterfaceType(methods, nil).Complete(), true
}

func ps6091SubstituteSelfSignature(signature *types.Signature, self *types.TypeParam, replacement types.Type) (*types.Signature, bool) {
	if signature.TypeParams().Len() != 0 || signature.RecvTypeParams().Len() != 0 {
		return nil, false
	}
	params, ok := ps6091SubstituteSelfTuple(signature.Params(), self, replacement)
	if !ok {
		return nil, false
	}
	results, ok := ps6091SubstituteSelfTuple(signature.Results(), self, replacement)
	if !ok {
		return nil, false
	}
	return types.NewSignatureType(nil, nil, nil, params, results, signature.Variadic()), true
}

func ps6091SubstituteSelfTuple(tuple *types.Tuple, self *types.TypeParam, replacement types.Type) (*types.Tuple, bool) {
	variables := make([]*types.Var, tuple.Len())
	for index := range tuple.Len() {
		variable := tuple.At(index)
		typeValue, ok := ps6091SubstituteSelf(variable.Type(), self, replacement)
		if !ok {
			return nil, false
		}
		variables[index] = types.NewVar(variable.Pos(), variable.Pkg(), variable.Name(), typeValue)
	}
	return types.NewTuple(variables...), true
}

func ps6091SubstituteSelf(value types.Type, self *types.TypeParam, replacement types.Type) (types.Type, bool) {
	switch typed := value.(type) {
	case *types.Alias:
		return ps6091SubstituteSelf(types.Unalias(typed), self, replacement)
	case *types.TypeParam:
		if typed == self {
			return replacement, true
		}
		// Feasibility can depend on a different enclosing type parameter. Its
		// instantiation is unavailable here, so retain the term conservatively.
		return nil, false
	case *types.Basic:
		return typed, true
	case *types.Array:
		element, ok := ps6091SubstituteSelf(typed.Elem(), self, replacement)
		if !ok {
			return nil, false
		}
		return types.NewArray(element, typed.Len()), true
	case *types.Slice:
		element, ok := ps6091SubstituteSelf(typed.Elem(), self, replacement)
		if !ok {
			return nil, false
		}
		return types.NewSlice(element), true
	case *types.Pointer:
		element, ok := ps6091SubstituteSelf(typed.Elem(), self, replacement)
		if !ok {
			return nil, false
		}
		return types.NewPointer(element), true
	case *types.Map:
		key, keyOK := ps6091SubstituteSelf(typed.Key(), self, replacement)
		element, elementOK := ps6091SubstituteSelf(typed.Elem(), self, replacement)
		if !keyOK || !elementOK {
			return nil, false
		}
		return types.NewMap(key, element), true
	case *types.Chan:
		element, ok := ps6091SubstituteSelf(typed.Elem(), self, replacement)
		if !ok {
			return nil, false
		}
		return types.NewChan(typed.Dir(), element), true
	case *types.Signature:
		return ps6091SubstituteSelfSignature(typed, self, replacement)
	case *types.Struct:
		fields := make([]*types.Var, typed.NumFields())
		tags := make([]string, typed.NumFields())
		for index := range typed.NumFields() {
			field := typed.Field(index)
			fieldType, ok := ps6091SubstituteSelf(field.Type(), self, replacement)
			if !ok {
				return nil, false
			}
			fields[index] = types.NewField(field.Pos(), field.Pkg(), field.Name(), fieldType, field.Embedded())
			tags[index] = typed.Tag(index)
		}
		return types.NewStruct(fields, tags), true
	case *types.Interface:
		methods := make([]*types.Func, typed.NumExplicitMethods())
		for index := range typed.NumExplicitMethods() {
			method := typed.ExplicitMethod(index)
			signature, ok := method.Type().(*types.Signature)
			if !ok {
				return nil, false
			}
			substituted, ok := ps6091SubstituteSelfSignature(signature, self, replacement)
			if !ok {
				return nil, false
			}
			methods[index] = types.NewFunc(method.Pos(), method.Pkg(), method.Name(), substituted)
		}
		embeddeds := make([]types.Type, typed.NumEmbeddeds())
		for index := range typed.NumEmbeddeds() {
			embedded, ok := ps6091SubstituteSelf(typed.EmbeddedType(index), self, replacement)
			if !ok {
				return nil, false
			}
			embeddeds[index] = embedded
		}
		return types.NewInterfaceType(methods, embeddeds).Complete(), true
	case *types.Union:
		terms := make([]*types.Term, typed.Len())
		for index := range typed.Len() {
			term := typed.Term(index)
			termType, ok := ps6091SubstituteSelf(term.Type(), self, replacement)
			if !ok {
				return nil, false
			}
			terms[index] = types.NewTerm(term.Tilde(), termType)
		}
		return types.NewUnion(terms), true
	case *types.Named:
		arguments := typed.TypeArgs()
		if arguments.Len() == 0 {
			return typed, true
		}
		substituted := make([]types.Type, arguments.Len())
		for index := range arguments.Len() {
			argument, ok := ps6091SubstituteSelf(arguments.At(index), self, replacement)
			if !ok {
				return nil, false
			}
			substituted[index] = argument
		}
		instance, err := types.Instantiate(nil, typed.Origin(), substituted, false)
		return instance, err == nil
	default:
		return nil, false
	}
}

func ps6091UnderlyingMethodConflict(underlying types.Type, constraint *types.Interface) bool {
	structure, ok := types.Unalias(underlying).Underlying().(*types.Struct)
	if !ok {
		return false
	}
	for fieldIndex := range structure.NumFields() {
		fieldName := structure.Field(fieldIndex).Name()
		for methodIndex := range constraint.NumMethods() {
			if fieldName == constraint.Method(methodIndex).Name() {
				return true
			}
		}
	}
	return false
}

func ps6091MethodReceiverUnderlying(underlying types.Type) bool {
	switch value := types.Unalias(underlying).Underlying().(type) {
	case *types.Pointer, *types.Interface:
		return false
	case *types.Basic:
		return value.Kind() != types.Invalid && value.Kind() != types.UntypedNil && value.Kind() != types.UnsafePointer
	case *types.Array, *types.Chan, *types.Map, *types.Signature, *types.Slice, *types.Struct:
		return true
	default:
		return false
	}
}

func ps6091TildeUnderlying(underlying types.Type) bool {
	switch value := types.Unalias(underlying).Underlying().(type) {
	case *types.Basic:
		return value.Kind() != types.Invalid && value.Kind() != types.UntypedNil
	case *types.Array, *types.Chan, *types.Map, *types.Pointer, *types.Signature, *types.Slice, *types.Struct:
		return true
	default:
		return false
	}
}

func ps6091WitnessPackage(constraint *types.Interface) *types.Package {
	var witnessPackage *types.Package
	for index := range constraint.NumMethods() {
		method := constraint.Method(index)
		if method.Exported() {
			continue
		}
		if method.Pkg() == nil {
			return nil
		}
		if witnessPackage == nil {
			witnessPackage = method.Pkg()
			continue
		}
		if witnessPackage.Path() != method.Pkg().Path() {
			return nil
		}
	}
	if witnessPackage == nil {
		witnessPackage = types.NewPackage("perfscan.invalid/ps6091-witness", "ps6091witness")
	}
	return witnessPackage
}

type ps6091TypeTerm struct {
	tilde bool
	value types.Type
}

type ps6091TypeSet struct {
	unrestricted bool
	terms        []ps6091TypeTerm
}

// go/types deliberately keeps an interface's normalized type set internal.
// This small normalizer expands only the constructs that can constrain an
// element type: aliases, named/embedded interfaces, unions and intersections.
// Method and comparable restrictions can narrow a represented term but cannot
// introduce a non-integer underlying type, so retaining that term is a safe
// over-approximation for the all-integer proof.
func ps6091ConstraintTypeSet(value types.Type, visiting map[types.Type]bool) (ps6091TypeSet, bool) {
	value = types.Unalias(value)
	if visiting[value] {
		return ps6091TypeSet{}, false
	}
	switch typed := value.(type) {
	case *types.TypeParam:
		visiting[value] = true
		set, ok := ps6091ConstraintTypeSet(typed.Constraint(), visiting)
		delete(visiting, value)
		return set, ok
	case *types.Named:
		if _, ok := typed.Underlying().(*types.Interface); !ok {
			return ps6091TypeSet{terms: []ps6091TypeTerm{{value: typed}}}, true
		}
		visiting[value] = true
		set, ok := ps6091ConstraintTypeSet(typed.Underlying(), visiting)
		delete(visiting, value)
		return set, ok
	case *types.Interface:
		visiting[value] = true
		typed.Complete()
		set := ps6091TypeSet{unrestricted: true}
		for index := range typed.NumEmbeddeds() {
			embedded, ok := ps6091ConstraintTypeSet(typed.EmbeddedType(index), visiting)
			if !ok {
				delete(visiting, value)
				return ps6091TypeSet{}, false
			}
			set = ps6091IntersectTypeSets(set, embedded)
		}
		delete(visiting, value)
		return set, true
	case *types.Union:
		set := ps6091TypeSet{}
		for index := range typed.Len() {
			term := typed.Term(index)
			var alternative ps6091TypeSet
			if term.Tilde() {
				termType := types.Unalias(term.Type())
				if _, invalid := termType.Underlying().(*types.Interface); invalid {
					return ps6091TypeSet{}, false
				}
				alternative.terms = []ps6091TypeTerm{{tilde: true, value: termType}}
			} else {
				var ok bool
				alternative, ok = ps6091ConstraintTypeSet(term.Type(), visiting)
				if !ok {
					return ps6091TypeSet{}, false
				}
			}
			set = ps6091UnionTypeSets(set, alternative)
		}
		return set, true
	default:
		return ps6091TypeSet{terms: []ps6091TypeTerm{{value: value}}}, true
	}
}

func ps6091UnionTypeSets(left, right ps6091TypeSet) ps6091TypeSet {
	if left.unrestricted || right.unrestricted {
		return ps6091TypeSet{unrestricted: true}
	}
	result := ps6091TypeSet{terms: slices.Clone(left.terms)}
	for _, term := range right.terms {
		result.terms = ps6091AppendTypeTerm(result.terms, term)
	}
	return result
}

func ps6091IntersectTypeSets(left, right ps6091TypeSet) ps6091TypeSet {
	if left.unrestricted {
		return right
	}
	if right.unrestricted {
		return left
	}
	var result ps6091TypeSet
	for _, leftTerm := range left.terms {
		for _, rightTerm := range right.terms {
			term, ok := ps6091IntersectTypeTerms(leftTerm, rightTerm)
			if ok {
				result.terms = ps6091AppendTypeTerm(result.terms, term)
			}
		}
	}
	return result
}

func ps6091IntersectTypeTerms(left, right ps6091TypeTerm) (ps6091TypeTerm, bool) {
	leftType := types.Unalias(left.value)
	rightType := types.Unalias(right.value)
	switch {
	case !left.tilde && !right.tilde:
		return left, types.Identical(leftType, rightType)
	case left.tilde && right.tilde:
		return left, types.Identical(leftType.Underlying(), rightType.Underlying())
	case left.tilde:
		return right, types.Identical(leftType.Underlying(), rightType.Underlying())
	default:
		return left, types.Identical(leftType.Underlying(), rightType.Underlying())
	}
}

func ps6091AppendTypeTerm(terms []ps6091TypeTerm, candidate ps6091TypeTerm) []ps6091TypeTerm {
	for _, term := range terms {
		if term.tilde == candidate.tilde && types.Identical(types.Unalias(term.value), types.Unalias(candidate.value)) {
			return terms
		}
	}
	return append(terms, candidate)
}

func ps6091ErrorType(value types.Type) bool {
	errorType := types.Universe.Lookup("error").Type()
	return types.AssignableTo(value, errorType)
}

func ps6091OnlyFirstIndexReads(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node, object types.Object) bool {
	uses := 0
	valid := true
	ast.Inspect(body, func(node ast.Node) bool {
		if !valid {
			return false
		}
		identifier, ok := node.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[identifier] != object {
			return true
		}
		index, ok := ps6091IndexUse(identifier, parents)
		if !ok || !ps6091ConstantInteger(pass, index.Index, 0) ||
			!ps6091ReadOnlyIndex(pass, index, parents) || ps6091InsideClosure(index, body, parents) {
			valid = false
			return false
		}
		uses++
		return true
	})
	return valid && uses > 0
}

func ps6091IndexUse(identifier *ast.Ident, parents map[ast.Node]ast.Node) (*ast.IndexExpr, bool) {
	var node ast.Node = identifier
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		index, ok := parent.(*ast.IndexExpr)
		return index, ok && ps2110Unparen(index.X) == identifier
	}
}

func ps6091ReadOnlyIndex(pass *analysis.Pass, index *ast.IndexExpr, parents map[ast.Node]ast.Node) bool {
	var node ast.Node = index
	for {
		parent := parents[node]
		if paren, ok := parent.(*ast.ParenExpr); ok {
			node = paren
			continue
		}
		switch value := parent.(type) {
		case *ast.AssignStmt:
			for _, lhs := range value.Lhs {
				if lhs == node {
					return false
				}
			}
		case *ast.IncDecStmt:
			return false
		case *ast.RangeStmt:
			if value.Key == node || value.Value == node {
				return false
			}
		case *ast.UnaryExpr:
			if value.Op == token.AND {
				return false
			}
		case *ast.SelectorExpr:
			// A value-receiver method observes a copy of the scalar. A pointer-
			// receiver method implicitly takes the address of the addressable
			// slice element and can mutate or retain it.
			selection := pass.TypesInfo.Selections[value]
			if selection == nil {
				return false
			}
			signature, ok := selection.Obj().Type().(*types.Signature)
			if !ok || signature.Recv() == nil {
				return false
			}
			_, pointerReceiver := types.Unalias(signature.Recv().Type()).(*types.Pointer)
			return !pointerReceiver
		}
		return true
	}
}

func ps6091InsideClosure(node ast.Node, body *ast.BlockStmt, parents map[ast.Node]ast.Node) bool {
	for node != nil && node != body {
		node = parents[node]
		if _, ok := node.(*ast.FuncLit); ok {
			return true
		}
	}
	return false
}
