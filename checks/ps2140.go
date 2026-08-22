package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS2140 reports operation functions whose result allocation dominates their
// cost: they make an input-sized []T buffer, fully overwrite it, and return
// it, so a hot caller can never reuse the buffer. A caller-owned Into/out
// variant lets the caller supply (and reuse) the destination.
//
// Domain check: which element types mark a "hot operation output" is project
// vocabulary (config.outputBufferElemTypes), because Go has no language-level
// signal for it. With no vocabulary the check stays silent.
var PS2140 = register(&lint.Check{
	ID:          "PS2140",
	Category:    "alloc",
	Slug:        "output-alloc-dominated",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"outputBufferElemTypes"},
	Doc: lint.Documentation{
		Title: "an operation allocates an input-sized result it fully overwrites and returns; a caller-owned Into variant lets hot callers reuse the buffer",
		Text: `A hot elementwise operation can spend nearly all of its allocation
volume on a single result buffer: it allocates a slice whose size derives from
its input, fully overwrites every element, and returns the slice. The payload
is unavoidable the first time, but a caller that runs the operation in a loop
(an inference step, a training iteration) pays the whole allocation again on
every call, and the buffers become immediate garbage. Pooling below the
operation cannot fix this safely — ownership of the returned slice belongs to
the caller — but an explicit destination does: an Into/out variant
(FooInto(dst, x) or Foo(x, dst)) that writes into a caller-supplied slice lets
the caller allocate once and reuse.

This is advisory only — there is no safe mechanical fix, because adding a
destination parameter is an API change with lifetime, aliasing and
zero-initialization contracts only the author can weigh — so PS2140 never
attaches a SuggestedFix.

The match is deliberately narrow, to keep it near-zero false positive:

  - the function returns a slice []T whose element type T is named in
    config.outputBufferElemTypes (the opt-in; empty -> silent);
  - its body binds dst := make([]T, n) (or make([]T, n, c)) where n derives
    from a parameter (a parameter identifier or len(param)); a constant or
    literal size is not input-derived and is skipped;
  - every use of dst is a plain write dst[i] = v (token "=", never += which
    would read the zero value) inside exactly ONE loop that ranges over all of
    dst (for i := range dst, or for i := 0; i < len(dst); i++), with the write
    index equal to the loop variable — this proves a full overwrite with no
    read of the zero value, so zero-initialization is never observable and the
    write is not partial;
  - dst is returned directly (return dst, not return f(dst) or return dst[1:]);
  - the package (or the receiver type, for a method) has no <Name>Into sibling
    already, and the function is not a New*/Make* constructor.

Anything outside this shape — a read of dst, a slice of it, passing it to a
call, a second loop, a partial write, a non-input size — drops the candidate.`,
		Before: `// no destination: the caller cannot reuse the buffer
func SiLU(x []float64) []float64 {
	out := make([]float64, len(x))
	for i := range out {
		out[i] = x[i] / (1 + math.Exp(-x[i]))
	}
	return out
}`,
		After: `// caller-owned destination: allocate once, reuse across calls
func SiLUInto(dst, x []float64) {
	for i := range dst {
		dst[i] = x[i] / (1 + math.Exp(-x[i]))
	}
}`,
		MeasuredWin: `Benchmark the Into form against the allocating form with
ReportAllocs and a preallocated destination reused across b.N iterations. For a
[256,1408] float64 output the allocating op measures ~2.88 MB/op (the returned
payload) that the Into form removes entirely, eliminating the per-call
allocation and the GC pressure it creates in inference loops.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2140",
		Doc:  "operation allocates an input-sized result it fully overwrites and returns; recommend a caller-owned Into variant",
		Run:  runPS2140,
	},
})

func runPS2140(pass *analysis.Pass) (any, error) {
	elems := config.Current().OutputBufferElemTypes
	if len(elems) == 0 {
		return nil, nil // opt-in: no vocabulary, no reports
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			// Skip constructors: their job is to allocate a fresh value.
			if strings.HasPrefix(fn.Name.Name, "New") || strings.HasPrefix(fn.Name.Name, "Make") {
				continue
			}
			if ps2140HasIntoSibling(pass, fn) {
				continue
			}
			call, elemName := ps2140Candidate(pass, fn, elems)
			if call == nil {
				continue
			}
			pass.Report(analysis.Diagnostic{
				Pos:     call.Pos(),
				End:     call.End(),
				Message: "operation allocates an input-sized []" + elemName + " result, fully overwrites it, and returns it; hot callers cannot reuse the buffer — consider a caller-owned " + fn.Name.Name + "Into variant that writes a supplied destination",
			})
		}
	}
	return nil, nil
}

// ps2140Candidate returns the make(...) call and element-type name of the
// output buffer when fn matches the output-allocation-dominated shape, or
// (nil, "") otherwise.
func ps2140Candidate(pass *analysis.Pass, fn *ast.FuncDecl, elems map[string]bool) (*ast.CallExpr, string) {
	params := ps2140ParamObjs(pass, fn)
	if len(params) == 0 {
		return nil, "" // size cannot derive from an input that does not exist
	}
	// Find each `dst := make([]T, n...)` (or `dst = make(...)`) binding.
	var found *ast.CallExpr
	var foundElem string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
			return true
		}
		lhs, ok := as.Lhs[0].(*ast.Ident)
		if !ok {
			return true
		}
		call, ok := as.Rhs[0].(*ast.CallExpr)
		if !ok || !ps2140IsMake(pass, call) || len(call.Args) < 2 {
			return true
		}
		// Element type of the []T being made, from type info.
		st, ok := pass.TypesInfo.TypeOf(call).(*types.Slice)
		if !ok {
			return true
		}
		elemName := ps2140ElemName(st.Elem())
		if elemName == "" || !elems[elemName] {
			return true
		}
		// Size (Args[1]) must derive from a parameter.
		if !ps2140SizeFromParam(pass, call.Args[1], params) {
			return true
		}
		obj := pass.TypesInfo.ObjectOf(lhs)
		if obj == nil {
			return true
		}
		if ps2140FullOverwriteReturn(pass, fn, obj) {
			found, foundElem = call, elemName
			return false
		}
		return true
	})
	return found, foundElem
}

// ps2140FullOverwriteReturn reports whether every use of obj in fn is a plain
// indexed write inside a single full-range loop over obj plus a direct return
// of obj — the shape that proves an input-sized buffer is fully overwritten
// (no zero-value read, no partial write) and handed back.
func ps2140FullOverwriteReturn(pass *analysis.Pass, fn *ast.FuncDecl, obj types.Object) bool {
	var (
		writeLoops = map[ast.Node]bool{} // distinct loops holding a write
		returned   bool
		writes     int
		ok         = true
	)
	astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
		if !ok {
			return false
		}
		id, isID := n.(*ast.Ident)
		if !isID || pass.TypesInfo.ObjectOf(id) != obj {
			return true
		}
		parent := stack[len(stack)-1]
		switch p := parent.(type) {
		case *ast.AssignStmt:
			// The defining `dst := make(...)` binding: id is on the LHS.
			for _, l := range p.Lhs {
				if l == id {
					return true // the definition itself — fine
				}
			}
			ok = false // any other bare-ident assignment use (re-read/re-assign)
			return false
		case *ast.ReturnStmt:
			returned = true // `return ... dst ...` directly
			return true
		case *ast.RangeStmt:
			if p.X == id {
				return true // `for i := range dst` — the overwrite loop's header
			}
			ok = false
			return false
		case *ast.CallExpr:
			// `len(dst)` / `cap(dst)` in the for-loop condition are structural,
			// not reads of element data. Any other call means dst escapes.
			if fnID, isID := p.Fun.(*ast.Ident); isID && (fnID.Name == "len" || fnID.Name == "cap") {
				return true
			}
			ok = false
			return false
		case *ast.IndexExpr:
			if p.X != id {
				ok = false // dst used as an index value, not the container
				return false
			}
			// Must be the LHS container of a plain `dst[i] = v` assignment.
			if len(stack) < 2 {
				ok = false
				return false
			}
			gp, isAssign := stack[len(stack)-2].(*ast.AssignStmt)
			if !isAssign || gp.Tok != token.ASSIGN || !ps2140IndexIsLHS(gp, p) {
				ok = false // a read (RHS, or += which reads the zero value)
				return false
			}
			loop, inLoop := astutil.InLoop(stack)
			if !inLoop || !ps2140IndexMatchesLoop(pass, p.Index, loop, obj) {
				ok = false
				return false
			}
			writeLoops[loop] = true
			writes++
			return true
		default:
			ok = false // any other use (slice, &dst, call arg, ...) disqualifies
			return false
		}
	})
	return ok && returned && writes > 0 && len(writeLoops) == 1
}

// ps2140IndexIsLHS reports whether the IndexExpr is one of the assignment's
// left-hand targets.
func ps2140IndexIsLHS(as *ast.AssignStmt, ix *ast.IndexExpr) bool {
	for _, l := range as.Lhs {
		if l == ix {
			return true
		}
	}
	return false
}

// ps2140IndexMatchesLoop reports whether index is exactly the loop variable of
// a full-range loop over obj: `for i := range dst` or
// `for i := 0; i < len(dst); i++`.
func ps2140IndexMatchesLoop(pass *analysis.Pass, index ast.Expr, loop ast.Node, obj types.Object) bool {
	idxID, ok := index.(*ast.Ident)
	if !ok {
		return false
	}
	idxObj := pass.TypesInfo.ObjectOf(idxID)
	switch l := loop.(type) {
	case *ast.RangeStmt:
		// for i := range dst { ... } — Key is i, X is dst, no Value.
		if l.Value != nil {
			return false
		}
		key, ok := l.Key.(*ast.Ident)
		if !ok || pass.TypesInfo.ObjectOf(key) != idxObj {
			return false
		}
		x, ok := l.X.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(x) == obj
	case *ast.ForStmt:
		// for i := 0; i < len(dst); i++ { ... }
		return ps2140IsCanonicalForOver(pass, l, idxObj, obj)
	}
	return false
}

// ps2140IsCanonicalForOver matches `for i := 0; i < len(dst); i++`.
func ps2140IsCanonicalForOver(pass *analysis.Pass, l *ast.ForStmt, idxObj, obj types.Object) bool {
	// init: i := 0
	init, ok := l.Init.(*ast.AssignStmt)
	if !ok || len(init.Lhs) != 1 || len(init.Rhs) != 1 {
		return false
	}
	initID, ok := init.Lhs[0].(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(initID) != idxObj {
		return false
	}
	if lit, ok := init.Rhs[0].(*ast.BasicLit); !ok || lit.Value != "0" {
		return false
	}
	// cond: i < len(dst)
	cond, ok := l.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.LSS {
		return false
	}
	lo, ok := cond.X.(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(lo) != idxObj {
		return false
	}
	lenCall, ok := cond.Y.(*ast.CallExpr)
	if !ok || len(lenCall.Args) != 1 {
		return false
	}
	if fnID, ok := lenCall.Fun.(*ast.Ident); !ok || fnID.Name != "len" {
		return false
	}
	arg, ok := lenCall.Args[0].(*ast.Ident)
	if !ok || pass.TypesInfo.ObjectOf(arg) != obj {
		return false
	}
	// post: i++
	post, ok := l.Post.(*ast.IncDecStmt)
	if !ok || post.Tok != token.INC {
		return false
	}
	postID, ok := post.X.(*ast.Ident)
	return ok && pass.TypesInfo.ObjectOf(postID) == idxObj
}

// ps2140ParamObjs collects the parameter variable objects of fn.
func ps2140ParamObjs(pass *analysis.Pass, fn *ast.FuncDecl) map[types.Object]bool {
	out := map[types.Object]bool{}
	if fn.Type.Params == nil {
		return out
	}
	for _, field := range fn.Type.Params.List {
		for _, name := range field.Names {
			if obj := pass.TypesInfo.ObjectOf(name); obj != nil {
				out[obj] = true
			}
		}
	}
	return out
}

// ps2140SizeFromParam reports whether size references a parameter (directly or
// via len(param)/arithmetic on it).
func ps2140SizeFromParam(pass *analysis.Pass, size ast.Expr, params map[types.Object]bool) bool {
	found := false
	ast.Inspect(size, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			if params[pass.TypesInfo.ObjectOf(id)] {
				found = true
			}
		}
		return !found
	})
	return found
}

// ps2140IsMake reports whether call is the builtin make.
func ps2140IsMake(pass *analysis.Pass, call *ast.CallExpr) bool {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "make" {
		return false
	}
	// Builtin, not a shadowing local named make.
	b, ok := pass.TypesInfo.ObjectOf(id).(*types.Builtin)
	return ok && b.Name() == "make"
}

// ps2140ElemName returns the type name for membership testing: a basic type's
// name (float64) or a named type's name (MyScalar); "" for anything else.
func ps2140ElemName(t types.Type) string {
	switch e := t.(type) {
	case *types.Basic:
		return e.Name()
	case *types.Named:
		return e.Obj().Name()
	}
	return ""
}

// ps2140HasIntoSibling reports whether an <Name>Into variant already exists —
// a package-level function for a plain func, or a method on the same receiver
// type for a method — in which case the destination API is already offered.
func ps2140HasIntoSibling(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	into := fn.Name.Name + "Into"
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return pass.Pkg.Scope().Lookup(into) != nil
	}
	// Method: look for the same-named method on the receiver's named type.
	recvType := pass.TypesInfo.TypeOf(fn.Recv.List[0].Type)
	if recvType == nil {
		return false
	}
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
	}
	named, ok := recvType.(*types.Named)
	if !ok {
		return false
	}
	for i := 0; i < named.NumMethods(); i++ {
		if named.Method(i).Name() == into {
			return true
		}
	}
	return false
}
