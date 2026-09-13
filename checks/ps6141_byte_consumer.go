package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/analysis"
	"strings"
)

type ps6141ByteConsumerLayout struct {
	stride                               int64
	outer, lane                          *ast.RangeStmt
	root                                 ps6141Root
	slices                               map[ast.Expr]ps6141Root
	views                                map[types.Object]ps6141Root
	indices                              map[*ast.IndexExpr]ps6141Root
	reads                                map[*ast.CallExpr]ps6141Root
	guards                               map[*ast.IfStmt]bool
	storageEffectsKnown, completionKnown bool
}

// Typed source geometry only; numeric protocol meaning remains reviewed. One
// whole packed traversal and current-block lane traversal, never output rows.
func ps6141ByteConsumer(pass *analysis.Pass, decl *ast.FuncDecl) *ps6141ByteConsumerLayout {
	for _, file := range pass.Files {
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if strings.Contains(comment.Text, "go:linkname") {
					return nil
				}
			}
		}
	}
	fn, _ := pass.TypesInfo.Defs[decl.Name].(*types.Func)
	if fn == nil {
		return nil
	}
	sig := fn.Type().(*types.Signature)
	if sig.Recv() != nil || sig.Variadic() || sig.TypeParams().Len() != 0 || sig.Params().Len() != 2 || sig.Results().Len() != 1 || !ps6141NumericScalar(sig.Results().At(0).Type()) {
		return nil
	}
	roots := map[types.Object]ps6141Root{}
	for i := 0; i < 2; i++ {
		param := sig.Params().At(i)
		slice, ok := types.Unalias(param.Type()).Underlying().(*types.Slice)
		if !ok || !types.Identical(slice.Elem(), types.Typ[types.Uint8]) {
			return nil
		}
		roots[param] = ps6141Root(i + 1)
	}
	same := func(e ast.Expr, o types.Object) bool {
		id, ok := ps2110Unparen(e).(*ast.Ident)
		return ok && identObject(pass, id) == o
	}
	lenRoot := func(e ast.Expr) ps6141Root {
		call, ok := ps2110Unparen(e).(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			return 0
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			return 0
		}
		b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
		arg, direct := call.Args[0].(*ast.Ident)
		if !ok || b.Name() != "len" || !direct {
			return 0
		}
		return roots[identObject(pass, arg)]
	}
	terminating := func(g *ast.IfStmt) bool {
		if g.Init != nil || g.Else != nil || len(g.Body.List) != 1 {
			return false
		}
		s, ok := g.Body.List[0].(*ast.ExprStmt)
		if !ok {
			return false
		}
		call, ok := s.X.(*ast.CallExpr)
		if !ok || len(call.Args) != 1 || call.Ellipsis.IsValid() {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		if !ok {
			return false
		}
		b, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
		return ok && b.Name() == "panic" && pass.TypesInfo.Types[call.Args[0]].Value != nil && ps6141SummaryScalar(pass.TypesInfo.TypeOf(call.Args[0]))
	}
	layout := &ps6141ByteConsumerLayout{slices: map[ast.Expr]ps6141Root{}, views: map[types.Object]ps6141Root{}, indices: map[*ast.IndexExpr]ps6141Root{}, reads: map[*ast.CallExpr]ps6141Root{}, guards: map[*ast.IfStmt]bool{}}
	equal, complete := false, false
	for _, stmt := range decl.Body.List {
		if g, ok := stmt.(*ast.IfStmt); ok && terminating(g) {
			if cmp, ok := ps2110Unparen(g.Cond).(*ast.BinaryExpr); ok && cmp.Op == token.NEQ {
				a, b := lenRoot(cmp.X), lenRoot(cmp.Y)
				if a != 0 && b != 0 && a != b {
					equal = true
					layout.guards[g] = true
				}
			}
		}
		loop, ok := stmt.(*ast.RangeStmt)
		if !ok {
			continue
		}
		division, ok := ps2110Unparen(loop.X).(*ast.BinaryExpr)
		if !ok || division.Op != token.QUO {
			continue
		}
		root := lenRoot(division.X)
		v := pass.TypesInfo.Types[division.Y].Value
		if root == 0 || v == nil {
			continue
		}
		if v.Kind() != constant.Int {
			return nil
		}
		stride, exact := constant.Int64Val(v)
		if !exact || stride < 3 || stride > 1<<20 {
			return nil
		}
		if layout.outer != nil || !equal || loop.Value != nil || loop.Tok != token.DEFINE {
			return nil
		}
		layout.outer = loop
		layout.root = root
		layout.stride = stride
		for _, prefix := range decl.Body.List {
			if prefix.Pos() >= loop.Pos() {
				break
			}
			g, ok := prefix.(*ast.IfStmt)
			if !ok || !terminating(g) {
				continue
			}
			cmp, ok := ps2110Unparen(g.Cond).(*ast.BinaryExpr)
			if !ok || cmp.Op != token.NEQ || !ps6141ConstantInt(pass, cmp.Y, 0) {
				continue
			}
			mod, ok := ps2110Unparen(cmp.X).(*ast.BinaryExpr)
			if ok && mod.Op == token.REM && lenRoot(mod.X) == root && ps6141ConstantInt(pass, mod.Y, stride) {
				complete = true
				layout.guards[g] = true
			}
		}
	}
	if layout.outer == nil || !complete {
		return nil
	}
	key, ok := layout.outer.Key.(*ast.Ident)
	if !ok {
		return nil
	}
	block := pass.TypesInfo.Defs[key]
	affine := func(e ast.Expr) bool {
		x, ok := ps2110Unparen(e).(*ast.BinaryExpr)
		return ok && x.Op == token.MUL && same(x.X, block) && ps6141ConstantInt(pass, x.Y, layout.stride)
	}
	var viewCounts [3]int
	for _, stmt := range layout.outer.Body.List {
		assignment, ok := stmt.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			continue
		}
		slice, ok := assignment.Rhs[0].(*ast.SliceExpr)
		if !ok {
			continue
		}
		if slice.Slice3 {
			return nil
		}
		base, ok := slice.X.(*ast.Ident)
		if !ok {
			return nil
		}
		root := roots[identObject(pass, base)]
		if root == 0 || !affine(slice.Low) {
			return nil
		}
		high, ok := ps2110Unparen(slice.High).(*ast.BinaryExpr)
		if !ok || high.Op != token.ADD || !affine(high.X) || !ps6141ConstantInt(pass, high.Y, layout.stride) {
			return nil
		}
		id, ok := assignment.Lhs[0].(*ast.Ident)
		if !ok {
			return nil
		}
		layout.views[pass.TypesInfo.Defs[id]] = root
		layout.slices[slice] = root
		viewCounts[root]++
	}
	if viewCounts[1] != 1 || viewCounts[2] != 1 {
		return nil
	}
	for _, stmt := range layout.outer.Body.List {
		lane, ok := stmt.(*ast.RangeStmt)
		if !ok {
			continue
		}
		slice, ok := lane.X.(*ast.SliceExpr)
		if !ok || slice.Slice3 || slice.High != nil || !ps6141ConstantInt(pass, slice.Low, 2) {
			return nil
		}
		base, ok := slice.X.(*ast.Ident)
		if !ok || layout.views[identObject(pass, base)] != layout.root || lane.Tok != token.DEFINE || lane.Value == nil || layout.lane != nil {
			return nil
		}
		layout.lane = lane
		layout.slices[slice] = layout.root
	}
	if layout.lane == nil {
		return nil
	}
	laneID, ok := layout.lane.Key.(*ast.Ident)
	if !ok || laneID.Name == "_" {
		return nil
	}
	lane := pass.TypesInfo.Defs[laneID]
	if lane == block {
		return nil
	}
	valid := true
	ast.Inspect(layout.outer.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					object := pass.TypesInfo.Uses[id]
					if object != nil && (object == block || object == lane || roots[object] != 0 || layout.views[object] != 0) {
						valid = false
					}
				}
			}
		case *ast.IncDecStmt:
			if same(node.X, block) || same(node.X, lane) {
				valid = false
			}
		case *ast.IndexExpr:
			base, ok := node.X.(*ast.Ident)
			if !ok {
				return true
			}
			root := layout.views[identObject(pass, base)]
			if root == 0 {
				return true
			}
			index, ok := ps2110Unparen(node.Index).(*ast.BinaryExpr)
			if !ok || index.Op != token.ADD || !ps6141ConstantInt(pass, index.X, 2) || !same(index.Y, lane) {
				valid = false
				return true
			}
			layout.indices[node] = root
		case *ast.CallExpr:
			selector, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			selection := pass.TypesInfo.Selections[selector]
			if selection == nil || selection.Kind() != types.MethodVal {
				return true
			}
			method, _ := selection.Obj().(*types.Func)
			receiver, ok := selector.X.(*ast.SelectorExpr)
			if !ok || method == nil || method.Pkg() == nil || method.Pkg().Path() != "encoding/binary" || method.Name() != "Uint16" || len(node.Args) != 1 || node.Ellipsis.IsValid() {
				return true
			}
			object := pass.TypesInfo.Uses[receiver.Sel]
			imported := false
			if object != nil && object.Pkg() != pass.Pkg && object.Name() == "LittleEndian" {
				for _, pkg := range pass.Pkg.Imports() {
					if pkg == object.Pkg() && pkg.Scope().Lookup("LittleEndian") == object {
						imported = true
					}
				}
			}
			if !imported {
				return true
			}
			arg, ok := node.Args[0].(*ast.Ident)
			if !ok {
				return true
			}
			root := layout.views[identObject(pass, arg)]
			if root != 0 {
				layout.reads[node] = root
			}
		}
		return true
	})
	if !valid {
		return nil
	}
	proof := ps6141ByteConsumerEffects(pass, decl, layout, roots, block, lane)
	layout.storageEffectsKnown = proof.effectsKnown
	layout.completionKnown = proof.completionKnown
	return layout
}

// Guard execution and the complete storage-reference census are independent
// prerequisites. Only direct, typed bounded reads receive storage permissions;
// recursively inspected scalar helpers never inherit these permissions.
func ps6141ByteConsumerEffects(pass *analysis.Pass, decl *ast.FuncDecl, layout *ps6141ByteConsumerLayout, roots map[types.Object]ps6141Root, block, lane types.Object) ps6141ScalarEffect {
	storage := &ps6141ByteBlockLayout{storageExpressions: map[ast.Node]bool{}, storageObjects: map[types.Object]bool{}, storageCalls: map[*ast.CallExpr]bool{}, finiteRanges: map[*ast.RangeStmt]bool{layout.outer: true, layout.lane: true}}
	allowed := map[*ast.Ident]bool{}
	permit := func(n ast.Node) {
		storage.storageExpressions[n] = true
		ast.Inspect(n, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				allowed[id] = true
			}
			return true
		})
	}
	for object := range roots {
		storage.storageObjects[object] = true
	}
	for object := range layout.views {
		storage.storageObjects[object] = true
	}
	for expr := range layout.slices {
		permit(expr)
	}
	for expr := range layout.indices {
		permit(expr)
	}
	for call := range layout.reads {
		permit(call)
	}
	valid := true
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.LabeledStmt, *ast.BranchStmt, *ast.FuncLit:
			valid = false
		case *ast.ReturnStmt:
			if node.Pos() < layout.outer.Pos() {
				valid = false
			}
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					object := identObject(pass, id)
					if storage.storageObjects[object] {
						if pass.TypesInfo.Defs[id] != nil && layout.views[object] != 0 {
							allowed[id] = true
						} else {
							valid = false
						}
					}
					if pass.TypesInfo.Uses[id] != nil && (object == block || object == lane) {
						valid = false
					}
				} else {
					valid = false
				}
			}
		case *ast.IncDecStmt:
			if id, ok := node.X.(*ast.Ident); ok {
				object := identObject(pass, id)
				if object == block || object == lane || storage.storageObjects[object] {
					valid = false
				}
			}
		case *ast.CallExpr:
			id, ok := node.Fun.(*ast.Ident)
			if !ok || len(node.Args) != 1 {
				break
			}
			builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
			arg, direct := node.Args[0].(*ast.Ident)
			if ok && builtin.Name() == "len" && direct && roots[identObject(pass, arg)] != 0 {
				permit(node)
			}
		}
		return true
	})
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && storage.storageObjects[identObject(pass, id)] && !allowed[id] {
			valid = false
		}
		return true
	})
	if !valid {
		return ps6141ScalarEffect{}
	}
	// The recognized guards are top-level, precede the traversal, and terminate
	// only the rejected shape path. With branch/label entry and all root mutation
	// denied, the normal path must execute them on the same immutable inputs.
	copyDecl := *decl
	copyBody := *decl.Body
	copyBody.List = nil
	for _, stmt := range decl.Body.List {
		if guard, ok := stmt.(*ast.IfStmt); ok && layout.guards[guard] {
			if guard.Pos() >= layout.outer.Pos() {
				return ps6141ScalarEffect{}
			}
			continue
		}
		copyBody.List = append(copyBody.List, stmt)
	}
	copyDecl.Body = &copyBody
	index := &ps6141ScalarEffectIndex{pass: pass, declarations: ps6099LocalFunctionDeclarations(pass), memo: map[*types.Func]ps6141ScalarEffect{}, active: map[*types.Func]bool{}, remaining: 20000}
	return index.body(&copyDecl, storage)
}

func (b *ps6141SummaryBody) byteRange(s *ast.RangeStmt) {
	layout := b.byteConsumer
	outer := s == layout.outer
	if outer && b.loop != 0 || !outer && b.loop != 1 {
		b.summary.valid = false
		return
	}
	id, ok := s.Key.(*ast.Ident)
	if !ok {
		b.summary.valid = false
		return
	}
	object := b.index.pass.TypesInfo.Defs[id]
	b.scalars[object] = ps6141Deps{}
	b.scalarLoops[object] = b.loop + 1
	root := layout.root
	if outer {
		b.summary.traversals[root] = min(2, b.summary.traversals[root]+1)
		b.traversing[root] = true
		b.outerIndex = object
		b.outerRoot = root
	} else {
		b.summary.lanePasses[root] = min(2, b.summary.lanePasses[root]+1)
		b.laneIndex = object
		b.laneExtent = layout.stride - 2
		value, ok := s.Value.(*ast.Ident)
		if !ok || value.Name == "_" {
			b.summary.valid = false
			return
		}
		q := b.index.pass.TypesInfo.Defs[value]
		b.scalars[q] = ps6141Deps{root: true, 1000 + root: true}
		b.scalarLoops[q] = 2
	}
	previous := b.loopIndex
	b.loopIndex = object
	b.loop++
	b.block(s.Body)
	b.loop--
	b.loopIndex = previous
	if outer {
		b.outerIndex = nil
		b.outerRoot = 0
		delete(b.traversing, root)
	} else {
		b.laneIndex = nil
		b.laneExtent = 0
	}
}

func ps6141ByteBitOperator(op token.Token) bool {
	return op == token.AND || op == token.OR || op == token.XOR || op == token.AND_NOT || op == token.SHL || op == token.SHR || op == token.AND_ASSIGN || op == token.OR_ASSIGN || op == token.XOR_ASSIGN || op == token.AND_NOT_ASSIGN || op == token.SHL_ASSIGN || op == token.SHR_ASSIGN
}
func ps6141ByteDropLaneTokens(deps ps6141Deps) ps6141Deps {
	out := ps6141Deps{}
	for root := range deps {
		if root < 1000 || root >= 2000 && root < 3000 || root >= 4000 {
			out[root] = true
		}
	}
	return out
}

// A scalar helper with unsigned/bitwise transforms cannot attest preserving a
// signed byte interpretation merely because its returned value has that origin.
func ps6141ByteLaneTransformUnknown(pass *analysis.Pass, decl *ast.FuncDecl) bool {
	unknown := false
	ast.Inspect(decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			unknown = unknown || ps6141ByteBitOperator(x.Tok)
		case *ast.UnaryExpr:
			unknown = unknown || x.Op == token.XOR
		case *ast.BinaryExpr:
			unknown = unknown || ps6141ByteBitOperator(x.Op)
		case *ast.CallExpr:
			if pass.TypesInfo.Types[x.Fun].IsType() {
				if basic, ok := types.Unalias(pass.TypesInfo.TypeOf(x)).Underlying().(*types.Basic); ok && basic.Info()&types.IsUnsigned != 0 {
					unknown = true
				}
			}
		}
		return true
	})
	return unknown
}
