package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

// Source summaries describe storage and effects, never quantization arithmetic
// equivalence. Semantic API selection remains an independently reviewed contract.
// Roots are invocation-local: a parameter or one fresh allocation, not a name,
// SSA instruction shared across loop iterations, or an assumed heap allocation.
type ps6141Root int
type ps6141Deps map[ps6141Root]bool
type ps6141SourceSummary struct {
	byteLaneTransformUnknown bool
	byteConsumerStride       int64
	byteConsumerInputs       ps6141Deps
	byteProducer             *ps6141ByteProducerProof
	completionBounds         map[ps6141Root]map[ps6141Root]int64
	valid                    bool
	result                   ps6141Root
	resultDeps               ps6141Deps
	writes                   map[ps6141Root]ps6141Deps
	traversals               map[ps6141Root]int
	reductions               map[ps6141Root]int
	lanePasses               map[ps6141Root]int
	laneReductions           map[ps6141Root]int
	resultReductions         ps6141Deps
	resultLaneReductions     ps6141Deps
	resultSuppressed         bool
}
type ps6141SummaryIndex struct {
	pass         *analysis.Pass
	declarations map[*types.Func]*ast.FuncDecl
	memo         map[*types.Func]ps6141SourceSummary
	active       map[*types.Func]bool
	remaining    int
}
type ps6141SummaryBody struct {
	byteConsumer      *ps6141ByteConsumerLayout
	index             *ps6141SummaryIndex
	summary           ps6141SourceSummary
	roots             map[types.Object]ps6141Root
	scalars           map[types.Object]ps6141Deps
	scalarLoops       map[types.Object]int
	fresh             ps6141Root
	returned          bool
	loop              int
	loopIndex         types.Object
	traversing        map[ps6141Root]bool
	outerRoot         ps6141Root
	outerIndex        types.Object
	laneIndex         types.Object
	laneField         *types.Var
	laneExtent        int64
	bounds            map[ps6141Root]map[ps6141Root]int64
	contributions     map[types.Object]ps6141Deps
	laneContributions map[types.Object]ps6141Deps
	callResults       map[*ast.CallExpr]ps6141SourceSummary
	suppressedValues  map[types.Object]bool
}

func ps6141ComposedBoundary(pass *analysis.Pass, producer, consumer *ast.FuncDecl, c *config.SingleUseQuantizationContract) (types.Type, *ps6141ByteProducerProof) {
	if producer == nil || consumer == nil {
		return nil, nil
	}
	p, _ := pass.TypesInfo.Defs[producer.Name].(*types.Func)
	q, _ := pass.TypesInfo.Defs[consumer.Name].(*types.Func)
	if p == nil || q == nil {
		return nil, nil
	}
	ps, qs := p.Type().(*types.Signature), q.Type().(*types.Signature)
	destination := c.ProducerForm == "destination"
	if (!destination && (ps.Params().Len() != 1 || ps.Results().Len() != 1)) || (destination && (ps.Params().Len() != 2 || ps.Results().Len() != 0)) || qs.Params().Len() != 2 || qs.Results().Len() != 1 {
		return nil, nil
	}
	input, ok := types.Unalias(ps.Params().At(c.FloatInputArgument).Type()).Underlying().(*types.Slice)
	if !ok || !(types.Identical(input.Elem(), types.Typ[types.Float32]) || types.Identical(input.Elem(), types.Typ[types.Float64])) {
		return nil, nil
	}
	var packed types.Type
	if destination {
		packed = ps.Params().At(c.DestinationArgument).Type()
	} else {
		packed = ps.Results().At(0).Type()
	}
	layout, ok := types.Unalias(packed).Underlying().(*types.Slice)
	if !ok || !ps6141SummaryLayout(layout.Elem(), 0) || !types.Identical(qs.Params().At(c.PackedArgument).Type(), packed) || !types.Identical(qs.Params().At(c.WeightArgument).Type(), packed) {
		return nil, nil
	}
	index := &ps6141SummaryIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
	a, b := index.producer(p), index.function(q)
	pr, wr := ps6141Root(c.PackedArgument+1), ps6141Root(c.WeightArgument+1)
	if c.ConsumerForm == "packedByteDot" && (a.byteProducer == nil || b.byteConsumerStride != a.byteProducer.layout.byteStride || !b.byteConsumerInputs[pr] || !b.byteConsumerInputs[wr]) {
		return nil, nil
	}
	if !b.valid || b.result != 0 || b.traversals[pr] != 1 || b.traversals[wr] != 0 || b.reductions[pr] != 1 || !b.resultReductions[pr] || b.lanePasses[pr] > 1 || b.lanePasses[pr] == 1 && (b.laneReductions[pr] != 1 || !b.resultLaneReductions[pr]) || len(b.writes) != 0 || !b.resultDeps[pr] || !b.resultDeps[wr] {
		return nil, nil
	}
	if !a.valid && !destination && a.byteProducer != nil {
		proof := a.byteProducer
		if proof.bodyEffectsKnown && proof.finiteBodyKnown && proof.floatViewBoundsKnown && proof.laneSourceOriginKnown {
			return packed, proof
		}
		return nil, nil
	}
	produced := a.result
	if destination {
		produced = ps6141Root(c.DestinationArgument + 1)
	}
	if !a.valid || (!destination && a.result >= 0) || (destination && (a.result != 0 || len(a.writes) != 1)) || !a.writes[produced][ps6141Root(c.FloatInputArgument+1)] {
		return nil, nil
	}
	// A producer may read its float formal but must never mutate it.
	if _, mutated := a.writes[ps6141Root(c.FloatInputArgument+1)]; mutated {
		return nil, nil
	}
	return packed, nil
}

func (index *ps6141SummaryIndex) function(fn *types.Func) ps6141SourceSummary {
	fn = fn.Origin()
	if s, ok := index.memo[fn]; ok {
		return s
	}
	if index.declarations[fn] == nil && ps6141ImportedSDKScalarLeaf(index.pass, fn) {
		// The reviewed SDK leaves evaluate and return their scalar argument's
		// value flow. This is not a general exemption for imported helpers.
		sig := fn.Type().(*types.Signature)
		unsigned := false
		if sig.Results().Len() == 1 {
			if basic, ok := types.Unalias(sig.Results().At(0).Type()).Underlying().(*types.Basic); ok {
				unsigned = basic.Info()&types.IsUnsigned != 0
			}
		}
		return ps6141SourceSummary{valid: true, resultDeps: ps6141Deps{1: true}, byteLaneTransformUnknown: unsigned}
	}
	if index.active[fn] || index.remaining <= 0 {
		return ps6141SourceSummary{}
	}
	decl := index.declarations[fn]
	sig := fn.Type().(*types.Signature)
	// Formal roots must never overlap the reserved 1000+ protocol namespaces.
	if decl == nil || decl.Body == nil || sig.Params().Len() >= 1000 || sig.Recv() != nil || sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Results().Len() > 1 {
		return ps6141SourceSummary{}
	}
	index.active[fn] = true
	defer delete(index.active, fn)
	body := &ps6141SummaryBody{index: index, summary: ps6141SourceSummary{valid: true, writes: map[ps6141Root]ps6141Deps{}, traversals: map[ps6141Root]int{}, reductions: map[ps6141Root]int{}, lanePasses: map[ps6141Root]int{}, laneReductions: map[ps6141Root]int{}}, roots: map[types.Object]ps6141Root{}, scalars: map[types.Object]ps6141Deps{}, scalarLoops: map[types.Object]int{}, traversing: map[ps6141Root]bool{}, bounds: map[ps6141Root]map[ps6141Root]int64{}}
	body.contributions = map[types.Object]ps6141Deps{}
	body.laneContributions = map[types.Object]ps6141Deps{}
	body.callResults = map[*ast.CallExpr]ps6141SourceSummary{}
	body.suppressedValues = map[types.Object]bool{}
	if layout := ps6141ByteConsumer(index.pass, decl); layout != nil && layout.storageEffectsKnown && layout.completionKnown {
		body.byteConsumer = layout
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		root := ps6141Root(i + 1)
		if _, ok := types.Unalias(param.Type()).Underlying().(*types.Slice); ok {
			body.roots[param] = root
		} else if ps6141SummaryScalar(param.Type()) {
			body.scalars[param] = ps6141Deps{root: true}
		} else {
			body.summary.valid = false
		}
	}
	body.block(decl.Body)
	if body.byteConsumer != nil {
		body.summary.byteConsumerStride = body.byteConsumer.stride
		body.summary.byteConsumerInputs = ps6141Deps{}
		retain := func(root ps6141Root) {
			if body.summary.resultDeps[1000+root] && body.summary.resultDeps[2000+root] && body.summary.resultDeps[3000+root] && !body.summary.resultSuppressed {
				body.summary.byteConsumerInputs[root] = true
			}
		}
		retain(1)
		retain(2)
	}
	body.summary.completionBounds = body.bounds
	if sig.Results().Len() == 1 && !body.returned {
		body.summary.valid = false
	}
	laneUnknown := body.summary.byteLaneTransformUnknown
	if !body.summary.valid {
		body.summary = index.immutableScalarReturns(fn)
	}
	if sig.Results().Len() == 1 {
		body.summary.byteLaneTransformUnknown = laneUnknown || ps6141ByteLaneTransformUnknown(index.pass, decl)
	}
	index.memo[fn] = body.summary
	return body.summary
}
func ps6141SummaryScalar(t types.Type) bool {
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && b.Info()&(types.IsNumeric|types.IsBoolean|types.IsString) != 0
}

// Pointer-free value layouts avoid mistaking a fresh outer block slice for
// ownership of references stored inside its elements.
func ps6141SummaryLayout(t types.Type, depth int) bool {
	if depth > 32 {
		return false
	}
	switch x := types.Unalias(t).Underlying().(type) {
	case *types.Basic:
		return x.Info()&types.IsNumeric != 0
	case *types.Array:
		return ps6141SummaryLayout(x.Elem(), depth+1)
	case *types.Struct:
		for i := 0; i < x.NumFields(); i++ {
			if !ps6141SummaryLayout(x.Field(i).Type(), depth+1) {
				return false
			}
		}
		return x.NumFields() > 0
	}
	return false
}
func ps6141MergeDeps(a, b ps6141Deps) ps6141Deps {
	out := ps6141Deps{}
	for r := range a {
		out[r] = true
	}
	for r := range b {
		out[r] = true
	}
	return out
}
func (b *ps6141SummaryBody) take() bool {
	b.index.remaining--
	if b.index.remaining < 0 {
		b.summary.valid = false
	}
	return b.summary.valid
}
func (b *ps6141SummaryBody) root(expr ast.Expr) ps6141Root {
	switch x := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		return b.roots[identObject(b.index.pass, x)]
	case *ast.IndexExpr:
		return b.root(x.X)
	case *ast.SelectorExpr:
		if b.index.pass.TypesInfo.Selections[x] != nil {
			return b.root(x.X)
		}
	}
	return 0
}

// A root is only an identity lookup. Every path still needs effect and type
// validation: a nested index can execute a call even when its base is known.
func (b *ps6141SummaryBody) path(expr ast.Expr) bool {
	if !b.take() {
		return false
	}
	switch x := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		return b.root(x) != 0
	case *ast.SelectorExpr:
		selection := b.index.pass.TypesInfo.Selections[x]
		return selection != nil && selection.Kind() == types.FieldVal && b.path(x.X)
	case *ast.IndexExpr:
		b.expression(x.Index)
		if !b.path(x.X) {
			return false
		}
		switch types.Unalias(b.index.pass.TypesInfo.TypeOf(x.X)).Underlying().(type) {
		case *types.Slice:
			// One slice traversal does not establish single consumption when reads
			// repeatedly select a constant or another block/row index.
			id, ok := ps2110Unparen(x.Index).(*ast.Ident)
			if b.loop == 0 {
				return false
			}
			if b.laneIndex != nil {
				root := b.root(x.X)
				if ok && identObject(b.index.pass, id) == b.outerIndex {
					if root != b.outerRoot && b.bounds[root][b.outerRoot] != 1 {
						return false
					}
				} else if b.bounds[root][b.outerRoot] != b.laneExtent || !b.flatLaneIndex(x.Index) {
					return false
				}
			} else if !ok || identObject(b.index.pass, id) != b.loopIndex {
				return false
			}
		case *types.Array:
			if b.laneIndex != nil {
				selector, ok := ps2110Unparen(x.X).(*ast.SelectorExpr)
				id, indexOK := ps2110Unparen(x.Index).(*ast.Ident)
				if !ok || !indexOK || identObject(b.index.pass, id) != b.laneIndex {
					return false
				}
				selection := b.index.pass.TypesInfo.Selections[selector]
				array := types.Unalias(b.index.pass.TypesInfo.TypeOf(x.X)).Underlying().(*types.Array)
				if selection == nil || selection.Obj() != b.laneField || array.Len() != b.laneExtent {
					return false
				}
			}
		default:
			return false
		}
		return b.summary.valid
	}
	return false
}
func (b *ps6141SummaryBody) flatLaneIndex(expr ast.Expr) bool {
	sum, ok := ps2110Unparen(expr).(*ast.BinaryExpr)
	if !ok || sum.Op != token.ADD {
		return false
	}
	lane, ok := ps2110Unparen(sum.Y).(*ast.Ident)
	if !ok || identObject(b.index.pass, lane) != b.laneIndex {
		return false
	}
	product, ok := ps2110Unparen(sum.X).(*ast.BinaryExpr)
	if !ok || product.Op != token.MUL || !ps6141ConstantInt(b.index.pass, product.Y, b.laneExtent) {
		return false
	}
	block, ok := ps2110Unparen(product.X).(*ast.Ident)
	return ok && identObject(b.index.pass, block) == b.outerIndex
}

// Fixed-array lanes refine one current block; they are not another packed
// activation traversal or an arbitrary nested output-row loop.
func (b *ps6141SummaryBody) lanes(s *ast.RangeStmt) {
	if s.Value != nil || s.Tok != token.DEFINE || !b.path(s.X) {
		b.summary.valid = false
		return
	}
	selector, ok := ps2110Unparen(s.X).(*ast.SelectorExpr)
	if !ok || b.root(s.X) != b.outerRoot {
		b.summary.valid = false
		return
	}
	current, ok := ps2110Unparen(selector.X).(*ast.IndexExpr)
	if !ok {
		b.summary.valid = false
		return
	}
	currentID, ok := ps2110Unparen(current.Index).(*ast.Ident)
	if !ok || identObject(b.index.pass, currentID) != b.outerIndex {
		b.summary.valid = false
		return
	}
	selection := b.index.pass.TypesInfo.Selections[selector]
	array, ok := types.Unalias(b.index.pass.TypesInfo.TypeOf(s.X)).Underlying().(*types.Array)
	if !ok || array.Len() <= 0 || array.Len() > 1<<20 || !ps6141SummaryLayout(array.Elem(), 0) || selection == nil || selection.Kind() != types.FieldVal {
		b.summary.valid = false
		return
	}
	field, ok := selection.Obj().(*types.Var)
	if !ok {
		b.summary.valid = false
		return
	}
	id, ok := s.Key.(*ast.Ident)
	if !ok || id.Name == "_" {
		b.summary.valid = false
		return
	}
	index := b.index.pass.TypesInfo.Defs[id]
	if index == nil || index == b.outerIndex {
		b.summary.valid = false
		return
	}
	b.scalars[index] = ps6141Deps{}
	b.scalarLoops[index] = 2
	b.laneIndex = index
	b.laneField = field
	b.laneExtent = array.Len()
	b.loop = 2
	b.summary.lanePasses[b.outerRoot] = min(2, b.summary.lanePasses[b.outerRoot]+1)
	b.block(s.Body)
	b.loop = 1
	b.laneIndex = nil
	b.laneField = nil
	b.laneExtent = 0
}
func (b *ps6141SummaryBody) lenRoot(expr ast.Expr) ps6141Root {
	call, ok := ps2110Unparen(expr).(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return 0
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || b.index.pass.TypesInfo.Uses[fn] != types.Universe.Lookup("len") {
		return 0
	}
	id, ok := ps2110Unparen(call.Args[0]).(*ast.Ident)
	if !ok {
		return 0
	}
	return b.root(id)
}
func (b *ps6141SummaryBody) recordBounds(expr ast.Expr) {
	if !b.summary.valid || b.loop != 0 {
		return
	}
	comparison, ok := ps2110Unparen(expr).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.NEQ {
		return
	}
	left, right := b.lenRoot(comparison.X), b.lenRoot(comparison.Y)
	set := func(a, c ps6141Root, n int64) {
		if a == 0 || c == 0 {
			return
		}
		if b.bounds[a] == nil {
			b.bounds[a] = map[ps6141Root]int64{}
		}
		b.bounds[a][c] = n
	}
	if left != 0 && right != 0 {
		set(left, right, 1)
		set(right, left, 1)
		return
	}
	product, ok := ps2110Unparen(comparison.Y).(*ast.BinaryExpr)
	// A quotient guard bounds block*extent+lane without overflowing int.
	// len(float)==len(blocks)*extent alone is not that proof: multiplication
	// can wrap for sufficiently large slice lengths.
	if !ok || product.Op != token.QUO || left == 0 {
		return
	}
	right = b.lenRoot(product.X)
	value := b.index.pass.TypesInfo.Types[product.Y].Value
	if value == nil || value.Kind() != constant.Int {
		return
	}
	extent, exact := constant.Int64Val(value)
	if exact && extent > 0 && extent <= 1<<20 {
		set(right, left, extent)
	}
}

// Contribution markers are value-flow facts, separate from effect counters.
// Overwrites discard old markers; a reduction that is reset or never returned
// cannot qualify merely because its update statement was visited.
func (b *ps6141SummaryBody) markers(expr ast.Expr, lane bool) ps6141Deps {
	if expr == nil || !b.take() {
		return nil
	}
	if b.erases(expr) {
		return nil
	}
	switch x := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		if lane {
			return b.laneContributions[identObject(b.index.pass, x)]
		}
		return b.contributions[identObject(b.index.pass, x)]
	case *ast.BinaryExpr:
		if x.Op == token.MUL && (ps6141ConstantInt(b.index.pass, x.X, 0) || ps6141ConstantInt(b.index.pass, x.Y, 0)) {
			return nil
		}
		return ps6141MergeDeps(b.markers(x.X, lane), b.markers(x.Y, lane))
	case *ast.UnaryExpr:
		return b.markers(x.X, lane)
	case *ast.CallExpr:
		if b.index.pass.TypesInfo.Types[x.Fun].IsType() && len(x.Args) == 1 {
			return b.markers(x.Args[0], lane)
		}
		result := b.callResults[x]
		if lane {
			return result.resultLaneReductions
		}
		return result.resultReductions
	}
	return nil
}

// This is conservative admission suppression, not a floating-point identity:
// multiplying by zero can retain NaN/Inf behavior. Integer &0 does ignore its
// operand value, but evaluating that operand may still have effects.
func (b *ps6141SummaryBody) erases(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	switch x := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		return b.suppressedValues[identObject(b.index.pass, x)]
	case *ast.BinaryExpr:
		if (x.Op == token.MUL || x.Op == token.AND) && (b.zero(x.X) || b.zero(x.Y)) {
			return true
		}
		return b.erases(x.X) || b.erases(x.Y)
	case *ast.UnaryExpr:
		return b.erases(x.X)
	case *ast.CallExpr:
		if b.index.pass.TypesInfo.Types[x.Fun].IsType() {
			for _, arg := range x.Args {
				if b.erases(arg) {
					return true
				}
			}
			return false
		}
		return b.callResults[x].resultSuppressed
	}
	return false
}
func (b *ps6141SummaryBody) zero(expr ast.Expr) bool {
	value := b.index.pass.TypesInfo.Types[expr].Value
	return value != nil && (value.Kind() == constant.Int || value.Kind() == constant.Float || value.Kind() == constant.Complex) && constant.Compare(value, token.EQL, constant.MakeInt64(0))
}
func (b *ps6141SummaryBody) additiveRecurrence(expr ast.Expr, object types.Object) bool {
	binary, ok := ps2110Unparen(expr).(*ast.BinaryExpr)
	if !ok || binary.Op != token.ADD && binary.Op != token.SUB {
		return false
	}
	isAccumulator := func(expr ast.Expr) bool {
		id, ok := ps2110Unparen(expr).(*ast.Ident)
		return ok && identObject(b.index.pass, id) == object
	}
	return isAccumulator(binary.X) || binary.Op == token.ADD && isAccumulator(binary.Y)
}
func (b *ps6141SummaryBody) expression(expr ast.Expr) ps6141Deps {
	if expr == nil || !b.take() {
		return nil
	}
	if b.byteConsumer != nil {
		if root := b.byteConsumer.slices[expr]; root != 0 {
			return ps6141Deps{root: true}
		}
		if index, ok := expr.(*ast.IndexExpr); ok {
			if root := b.byteConsumer.indices[index]; root != 0 {
				return ps6141Deps{root: true, 1000 + root: true}
			}
		}
		if call, ok := expr.(*ast.CallExpr); ok {
			if root := b.byteConsumer.reads[call]; root != 0 {
				return ps6141Deps{root: true, 2000 + root: true}
			}
		}
	}
	switch x := ps2110Unparen(expr).(type) {
	case *ast.BasicLit:
		return ps6141Deps{}
	case *ast.Ident:
		object := identObject(b.index.pass, x)
		if _, ok := object.(*types.Const); ok {
			return ps6141Deps{}
		}
		if r := b.roots[object]; r != 0 {
			return ps6141Deps{r: true}
		}
		if d, ok := b.scalars[object]; ok {
			return d
		}
	case *ast.BinaryExpr:
		deps := ps6141MergeDeps(b.expression(x.X), b.expression(x.Y))
		if ps6141ByteBitOperator(x.Op) {
			deps = ps6141ByteDropLaneTokens(deps)
		}
		if x.Op == token.AND && (ps6141ConstantInt(b.index.pass, x.X, 0) || ps6141ConstantInt(b.index.pass, x.Y, 0)) {
			return nil
		}
		return deps
	case *ast.UnaryExpr:
		if x.Op != token.AND && x.Op != token.ARROW && x.Op != token.MUL {
			deps := b.expression(x.X)
			if x.Op == token.XOR {
				deps = ps6141ByteDropLaneTokens(deps)
			}
			return deps
		}
	case *ast.IndexExpr:
		r := b.root(x.X)
		if r != 0 {
			if !b.path(x) {
				b.summary.valid = false
			}
			return ps6141Deps{r: true}
		}
	case *ast.SelectorExpr:
		selection := b.index.pass.TypesInfo.Selections[x]
		if selection != nil && selection.Kind() == types.FieldVal {
			return b.expression(x.X)
		}
	case *ast.CallExpr:
		if b.index.pass.TypesInfo.Types[x.Fun].IsType() {
			if len(x.Args) == 1 && ps6141SummaryScalar(b.index.pass.TypesInfo.TypeOf(x)) {
				deps := b.expression(x.Args[0])
				if basic, ok := types.Unalias(b.index.pass.TypesInfo.TypeOf(x)).Underlying().(*types.Basic); ok && basic.Info()&types.IsUnsigned != 0 {
					deps = ps6141ByteDropLaneTokens(deps)
				}
				if types.Identical(types.Unalias(b.index.pass.TypesInfo.TypeOf(x)).Underlying(), types.Typ[types.Int8]) {
					retain := func(root ps6141Root) {
						if deps[1000+root] {
							deps = ps6141MergeDeps(deps, ps6141Deps{3000 + root: true})
						}
					}
					retain(1)
					retain(2)
				}
				return deps
			}
		}
		if id, ok := x.Fun.(*ast.Ident); ok {
			if builtin, ok := b.index.pass.TypesInfo.Uses[id].(*types.Builtin); ok {
				switch builtin.Name() {
				case "len", "cap":
					if len(x.Args) == 1 && b.path(x.Args[0]) {
						return ps6141Deps{}
					}
				case "panic":
					if len(x.Args) == 1 && ps6141SummaryScalar(b.index.pass.TypesInfo.TypeOf(x.Args[0])) {
						return b.expression(x.Args[0])
					}
				}
			}
		}
		s := b.call(x)
		if s.valid && s.result == 0 {
			return s.resultDeps
		}
	}
	b.summary.valid = false
	return nil
}

// Substitute callee roots with actual roots. Calls with memory-valued scalar
// arguments, duplicate storage arguments, opaque bodies or nested traversal
// effects fail closed. Scalar helper arithmetic may be arbitrary source syntax.
func (b *ps6141SummaryBody) call(call *ast.CallExpr) ps6141SourceSummary {
	fn, sig, ok := typedCallee(b.index.pass, call.Fun)
	if !ok || sig.Recv() != nil || call.Ellipsis.IsValid() || len(call.Args) != sig.Params().Len() {
		b.summary.valid = false
		return ps6141SourceSummary{}
	}
	s := b.index.function(fn)
	b.summary.byteLaneTransformUnknown = b.summary.byteLaneTransformUnknown || s.byteLaneTransformUnknown
	if !s.valid {
		b.summary.valid = false
		return s
	}
	bindings := map[ps6141Root]ps6141Deps{}
	seen := map[ps6141Root]bool{}
	// Symbolic-map domain: seen keys are actual invocation roots, including negative fresh-allocation identities
	for i, arg := range call.Args {
		if _, slice := types.Unalias(sig.Params().At(i).Type()).Underlying().(*types.Slice); slice {
			r := b.root(arg)
			_, direct := ps2110Unparen(arg).(*ast.Ident)
			if r == 0 || seen[r] || !b.path(arg) || !direct { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
				b.summary.valid = false
				return ps6141SourceSummary{}
			}
			seen[r] = true
			bindings[ps6141Root(i+1)] = ps6141Deps{r: true}
		} else {
			bindings[ps6141Root(i+1)] = b.expression(arg)
		}
	}
	if s.result < 0 {
		if b.loop != 0 {
			b.summary.valid = false
			return ps6141SourceSummary{}
		}
		b.fresh--
		bindings[s.result] = ps6141Deps{b.fresh: true}
	}
	remap := func(d ps6141Deps) ps6141Deps {
		out := ps6141Deps{}
		// Symbolic-map domain: bindings include negative fresh roots and sparse returned/protocol origin identities
		for r := range d {
			out = ps6141MergeDeps(out, bindings[r]) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
		}
		return out
	}
	out := ps6141SourceSummary{valid: b.summary.valid, resultDeps: remap(s.resultDeps), resultReductions: remap(s.resultReductions), resultLaneReductions: remap(s.resultLaneReductions), resultSuppressed: s.resultSuppressed}
	out.byteConsumerStride = s.byteConsumerStride
	out.byteConsumerInputs = remap(s.byteConsumerInputs)
	if s.byteLaneTransformUnknown {
		out.resultDeps = ps6141ByteDropLaneTokens(out.resultDeps)
	}
	// Symbolic-map domain: resultDeps combines sparse formal origins with 1000/2000/3000 protocol namespaces
	for i, arg := range call.Args {
		if s.resultDeps[ps6141Root(i+1)] && ps6141SummaryScalar(sig.Params().At(i).Type()) { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
			if b.erases(arg) {
				out.resultSuppressed = true
			}
			if out.resultSuppressed {
				continue
			}
			out.resultReductions = ps6141MergeDeps(out.resultReductions, b.markers(arg, false))
			out.resultLaneReductions = ps6141MergeDeps(out.resultLaneReductions, b.markers(arg, true))
		}
	}
	if out.resultSuppressed {
		out.resultReductions = nil
		out.resultLaneReductions = nil
	}
	if s.result != 0 {
		for r := range bindings[s.result] {
			out.result = r
		}
		if out.result == 0 {
			b.summary.valid = false
			out.valid = false
		}
	}
	// Symbolic-map domain: writes/bindings are sparse invocation roots including negative fresh allocations
	for root, deps := range s.writes {
		for r := range bindings[root] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
			b.summary.writes[r] = ps6141MergeDeps(b.summary.writes[r], remap(deps)) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
		}
	}
	// A normally completed helper establishes its source-checked shape guard
	// in this actual call context. Slice lengths cannot be rebound by helpers.
	if b.loop == 0 {
		// Symbolic-map domain: bounds substitute sparse actual roots including negative destination allocations
		for from, targets := range s.completionBounds {
			for to, extent := range targets {
				for actualFrom := range bindings[from] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
					for actualTo := range bindings[to] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
						if b.bounds[actualFrom] == nil { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
							b.bounds[actualFrom] = map[ps6141Root]int64{}
						}
						if old := b.bounds[actualFrom][actualTo]; old != 0 && old != extent { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
							b.summary.valid = false
						}
						b.bounds[actualFrom][actualTo] = extent //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
					}
				}
			}
		}
	}
	// Symbolic-map domain: traversals/bindings use sparse invocation identities including negative fresh roots
	for root, n := range s.traversals {
		for r := range bindings[root] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
			if b.loop != 0 && n != 0 {
				b.summary.valid = false
			}
			b.summary.traversals[r] = min(2, b.summary.traversals[r]+n) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
		}
	}
	// Symbolic-map domain: reduction roots include sparse protocol tags 1000/2000/3000 in addition to actual storage roots
	for root, n := range s.reductions {
		for r := range bindings[root] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
			b.summary.reductions[r] = min(2, b.summary.reductions[r]+n) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
		}
	}
	// Symbolic-map domain: lane passes substitute sparse invocation storage roots, not current lane offsets
	for root, n := range s.lanePasses {
		for r := range bindings[root] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
			b.summary.lanePasses[r] = min(2, b.summary.lanePasses[r]+n) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
		}
	}
	// Symbolic-map domain: lane reduction identities include sparse protocol tags, not dense iteration positions
	for root, n := range s.laneReductions {
		for r := range bindings[root] { //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
			b.summary.laneReductions[r] = min(2, b.summary.laneReductions[r]+n) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
		}
	}
	out.valid = b.summary.valid
	b.callResults[call] = out
	return out
}
func (b *ps6141SummaryBody) block(block *ast.BlockStmt) {
	for _, statement := range block.List {
		if b.returned {
			return
		}
		if !b.take() {
			return
		}
		b.statement(statement)
	}
}
func (b *ps6141SummaryBody) statement(statement ast.Stmt) {
	switch s := statement.(type) {
	case *ast.DeclStmt:
		decl, ok := s.Decl.(*ast.GenDecl)
		if !ok || decl.Tok != token.VAR {
			b.summary.valid = false
			return
		}
		for _, spec := range decl.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) > 1 {
				b.summary.valid = false
				return
			}
			object := b.index.pass.TypesInfo.Defs[value.Names[0]]
			if object == nil || !ps6141NumericScalar(object.Type()) {
				b.summary.valid = false
				return
			}
			b.scalars[object] = ps6141Deps{}
			b.scalarLoops[object] = b.loop
			if len(value.Values) == 1 {
				b.scalars[object] = b.expression(value.Values[0])
				b.contributions[object] = b.markers(value.Values[0], false)
				b.laneContributions[object] = b.markers(value.Values[0], true)
				b.suppressedValues[object] = b.erases(value.Values[0])
			}
		}
	case *ast.AssignStmt:
		if len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			b.summary.valid = false
			return
		}
		if id, ok := s.Lhs[0].(*ast.Ident); ok && s.Tok == token.DEFINE {
			object := b.index.pass.TypesInfo.Defs[id]
			if object == nil {
				b.summary.valid = false
				return
			}
			if _, slice := types.Unalias(object.Type()).Underlying().(*types.Slice); slice {
				if b.byteConsumer != nil {
					if root := b.byteConsumer.views[object]; root != 0 && b.byteConsumer.slices[s.Rhs[0]] == root {
						b.roots[object] = root
						return
					}
				}
				call, ok := s.Rhs[0].(*ast.CallExpr)
				if !ok {
					b.summary.valid = false
					return
				}
				if fn, ok := call.Fun.(*ast.Ident); ok && b.index.pass.TypesInfo.Uses[fn] == types.Universe.Lookup("make") {
					if b.loop != 0 || len(call.Args) != 2 {
						b.summary.valid = false
						return
					}
					b.expression(call.Args[1])
					b.fresh--
					b.roots[object] = b.fresh
					return
				}
				result := b.call(call)
				if !result.valid || result.result >= 0 {
					b.summary.valid = false
					return
				}
				b.roots[object] = result.result
				return
			}
			if !ps6141SummaryScalar(object.Type()) {
				b.summary.valid = false
				return
			}
			b.scalars[object] = b.expression(s.Rhs[0])
			b.contributions[object] = b.markers(s.Rhs[0], false)
			b.laneContributions[object] = b.markers(s.Rhs[0], true)
			b.suppressedValues[object] = b.erases(s.Rhs[0])
			b.scalarLoops[object] = b.loop
			return
		}
		deps := b.expression(s.Rhs[0])
		if id, ok := s.Lhs[0].(*ast.Ident); ok {
			object := identObject(b.index.pass, id)
			// Object identity only denotes the current range position while the
			// range variable is immutable in this body. Scalar helper results,
			// aliases and compound assignments must not rebind that position.
			if b.loop > 0 && (object == b.loopIndex || object == b.outerIndex || object == b.laneIndex) {
				b.summary.valid = false
				return
			}
			if _, ok := b.scalars[object]; !ok {
				b.summary.valid = false
				return
			}
			outer, lane := b.markers(s.Rhs[0], false), b.markers(s.Rhs[0], true)
			updateDeps := deps
			suppressed := b.erases(s.Rhs[0])
			additive := !suppressed && (s.Tok == token.ADD_ASSIGN || s.Tok == token.SUB_ASSIGN || s.Tok == token.ASSIGN && b.additiveRecurrence(s.Rhs[0], object))
			if b.byteConsumer != nil && b.loop > 0 && b.scalarLoops[object] < b.loop && !additive {
				b.summary.valid = false
				return
			}
			if s.Tok != token.ASSIGN {
				deps = ps6141MergeDeps(b.scalars[object], deps)
				outer = ps6141MergeDeps(b.contributions[object], outer)
				lane = ps6141MergeDeps(b.laneContributions[object], lane)
				suppressed = suppressed || b.suppressedValues[object]
			}
			if (s.Tok == token.MUL_ASSIGN || s.Tok == token.AND_ASSIGN) && b.zero(s.Rhs[0]) {
				suppressed = true
			}
			if suppressed {
				outer = nil
				lane = nil
			}
			b.suppressedValues[object] = suppressed
			b.scalars[object] = deps
			if additive && b.loop > 0 && b.scalarLoops[object] == 0 {
				// Symbolic-map domain: contribution identities include sparse protocol namespaces, not loop positions
				for r := range updateDeps {
					if r > 0 {
						b.summary.reductions[r] = min(2, b.summary.reductions[r]+1) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
						outer = ps6141MergeDeps(outer, ps6141Deps{r: true})
					}
				}
			}
			if additive && b.loop == 2 && b.scalarLoops[object] < 2 {
				// Symbolic-map domain: lane contribution identities include sparse protocol namespaces, not lane indexes
				for r := range updateDeps {
					if r > 0 {
						b.summary.laneReductions[r] = min(2, b.summary.laneReductions[r]+1) //perfscan:ignore PS3003 symbolic invocation roots include negative fresh identities and sparse 1000/2000/3000 protocol namespaces, not traversal indexes
						lane = ps6141MergeDeps(lane, ps6141Deps{r: true})
					}
				}
			}
			b.contributions[object] = outer
			b.laneContributions[object] = lane
			return
		}
		root := b.root(s.Lhs[0])
		if root == 0 {
			b.summary.valid = false
			return
		}
		// Validate lvalue paths and index arithmetic without requiring a read of
		// the destination parameter (which a block-filling helper only writes).
		valid := true
		ast.Inspect(s.Lhs[0], func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.IndexExpr:
				b.expression(x.Index)
			case *ast.UnaryExpr:
				valid = false
			case *ast.CallExpr:
				valid = false
			}
			return true
		})
		if !valid || b.laneIndex != nil && !b.path(s.Lhs[0]) {
			b.summary.valid = false
			return
		}
		b.summary.writes[root] = ps6141MergeDeps(b.summary.writes[root], deps)
	case *ast.ExprStmt:
		if call, ok := s.X.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && b.index.pass.TypesInfo.Uses[id] == types.Universe.Lookup("panic") {
				b.expression(call)
				b.summary.valid = false // Unconditional panic has no composed completion.
				return
			}
			result := b.call(call)
			if result.result != 0 {
				b.summary.valid = false
			}
			return
		}
		b.summary.valid = false
	case *ast.ReturnStmt:
		if b.loop != 0 || b.returned || len(s.Results) > 1 {
			b.summary.valid = false
			return
		}
		b.returned = true
		if len(s.Results) == 0 {
			return
		}
		expr := s.Results[0]
		if id, ok := expr.(*ast.Ident); ok && b.roots[identObject(b.index.pass, id)] != 0 {
			b.summary.result = b.root(expr)
			return
		}
		if call, ok := expr.(*ast.CallExpr); ok && !b.index.pass.TypesInfo.Types[call.Fun].IsType() {
			result := b.call(call)
			b.summary.result = result.result
			b.summary.resultDeps = result.resultDeps
			b.summary.resultReductions = result.resultReductions
			b.summary.resultLaneReductions = result.resultLaneReductions
			b.summary.resultSuppressed = result.resultSuppressed
			b.summary.byteConsumerStride = result.byteConsumerStride
			b.summary.byteConsumerInputs = result.byteConsumerInputs
			return
		}
		b.summary.resultDeps = b.expression(expr)
		b.summary.resultReductions = b.markers(expr, false)
		b.summary.resultLaneReductions = b.markers(expr, true)
		b.summary.resultSuppressed = b.erases(expr)
	case *ast.RangeStmt:
		if b.byteConsumer != nil && (s == b.byteConsumer.outer || s == b.byteConsumer.lane) {
			b.byteRange(s)
			return
		}
		if b.loop == 1 {
			b.lanes(s)
			return
		}
		root := b.root(s.X)
		_, direct := ps2110Unparen(s.X).(*ast.Ident)
		_, slice := types.Unalias(b.index.pass.TypesInfo.TypeOf(s.X)).Underlying().(*types.Slice)
		pathOK := b.path(s.X)
		if root == 0 || !pathOK || !direct || !slice || b.loop != 0 || s.Value != nil || s.Tok != token.DEFINE {
			b.summary.valid = false
			return
		}
		id, ok := s.Key.(*ast.Ident)
		if !ok || id.Name == "_" {
			b.summary.valid = false
			return
		}
		b.scalars[b.index.pass.TypesInfo.Defs[id]] = ps6141Deps{}
		b.summary.traversals[root] = min(2, b.summary.traversals[root]+1)
		b.traversing[root] = true
		b.loop++
		b.loopIndex = b.index.pass.TypesInfo.Defs[id]
		b.outerIndex = b.loopIndex
		b.outerRoot = root
		b.block(s.Body)
		b.loop--
		b.loopIndex = nil
		b.outerIndex = nil
		b.outerRoot = 0
		delete(b.traversing, root)
	case *ast.IfStmt:
		// Only a terminating panic guard: no state joins or alternate origins.
		if s.Init != nil || s.Else != nil || len(s.Body.List) != 1 {
			b.summary.valid = false
			return
		}
		stmt, ok := s.Body.List[0].(*ast.ExprStmt)
		if !ok {
			b.summary.valid = false
			return
		}
		call, ok := stmt.X.(*ast.CallExpr)
		if !ok {
			b.summary.valid = false
			return
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok || b.index.pass.TypesInfo.Uses[id] != types.Universe.Lookup("panic") {
			b.summary.valid = false
			return
		}
		b.expression(s.Cond)
		if value := b.index.pass.TypesInfo.Types[s.Cond].Value; value != nil && value.Kind() == constant.Bool {
			if constant.BoolVal(value) {
				b.summary.valid = false
			}
			return
		}
		b.expression(call)
		b.recordBounds(s.Cond)
	default:
		b.summary.valid = false
	}
}
