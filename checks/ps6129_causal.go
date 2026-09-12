package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
)

type ps6129CausalState struct {
	contract                         *config.CausalZeroGEMMContract
	fn                               *ast.FuncDecl
	rows, sequence, offset, geometry *ast.Ident
	sources                          map[types.Object]config.CausalZeroGEMMMatrix
	transposes                       map[types.Object]types.Object
	views                            map[types.Object]types.Object
	viewRows                         map[types.Object]types.Object
	initializers                     map[types.Object]ast.Expr
	pointers                         map[types.Object]bool
	parents                          map[ast.Node]ast.Node
	funcs                            map[string]bool
}

func runPS6129WithContracts(pass *analysis.Pass, contracts []config.CausalZeroGEMMContract) (any, error) {
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].OwnerSite]++
	}
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Type.TypeParams != nil {
				continue
			}
			for i := range contracts {
				c := &contracts[i]
				if !c.Valid() || counts[c.OwnerSite] != 1 || ps6113DeclarationID(pass, fn) != c.OwnerSite {
					continue
				}
				s, ok := ps6129CausalSetup(pass, fn, c)
				if !ok {
					continue
				}
				zero := map[types.Object]bool{}
				// Store historical source provenance: later source writes do not
				// change the already copied transpose, but output writes do.
				transposed := map[types.Object]types.Object{}
				calls := ps6022Calls(pass, fn.Body, s.parents)
				byCall := make(map[*ast.CallExpr]ps6022Call, len(calls))
				for _, call := range calls {
					byCall[call.call] = call
				}
				for _, stmt := range fn.Body.List {
					// Each row's suffix clear is the final operation, so the common
					// causal union bound follows even if live cells contain NaNs.
					if source, ok := s.clearedRows(pass, stmt); ok {
						for object := range s.writtenMatrices(pass, stmt) {
							delete(zero, object)
							delete(transposed, object)
						}
						zero[source] = true
						continue
					}
					if outputs, ok := s.transposeRows(pass, stmt); ok {
						for _, out := range outputs {
							if source := s.transposes[out]; zero[source] {
								transposed[out] = source
							} else {
								delete(transposed, out)
							}
						}
						continue
					}
					call := ps6032StatementCall(stmt)
					if call != nil && s.probability(pass, call) {
						object := ps6129Object(pass, call.Args[0])
						zero[object] = true
						continue
					}
					if collected, ok := byCall[call]; ok && ps6129GEMM(pass, collected, s.funcs) {
						out := ps6129Object(pass, call.Args[2])
						delete(zero, out)
						delete(transposed, out)
						input := ps6129Object(pass, call.Args[0])
						rows, depth := ast.Expr(s.rows), ast.Expr(s.sequence)
						kind := "contracts over cleared causal suffix"
						matched := zero[input]
						if transposed[input] != nil {
							matched = true
							rows, depth = s.sequence, s.rows
							kind = "computes cleared causal suffix output rows"
						}
						if matched && ps6129Int(pass, call.Args[3], 0) && ps6129Equal(pass, call.Args[4], rows) && ps6129Equal(pass, call.Args[5], depth) {
							live := exprTextRendered(s.geometry) + "." + c.OffsetField + "+" + s.offset.Name + "+" + s.rows.Name
							pass.Reportf(call.Pos(), "dense GEMM %s; when %s.%s and L=%s < %s, redundant rectangular work is %s*(%s-L)*%s multiply-accumulates; contract attests helper dispatch and exclusive scratch semantics, source proves row clears/transpose/full dimensions; advisory only: retain finite fallback, signed-zero/accumulation parity and clear skipped scratch output tails before ordered reduction; qualify frozen production and complete-workload latency/allocations on each target", kind, s.geometry.Name, c.CausalField, live, s.sequence.Name, s.rows.Name, s.sequence.Name, exprTextRendered(call.Args[6]))
						}
						continue
					}
					// Builtin copy touches only its first argument. Unknown reviewed
					// helpers may write any supplied matrix and invalidate its proof.
					writes := s.writtenMatrices(pass, stmt)
					for object := range writes {
						delete(zero, object)
						delete(transposed, object)
					}
				}
			}
		}
	}
	return nil, nil
}

func ps6129CausalSetup(pass *analysis.Pass, fn *ast.FuncDecl, c *config.CausalZeroGEMMContract) (ps6129CausalState, bool) {
	s := ps6129CausalState{contract: c, fn: fn, sources: map[types.Object]config.CausalZeroGEMMMatrix{}, transposes: map[types.Object]types.Object{}, views: map[types.Object]types.Object{}, viewRows: map[types.Object]types.Object{}, initializers: map[types.Object]ast.Expr{}, pointers: map[types.Object]bool{}, parents: ps6019Parents(fn.Body), funcs: map[string]bool{c.GEMMFunction: true}}
	bindings := map[string]*ast.Ident{}
	duplicates := map[string]bool{}
	ast.Inspect(fn, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && pass.TypesInfo.Defs[id] != nil {
			if bindings[id.Name] != nil {
				duplicates[id.Name] = true
			}
			bindings[id.Name] = id
		}
		assignment, ok := n.(*ast.AssignStmt)
		if ok && assignment.Tok == token.DEFINE && len(assignment.Lhs) == len(assignment.Rhs) {
			for i, lhs := range assignment.Lhs {
				if object := ps6129Object(pass, lhs); object != nil {
					s.initializers[object] = assignment.Rhs[i]
				}
			}
		}
		return true
	})
	lookup := func(name string) *ast.Ident {
		if duplicates[name] {
			return nil
		}
		return bindings[name]
	}
	s.rows, s.sequence, s.offset, s.geometry = lookup(c.RowsBinding), lookup(c.SequenceBinding), lookup(c.QueryOffsetBinding), lookup(c.GeometryBinding)
	if s.rows == nil || s.sequence == nil || s.offset == nil || s.geometry == nil {
		return s, false
	}
	for _, id := range []*ast.Ident{s.rows, s.sequence, s.offset} {
		if !types.Identical(pass.TypesInfo.TypeOf(id), types.Typ[types.Int]) {
			return s, false
		}
	}
	geometryType := pass.TypesInfo.TypeOf(s.geometry)
	structure, ok := types.Unalias(geometryType).Underlying().(*types.Struct)
	if !ok {
		return s, false
	}
	for name, want := range map[string]types.Type{c.SequenceField: types.Typ[types.Int], c.OffsetField: types.Typ[types.Int], c.CausalField: types.Typ[types.Bool]} {
		found := false
		for i := 0; i < structure.NumFields(); i++ {
			field := structure.Field(i)
			if field.Name() == name && types.Identical(field.Type(), want) {
				found = true
			}
		}
		if !found {
			return s, false
		}
	}
	sequenceInit, ok := s.initializers[ps6129Object(pass, s.sequence)].(*ast.SelectorExpr)
	if !ok || sequenceInit.Sel.Name != c.SequenceField || !ps6129Equal(pass, sequenceInit.X, s.geometry) || !ps6129Immutable(pass, fn.Body, s.rows, s.sequence, s.offset, s.geometry) {
		return s, false
	}
	type region struct {
		pointer   types.Object
		low, high int64
	}
	regions := []region{}
	var scalarSlice types.Type
	for _, m := range c.Matrices {
		source, transpose := lookup(m.SourceBinding), lookup(m.TransposeBinding)
		if source == nil || transpose == nil {
			return s, false
		}
		for _, id := range []*ast.Ident{source, transpose} {
			object := ps6129Object(pass, id)
			slice, ok := types.Unalias(object.Type()).(*types.Slice)
			if !ok || (!types.Identical(slice.Elem(), types.Typ[types.Float32]) && !types.Identical(slice.Elem(), types.Typ[types.Float64])) {
				return s, false
			}
			if scalarSlice != nil && !types.Identical(scalarSlice, object.Type()) {
				return s, false
			}
			scalarSlice = object.Type()
			pointer, lo, hi, ok := s.scratchRegion(pass, s.initializers[object])
			if !ok || hi-lo != 1 {
				return s, false
			}
			if !types.Identical(pointer.Type(), types.NewPointer(object.Type())) {
				return s, false
			}
			for _, r := range regions {
				if r.pointer == pointer && lo < r.high && r.low < hi {
					return s, false
				}
			}
			regions = append(regions, region{pointer, lo, hi})
			s.pointers[pointer] = true
		}
		sourceObject, transposeObject := ps6129Object(pass, source), ps6129Object(pass, transpose)
		s.sources[sourceObject] = m
		s.transposes[transposeObject] = sourceObject
	}
	// Discover only canonical full row views within full n-row loops.
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		loop, index, ok := ps6129RangeNode(n)
		if !ok || !ps6129Equal(pass, loop.X, s.rows) {
			return true
		}
		for _, stmt := range loop.Body.List {
			assignment, ok := stmt.(*ast.AssignStmt)
			if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != len(assignment.Rhs) {
				continue
			}
			for i, rhs := range assignment.Rhs {
				slice, ok := rhs.(*ast.SliceExpr)
				if !ok {
					continue
				}
				source := ps6129Object(pass, slice.X)
				if _, ok := s.sources[source]; !ok {
					continue
				}
				if s.rowSlice(pass, slice, index) {
					if object := ps6129Object(pass, assignment.Lhs[i]); object != nil {
						s.views[object] = source
						s.viewRows[object] = ps6129Object(pass, index)
					}
				}
			}
		}
		return true
	})
	if !s.closedOwnership(pass) {
		return s, false
	}
	valid := true
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, sig, ok := typedCallee(pass, call.Fun)
		if ok && ps6090FunctionID(callee) == c.BoundsMethod {
			if sig.Recv() == nil || !types.Identical(sig.Recv().Type(), geometryType) || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 2 || !types.Identical(sig.Params().At(0).Type(), types.Typ[types.Int]) || !types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int]) || !types.Identical(sig.Results().At(1).Type(), types.Typ[types.Int]) {
				valid = false
			}
		}
		return valid
	})
	if !valid {
		return s, false
	}
	return s, true
}

func ps6129RangeNode(n ast.Node) (*ast.RangeStmt, *ast.Ident, bool) {
	stmt, ok := n.(ast.Stmt)
	if !ok {
		return nil, nil, false
	}
	return ps6129Range(stmt)
}

func (s *ps6129CausalState) rowSlice(pass *analysis.Pass, slice *ast.SliceExpr, index *ast.Ident) bool {
	next := &ast.BinaryExpr{X: index, Op: token.ADD, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}
	return ps6129Product(pass, slice.Low, index, s.sequence) && ps6129Product(pass, slice.High, next, s.sequence) && (!slice.Slice3 || ps6129Equal(pass, slice.Max, slice.High))
}

// Scratch partitions are integer multiples of the complete matrix extent.
// Exact normalized multiplication proves pt/dat disjoint within one lease.
func (s *ps6129CausalState) coefficient(pass *analysis.Pass, e ast.Expr) (int64, bool) {
	terms := map[types.Object]int{}
	coefficient := int64(1)
	valid := true
	var visit func(ast.Expr)
	visit = func(e ast.Expr) {
		if e == nil {
			valid = false
			return
		}
		e = ps2110Unparen(e)
		if b, ok := e.(*ast.BinaryExpr); ok && b.Op == token.MUL {
			visit(b.X)
			visit(b.Y)
			return
		}
		if value := pass.TypesInfo.Types[e].Value; value != nil {
			v, ok := constant.Int64Val(value)
			if !ok || v < 0 || v > 8 {
				valid = false
				return
			}
			coefficient *= v
			return
		}
		object := ps6129Object(pass, e)
		if object == nil {
			valid = false
			return
		}
		terms[object]++
	}
	visit(e)
	return coefficient, valid && len(terms) == 2 && terms[ps6129Object(pass, s.rows)] == 1 && terms[ps6129Object(pass, s.sequence)] == 1
}

func (s *ps6129CausalState) scratchRegion(pass *analysis.Pass, e ast.Expr) (types.Object, int64, int64, bool) {
	var slice *ast.SliceExpr
	if value, ok := ps2110Unparen(e).(*ast.SliceExpr); ok {
		slice = value
		e = value.X
	}
	star, ok := ps2110Unparen(e).(*ast.StarExpr)
	if !ok {
		return nil, 0, 0, false
	}
	pointer := ps6129Object(pass, star.X)
	if pointer == nil {
		return nil, 0, 0, false
	}
	allocation, ok := s.initializers[pointer].(*ast.CallExpr)
	if !ok || len(allocation.Args) != 1 {
		return nil, 0, 0, false
	}
	fn, sig, ok := typedCallee(pass, allocation.Fun)
	if !ok || ps6090FunctionID(fn) != s.contract.ScratchAllocator || sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Params().Len() != 1 || !types.Identical(sig.Params().At(0).Type(), types.Typ[types.Int]) || sig.Results().Len() != 1 || !types.Identical(sig.Results().At(0).Type(), pointer.Type()) {
		return nil, 0, 0, false
	}
	hi, ok := s.coefficient(pass, allocation.Args[0])
	if !ok {
		return nil, 0, 0, false
	}
	lo := int64(0)
	allocated := hi
	if slice != nil {
		if slice.Slice3 {
			return nil, 0, 0, false
		}
		if slice.Low != nil {
			lo, ok = s.coefficient(pass, slice.Low)
			if !ok {
				return nil, 0, 0, false
			}
		}
		if slice.High != nil {
			hi, ok = s.coefficient(pass, slice.High)
			if !ok {
				return nil, 0, 0, false
			}
		}
	}
	return pointer, lo, hi, lo >= 0 && hi > lo && hi <= allocated
}

func (s *ps6129CausalState) matrix(object types.Object) types.Object {
	if source := s.views[object]; source != nil {
		return source
	}
	if _, ok := s.sources[object]; ok {
		return object
	}
	if _, ok := s.transposes[object]; ok {
		return object
	}
	return nil
}

func (s *ps6129CausalState) closedOwnership(pass *analysis.Pass) bool {
	valid := true
	ast.Inspect(s.fn.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.GoStmt, *ast.FuncLit, *ast.BranchStmt, *ast.LabeledStmt, *ast.ReturnStmt:
			valid = false
			return false
		}
		if selector, ok := n.(*ast.SelectorExpr); ok {
			selection := pass.TypesInfo.Selections[selector]
			if selection != nil {
				if signature, ok := selection.Obj().Type().(*types.Signature); ok && signature.Recv() != nil {
					if _, pointer := types.Unalias(signature.Recv().Type()).(*types.Pointer); pointer {
						ast.Inspect(selector.X, func(child ast.Node) bool {
							if id, ok := child.(*ast.Ident); ok && identObject(pass, id) == ps6129Object(pass, s.geometry) {
								valid = false
							}
							return true
						})
					}
				}
			}
		}
		// Geometry's value must not be written or exposed through a field.
		if assignment, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range assignment.Lhs {
				if _, id := lhs.(*ast.Ident); id {
					continue
				}
				ast.Inspect(lhs, func(child ast.Node) bool {
					if id, ok := child.(*ast.Ident); ok && ps6129Object(pass, id) == ps6129Object(pass, s.geometry) {
						valid = false
					}
					return true
				})
			}
		}
		if increment, ok := n.(*ast.IncDecStmt); ok {
			ast.Inspect(increment.X, func(child ast.Node) bool {
				if id, ok := child.(*ast.Ident); ok && ps6129Object(pass, id) == ps6129Object(pass, s.geometry) {
					valid = false
				}
				return true
			})
		}
		if unary, ok := n.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			ast.Inspect(unary.X, func(child ast.Node) bool {
				if id, ok := child.(*ast.Ident); ok && ps6129Object(pass, id) == ps6129Object(pass, s.geometry) {
					valid = false
				}
				return true
			})
		}
		id, ok := n.(*ast.Ident)
		if !ok {
			return valid
		}
		object := identObject(pass, id)
		if s.matrix(object) == nil && !s.pointers[object] {
			return valid
		}
		var owner ast.Node = id
		for {
			if paren, ok := s.parents[owner].(*ast.ParenExpr); ok {
				owner = paren
			} else {
				break
			}
		}
		switch parent := s.parents[owner].(type) {
		case *ast.AssignStmt:
			if parent.Tok != token.DEFINE || pass.TypesInfo.Defs[id] != object {
				valid = false
				break
			}
			found := false
			for _, lhs := range parent.Lhs {
				if lhs == id {
					found = true
				}
			}
			if !found {
				valid = false
			}
		case *ast.StarExpr:
			if !s.pointers[object] {
				valid = false
				break
			}
			var expression ast.Node = parent
			for {
				if p, ok := s.parents[expression].(*ast.ParenExpr); ok {
					expression = p
				} else {
					break
				}
			}
			if slice, ok := s.parents[expression].(*ast.SliceExpr); ok {
				expression = slice
			}
			assignment, ok := s.parents[expression].(*ast.AssignStmt)
			if !ok || assignment.Tok != token.DEFINE {
				valid = false
				break
			}
			found := false
			for i, rhs := range assignment.Rhs {
				if rhs == expression && i < len(assignment.Lhs) && s.matrix(ps6129Object(pass, assignment.Lhs[i])) != nil {
					found = true
				}
			}
			if !found {
				valid = false
			}
		case *ast.SliceExpr:
			if s.pointers[object] {
				valid = false
				break
			}
			if assignment, ok := s.parents[parent].(*ast.AssignStmt); ok && assignment.Tok == token.DEFINE {
				found := false
				for i, rhs := range assignment.Rhs {
					if rhs == parent && i < len(assignment.Lhs) && s.views[ps6129Object(pass, assignment.Lhs[i])] == s.matrix(object) {
						found = true
					}
				}
				if !found {
					valid = false
				}
				break
			}
			call, ok := s.parents[parent].(*ast.CallExpr)
			if !ok || !ps6129Builtin(pass, call, "clear") {
				valid = false
			}
		case *ast.IndexExpr:
			if s.pointers[object] || parent.X != owner {
				valid = false
				break
			}
			var expr ast.Node = parent
			for {
				if p, ok := s.parents[expr].(*ast.ParenExpr); ok {
					expr = p
				} else {
					break
				}
			}
			if unary, ok := s.parents[expr].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				valid = false
			}
		case *ast.CallExpr:
			fn, sig, ok := typedCallee(pass, parent.Fun)
			if s.pointers[object] {
				deferred, isDeferred := s.parents[parent].(*ast.DeferStmt)
				if !isDeferred || s.parents[deferred] != s.fn.Body || !ok || ps6090FunctionID(fn) != s.contract.ScratchRelease || sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 0 || !types.Identical(sig.Params().At(0).Type(), object.Type()) {
					valid = false
				}
				break
			}
			if ps6129Builtin(pass, parent, "copy") {
				break
			}
			if !ok || sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Results().Len() != 0 {
				valid = false
				break
			}
			name := ps6090FunctionID(fn)
			allowed := name == s.contract.GEMMFunction || name == s.contract.ProbabilityHelper
			for _, helper := range s.contract.NonRetainingBufferHelpers {
				allowed = allowed || name == helper
			}
			if !allowed {
				valid = false
			}
		default:
			valid = false
		}
		return valid
	})
	return valid
}

func ps6129Builtin(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
	return ok && builtin.Name() == name
}

func (s *ps6129CausalState) bounds(pass *analysis.Pass, call *ast.CallExpr, index *ast.Ident) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || ps6090FunctionID(fn) != s.contract.BoundsMethod || len(call.Args) != 1 || sig.Recv() == nil || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 2 || !types.Identical(sig.Params().At(0).Type(), types.Typ[types.Int]) || !types.Identical(sig.Results().At(0).Type(), types.Typ[types.Int]) || !types.Identical(sig.Results().At(1).Type(), types.Typ[types.Int]) || !types.Identical(sig.Recv().Type(), pass.TypesInfo.TypeOf(s.geometry)) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !ps6129Equal(pass, selector.X, s.geometry) {
		return false
	}
	expected := &ast.BinaryExpr{X: s.offset, Op: token.ADD, Y: index}
	return ps6129Equal(pass, call.Args[0], expected)
}

func (s *ps6129CausalState) clearedRows(pass *analysis.Pass, stmt ast.Stmt) (types.Object, bool) {
	loop, index, ok := ps6129Range(stmt)
	if !ok || !ps6129Equal(pass, loop.X, s.rows) || len(loop.Body.List) == 0 {
		return nil, false
	}
	upper := types.Object(nil)
	for _, stmt := range loop.Body.List {
		assignment, ok := stmt.(*ast.AssignStmt)
		if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 || assignment.Tok != token.DEFINE {
			continue
		}
		call, ok := assignment.Rhs[0].(*ast.CallExpr)
		if ok && s.bounds(pass, call, index) {
			upper = ps6129Object(pass, assignment.Lhs[1])
		}
	}
	if upper == nil {
		return nil, false
	}
	call := ps6032StatementCall(loop.Body.List[len(loop.Body.List)-1])
	if call == nil || !ps6129Builtin(pass, call, "clear") || len(call.Args) != 1 {
		return nil, false
	}
	slice, ok := call.Args[0].(*ast.SliceExpr)
	if !ok || slice.High != nil || slice.Slice3 || ps6129Object(pass, slice.Low) != upper {
		return nil, false
	}
	view := ps6129Object(pass, slice.X)
	source := s.views[view]
	if source == nil {
		return nil, false
	}
	valid := true
	ast.Inspect(loop.Body, func(n ast.Node) bool {
		switch n.(type) {
		case *ast.BranchStmt, *ast.ReturnStmt, *ast.GoStmt, *ast.DeferStmt, *ast.FuncLit:
			valid = false
		}
		return valid
	})
	if !valid {
		return nil, false
	}
	// upper may not be rebound after the exact bounds call.
	if !ps6129Immutable(pass, loop.Body, slice.Low, index) {
		return nil, false
	}
	if !s.currentRowWrites(pass, loop.Body, source, index) {
		return nil, false
	}
	return source, true
}

// A final clear cannot repair contamination of a PREVIOUS row. All writes
// to this matrix must use a view tied to the current range induction object;
// opaque full-buffer or row-buffer helper calls cannot establish confinement.
func (s *ps6129CausalState) currentRowWrites(pass *analysis.Pass, body *ast.BlockStmt, source types.Object, index *ast.Ident) bool {
	valid := true
	mentions := func(n ast.Node) bool {
		found := false
		ast.Inspect(n, func(child ast.Node) bool {
			if id, ok := child.(*ast.Ident); ok && s.matrix(identObject(pass, id)) == source {
				found = true
			}
			return true
		})
		return found
	}
	current := func(e ast.Expr) bool {
		if slice, ok := ps2110Unparen(e).(*ast.SliceExpr); ok {
			e = slice.X
		}
		view := ps6129Object(pass, e)
		return s.views[view] == source && s.viewRows[view] == ps6129Object(pass, index)
	}
	write := func(e ast.Expr) {
		if !mentions(e) {
			return
		}
		indexed, ok := ps2110Unparen(e).(*ast.IndexExpr)
		if !ok || !current(indexed.X) {
			valid = false
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if assignment, ok := n.(*ast.AssignStmt); ok && assignment.Tok != token.DEFINE {
			for _, lhs := range assignment.Lhs {
				write(lhs)
			}
		}
		if increment, ok := n.(*ast.IncDecStmt); ok {
			write(increment.X)
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return valid
		}
		if ps6129Builtin(pass, call, "clear") || ps6129Builtin(pass, call, "copy") {
			if len(call.Args) > 0 && mentions(call.Args[0]) && !current(call.Args[0]) {
				valid = false
			}
			return valid
		}
		if pass.TypesInfo.Types[call.Fun].IsType() {
			return valid
		}
		for _, arg := range call.Args {
			if !mentions(arg) {
				continue
			}
			typ := pass.TypesInfo.TypeOf(arg)
			if typ == nil {
				valid = false
				continue
			}
			switch types.Unalias(typ).Underlying().(type) {
			case *types.Slice, *types.Pointer, *types.Interface:
				valid = false
			}
		}
		return valid
	})
	return valid
}

func (s *ps6129CausalState) probability(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || ps6090FunctionID(fn) != s.contract.ProbabilityHelper || len(call.Args) != 5 || sig.Recv() != nil || sig.TypeParams().Len() != 0 || sig.Variadic() || sig.Params().Len() != 5 || sig.Results().Len() != 0 {
		return false
	}
	matrix, ok := s.sources[ps6129Object(pass, call.Args[0])]
	if !ok || !matrix.Probability || !types.Identical(sig.Params().At(0).Type(), pass.TypesInfo.TypeOf(call.Args[0])) || !types.Identical(sig.Params().At(1).Type(), pass.TypesInfo.TypeOf(s.geometry)) {
		return false
	}
	for i := 2; i < 5; i++ {
		if !types.Identical(sig.Params().At(i).Type(), types.Typ[types.Int]) {
			return false
		}
	}
	return ps6129Equal(pass, call.Args[1], s.geometry) && ps6129Equal(pass, call.Args[3], s.offset) && ps6129Equal(pass, call.Args[4], s.rows)
}

func (s *ps6129CausalState) transposeRows(pass *analysis.Pass, stmt ast.Stmt) ([]types.Object, bool) {
	loop, ri, ok := ps6129Range(stmt)
	if !ok || !ps6129Equal(pass, loop.X, s.rows) {
		return nil, false
	}
	var inner *ast.RangeStmt
	var ji *ast.Ident
	for i, statement := range loop.Body.List {
		if child, index, ok := ps6129Range(statement); ok {
			if inner != nil || i != len(loop.Body.List)-1 || !ps6129Equal(pass, child.X, s.sequence) {
				return nil, false
			}
			inner, ji = child, index
			continue
		}
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != len(assignment.Rhs) {
			return nil, false
		}
		for j, rhs := range assignment.Rhs {
			view, ok := rhs.(*ast.SliceExpr)
			if !ok || s.views[ps6129Object(pass, assignment.Lhs[j])] == nil || !s.rowSlice(pass, view, ri) {
				return nil, false
			}
		}
	}
	if inner == nil || len(inner.Body.List) == 0 {
		return nil, false
	}
	outputs := []types.Object{}
	for _, statement := range inner.Body.List {
		assignment, ok := statement.(*ast.AssignStmt)
		if !ok || assignment.Tok != token.ASSIGN || len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
			return nil, false
		}
		dst, ok := assignment.Lhs[0].(*ast.IndexExpr)
		if !ok {
			return nil, false
		}
		src, ok := assignment.Rhs[0].(*ast.IndexExpr)
		if !ok {
			return nil, false
		}
		out := ps6129Object(pass, dst.X)
		source := s.transposes[out]
		if source == nil {
			return nil, false
		}
		fromObject := ps6129Object(pass, src.X)
		if fromObject == source {
			expected := &ast.BinaryExpr{X: &ast.BinaryExpr{X: ri, Op: token.MUL, Y: s.sequence}, Op: token.ADD, Y: ji}
			if !ps6129Equal(pass, src.Index, expected) {
				return nil, false
			}
		} else if s.views[fromObject] != source || !ps6129Equal(pass, src.Index, ji) {
			return nil, false
		}
		to := &ast.BinaryExpr{X: &ast.BinaryExpr{X: ji, Op: token.MUL, Y: s.rows}, Op: token.ADD, Y: ri}
		if !ps6129Equal(pass, dst.Index, to) {
			return nil, false
		}
		outputs = append(outputs, out)
	}
	return outputs, true
}

func (s *ps6129CausalState) writtenMatrices(pass *analysis.Pass, stmt ast.Stmt) map[types.Object]bool {
	writes := map[types.Object]bool{}
	mark := func(n ast.Node) {
		ast.Inspect(n, func(child ast.Node) bool {
			if id, ok := child.(*ast.Ident); ok {
				if matrix := s.matrix(identObject(pass, id)); matrix != nil {
					writes[matrix] = true
				}
			}
			return true
		})
	}
	ast.Inspect(stmt, func(n ast.Node) bool {
		if assignment, ok := n.(*ast.AssignStmt); ok && assignment.Tok != token.DEFINE {
			for _, lhs := range assignment.Lhs {
				mark(lhs)
			}
		}
		if unary, ok := n.(*ast.IncDecStmt); ok {
			mark(unary.X)
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ps6129Builtin(pass, call, "copy") {
			if len(call.Args) > 0 {
				mark(call.Args[0])
			}
			return true
		}
		if ps6129Builtin(pass, call, "clear") {
			if len(call.Args) > 0 {
				mark(call.Args[0])
			}
			return true
		}
		fn, _, ok := typedCallee(pass, call.Fun)
		if ok && ps6090FunctionID(fn) == s.contract.BoundsMethod {
			return true
		}
		// Type conversions cannot mutate the storage from which a scalar was read.
		if pass.TypesInfo.Types[call.Fun].IsType() {
			return true
		}
		for _, arg := range call.Args {
			mark(arg)
		}
		return true
	})
	return writes
}
