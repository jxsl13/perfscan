package checks

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS4101 reports element-copy loops replaceable by the copy builtin.
var PS4101 = register(&lint.Check{
	ID:       "PS4101",
	Category: "vector",
	Slug:     "loop-copy",
	Level:    lint.LevelIdiomatic,
	AutoFix:  true,
	Doc: lint.Documentation{
		Title: "an element-copy loop replaceable by the copy builtin",
		Text: `for i := range src { dst[i] = src[i] } performs one
bounds-checked load/store pair per element; the copy builtin is a single
memmove. The rewrite is mechanical.

The one case where copy and a forward element loop DIVERGE is overlapping
slices of the same backing array: copy behaves like memmove, whereas a
forward loop whose dst starts after src PROPAGATES earlier elements (the
classic LZ/run-length back-reference — offset < length). Rewriting such a
loop to copy silently corrupts output.

copy can also diverge on LENGTH: the loop writes dst[i] for every i in src
and PANICS if len(dst) < len(src), whereas copy stops at the shorter slice.

Because deciding non-aliasing is undecidable and any unfollowed provenance
would fail OPEN into a corrupting fix, the fix fires only on the single shape
that rules out BOTH divergences soundly: dst is a fresh, non-escaping
make([]T, len(src)) (or cap(src)) and src is never reassigned. Then len(dst)
>= len(src) (no panic gap) and nothing can share dst's backing (no overlap),
so copy is bit-identical. This is deliberately narrow — copies between two
parameters, two slices of a shared array, or into an arbitrary-length buffer
are left unfixed rather than risk corruption or a swallowed panic.`,
		Before: `for i := range src {
	dst[i] = src[i]
}`,
		After: `copy(dst, src)`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS4101",
		Doc:  "element-copy loop replaceable by copy()",
		Run:  runPS4101,
	},
})

func runPS4101(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			dst, src, ok := copyLoopShape(n)
			if !ok {
				return true
			}
			// Both sides must be slices (not maps/strings), same element
			// type.
			dt := pass.TypesInfo.TypeOf(dstExprOf(n))
			st := pass.TypesInfo.TypeOf(srcExprOf(n))
			ds, ok1 := underlyingSlice(dt)
			ss, ok2 := underlyingSlice(st)
			if !ok1 || !ok2 || !types.Identical(ds.Elem(), ss.Elem()) {
				return true
			}
			// Bit-identical guard: copy() diverges from a forward element loop
			// two ways — aliasing (overlapping same-array slices: the loop
			// propagates, copy memmoves) and length (the loop panics when
			// len(dst)<len(src); copy truncates). Fire only on the one shape that
			// rules out both: dst is a fresh, non-escaping make([]T, len(src)).
			dstIdent, _ := dstExprOf(n).(*ast.Ident)
			srcIdent, _ := srcExprOf(n).(*ast.Ident)
			if dstIdent == nil || srcIdent == nil ||
				!provablySafeLoopCopy(pass, n, dstIdent, srcIdent) {
				return true
			}
			// The rewrite calls the builtin copy. If `copy` is shadowed at the
			// fix site (a local var, a package-level func, an import alias), the
			// emitted copy(dst, src) would call the wrong thing or fail to
			// compile — so verify it still resolves to the universe builtin.
			if !builtinInScope(pass, n.Pos(), "copy") {
				return true
			}
			loop := n.(ast.Stmt)
			pass.Report(analysis.Diagnostic{
				Pos:     loop.Pos(),
				End:     loop.End(),
				Message: fmt.Sprintf("element-copy loop from %s to %s is a single memmove; replace with copy(%s, %s)", src, dst, dst, src),
				SuggestedFixes: []analysis.SuggestedFix{{
					Message: fmt.Sprintf("replace loop with copy(%s, %s)", dst, src),
					TextEdits: []analysis.TextEdit{
						{Pos: loop.Pos(), End: loop.End(), NewText: fmt.Appendf(nil, "copy(%s, %s)", dst, src)},
					},
				}},
			})
			return true
		})
	}
	return nil, nil
}

// copyLoopShape matches
//
//	for i := range src { dst[i] = src[i] }
//	for i, v := range src { dst[i] = v }
//	for i := 0; i < len(src); i++ { dst[i] = src[i] }
//
// returning the dst and src identifier names.
func copyLoopShape(n ast.Node) (dst, src string, ok bool) {
	switch l := n.(type) {
	case *ast.RangeStmt:
		// The key must be DEFINED by the range (i := range), not assigned to a
		// pre-declared variable (i = range) whose final value would survive the
		// loop — deleting the loop would drop that observable side effect.
		if l.Tok != token.DEFINE {
			return "", "", false
		}
		srcID, o := l.X.(*ast.Ident)
		if !o {
			return "", "", false
		}
		key, o := l.Key.(*ast.Ident)
		if !o || key.Name == "_" || len(l.Body.List) != 1 {
			return "", "", false
		}
		as, o := l.Body.List[0].(*ast.AssignStmt)
		if !o || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Tok.String() != "=" {
			return "", "", false
		}
		ix, o := as.Lhs[0].(*ast.IndexExpr)
		if !o {
			return "", "", false
		}
		dstID, o := ix.X.(*ast.Ident)
		if !o || dstID.Name == srcID.Name {
			return "", "", false
		}
		if idxID, o := ix.Index.(*ast.Ident); !o || idxID.Name != key.Name {
			return "", "", false
		}
		// RHS: src[i], or the range value variable.
		switch rhs := as.Rhs[0].(type) {
		case *ast.IndexExpr:
			rx, o := rhs.X.(*ast.Ident)
			if !o || rx.Name != srcID.Name {
				return "", "", false
			}
			if ri, o := rhs.Index.(*ast.Ident); !o || ri.Name != key.Name {
				return "", "", false
			}
			return dstID.Name, srcID.Name, true
		case *ast.Ident:
			if val, o := l.Value.(*ast.Ident); o && val.Name == rhs.Name && rhs.Name != "_" {
				return dstID.Name, srcID.Name, true
			}
		}
		return "", "", false
	case *ast.ForStmt:
		// i := 0; i < len(src); i++
		init, o := l.Init.(*ast.AssignStmt)
		if !o || init.Tok != token.DEFINE || len(init.Lhs) != 1 {
			return "", "", false // Init must DEFINE i, so it is loop-local
		}
		iv, o := init.Lhs[0].(*ast.Ident)
		if !o {
			return "", "", false
		}
		lit, o := init.Rhs[0].(*ast.BasicLit)
		if !o || lit.Value != "0" {
			return "", "", false
		}
		// Post must be exactly i++ — any other post statement (or none) means
		// the for is not a plain counted copy, and replacing it with copy()
		// would drop that side effect.
		inc, o := l.Post.(*ast.IncDecStmt)
		if !o || inc.Tok != token.INC {
			return "", "", false
		}
		if pid, o := inc.X.(*ast.Ident); !o || pid.Name != iv.Name {
			return "", "", false
		}
		cond, o := l.Cond.(*ast.BinaryExpr)
		if !o || cond.Op.String() != "<" {
			return "", "", false
		}
		lenCall, o := cond.Y.(*ast.CallExpr)
		if !o || !calleeIsLen(lenCall) || len(lenCall.Args) != 1 {
			return "", "", false
		}
		srcID, o := lenCall.Args[0].(*ast.Ident)
		if !o || len(l.Body.List) != 1 {
			return "", "", false
		}
		as, o := l.Body.List[0].(*ast.AssignStmt)
		if !o || len(as.Lhs) != 1 || len(as.Rhs) != 1 || as.Tok.String() != "=" {
			return "", "", false
		}
		ix, o := as.Lhs[0].(*ast.IndexExpr)
		if !o {
			return "", "", false
		}
		dstID, o := ix.X.(*ast.Ident)
		if !o || dstID.Name == srcID.Name {
			return "", "", false
		}
		idxID, o := ix.Index.(*ast.Ident)
		if !o || idxID.Name != iv.Name {
			return "", "", false
		}
		rhs, o := as.Rhs[0].(*ast.IndexExpr)
		if !o {
			return "", "", false
		}
		rx, o := rhs.X.(*ast.Ident)
		if !o || rx.Name != srcID.Name {
			return "", "", false
		}
		if ri, o := rhs.Index.(*ast.Ident); !o || ri.Name != iv.Name {
			return "", "", false
		}
		return dstID.Name, srcID.Name, true
	}
	return "", "", false
}

// dstExprOf / srcExprOf re-locate the dst/src expressions for typing.
func dstExprOf(n ast.Node) ast.Expr {
	body := loopBodyOf(n.(ast.Stmt))
	as := body.List[0].(*ast.AssignStmt)
	return as.Lhs[0].(*ast.IndexExpr).X
}

func srcExprOf(n ast.Node) ast.Expr {
	switch l := n.(type) {
	case *ast.RangeStmt:
		return l.X
	case *ast.ForStmt:
		return l.Cond.(*ast.BinaryExpr).Y.(*ast.CallExpr).Args[0]
	}
	return nil
}

func underlyingSlice(t types.Type) (*types.Slice, bool) {
	if t == nil {
		return nil, false
	}
	s, ok := t.Underlying().(*types.Slice)
	return s, ok
}

// provablyDistinctBacking reports whether copy(dst, src) is PROVABLY safe at
// this loop — i.e. dst and src cannot reference the same backing array, so the
// memmove that copy performs cannot differ from the forward element loop.
//
// Deciding non-aliasing in general is undecidable, and a "fire unless proven
// aliased" design is unsafe: any provenance the analysis cannot follow (a call,
// a pointer deref, a package-level var, a struct field) would fail OPEN and emit
// a corrupting fix (the classic LZ/RLE overlapping back-reference). So this is a
// SOUND, deliberately narrow sufficient condition that fails CLOSED: it fires
// only when at least one operand is a FRESH, non-escaping local buffer. A buffer
// created in this function by make()/[]T{...}/[]byte(...) whose backing never
// leaves the variable (never passed to a call, addressed, re-sliced into another
// variable, assigned elsewhere, or reassigned) cannot be aliased by anything, so
// the OTHER operand provably has different backing. Everything else stays unfixed.
//
// Objects are compared by types.Object identity, never by name, so shadowed or
// sibling variables sharing a name are never conflated.
func provablySafeLoopCopy(pass *analysis.Pass, loop ast.Node, dstIdent, srcIdent *ast.Ident) bool {
	body := enclosingFuncBody(pass, loop)
	if body == nil {
		return false
	}
	dstObj := pass.TypesInfo.ObjectOf(dstIdent)
	srcObj := pass.TypesInfo.ObjectOf(srcIdent)
	if dstObj == nil || srcObj == nil || dstObj == srcObj {
		return false
	}
	// src's length must be STABLE between dst's make and the loop, or len(dst)
	// could no longer cover it. It can change out from under us three ways:
	//   - src is a package-level var a callee can reassign (grow via append);
	//   - src's address is taken (&src), letting a callee reassign it;
	//   - src is reassigned in this function.
	if srcObj.Parent() == pass.Pkg.Scope() || addrTaken(pass, body, srcObj) ||
		objReassigned(pass, body, srcObj) {
		return false
	}
	return dstIsPrivateMakeSizedFromSrc(pass, body, loop, dstObj, srcObj)
}

// addrTaken reports whether &obj (the whole variable) is taken anywhere in body,
// which would let a callee reassign it (and thus change its length). &obj[i]
// (address of an element) does not qualify — it cannot change obj's length.
func addrTaken(pass *analysis.Pass, body *ast.BlockStmt, obj types.Object) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if u, ok := n.(*ast.UnaryExpr); ok && u.Op == token.AND {
			if id, ok := u.X.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == obj {
				found = true
			}
		}
		return true
	})
	return found
}

// objReassigned reports whether obj is assigned to beyond a single binding — it
// appears on the LHS of a plain assignment, is defined more than once, or is an
// inc-dec / range target. A never-reassigned variable has a stable length.
func objReassigned(pass *analysis.Pass, body *ast.BlockStmt, obj types.Object) bool {
	defines, reassigned := 0, false
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || pass.TypesInfo.ObjectOf(id) != obj {
					continue
				}
				if s.Tok == token.DEFINE {
					defines++
				} else {
					reassigned = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := s.X.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == obj {
				reassigned = true
			}
		case *ast.RangeStmt:
			for _, k := range []ast.Expr{s.Key, s.Value} {
				if id, ok := k.(*ast.Ident); ok && pass.TypesInfo.ObjectOf(id) == obj {
					reassigned = true
				}
			}
		}
		return true
	})
	return reassigned || defines > 1
}

// dstIsPrivateMakeSizedFromSrc reports whether dst (obj) is a local variable
// that (1) is bound exactly once, by make([]T, len(src)) or make([]T, cap(src))
// — so len(dst) >= len(src) and neither the loop nor copy can panic where the
// other does not — and (2) never escapes: every occurrence of dst is that
// binding, an element access dst[i], a len/cap argument, a `range dst`, an
// in-place reslice dst = dst[...], a `return dst`, or inside the copy loop's
// BODY. Any other use (call argument, address-of, &dst[i], assignment to another
// variable, reassignment, composite element, a use in the loop's Init/Post, …)
// is an escape and makes the result false — after which src provably cannot come
// to share dst's backing. The allow-list keeps this SOUND: unrecognised uses
// fail closed. Uses are NOT exempted by lexical position — the loop may
// re-execute (enclosing loop / goto), so a later statement can run before the
// next copy.
func dstIsPrivateMakeSizedFromSrc(pass *analysis.Pass, body *ast.BlockStmt, loop ast.Node, obj, srcObj types.Object) bool {
	if _, ok := obj.(*types.Var); !ok || obj == nil {
		return false
	}
	loopBody := loopBodyOf(loop.(ast.Stmt))
	if loopBody == nil {
		return false
	}
	sized, escapes := false, false
	var stack []ast.Node
	ast.Inspect(body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		defer func() { stack = append(stack, n) }()
		id, ok := n.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(id) != obj {
			return true
		}
		// Occurrence inside the copy loop's BODY (dst[i] = src[i]): always safe,
		// it is the copy we are replacing. NOTE only the body is exempt — an
		// assignment in the loop's Init/Post (e.g. a counted-for post clause
		// reassigning dst) still counts as an escape.
		if loopBody.Pos() <= id.Pos() && id.End() <= loopBody.End() {
			return true
		}
		var parent, grand ast.Node
		if len(stack) > 0 {
			parent = stack[len(stack)-1]
		}
		if len(stack) > 1 {
			grand = stack[len(stack)-2]
		}
		// Any escape may enable aliasing. We do NOT exempt uses that are
		// lexically after the loop: the loop can re-execute (an enclosing loop,
		// a goto, a labeled continue), so a "later" statement can run before the
		// next copy. Only return and pure reads are treated as safe.
		esc := func() { escapes = true }
		switch p := parent.(type) {
		case *ast.ReturnStmt:
			// return obj hands the buffer to the caller after the loop and
			// cannot make src/dst alias within this function. Safe.
		case *ast.AssignStmt:
			// obj on the LHS. The sole sized-make binding (:=) is allowed; any
			// other assignment to obj changes/leaks its backing -> escape.
			for i, lhs := range p.Lhs {
				if lhs == ast.Node(id) {
					if p.Tok == token.DEFINE && i < len(p.Rhs) && isMakeSizedFrom(pass, p.Rhs[i], srcObj) {
						sized = true
					} else {
						esc()
					}
					return true
				}
			}
			// obj on the RHS assigned to some OTHER variable -> leaks backing.
			esc()
		case *ast.ValueSpec:
			// var obj = make([]T, len(src)) : the sized binding via a declaration.
			for i, nm := range p.Names {
				if nm == id {
					if i < len(p.Values) && isMakeSizedFrom(pass, p.Values[i], srcObj) {
						sized = true
					} else {
						esc()
					}
					return true
				}
			}
			esc() // appears as an initializer value for another var
		case *ast.IndexExpr:
			if p.X != ast.Expr(id) { // obj used as an index expression -> escape
				esc()
				return true
			}
			// obj[i] element access is safe, EXCEPT &obj[i], which leaks a
			// pointer into the backing that could be used to alias it.
			if u, ok := grand.(*ast.UnaryExpr); ok && u.Op == token.AND {
				esc()
			}
		case *ast.CallExpr:
			// Only len(obj)/cap(obj) are safe; every other call leaks obj.
			if fn, ok := p.Fun.(*ast.Ident); ok && (fn.Name == "len" || fn.Name == "cap") && p.Fun != ast.Expr(id) {
				return true
			}
			esc()
		case *ast.RangeStmt:
			if p.X != ast.Expr(id) { // obj as range key/value target -> escape
				esc()
			}
		case *ast.SliceExpr:
			// obj[a:b]. Safe as `return obj[a:b]` (hands a view out after the
			// loop) or as an in-place self-reslice obj = obj[a:b]; anything else
			// feeds the reslice to another variable / expression -> escape.
			if p.X != ast.Expr(id) {
				esc()
				return true
			}
			if _, ok := grand.(*ast.ReturnStmt); ok {
				return true
			}
			as, ok := grand.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 {
				esc()
				return true
			}
			lid, ok := as.Lhs[0].(*ast.Ident)
			if !ok || pass.TypesInfo.ObjectOf(lid) != obj {
				esc()
			}
		default:
			esc()
		}
		return true
	})
	return sized && !escapes
}

// isMakeSizedFrom reports whether e is make([]T, len(src)) or make([]T, cap(src))
// — the builtin make whose length argument is len/cap of srcObj — so the result
// has len >= len(src). The builtin identities of make and len/cap are verified
// through the type info (a shadowed user-defined make/len does not count), and
// the length argument must name exactly srcObj.
func isMakeSizedFrom(pass *analysis.Pass, e ast.Expr, srcObj types.Object) bool {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) < 2 {
		return false // make([]T) with no length is len 0 -> not sized
	}
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "make" {
		return false
	}
	if b, ok := pass.TypesInfo.Uses[fn].(*types.Builtin); !ok || b.Name() != "make" {
		return false
	}
	// Length argument (Args[1]) must be len(src) or cap(src) for the SAME src.
	lc, ok := call.Args[1].(*ast.CallExpr)
	if !ok || len(lc.Args) != 1 {
		return false
	}
	lf, ok := lc.Fun.(*ast.Ident)
	if !ok || (lf.Name != "len" && lf.Name != "cap") {
		return false
	}
	if b, ok := pass.TypesInfo.Uses[lf].(*types.Builtin); !ok || (b.Name() != "len" && b.Name() != "cap") {
		return false
	}
	argID, ok := lc.Args[0].(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(argID) == srcObj
}

// enclosingFuncBody returns the body of the innermost function (declaration or
// literal) that directly encloses target — the scope in which target's operands
// are bound — or nil if none does.
func enclosingFuncBody(pass *analysis.Pass, target ast.Node) *ast.BlockStmt {
	for _, f := range pass.Files {
		if target.Pos() < f.Pos() || target.Pos() > f.End() {
			continue
		}
		var found *ast.BlockStmt
		ast.Inspect(f, func(n ast.Node) bool {
			if n == nil {
				return false
			}
			var body *ast.BlockStmt
			switch fn := n.(type) {
			case *ast.FuncDecl:
				body = fn.Body
			case *ast.FuncLit:
				body = fn.Body
			}
			if body != nil && body.Pos() <= target.Pos() && target.End() <= body.End() {
				found = body // innermost enclosing wins (deeper matches overwrite)
			}
			return true
		})
		return found
	}
	return nil
}
