package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/lint"
	"golang.org/x/tools/go/analysis"
)

var PS6137 = register(&lint.Check{
	ID: "PS6137", Category: "alloc", Slug: "native-snapshot-caller-owned-output", Level: lint.LevelAggressive,
	AutoFix: false, NeedsConfig: true, Vocab: []string{"nativeSnapshotReuseContracts"},
	Doc: lint.Documentation{
		Title: "native snapshot acquisition freshly materializes owned output",
		Text: `PS6137 is opt-in and requires exact typed nativeSnapshotReuseContracts.
Source acquisition, pointer/count/status/scalar roles and closed materialization
and owning-string helper bodies are distinct from reviewed native lifetime,
readonly extraction, token semantics and repeated-use policy. These opaque
facts are not inferred from names or configuration acknowledgements.

The supported closed grammar has a checked signed native count, a count==1
C.GoString/composite-slice fast branch, and a same-count make/unsafe.Slice path
whose second token acquisition is validated before its typed fill call. That
auxiliary acquisition may fail after fresh allocation. The fill has fresh
zero extraction-local cache state, canonical indexed event/scalar fields and
optional EventSpan; the owning helper has inline token/content caches plus
overflow maps, a bounded NUL scan and strings.Clone. Native label extent is
checked against its array type; termination, native storage length/lifetime
and token meaning remain reviewed project facts. Other control/helper shapes,
callbacks, aliases/rebinding/escapes, borrowed keys/returns, pointer arithmetic
and unknown calls remain silent. Direct slices with the same supported grammar
and struct-with-slice results are supported; this is not every cgo output API.

There is no automatic fix. Consider an additive caller-owned Into API while
preserving the allocating convenience API. Perform all fallible acquisition
and validation before mutating caller storage; allow capacity growth, then
reuse sufficient capacity. Reuse only existing Go-owned strings after exact
content equality, never return or retain borrowed native views. Verify every
scalar and EventSpan, zero/one/many counts, changed and reordered labels,
cache overflow, errors leaving the destination unchanged and ownership after
native release. Measure warm/cold workloads and convenience-API nonregression.
An exact supported Into sibling changes the advice, but its signature alone
does not establish reuse, parity, error atomicity or a performance gain.
A compact by-value native snapshot view is separate platform/ABI-sensitive
benchmark advice, not a safe rewrite or a source-proven optimization.`,
		Before: "// Checked native snapshot copied into newly allocated Go-owned output.",
		After:  "// Project-reviewed additive Into API; no automatic rewrite.",
		MeasuredWin: `GoAI issue856 / PR1184 reports three order-alternated count-seven
340-event campaigns on Apple M2 Pro: Profile medians2.713/2.735/2.786us,
ProfileInto1.697/1.691/1.827us (1.60x/1.62x/1.52x),14400B/6allocs to0B/0allocs.
These are owner native measurements, not a universal or perfscan-measured gain.
The allocating Profile remains unchanged; native view ABI gains cannot be
isolated from caller-owned output and exact owned-label reuse by these numbers.`,
	},
	Analyzer: &analysis.Analyzer{Name: "PS6137", Doc: "checked native snapshots freshly materialize owned output", Run: runPS6137},
})

func runPS6137(pass *analysis.Pass) (any, error) {
	return runPS6137WithContracts(pass, config.Current().NativeSnapshotReuseContracts)
}

type ps6137Function struct {
	file *ast.File
	decl *ast.FuncDecl
	fn   *types.Func
	sig  *types.Signature
}

func ps6137Functions(pass *analysis.Pass) map[string]ps6137Function {
	out := make(map[string]ps6137Function)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			f, ok := decl.(*ast.FuncDecl)
			if !ok || f.Body == nil {
				continue
			}
			fn, ok := pass.TypesInfo.Defs[f.Name].(*types.Func)
			if !ok {
				continue
			}
			sig, ok := fn.Type().(*types.Signature)
			if ok {
				out[ps6090FunctionID(fn)] = ps6137Function{file, f, fn, sig}
			}
		}
	}
	return out
}

func ps6137Call(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, id string) bool {
	kind := config.NativeSnapshotCallFunction
	if strings.HasPrefix(id, "C.") {
		kind = config.NativeSnapshotCallCgo
	}
	return ps6110CallMatches(pass, file, call, id, kind)
}

func ps6137CallID(pass *analysis.Pass, call *ast.CallExpr) string {
	if selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr); ok {
		if qualifier, ok := selector.X.(*ast.Ident); ok {
			if pkg, ok := identObject(pass, qualifier).(*types.PkgName); ok && pkg.Imported().Path() == "unsafe" {
				if builtin, ok := identObject(pass, selector.Sel).(*types.Builtin); ok {
					return "unsafe." + builtin.Name()
				}
			}
		}
	}
	return ps6087FunctionID(pass, call)
}

func ps6137Local(pass *analysis.Pass, e ast.Expr) *types.Var {
	id, ok := ps2110Unparen(e).(*ast.Ident)
	if !ok {
		return nil
	}
	v, _ := identObject(pass, id).(*types.Var)
	return v
}

func ps6137AddressLocal(pass *analysis.Pass, e ast.Expr) *types.Var {
	u, ok := ps2110Unparen(e).(*ast.UnaryExpr)
	if !ok || u.Op != token.AND {
		return nil
	}
	return ps6137Local(pass, u.X)
}

func ps6137Integer(t types.Type) bool {
	b, ok := types.Unalias(t).Underlying().(*types.Basic)
	return ok && b.Info()&types.IsInteger != 0
}

// A tracked native pointer is a local pointer-to-record obtained only through
// its configured native out-argument. All scalar roles are distinct integer
// locals; validating signatures prevents interface/opaque ABI stand-ins.
type ps6137Acquisition struct {
	index                  int
	call                   *ast.CallExpr
	status, pointer, count *types.Var
	scalars                []*types.Var
}

func ps6137Acquire(pass *analysis.Pass, f ps6137Function, c *config.NativeSnapshotReuseContract) (ps6137Acquisition, bool) {
	var a ps6137Acquisition
	for i, stmt := range f.decl.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}
		call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
		if !ok || !ps6137Call(pass, f.file, call, c.AcquireCallable) {
			continue
		}
		if _, generated := ps2110Unparen(call.Fun).(*ast.FuncLit); generated {
			call, ok = ps6137GeneratedArguments(pass, call)
			if !ok {
				return a, false
			}
		}
		if a.call != nil || call.Ellipsis.IsValid() || len(call.Args) != 3+len(c.ScalarOutArguments) {
			return a, false
		}
		a.index = i
		a.call = call
		a.status = ps6137Local(pass, assign.Lhs[0])
		a.pointer = ps6137AddressLocal(pass, call.Args[c.PointerOutArgument])
		a.count = ps6137AddressLocal(pass, call.Args[c.CountOutArgument])
		if a.status == nil || a.pointer == nil || a.count == nil || !ps6137Integer(a.status.Type()) || !ps6137Integer(a.count.Type()) {
			return a, false
		}
		if basic, ok := types.Unalias(a.count.Type()).Underlying().(*types.Basic); !ok || basic.Info()&types.IsUnsigned != 0 {
			return a, false
		}
		p, ok := types.Unalias(a.pointer.Type()).(*types.Pointer)
		if !ok {
			return a, false
		}
		if _, ok := types.Unalias(p.Elem()).Underlying().(*types.Struct); !ok {
			return a, false
		}
		seen := map[*types.Var]bool{a.pointer: true, a.count: true, a.status: true}
		for _, role := range c.ScalarOutArguments {
			v := ps6137AddressLocal(pass, call.Args[role])
			if v == nil || seen[v] || !ps6137Integer(v.Type()) {
				return a, false
			}
			seen[v] = true
			a.scalars = append(a.scalars, v)
		}
		handle, ok := ps2110Unparen(call.Args[c.HandleArgument]).(*ast.SelectorExpr)
		if !ok || handle.Sel.Name != c.ReceiverHandleField || !ps6135Object(pass, handle.X, f.sig.Recv()) {
			return a, false
		}
		_, sig, ok := typedCallee(pass, call.Fun)
		if ok {
			if sig.Variadic() || sig.Results().Len() != 1 || sig.Params().Len() != len(call.Args) {
				return a, false
			}
			for j, arg := range call.Args {
				if !types.Identical(pass.TypesInfo.TypeOf(arg), sig.Params().At(j).Type()) {
					return a, false
				}
			}
		}
	}
	return a, a.call != nil
}

// Recognizing a sibling signature establishes existence only. It intentionally
// makes no statement about its body, destination atomicity, lifetime or cost.
func ps6137IntoSibling(candidate, sibling ps6137Function, result types.Type) bool {
	s := sibling.sig
	if s == nil || !sibling.fn.Exported() || s.TypeParams().Len() != 0 || s.Variadic() || s.Recv() == nil || !types.Identical(s.Recv().Type(), candidate.sig.Recv().Type()) {
		return false
	}
	errorType := types.Universe.Lookup("error").Type()
	if s.Params().Len() != 1 {
		return false
	}
	p, ok := types.Unalias(s.Params().At(0).Type()).(*types.Pointer)
	if ok && types.Identical(p.Elem(), result) {
		return s.Results().Len() == 1 && types.Identical(s.Results().At(0).Type(), errorType)
	}
	if _, ok := types.Unalias(result).Underlying().(*types.Slice); !ok {
		return false
	}
	return types.Identical(s.Params().At(0).Type(), result) && s.Results().Len() == 2 && types.Identical(s.Results().At(0).Type(), result) && types.Identical(s.Results().At(1).Type(), errorType)
}

func ps6137Result(f ps6137Function, c *config.NativeSnapshotReuseContract) (types.Type, *types.Slice, bool) {
	if f.sig.Recv() != nil {
		receiver := types.Unalias(f.sig.Recv().Type())
		p, ok := receiver.(*types.Pointer)
		if !ok {
			return nil, nil, false
		}
		named, ok := types.Unalias(p.Elem()).(*types.Named)
		if !ok || named.TypeParams().Len() != 0 {
			return nil, nil, false
		}
	}
	if f.sig.Recv() == nil || f.sig.TypeParams().Len() != 0 || f.sig.Variadic() || f.sig.Params().Len() != 0 || f.sig.Results().Len() != 2 || !types.Identical(f.sig.Results().At(1).Type(), types.Universe.Lookup("error").Type()) {
		return nil, nil, false
	}
	result := f.sig.Results().At(0).Type()
	recordSlice := result
	if c.ResultSliceField != "" {
		record, ok := types.Unalias(result).Underlying().(*types.Struct)
		if !ok {
			return nil, nil, false
		}
		found := false
		for i := 0; i < record.NumFields(); i++ {
			field := record.Field(i)
			if field.Name() == c.ResultSliceField && !field.Embedded() {
				recordSlice = field.Type()
				found = true
				break
			}
		}
		if !found {
			return nil, nil, false
		}
	}
	slice, ok := types.Unalias(recordSlice).Underlying().(*types.Slice)
	if !ok {
		return nil, nil, false
	}
	event, ok := types.Unalias(slice.Elem()).Underlying().(*types.Struct)
	if !ok {
		return nil, nil, false
	}
	for i := 0; i < event.NumFields(); i++ {
		field := event.Field(i)
		if field.Name() == c.ResultStringField && !field.Embedded() && types.Identical(field.Type(), types.Typ[types.String]) {
			return result, slice, true
		}
	}
	return nil, nil, false
}

func ps6137Zero(pass *analysis.Pass, e ast.Expr) bool {
	v := pass.TypesInfo.Types[ps2110Unparen(e)].Value
	return v != nil && v.Kind() == constant.Int && constant.Sign(v) == 0
}

func ps6137Binary(pass *analysis.Pass, e ast.Expr, op token.Token, left types.Object, rightZero bool) bool {
	b, ok := ps2110Unparen(e).(*ast.BinaryExpr)
	return ok && b.Op == op && ps6135Object(pass, b.X, left) && rightZero && ps6137Zero(pass, b.Y)
}

func ps6137Nil(pass *analysis.Pass, e ast.Expr) bool {
	id, ok := ps2110Unparen(e).(*ast.Ident)
	return ok && identObject(pass, id) == types.Universe.Lookup("nil")
}

func ps6137ErrorGuard(pass *analysis.Pass, stmt ast.Stmt, condition func(ast.Expr) bool) bool {
	g, ok := stmt.(*ast.IfStmt)
	if !ok || g.Init != nil || g.Else != nil || len(g.Body.List) != 1 || !condition(g.Cond) {
		return false
	}
	r, ok := g.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(r.Results) != 2 {
		return false
	}
	call, ok := ps2110Unparen(r.Results[1]).(*ast.CallExpr)
	if !ok || ps6137CallID(pass, call) != "fmt.Errorf" {
		return false
	}
	zero, ok := ps2110Unparen(r.Results[0]).(*ast.CompositeLit)
	if !ps6137Nil(pass, r.Results[0]) && (!ok || len(zero.Elts) != 0) {
		return false
	}
	_, sig, ok := typedCallee(pass, call.Fun)
	return ok && sig.Results().Len() == 1 && types.Identical(sig.Results().At(0).Type(), types.Universe.Lookup("error").Type())
}

func ps6137StorageCondition(pass *analysis.Pass, e ast.Expr, a ps6137Acquisition) bool {
	or, ok := ps2110Unparen(e).(*ast.BinaryExpr)
	if !ok || or.Op != token.LOR || !ps6137Binary(pass, or.X, token.LSS, a.count, true) {
		return false
	}
	and, ok := ps2110Unparen(or.Y).(*ast.BinaryExpr)
	if !ok || and.Op != token.LAND || !ps6137Binary(pass, and.X, token.GTR, a.count, true) {
		return false
	}
	eq, ok := ps2110Unparen(and.Y).(*ast.BinaryExpr)
	return ok && eq.Op == token.EQL && ps6135Object(pass, eq.X, a.pointer) && ps6137Nil(pass, eq.Y)
}

func ps6137AcquisitionGuards(pass *analysis.Pass, f ps6137Function, a ps6137Acquisition) bool {
	list := f.decl.Body.List
	if a.index+2 >= len(list) {
		return false
	}
	return ps6137ErrorGuard(pass, list[a.index+1], func(e ast.Expr) bool { return ps6137Binary(pass, e, token.NEQ, a.status, true) }) &&
		ps6137ErrorGuard(pass, list[a.index+2], func(e ast.Expr) bool { return ps6137StorageCondition(pass, e, a) })
}

func ps6137Lifecycle(candidate, lifecycle ps6137Function) bool {
	s := lifecycle.sig
	return s != nil && s.Recv() != nil && types.Identical(s.Recv().Type(), candidate.sig.Recv().Type()) && s.Params().Len() == 0 && s.Results().Len() == 0 && s.TypeParams().Len() == 0 && !s.Variadic()
}

func ps6137ReceiverGuard(pass *analysis.Pass, f ps6137Function, c *config.NativeSnapshotReuseContract) bool {
	if len(f.decl.Body.List) == 0 {
		return false
	}
	return ps6137ErrorGuard(pass, f.decl.Body.List[0], func(e ast.Expr) bool {
		or, ok := ps2110Unparen(e).(*ast.BinaryExpr)
		if !ok || or.Op != token.LOR {
			return false
		}
		left, ok := ps2110Unparen(or.X).(*ast.BinaryExpr)
		if !ok || left.Op != token.EQL || !ps6135Object(pass, left.X, f.sig.Recv()) || !ps6137Nil(pass, left.Y) {
			return false
		}
		right, ok := ps2110Unparen(or.Y).(*ast.BinaryExpr)
		if !ok || right.Op != token.EQL || !ps6137Nil(pass, right.Y) {
			return false
		}
		field, ok := ps2110Unparen(right.X).(*ast.SelectorExpr)
		return ok && field.Sel.Name == c.ReceiverHandleField && ps6135Object(pass, field.X, f.sig.Recv())
	})
}

func runPS6137WithContracts(pass *analysis.Pass, contracts []config.NativeSnapshotReuseContract) (any, error) {
	functions := ps6137Functions(pass)
	counts := make(map[string]int, len(contracts))
	for i := range contracts {
		counts[contracts[i].CandidateMethod]++
	}
	for i := range contracts {
		c := &contracts[i]
		if !c.Valid() || counts[c.CandidateMethod] != 1 {
			continue
		}
		f, ok := functions[c.CandidateMethod]
		if !ok || !f.fn.Exported() {
			continue
		}
		result, slice, ok := ps6137Result(f, c)
		if !ok {
			continue
		}
		lifecycle, ok := functions[c.LifecycleMethod]
		if !ok || !ps6137Lifecycle(f, lifecycle) || !ps6137ReceiverGuard(pass, f, c) {
			continue
		}
		own, ok := functions[c.OwnMethod]
		if !ok || !ps6137Own(pass, own) {
			continue
		}
		fill, ok := functions[c.FillCallable]
		if !ok || !ps6137Fill(pass, fill, own, c, result, slice) {
			continue
		}
		a, ok := ps6137Acquire(pass, f, c)
		if !ok || !ps6137Prelude(pass, f, a) || !ps6137AcquisitionGuards(pass, f, a) || !ps6137Materialize(pass, f, a, c, result, slice, fill) {
			continue
		}
		if !ps6137NativeLabelExtent(pass, a.pointer.Type(), own, c.NativeStringField) {
			continue
		}
		advice := "review an additive caller-owned Into API while preserving this allocating convenience API"
		if sibling, ok := functions[c.IntoMethod]; ok && ps6137IntoSibling(f, sibling, result) {
			advice = "an exact typed Into sibling already exists; review calling it and independently verify its reuse, parity and error semantics"
		}
		pass.Reportf(a.call.Pos(), "checked native snapshot freshly materializes a Go-owned result with source-bound pointer/count/scalar and closed owning-label helpers; %s; under reviewed repeated-extraction and native lifetime policy, move every fallible validation before destination mutation, grow insufficient capacity then reuse it, and reuse only Go-owned labels after exact content equality; verify scalar/EventSpan parity, zero/one/many counts, changed/reordered/overflow-cache labels and ownership after release, then benchmark steady-state and convenience nonregression; a compact by-value native view is platform/ABI benchmark advice only (PS6137 advisory, no automatic fix or unconditional gain)", advice)
	}
	return nil, nil
}

func ps6137Prelude(pass *analysis.Pass, f ps6137Function, a ps6137Acquisition) bool {
	if a.index < 2 {
		return false
	}
	declared := make(map[types.Object]bool, 3+len(a.scalars))
	for _, stmt := range f.decl.Body.List[1:a.index] {
		d, ok := stmt.(*ast.DeclStmt)
		if !ok {
			return false
		}
		g, ok := d.Decl.(*ast.GenDecl)
		if !ok || g.Tok != token.VAR {
			return false
		}
		for _, spec := range g.Specs {
			v, ok := spec.(*ast.ValueSpec)
			if !ok || len(v.Values) != 0 {
				return false
			}
			for _, name := range v.Names {
				object := identObject(pass, name)
				if object == nil || declared[object] {
					return false
				}
				declared[object] = true
			}
		}
	}
	if len(declared) != 2+len(a.scalars) || !declared[a.pointer] || !declared[a.count] {
		return false
	}
	for _, v := range a.scalars {
		if !declared[v] {
			return false
		}
	}
	return true
}
