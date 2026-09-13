package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Composed producer facts, not a qualifying boundary. Finite body completion
// is conditional on successful allocation and valid affine storage bounds.
// Those runtime obligations are not discharged by a scalar helper proof.
type ps6141ByteProducerProof struct {
	layout                                         ps6141ByteBlockLayout
	bodyEffectsKnown, finiteBodyKnown, boundsKnown bool
	floatViewBoundsKnown                           bool
	laneSourceOriginKnown                          bool
}

func ps6141ByteProducer(pass *analysis.Pass, fn *types.Func) ps6141ByteProducerProof {
	if pass == nil || fn == nil {
		return ps6141ByteProducerProof{}
	}
	declarations := ps6099LocalFunctionDeclarations(pass)
	decl := declarations[fn.Origin()]
	layout := ps6141ByteBlocks(pass, decl)
	proof := ps6141ByteProducerProof{layout: layout}
	if !layout.established {
		return proof
	}
	// count=floor(len(x)/extent), 0<=block<count. Thus
	// (block+1)*extent<=len(x), including intermediate positive products;
	// neither endpoint overflows int. The byte-stride product is different.
	proof.floatViewBoundsKnown = true
	index := &ps6141ScalarEffectIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141ScalarEffect{}, active: map[*types.Func]bool{fn.Origin(): true}, remaining: 20000}
	summary := index.body(decl, &layout)
	proof.bodyEffectsKnown = summary.effectsKnown
	proof.finiteBodyKnown = summary.completionKnown
	if proof.bodyEffectsKnown && layout.laneValueObject != nil {
		immutable := true
		ast.Inspect(decl.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == layout.laneValueObject {
						immutable = false
					}
				}
			case *ast.IncDecStmt:
				if id, ok := node.X.(*ast.Ident); ok && pass.TypesInfo.Uses[id] == layout.laneValueObject {
					immutable = false
				}
			}
			return true
		})
		// Trace the actual view-lane value through source helpers, with normal
		// argument substitution. Pure/effect-free does not imply returned use.
		flow := &ps6141SummaryIndex{pass: pass, declarations: declarations, memo: map[*types.Func]ps6141SourceSummary{}, active: map[*types.Func]bool{}, remaining: 20000}
		body := &ps6141SummaryBody{index: flow, summary: ps6141SourceSummary{valid: true}, roots: map[types.Object]ps6141Root{}, scalars: map[types.Object]ps6141Deps{}, contributions: map[types.Object]ps6141Deps{}, laneContributions: map[types.Object]ps6141Deps{}, callResults: map[*ast.CallExpr]ps6141SourceSummary{}, suppressedValues: map[types.Object]bool{}}
		for _, object := range pass.TypesInfo.Defs {
			if object != nil && object.Pos() >= decl.Pos() && object.Pos() < decl.End() && ps6141NumericScalar(object.Type()) {
				body.scalars[object] = nil
			}
		}
		body.scalars[layout.laneValueObject] = ps6141Deps{1: true}
		deps := body.expression(layout.laneWriteRHS)
		proof.laneSourceOriginKnown = layout.laneFillUnconditional && immutable && body.summary.valid && deps[1] && !body.erases(layout.laneWriteRHS)
	}
	// Allocation overflow and actual output bounds remain unknown.
	return proof
}

// An adjacent terminating guard dominates this producer call and bounds the
// exact incoming float argument's block count. No guard aliases, multiplication
// overflow, opaque condition effects, scalar rebinding or unknown target sizes.
func ps6141CheckedByteOwner(pass *analysis.Pass, owner *types.Func, body *ast.BlockStmt, statements []ast.Stmt, at int, call *ast.CallExpr, proof *ps6141ByteProducerProof) bool {
	if proof == nil || !proof.bodyEffectsKnown || !proof.finiteBodyKnown || !proof.floatViewBoundsKnown || !proof.laneSourceOriginKnown || pass.TypesSizes == nil || at == 0 || len(call.Args) != 1 {
		return false
	}
	structured := true
	ast.Inspect(body, func(n ast.Node) bool {
		if _, label := n.(*ast.LabeledStmt); label {
			structured = false
		}
		if branch, ok := n.(*ast.BranchStmt); ok && branch.Tok == token.GOTO {
			structured = false
		}
		return true
	})
	if !structured {
		return false
	}
	input, ok := call.Args[0].(*ast.Ident)
	if !ok {
		return false
	}
	object := pass.TypesInfo.Uses[input]
	if !ps6141OwnerFormal(owner, object) || ps6141ObjectUses(pass, body, object) != 2 {
		return false
	}
	guard, ok := statements[at-1].(*ast.IfStmt)
	if !ok || guard.Init != nil || guard.Else != nil || len(guard.Body.List) != 1 {
		return false
	}
	comparison, ok := ps2110Unparen(guard.Cond).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.GTR && comparison.Op != token.NEQ {
		return false
	}
	lengthExpr := comparison.X
	if comparison.Op == token.GTR {
		division, ok := ps2110Unparen(comparison.X).(*ast.BinaryExpr)
		if !ok || division.Op != token.QUO || !ps6141ConstantInt(pass, division.Y, proof.layout.floatExtent) {
			return false
		}
		lengthExpr = division.X
	}
	length, ok := ps2110Unparen(lengthExpr).(*ast.CallExpr)
	if !ok || len(length.Args) != 1 || length.Ellipsis.IsValid() {
		return false
	}
	fn, ok := length.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[fn].(*types.Builtin)
	if !ok || builtin.Name() != "len" {
		return false
	}
	arg, ok := length.Args[0].(*ast.Ident)
	if !ok || pass.TypesInfo.Uses[arg] != object {
		return false
	}
	limit := pass.TypesInfo.Types[comparison.Y].Value
	width := pass.TypesSizes.Sizeof(types.Typ[types.Int])
	if width != 4 && width != 8 {
		return false
	}
	if limit == nil || limit.Kind() != constant.Int || constant.Sign(limit) <= 0 {
		return false
	}
	if comparison.Op == token.NEQ {
		extent := constant.MakeInt64(proof.layout.floatExtent)
		if constant.Sign(constant.BinaryOp(limit, token.REM, extent)) != 0 {
			return false
		}
		limit = constant.BinaryOp(limit, token.QUO, extent)
	}
	maximum := constant.BinaryOp(constant.Shift(constant.MakeInt64(1), token.SHL, uint(width*8-1)), token.SUB, constant.MakeInt64(1))
	maximum = constant.BinaryOp(maximum, token.QUO, constant.MakeInt64(proof.layout.byteStride))
	if constant.Compare(limit, token.GTR, maximum) {
		return false
	}
	effect, ok := guard.Body.List[0].(*ast.ExprStmt)
	if !ok {
		return false
	}
	panicCall, ok := effect.X.(*ast.CallExpr)
	if !ok || len(panicCall.Args) != 1 || panicCall.Ellipsis.IsValid() {
		return false
	}
	panicID, ok := panicCall.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	panicBuiltin, ok := pass.TypesInfo.Uses[panicID].(*types.Builtin)
	// Scalar literal payload only; a recovered panic cannot retain storage.
	return ok && panicBuiltin.Name() == "panic" && pass.TypesInfo.Types[panicCall.Args[0]].Value != nil && ps6141SummaryScalar(pass.TypesInfo.TypeOf(panicCall.Args[0]))
}

// The existing source-summary boundary consumes the composed producer stage.
// Partial facts never set valid: storage/effects alone cannot waive runtime
// arithmetic, completion, or the independently summarized consumer obligations.
func (index *ps6141SummaryIndex) producer(fn *types.Func) ps6141SourceSummary {
	summary := index.function(fn)
	if summary.valid {
		return summary
	}
	proof := ps6141ByteProducer(index.pass, fn)
	if proof.layout.established {
		summary.byteProducer = &proof
	}
	return summary
}
