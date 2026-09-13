package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// A partial producer-layout proof, deliberately NOT boundary admission. Affine
// intervals are conditional on non-overflow allocation arithmetic. Scalar
// effects/completion and consumer multiplicity remain separate unknowns.
// readOnlyViews covers direct source-visible storage paths only: unknown scalar
// callees could still mutate independently aliased globals. It is not a whole
// function purity or feasibility assertion.
type ps6141ByteBlockLayout struct {
	laneFillUnconditional                                          bool
	laneWriteRHS                                                   ast.Expr
	laneValueObject                                                types.Object
	storageExpressions                                             map[ast.Node]bool
	storageObjects                                                 map[types.Object]bool
	finiteRanges                                                   map[*ast.RangeStmt]bool
	storageCalls                                                   map[*ast.CallExpr]bool
	established                                                    bool
	floatExtent, byteStride, metadataWidth                         int64
	freshOutput, readOnlyViews                                     bool
	allocationArithmeticKnown, scalarEffectsKnown, completionKnown bool
}

// Object identities and typed layouts, not a configured codec function name,
// associate the owner allocation, current block view and current lane writes.
func ps6141ByteBlocks(pass *analysis.Pass, fn *ast.FuncDecl) ps6141ByteBlockLayout {
	unknown := ps6141ByteBlockLayout{}
	if pass == nil || fn == nil || fn.Body == nil || len(fn.Body.List) != 4 {
		return unknown
	}
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.Contains(comment.Text, "go:linkname") {
					return unknown
				}
			}
		}
	}
	object, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if object == nil {
		return unknown
	}
	sig := object.Type().(*types.Signature)
	if sig.Recv() != nil || sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 1 || sig.Results().Len() != 1 {
		return unknown
	}
	input, ok := types.Unalias(sig.Params().At(0).Type()).Underlying().(*types.Slice)
	output, okOutput := types.Unalias(sig.Results().At(0).Type()).Underlying().(*types.Slice)
	if !ok || !okOutput || !types.Identical(input.Elem(), types.Typ[types.Float32]) || !types.Identical(output.Elem(), types.Typ[types.Uint8]) {
		return unknown
	}
	define := func(stmt ast.Stmt) (types.Object, ast.Expr) {
		s, ok := stmt.(*ast.AssignStmt)
		if !ok || s.Tok != token.DEFINE || len(s.Lhs) != 1 || len(s.Rhs) != 1 {
			return nil, nil
		}
		id, ok := s.Lhs[0].(*ast.Ident)
		if !ok {
			return nil, nil
		}
		return pass.TypesInfo.Defs[id], s.Rhs[0]
	}
	same := func(expr ast.Expr, obj types.Object) bool {
		id, ok := ps2110Unparen(expr).(*ast.Ident)
		return ok && identObject(pass, id) == obj
	}
	integer := func(expr ast.Expr) int64 {
		if expr == nil {
			return 0
		}
		v := pass.TypesInfo.Types[expr].Value
		if v == nil || v.Kind() != constant.Int {
			return 0
		}
		n, ok := constant.Int64Val(v)
		if !ok || n <= 0 || n > 1<<20 {
			return 0
		}
		return n
	}
	count, countExpr := define(fn.Body.List[0])
	division, ok := countExpr.(*ast.BinaryExpr)
	if count == nil || !ok || division.Op != token.QUO {
		return unknown
	}
	length, ok := division.X.(*ast.CallExpr)
	if !ok || len(length.Args) != 1 || !same(length.Args[0], sig.Params().At(0)) {
		return unknown
	}
	lengthID, ok := length.Fun.(*ast.Ident)
	if !ok {
		return unknown
	}
	builtin, ok := pass.TypesInfo.Uses[lengthID].(*types.Builtin)
	if !ok || builtin.Name() != "len" {
		return unknown
	}
	extent := integer(division.Y)
	if extent == 0 {
		return unknown
	}
	out, allocationExpr := define(fn.Body.List[1])
	allocation, ok := allocationExpr.(*ast.CallExpr)
	if out == nil || !ok || len(allocation.Args) != 2 || allocation.Ellipsis.IsValid() || !types.Identical(pass.TypesInfo.TypeOf(allocation), sig.Results().At(0).Type()) {
		return unknown
	}
	makeID, ok := allocation.Fun.(*ast.Ident)
	if !ok {
		return unknown
	}
	makeBuiltin, ok := pass.TypesInfo.Uses[makeID].(*types.Builtin)
	if !ok || makeBuiltin.Name() != "make" {
		return unknown
	}
	product, ok := allocation.Args[1].(*ast.BinaryExpr)
	if !ok || product.Op != token.MUL || !same(product.X, count) {
		return unknown
	}
	stride := integer(product.Y)
	if stride == 0 {
		return unknown
	}
	outer, ok := fn.Body.List[2].(*ast.RangeStmt)
	if !ok || outer.Tok != token.DEFINE || outer.Value != nil || !same(outer.X, count) {
		return unknown
	}
	blockID, ok := outer.Key.(*ast.Ident)
	if !ok {
		return unknown
	}
	block := pass.TypesInfo.Defs[blockID]
	ret, ok := fn.Body.List[3].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 || !same(ret.Results[0], out) {
		return unknown
	}
	allowed := map[*ast.Ident]bool{}
	storageExpressions := map[ast.Node]bool{length: true, allocation: true}
	finiteRanges := map[*ast.RangeStmt]bool{outer: true}
	storageCalls := map[*ast.CallExpr]bool{}
	mark := func(expr ast.Expr) {
		ast.Inspect(expr, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				allowed[id] = true
			}
			return true
		})
	}
	mark(length.Args[0])
	mark(product.X)
	mark(outer.X)
	mark(ret.Results[0])
	affine := func(expr ast.Expr, obj types.Object, factor int64) bool {
		p, ok := ps2110Unparen(expr).(*ast.BinaryExpr)
		return ok && p.Op == token.MUL && same(p.X, obj) && integer(p.Y) == factor
	}
	var view, offset types.Object
	var laneIndexes []types.Object
	metadata, lanes := 0, 0
	var laneWriteRHS ast.Expr
	var laneValueObject types.Object
	laneFillUnconditional := false
	valid := true
	for _, stmt := range outer.Body.List {
		obj, expr := define(stmt)
		if slice, ok := expr.(*ast.SliceExpr); ok {
			if view != nil || slice.Slice3 || !same(slice.X, sig.Params().At(0)) || !affine(slice.Low, block, extent) {
				return unknown
			}
			high, ok := ps2110Unparen(slice.High).(*ast.BinaryExpr)
			if !ok || high.Op != token.MUL || integer(high.Y) != extent {
				return unknown
			}
			plus, ok := ps2110Unparen(high.X).(*ast.BinaryExpr)
			if !ok || plus.Op != token.ADD || !same(plus.X, block) || !ps6141ConstantInt(pass, plus.Y, 1) {
				return unknown
			}
			view = obj
			storageExpressions[slice] = true
			mark(slice)
			continue
		}
		if obj != nil && affine(expr, block, stride) {
			if offset != nil {
				return unknown
			}
			offset = obj
			mark(expr)
			continue
		}
		if loop, ok := stmt.(*ast.RangeStmt); ok {
			if view == nil || !same(loop.X, view) || loop.Tok != token.DEFINE {
				return unknown
			}
			mark(loop.X)
			finiteRanges[loop] = true
			if key, ok := loop.Key.(*ast.Ident); ok && key.Name == "_" {
				storageExpressions[key] = true
			}
			var lane types.Object
			if key, ok := loop.Key.(*ast.Ident); ok && key.Name != "_" {
				lane = pass.TypesInfo.Defs[key]
				laneIndexes = append(laneIndexes, lane)
			}
			ast.Inspect(loop.Body, func(n ast.Node) bool {
				if _, nested := n.(*ast.RangeStmt); nested {
					valid = false
					return false
				}
				if _, nested := n.(*ast.ForStmt); nested {
					valid = false
					return false
				}
				assignment, ok := n.(*ast.AssignStmt)
				if !ok {
					return true
				}
				for _, lhs := range assignment.Lhs {
					indexed, ok := lhs.(*ast.IndexExpr)
					if !ok || !same(indexed.X, out) {
						continue
					}
					if lane == nil || offset == nil || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
						valid = false
						continue
					}
					index, ok := ps2110Unparen(indexed.Index).(*ast.BinaryExpr)
					if !ok || index.Op != token.ADD || !same(index.Y, lane) {
						valid = false
						continue
					}
					prefix, ok := ps2110Unparen(index.X).(*ast.BinaryExpr)
					if !ok || prefix.Op != token.ADD || !same(prefix.X, offset) || !ps6141ConstantInt(pass, prefix.Y, 2) || stride != extent+2 {
						valid = false
						continue
					}
					lanes++
					laneWriteRHS = assignment.Rhs[0]
					for _, direct := range loop.Body.List {
						if direct == assignment {
							laneFillUnconditional = true
						}
					}
					if value, ok := loop.Value.(*ast.Ident); ok {
						laneValueObject = pass.TypesInfo.Defs[value]
					}
					storageExpressions[indexed] = true
					mark(indexed)
				}
				return true
			})
			continue
		}
		if effect, ok := stmt.(*ast.ExprStmt); ok {
			call, ok := effect.X.(*ast.CallExpr)
			if !ok || offset == nil || len(call.Args) != 2 || call.Ellipsis.IsValid() {
				continue
			}
			selection, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				continue
			}
			method := pass.TypesInfo.Selections[selection]
			if method == nil || method.Kind() != types.MethodVal {
				continue
			}
			typedMethod, _ := method.Obj().(*types.Func)
			// SDK go1.26 encoding/binary/binary.go:75: exact littleEndian method
			// writes b[0:2], no retaining storage or callback. Do not trust user methods
			// merely named PutUint16 or mutable ByteOrder interface receivers.
			receiver, ok := selection.X.(*ast.SelectorExpr)
			if !ok || typedMethod == nil || typedMethod.Pkg() == nil || typedMethod.Pkg().Path() != "encoding/binary" || typedMethod.Name() != "PutUint16" {
				continue
			}
			receiverObj := pass.TypesInfo.Uses[receiver.Sel]
			if receiverObj == nil || receiverObj.Pkg() == nil || receiverObj.Pkg().Path() != "encoding/binary" || receiverObj.Name() != "LittleEndian" {
				continue
			}
			importedIdentity := false
			if pass.Pkg != nil && receiverObj.Pkg() != pass.Pkg {
				for _, imported := range pass.Pkg.Imports() {
					if imported == receiverObj.Pkg() && imported.Scope().Lookup("LittleEndian") == receiverObj {
						importedIdentity = true
					}
				}
			}
			if !importedIdentity {
				continue
			}
			bytes, ok := call.Args[0].(*ast.SliceExpr)
			if !ok || bytes.Slice3 || bytes.High != nil || !same(bytes.X, out) || !same(bytes.Low, offset) || stride < 2 {
				return unknown
			}
			metadata++
			storageExpressions[bytes] = true
			storageExpressions[call.Fun] = true
			storageCalls[call] = true
			mark(bytes)
		}
	}
	if !valid || view == nil || offset == nil || metadata != 1 || lanes != 1 {
		return unknown
	}
	protected := map[types.Object]bool{sig.Params().At(0): true, out: true, view: true, count: true, block: true, offset: true}
	for _, lane := range laneIndexes {
		protected[lane] = true
	}
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			valid = false
			return false
		}
		if id, ok := n.(*ast.Ident); ok && pass.TypesInfo.Uses[id] != nil && protected[pass.TypesInfo.Uses[id]] && !allowed[id] {
			valid = false
		}
		return true
	})
	if !valid {
		return unknown
	}
	return ps6141ByteBlockLayout{established: true, floatExtent: extent, byteStride: stride, metadataWidth: 2, freshOutput: true, readOnlyViews: true, storageExpressions: storageExpressions, storageObjects: map[types.Object]bool{sig.Params().At(0): true, out: true, view: true}, finiteRanges: finiteRanges, storageCalls: storageCalls, laneWriteRHS: laneWriteRHS, laneValueObject: laneValueObject, laneFillUnconditional: laneFillUnconditional}
}
