package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/cfg"
)

// Source-proved admission for the opt-in verification advisory; no rewrite.
type ps6141Candidate struct{ quantizer, consumer *ast.CallExpr }

func ps6141Candidates(pass *analysis.Pass, owner *ast.FuncDecl, contract *config.SingleUseQuantizationContract) []ps6141Candidate {
	if pass == nil || owner == nil || owner.Body == nil || !contract.Valid() {
		return nil
	}
	// Raw compiler directives can change linkage independently of the visible
	// function body. CommentGroup.Text strips directives; inspect raw comments
	// throughout the source partition, including all summarized helper symbols.
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.Contains(comment.Text, "go:linkname") {
					return nil
				}
			}
		}
	}
	ownerObject, _ := pass.TypesInfo.Defs[owner.Name].(*types.Func)
	if ownerObject == nil {
		return nil
	}
	var quantizer, consumer *ast.FuncDecl
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if object != nil && ps6090FunctionID(object) == contract.Quantizer {
				quantizer = fn
			}
			if object != nil && ps6090FunctionID(object) == contract.Consumer {
				consumer = fn
			}
		}
	}
	packedType := ps6141FreshQuantizer(pass, quantizer)
	var checkedByteProducer *ps6141ByteProducerProof
	if contract.ConsumerForm == "sourceSummary" || contract.ConsumerForm == "packedByteDot" {
		packedType, checkedByteProducer = ps6141ComposedBoundary(pass, quantizer, consumer, contract)
		if packedType == nil {
			return nil
		}
	} else {
		if packedType == nil || !ps6141SinglePassConsumer(pass, consumer, packedType, contract) {
			return nil
		}
	}
	live := ps6141ExecutableNodes(pass, owner.Body)
	var destinationExtent int64
	if contract.ProducerForm == "destination" {
		fn, _ := pass.TypesInfo.Defs[quantizer.Name].(*types.Func)
		index := &ps6141SummaryIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
		summary := index.function(fn)
		destinationExtent = summary.completionBounds[ps6141Root(contract.FloatInputArgument+1)][ps6141Root(contract.DestinationArgument+1)]
		if destinationExtent == 0 {
			return nil
		}
	}
	var result []ps6141Candidate
	ast.Inspect(owner.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		block, ok := node.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i := 0; i+1 < len(block.List); i++ {
			statement := block.List[i]
			for {
				labelled, ok := statement.(*ast.LabeledStmt)
				if !ok {
					break
				}
				statement = labelled.Stmt
			}
			assignment, ok := statement.(*ast.AssignStmt)
			if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 || !slices.Contains(live, ast.Node(assignment)) {
				continue
			}
			id, ok := assignment.Lhs[0].(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			object := pass.TypesInfo.Defs[id]
			quant, ok := assignment.Rhs[0].(*ast.CallExpr)
			nextIndex, wantedUses := i+1, 1
			if contract.ProducerForm == "destination" {
				if !ok || i+2 >= len(block.List) {
					continue
				}
				allocation := quant
				fill, direct := block.List[i+1].(*ast.ExprStmt)
				if !direct || !slices.Contains(live, ast.Node(fill)) {
					continue
				}
				quant, ok = fill.X.(*ast.CallExpr)
				if !ok || len(quant.Args) != 2 {
					continue
				}
				dst, direct := quant.Args[contract.DestinationArgument].(*ast.Ident)
				if !direct || pass.TypesInfo.Uses[dst] != object {
					continue
				}
				input, direct := quant.Args[contract.FloatInputArgument].(*ast.Ident)
				if !direct || !ps6141OwnerFormal(ownerObject, pass.TypesInfo.Uses[input]) {
					continue
				}
				if !ps6141OwnerMake(pass, allocation, packedType, pass.TypesInfo.Uses[input], destinationExtent) {
					continue
				}
				nextIndex, wantedUses = i+2, 2
			} else if !ok || len(quant.Args) != 1 {
				continue
			}
			if ps6087FunctionID(pass, quant) != contract.Quantizer || quant.Ellipsis.IsValid() {
				continue
			}
			if checkedByteProducer != nil && !ps6141CheckedByteOwner(pass, ownerObject, owner.Body, block.List, i, quant, checkedByteProducer) {
				continue
			}
			var consume *ast.CallExpr
			switch next := block.List[nextIndex].(type) {
			case *ast.ReturnStmt:
				if len(next.Results) == 1 {
					consume, _ = next.Results[0].(*ast.CallExpr)
				}
			case *ast.ExprStmt:
				consume, _ = next.X.(*ast.CallExpr)
			}
			if consume == nil || !slices.Contains(live, ast.Node(block.List[nextIndex])) || ps6087FunctionID(pass, consume) != contract.Consumer || len(consume.Args) != 2 || consume.Ellipsis.IsValid() {
				continue
			}
			_, signature, typed := typedCallee(pass, consume.Fun)
			if !typed || signature.Recv() != nil || signature.Variadic() || signature.TypeParams().Len() != 0 || signature.Params().Len() != 2 ||
				!types.Identical(signature.Params().At(contract.PackedArgument).Type(), packedType) {
				continue
			}
			packed, ok := consume.Args[contract.PackedArgument].(*ast.Ident)
			if !ok || pass.TypesInfo.Uses[packed] != object || !types.Identical(pass.TypesInfo.TypeOf(packed), packedType) {
				continue
			}
			if contract.ConsumerForm == "twoInputDot" || contract.ConsumerForm == "sourceSummary" || contract.ConsumerForm == "packedByteDot" {
				weight, direct := consume.Args[contract.WeightArgument].(*ast.Ident)
				if !direct || !types.Identical(pass.TypesInfo.TypeOf(weight), packedType) {
					continue
				}
				// Restrict to a distinct incoming integer slice, with no alias or
				// rebinding/use elsewhere in this invocation's owner body.
				weightObject := pass.TypesInfo.Uses[weight]
				ownerSignature := ownerObject.Type().(*types.Signature)
				formal := false
				for j := 0; j < ownerSignature.Params().Len(); j++ {
					formal = formal || ownerSignature.Params().At(j) == weightObject
				}
				if !formal || weightObject == object || ps6141ObjectUses(pass, owner.Body, weightObject) != 1 {
					continue
				}
			} else {
				if !types.Identical(signature.Params().At(contract.RowsArgument).Type(), types.Typ[types.Int]) || !ps6141ConstantInt(pass, consume.Args[contract.RowsArgument], 1) {
					continue
				}
			}
			if ps6141ObjectUses(pass, owner.Body, object) == wantedUses {
				candidate := ps6141Candidate{quant, consume}
				var guard ast.Stmt
				if i > 0 {
					guard = block.List[i-1]
				}
				if !ps6141Exempt(pass, owner, candidate, contract, guard) {
					result = append(result, candidate)
				}
			}
		}
		return true
	})
	return result
}

func ps6141OwnerFormal(owner *types.Func, object types.Object) bool {
	sig := owner.Type().(*types.Signature)
	for i := 0; i < sig.Params().Len(); i++ {
		if sig.Params().At(i) == object {
			return true
		}
	}
	return false
}

// Allocation geometry is deliberately bounded and effect-free. In particular,
// an opaque size call is not hidden behind the freshness of make itself.
func ps6141OwnerMake(pass *analysis.Pass, call *ast.CallExpr, packed types.Type, input types.Object, extent int64) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || len(call.Args) != 2 || call.Ellipsis.IsValid() || !types.Identical(pass.TypesInfo.TypeOf(call), packed) {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || builtin.Name() != "make" {
		return false
	}
	expr := ps2110Unparen(call.Args[1])
	if extent != 1 {
		division, ok := expr.(*ast.BinaryExpr)
		if !ok || division.Op != token.QUO || !ps6141ConstantInt(pass, division.Y, extent) {
			return false
		}
		expr = ps2110Unparen(division.X)
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		fn, ok := e.Fun.(*ast.Ident)
		if !ok || len(e.Args) != 1 || e.Ellipsis.IsValid() {
			return false
		}
		b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin)
		arg, direct := e.Args[0].(*ast.Ident)
		return ok && b.Name() == "len" && direct && pass.TypesInfo.Uses[arg] == input
	}
	return false
}

func ps6141ObjectUses(pass *analysis.Pass, body *ast.BlockStmt, object types.Object) int {
	uses := 0
	ast.Inspect(body, func(node ast.Node) bool {
		if use, ok := node.(*ast.Ident); ok && pass.TypesInfo.Uses[use] == object {
			uses++
		}
		return true
	})
	return uses
}

// A complete canonical fresh-storage function: make(len(float input)), one range
// conversion assignment per element, return the allocation. No opaque factory,
// cache, alias, closure, errors, rebinding or hidden uses are admitted.
func ps6141FreshQuantizer(pass *analysis.Pass, fn *ast.FuncDecl) types.Type {
	if fn == nil || fn.Recv != nil || fn.Body == nil || len(fn.Body.List) != 3 {
		return nil
	}
	object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if object == nil {
		return nil
	}
	sig := object.Type().(*types.Signature)
	if sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return nil
	}
	input, ok := sig.Params().At(0).Type().(*types.Slice)
	if !ok || !(types.Identical(input.Elem(), types.Typ[types.Float32]) || types.Identical(input.Elem(), types.Typ[types.Float64])) {
		return nil
	}
	output, ok := sig.Results().At(0).Type().(*types.Slice)
	if !ok || !(types.Identical(output.Elem(), types.Typ[types.Int8]) || types.Identical(output.Elem(), types.Typ[types.Uint8])) {
		return nil
	}
	first, ok := fn.Body.List[0].(*ast.AssignStmt)
	if !ok || first.Tok != token.DEFINE || len(first.Lhs) != 1 || len(first.Rhs) != 1 {
		return nil
	}
	id, ok := first.Lhs[0].(*ast.Ident)
	if !ok {
		return nil
	}
	storage := pass.TypesInfo.Defs[id]
	makeCall, ok := first.Rhs[0].(*ast.CallExpr)
	if !ok || len(makeCall.Args) != 2 || makeCall.Ellipsis.IsValid() {
		return nil
	}
	makeID, ok := makeCall.Fun.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[makeID] != types.Universe.Lookup("make") || !types.Identical(pass.TypesInfo.TypeOf(makeCall), output) {
		return nil
	}
	length, ok := makeCall.Args[1].(*ast.CallExpr)
	if !ok || len(length.Args) != 1 || length.Ellipsis.IsValid() {
		return nil
	}
	lenID, ok := length.Fun.(*ast.Ident)
	arg, direct := length.Args[0].(*ast.Ident)
	if !ok || !direct || pass.TypesInfo.Uses[lenID] != types.Universe.Lookup("len") || pass.TypesInfo.Uses[arg] != sig.Params().At(0) {
		return nil
	}
	loop, ok := fn.Body.List[1].(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || loop.Value != nil || len(loop.Body.List) != 1 {
		return nil
	}
	source, ok := loop.X.(*ast.Ident)
	index, key := loop.Key.(*ast.Ident)
	if !ok || !key || pass.TypesInfo.Uses[source] != sig.Params().At(0) {
		return nil
	}
	write, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || write.Tok != token.ASSIGN || len(write.Lhs) != 1 || len(write.Rhs) != 1 {
		return nil
	}
	lhs, ok := write.Lhs[0].(*ast.IndexExpr)
	if !ok || !ps6141Index(pass, lhs, storage, pass.TypesInfo.Defs[index]) {
		return nil
	}
	conversion, ok := write.Rhs[0].(*ast.CallExpr)
	if !ok || len(conversion.Args) != 1 || conversion.Ellipsis.IsValid() || !pass.TypesInfo.Types[conversion.Fun].IsType() || !types.Identical(pass.TypesInfo.TypeOf(conversion), output.Elem()) {
		return nil
	}
	rhs, ok := conversion.Args[0].(*ast.IndexExpr)
	if !ok || !ps6141Index(pass, rhs, sig.Params().At(0), pass.TypesInfo.Defs[index]) {
		return nil
	}
	returned, ok := fn.Body.List[2].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return nil
	}
	ret, ok := returned.Results[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[ret] != storage {
		return nil
	}
	return output
}

func ps6141Index(pass *analysis.Pass, index *ast.IndexExpr, source, induction types.Object) bool {
	x, ok := index.X.(*ast.Ident)
	i, direct := index.Index.(*ast.Ident)
	return ok && direct && pass.TypesInfo.Uses[x] == source && pass.TypesInfo.Uses[i] == induction
}

// Initial source-backed subset: one full integer self-dot traversal guarded by
// rows==1. The complete body grammar excludes retention, nested dispatch,
// repeated traversals and internally amplified or ignored row geometry. This
// is not a summary of arbitrary quantized matmul or imported native kernels.
func ps6141SinglePassConsumer(pass *analysis.Pass, fn *ast.FuncDecl, packedType types.Type, c *config.SingleUseQuantizationContract) bool {
	if c.ConsumerForm == "twoInputDot" {
		return ps6141TwoInputDot(pass, fn, packedType, c)
	}
	if fn == nil || fn.Recv != nil || fn.Body == nil || len(fn.Body.List) != 4 {
		return false
	}
	object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if object == nil {
		return false
	}
	sig := object.Type().(*types.Signature)
	if sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 2 || sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int]) || !types.Identical(sig.Params().At(c.PackedArgument).Type(), packedType) || !types.Identical(sig.Params().At(c.RowsArgument).Type(), types.Typ[types.Int]) {
		return false
	}
	guard, ok := fn.Body.List[0].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	condition, ok := guard.Cond.(*ast.BinaryExpr)
	if !ok || condition.Op != token.NEQ {
		return false
	}
	rows, ok := condition.X.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[rows] != sig.Params().At(c.RowsArgument) || !ps6141ConstantInt(pass, condition.Y, 1) {
		return false
	}
	panicStmt, ok := guard.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	panicCall, ok := panicStmt.X.(*ast.CallExpr)
	if !ok || len(panicCall.Args) != 1 || panicCall.Ellipsis.IsValid() {
		return false
	}
	panicID, ok := panicCall.Fun.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[panicID] != types.Universe.Lookup("panic") || pass.TypesInfo.Types[panicCall.Args[0]].Value == nil {
		return false
	}
	initial, ok := fn.Body.List[1].(*ast.AssignStmt)
	if !ok || initial.Tok != token.DEFINE || len(initial.Lhs) != 1 || len(initial.Rhs) != 1 || !ps6141ConstantInt(pass, initial.Rhs[0], 0) {
		return false
	}
	sum, ok := initial.Lhs[0].(*ast.Ident)
	if !ok {
		return false
	}
	sumObject := pass.TypesInfo.Defs[sum]
	loop, ok := fn.Body.List[2].(*ast.RangeStmt)
	if !ok || loop.Tok != token.DEFINE || len(loop.Body.List) != 1 {
		return false
	}
	key, ok := loop.Key.(*ast.Ident)
	if !ok || key.Name != "_" {
		return false
	}
	value, ok := loop.Value.(*ast.Ident)
	if !ok {
		return false
	}
	input, ok := loop.X.(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[input] != sig.Params().At(c.PackedArgument) {
		return false
	}
	update, ok := loop.Body.List[0].(*ast.AssignStmt)
	if !ok || update.Tok != token.ADD_ASSIGN || len(update.Lhs) != 1 || len(update.Rhs) != 1 {
		return false
	}
	target, ok := update.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[target] != sumObject {
		return false
	}
	multiply, ok := update.Rhs[0].(*ast.BinaryExpr)
	if !ok || multiply.Op != token.MUL {
		return false
	}
	converted := func(expr ast.Expr) bool {
		call, ok := expr.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return false
		}
		cast, ok := call.Fun.(*ast.Ident)
		if !ok || pass.TypesInfo.Uses[cast] != types.Universe.Lookup("int") {
			return false
		}
		arg, ok := call.Args[0].(*ast.Ident)
		return ok && pass.TypesInfo.Uses[arg] == pass.TypesInfo.Defs[value]
	}
	if !converted(multiply.X) || !converted(multiply.Y) {
		return false
	}
	returned, ok := fn.Body.List[3].(*ast.ReturnStmt)
	if !ok || len(returned.Results) != 1 {
		return false
	}
	result, ok := returned.Results[0].(*ast.Ident)
	return ok && pass.TypesInfo.Uses[result] == sumObject
}

func ps6141ConstantInt(pass *analysis.Pass, expr ast.Expr, value int64) bool {
	c := pass.TypesInfo.Types[expr].Value
	return c != nil && c.Kind() == constant.Int && constant.Compare(c, token.EQL, constant.MakeInt64(value))
}

func ps6141ExecutableNodes(pass *analysis.Pass, body *ast.BlockStmt) []ast.Node {
	var truth func(ast.Expr) (bool, bool)
	truth = func(expr ast.Expr) (bool, bool) {
		expr = ps2110Unparen(expr)
		if value := pass.TypesInfo.Types[expr].Value; value != nil && value.Kind() == constant.Bool {
			return constant.BoolVal(value), true
		}
		if binary, ok := expr.(*ast.BinaryExpr); ok && (binary.Op == token.LAND || binary.Op == token.LOR) {
			x, kx := truth(binary.X)
			y, ky := truth(binary.Y)
			if binary.Op == token.LAND {
				if kx && !x || ky && !y {
					return false, true
				}
				if kx && ky {
					return x && y, true
				}
			}
			if binary.Op == token.LOR {
				if kx && x || ky && y {
					return true, true
				}
				if kx && ky {
					return x || y, true
				}
			}
		}
		return false, false
	}
	graph := cfg.New(body, func(call *ast.CallExpr) bool { return !ps6079PanicCall(pass, call) })
	seen := make([]bool, len(graph.Blocks))
	queue := []*cfg.Block{graph.Blocks[0]}
	nodes := make([]ast.Node, 0, len(body.List))
	for len(queue) != 0 {
		block := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		if seen[block.Index] {
			continue
		}
		seen[block.Index] = true
		nodes = append(nodes, block.Nodes...)
		successors := block.Succs
		// cfg's true/false edge order is documented. Only prune an exact
		// if/for condition: switch/range/select successor semantics differ.
		if len(successors) == 2 && len(block.Nodes) != 0 {
			var condition ast.Expr
			switch stmt := successors[0].Stmt.(type) {
			case *ast.IfStmt:
				condition = stmt.Cond
			case *ast.ForStmt:
				condition = stmt.Cond
			}
			if condition != nil && block.Nodes[len(block.Nodes)-1] == condition {
				if value, known := truth(condition); known {
					if value {
						successors = successors[:1]
					} else {
						successors = successors[1:]
					}
				}
			}
		}
		queue = append(queue, successors...)
	}
	return nodes
}
