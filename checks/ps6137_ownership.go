package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Ownership is established from string-producing expressions and every cache
// write, not a naming convention or a reviewed-copy boolean. The materializer
// must separately prove that this receiver starts as fresh extraction-local
// zero state and is used only by this helper.
func ps6137Own(pass *analysis.Pass, f ps6137Function) bool {
	s := f.sig
	if s.Recv() == nil || !ps6137CacheType(s.Recv().Type()) || s.Variadic() || s.TypeParams().Len() != 0 || s.Params().Len() != 2 || s.Results().Len() != 1 || !types.Identical(s.Results().At(0).Type(), types.Typ[types.String]) || !types.Identical(s.Params().At(0).Type(), types.Typ[types.Uintptr]) {
		return false
	}
	native, ok := types.Unalias(s.Params().At(1).Type()).(*types.Pointer)
	if !ok || !ps6137Integer(native.Elem()) {
		return false
	}
	parents := ps6087Parents(f.decl.Body)
	owned := make(map[types.Object]bool)
	var clone, view *ast.CallExpr
	valid := true
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncLit, *ast.GoStmt, *ast.DeferStmt, *ast.SendStmt, *ast.ReturnStmt:
			if _, ok := x.(*ast.ReturnStmt); !ok {
				valid = false
				return false
			}
		case *ast.CallExpr:
			if pass.TypesInfo.Types[x.Fun].IsType() {
				return true
			}
			id := ps6137CallID(pass, x)
			switch id {
			case "strings.Clone":
				if clone != nil || len(x.Args) != 1 {
					valid = false
				}
				clone = x
			case "unsafe.String":
				if view != nil || len(x.Args) != 2 {
					valid = false
				}
				view = x
			case "unsafe.Slice", "unsafe.SliceData":
			default:
				ident, ok := ps2110Unparen(x.Fun).(*ast.Ident)
				if !ok {
					valid = false
					return true
				}
				builtin, ok := identObject(pass, ident).(*types.Builtin)
				if !ok || (builtin.Name() != "len" && builtin.Name() != "make") {
					valid = false
				}
			}
		case *ast.Ident:
			if identObject(pass, x) == s.Recv() {
				selector, ok := parents[x].(*ast.SelectorExpr)
				if !ok || selector.X != x {
					valid = false
				}
			}
		}
		return true
	})
	if !valid || clone == nil || view == nil || !ps6137BoundedView(pass, f, view, s.Params().At(1)) {
		return false
	}
	// Iterate local bindings: map/cache reads are owned only because all writes
	// below must themselves be proven owned. Borrowed view aliases never become
	// owned without strings.Clone.
	for changed := true; changed; {
		changed = false
		ast.Inspect(f.decl.Body, func(n ast.Node) bool {
			a, ok := n.(*ast.AssignStmt)
			if !ok || len(a.Rhs) != 1 {
				return true
			}
			if !ps6137OwnedExpression(pass, a.Rhs[0], s.Recv(), owned) {
				return true
			}
			for _, lhs := range a.Lhs {
				v := ps6137Local(pass, lhs)
				if v != nil && types.Identical(v.Type(), types.Typ[types.String]) && !owned[v] {
					owned[v] = true
					changed = true
				}
			}
			return true
		})
	}
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.ReturnStmt:
			if len(x.Results) != 1 || !ps6137OwnedExpression(pass, x.Results[0], s.Recv(), owned) {
				valid = false
			}
		case *ast.AssignStmt:
			for i, lhs := range x.Lhs {
				if v := ps6137Local(pass, lhs); v != nil {
					if v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
						valid = false
					}
				} else if !ps6137ReceiverRoot(pass, lhs, s.Recv()) {
					valid = false
				}
				if ps6137ReceiverRoot(pass, lhs, s.Recv()) {
					if key, ok := ps2110Unparen(lhs).(*ast.IndexExpr); ok && types.Identical(pass.TypesInfo.TypeOf(key.Index), types.Typ[types.String]) && !ps6137OwnedExpression(pass, key.Index, s.Recv(), owned) {
						valid = false
					}
					if len(x.Rhs) != len(x.Lhs) {
						valid = false
						continue
					}
					if types.Identical(pass.TypesInfo.TypeOf(lhs), types.Typ[types.String]) && !ps6137OwnedExpression(pass, x.Rhs[i], s.Recv(), owned) {
						valid = false
					}
					// Composite cache entries also require every string component
					// to originate in owned state, never the transient native view.
					ast.Inspect(x.Rhs[i], func(n ast.Node) bool {
						e, ok := n.(ast.Expr)
						if ok && pass.TypesInfo.Types[e].IsValue() && types.Identical(pass.TypesInfo.TypeOf(e), types.Typ[types.String]) && !ps6137OwnedExpression(pass, e, s.Recv(), owned) {
							valid = false
							return false
						}
						return true
					})
				} else if v := ps6137Local(pass, lhs); v != nil && owned[v] {
					if len(x.Rhs) == len(x.Lhs) && !ps6137OwnedExpression(pass, x.Rhs[i], s.Recv(), owned) {
						valid = false
					}
				}
			}
		case *ast.RangeStmt:
			if x.Tok != token.DEFINE || x.Value != nil {
				valid = false
			}
		case *ast.IncDecStmt:
			if v := ps6137Local(pass, x.X); v != nil {
				if v.Pkg() != nil && v.Parent() == v.Pkg().Scope() {
					valid = false
				}
			} else if !ps6137ReceiverRoot(pass, x.X, s.Recv()) {
				valid = false
			}
		}
		return true
	})
	return valid && ps6137CacheSemantics(pass, f)
}

func ps6137CacheType(t types.Type) bool {
	p, ok := types.Unalias(t).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(p.Elem()).(*types.Named)
	if !ok || named.TypeParams().Len() != 0 {
		return false
	}
	record, ok := named.Underlying().(*types.Struct)
	if !ok || record.NumFields() == 0 {
		return false
	}
	for i := 0; i < record.NumFields(); i++ {
		field := record.Field(i)
		if field.Embedded() {
			return false
		}
		switch t := types.Unalias(field.Type()).Underlying().(type) {
		case *types.Basic:
			if t.Info()&types.IsInteger == 0 {
				return false
			}
		case *types.Map:
			if !types.Identical(t.Elem(), types.Typ[types.String]) || !(types.Identical(t.Key(), types.Typ[types.Uintptr]) || types.Identical(t.Key(), types.Typ[types.String])) {
				return false
			}
		case *types.Array:
			entry, ok := types.Unalias(t.Elem()).Underlying().(*types.Struct)
			if !ok || entry.NumFields() != 2 {
				return false
			}
			strings, integers := 0, 0
			for j := 0; j < entry.NumFields(); j++ {
				if types.Identical(entry.Field(j).Type(), types.Typ[types.String]) {
					strings++
				} else if types.Identical(entry.Field(j).Type(), types.Typ[types.Uintptr]) {
					integers++
				} else {
					return false
				}
			}
			if strings != 1 || integers != 1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func ps6137ReceiverRoot(pass *analysis.Pass, e ast.Expr, receiver types.Object) bool {
	switch x := ps2110Unparen(e).(type) {
	case *ast.Ident:
		return identObject(pass, x) == receiver
	case *ast.SelectorExpr:
		return ps6137ReceiverRoot(pass, x.X, receiver)
	case *ast.IndexExpr:
		return ps6137ReceiverRoot(pass, x.X, receiver)
	}
	return false
}

func ps6137OwnedExpression(pass *analysis.Pass, e ast.Expr, receiver types.Object, owned map[types.Object]bool) bool {
	e = ps2110Unparen(e)
	switch x := e.(type) {
	case *ast.BasicLit:
		return x.Kind == token.STRING
	case *ast.Ident:
		return owned[identObject(pass, x)]
	case *ast.CallExpr:
		return ps6137CallID(pass, x) == "strings.Clone" && len(x.Args) == 1
	case *ast.IndexExpr:
		if m, ok := types.Unalias(pass.TypesInfo.TypeOf(x.X)).Underlying().(*types.Map); ok && types.Identical(m.Elem(), types.Typ[types.String]) && ps6137ReceiverRoot(pass, e, receiver) {
			return true
		}
		return ps6137ReceiverRoot(pass, e, receiver) && types.Identical(pass.TypesInfo.TypeOf(e), types.Typ[types.String])
	case *ast.SelectorExpr:
		return ps6137ReceiverRoot(pass, e, receiver) && types.Identical(pass.TypesInfo.TypeOf(e), types.Typ[types.String])
	}
	return false
}

func ps6137BoundedView(pass *analysis.Pass, f ps6137Function, view *ast.CallExpr, native types.Object) bool {
	data, ok := ps2110Unparen(view.Args[0]).(*ast.CallExpr)
	if !ok || ps6137CallID(pass, data) != "unsafe.SliceData" || len(data.Args) != 1 {
		return false
	}
	bytes := ps6137Local(pass, data.Args[0])
	length := ps6137Local(pass, view.Args[1])
	if bytes == nil || length == nil {
		return false
	}
	var slice *ast.CallExpr
	var scan *ast.ForStmt
	zero := false
	valid := true
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range x.Lhs {
				if ps6137Local(pass, lhs) == bytes {
					if slice != nil || x.Tok != token.DEFINE || len(x.Lhs) != 1 || len(x.Rhs) != 1 {
						valid = false
						continue
					}
					slice, _ = ps2110Unparen(x.Rhs[0]).(*ast.CallExpr)
				}
				if ps6137Local(pass, lhs) == length {
					if zero || x.Tok != token.DEFINE || len(x.Lhs) != 1 || len(x.Rhs) != 1 || !ps6137Zero(pass, x.Rhs[0]) {
						valid = false
					}
					zero = true
				}
				if ps6137ReceiverRoot(pass, lhs, bytes) || ps6137ReceiverRoot(pass, lhs, native) {
					if ps6137Local(pass, lhs) != bytes {
						valid = false
					}
				}
			}
		case *ast.ForStmt:
			post, ok := x.Post.(*ast.IncDecStmt)
			bodyPost := false
			if !ok && x.Post == nil && len(x.Body.List) == 1 {
				post, ok = x.Body.List[0].(*ast.IncDecStmt)
				bodyPost = ok
			}
			if !ok || ps6137Local(pass, post.X) != length {
				return true
			}
			if scan != nil || post.Tok != token.INC || x.Init != nil || !bodyPost && len(x.Body.List) != 0 {
				valid = false
			}
			scan = x
		case *ast.IncDecStmt:
			if ps6137Local(pass, x.X) == length {
				parents := ps6087Parents(f.decl.Body)
				loop, ok := parents[x].(*ast.ForStmt)
				if block, body := parents[x].(*ast.BlockStmt); body {
					loop, ok = parents[block].(*ast.ForStmt)
					if ok && (len(block.List) != 1 || block.List[0] != x) {
						ok = false
					}
				}
				if !ok || loop.Post != nil && loop.Post != x {
					valid = false
				}
			}
		}
		return true
	})
	if !valid || !zero || slice == nil || scan == nil || ps6137CallID(pass, slice) != "unsafe.Slice" || len(slice.Args) != 2 || slice.End() >= scan.Pos() || scan.End() >= view.Pos() {
		return false
	}
	// The fixed native byte extent must be a positive compile-time integer.
	bound := pass.TypesInfo.Types[ps2110Unparen(slice.Args[1])].Value
	if bound == nil || constant.Sign(bound) <= 0 {
		return false
	}
	containsNative := false
	ast.Inspect(slice.Args[0], func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && identObject(pass, id) == native {
			containsNative = true
		}
		return true
	})
	if !containsNative {
		return false
	}
	// Only the canonical native-character-pointer to byte-pointer conversion
	// is supported. Pointer arithmetic hidden inside conversions is not safe.
	bytePointer, ok := ps2110Unparen(slice.Args[0]).(*ast.CallExpr)
	if !ok || len(bytePointer.Args) != 1 || !pass.TypesInfo.Types[bytePointer.Fun].IsType() {
		return false
	}
	p, ok := types.Unalias(pass.TypesInfo.TypeOf(bytePointer)).(*types.Pointer)
	if !ok || !types.Identical(p.Elem(), types.Typ[types.Byte]) {
		return false
	}
	unsafePointer, ok := ps2110Unparen(bytePointer.Args[0]).(*ast.CallExpr)
	if !ok || len(unsafePointer.Args) != 1 || !pass.TypesInfo.Types[unsafePointer.Fun].IsType() || !types.Identical(pass.TypesInfo.TypeOf(unsafePointer), types.Typ[types.UnsafePointer]) || !ps6135Object(pass, unsafePointer.Args[0], native) {
		return false
	}
	// Reject aliases, address escapes and alternative reads of the native
	// pointer. It is consumed once by the bounded byte-view construction.
	uses := 0
	ast.Inspect(f.decl.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if ok && identObject(pass, id) == native {
			uses++
		}
		return true
	})
	if uses != 1 {
		return false
	}
	and, ok := ps2110Unparen(scan.Cond).(*ast.BinaryExpr)
	if !ok || and.Op != token.LAND {
		return false
	}
	less, ok := ps2110Unparen(and.X).(*ast.BinaryExpr)
	if !ok || less.Op != token.LSS || !ps6135Object(pass, less.X, length) {
		return false
	}
	lenCall, ok := ps2110Unparen(less.Y).(*ast.CallExpr)
	if !ok || len(lenCall.Args) != 1 || !ps6135Object(pass, lenCall.Args[0], bytes) {
		return false
	}
	id, ok := ps2110Unparen(lenCall.Fun).(*ast.Ident)
	if !ok || identObject(pass, id) != types.Universe.Lookup("len") {
		return false
	}
	neq, ok := ps2110Unparen(and.Y).(*ast.BinaryExpr)
	if !ok || neq.Op != token.NEQ || !ps6137Zero(pass, neq.Y) {
		return false
	}
	index, ok := ps2110Unparen(neq.X).(*ast.IndexExpr)
	return ok && ps6135Object(pass, index.X, bytes) && ps6135Object(pass, index.Index, length)
}

func ps6137NativeLabelExtent(pass *analysis.Pass, pointer types.Type, own ps6137Function, field string) bool {
	p, ok := types.Unalias(pointer).(*types.Pointer)
	if !ok {
		return false
	}
	record, ok := types.Unalias(p.Elem()).Underlying().(*types.Struct)
	if !ok {
		return false
	}
	var extent int64
	for i := 0; i < record.NumFields(); i++ {
		f := record.Field(i)
		if f.Name() == field {
			a, ok := types.Unalias(f.Type()).(*types.Array)
			if !ok || a.Len() <= 0 {
				return false
			}
			b, ok := types.Unalias(a.Elem()).Underlying().(*types.Basic)
			if !ok || (b.Kind() != types.Int8 && b.Kind() != types.Uint8) {
				return false
			}
			extent = a.Len()
		}
	}
	if extent == 0 {
		return false
	}
	matched := false
	ast.Inspect(own.decl.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if ok && ps6137CallID(pass, call) == "unsafe.Slice" && len(call.Args) == 2 {
			v := pass.TypesInfo.Types[ps2110Unparen(call.Args[1])].Value
			if v != nil && v.Kind() == constant.Int {
				n, ok := constant.Int64Val(v)
				matched = ok && n == extent
			}
		}
		return true
	})
	return matched
}
