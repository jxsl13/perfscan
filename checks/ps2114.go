package checks

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS2114 reports sync.Pool round trips over non-pointer values: a Put whose
// argument is a non-pointer, or a New func literal returning one. Each such
// value is boxed into an interface on the way into the pool, allocating —
// exactly the allocation the pool exists to avoid.
const ps2114OwnershipAdvice = "when ownership spans the pool lifetime, allocate the wrapper only on a cold miss, carry that same pointer through the owner, and return it to sync.Pool; a fresh wrapper at every Put still allocates; removing allocations alone does not prove a wall-time win, so benchmark the actual call site"

var PS2114 = register(&lint.Check{
	ID:       "PS2114",
	Category: "alloc",
	Slug:     "sync-pool-non-pointer",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "sync.Pool stores a non-pointer value, boxing it on every Put",
		Text: `sync.Pool traffics in interface{} values. A pointer, map,
channel, or function value is one word and rides in the interface's data
word directly — no allocation. Anything else (a []byte's three-word
header, a string, a struct, an array, an int) must be copied to a fresh
heap cell on EVERY conversion: every Put of a non-pointer value, and
every non-pointer value returned by New, allocates. The pool then caches
the box, not the value's backing storage semantics you might expect, and
the per-cycle allocation quietly cancels much of what the pool was
installed to save.

The remedy is to pool a pointer to the value instead:

  New: func() any { b := make([]byte, 0, n); return &b }
  p := pool.Get().(*[]byte)   // use *p
  pool.Put(p)                 // one-word pointer: no boxing allocation

or to pool a pointer to a small wrapper struct holding the slice.

The pointer must be a reusable ownership token, not merely fresh pointer
syntax at the release site. Allocate the wrapper only on a cold miss
(normally in Pool.New), carry that same pointer in the object that owns the
buffer from acquire through release, and return the same pointer to Put.
Taking &b for an escaping slice-header variable on every release still
allocates one wrapper per cycle.

Keep sync.Pool when its GC reclamation and per-P behavior are part of the
design; replacing it with an unbounded private freelist changes those
semantics. A public API that exposes only a raw slice is an explicit fallback
boundary because it cannot return the ownership token. Its raw-slice Put still
boxes and should still be reported even when a separate owned path exists. A
shared pool can bridge the forms with a type switch: preserve the token on the
owned path, and create or discard at most one token when crossing the raw
boundary while retaining the backing buffer. Suppress the raw fallback only
with evidence that the measured production path is intercepted by the owned
API.

Eliminating the boxed slice header establishes an allocation reduction, not a
universal wall-time improvement. Benchmark the actual call site with paired
measurements, especially for parallel consumers.

No automatic fix is attached, deliberately: a safe rewrite must change
the New function, every Put call site, and every Get consumer's type
assertion in one coherent step — for an exported pool those sites can
live in other files or packages the analyzer cannot edit — and it swaps
value semantics for pointer semantics at each consumer (a local copy of
the slice header becomes an aliasing indirection whose writes back into
*p are observable). That is a structural, multi-site refactor, not a
local bit-identical text edit, so the check only reports.

Interface-typed Put arguments (dynamic type unknown), type parameters,
pointer-like kinds (pointer, unsafe.Pointer, map, chan, func) and
zero-size values (which box against the runtime's zero base without
allocating) are deliberately not reported.`,
		Before: `var bufPool = sync.Pool{New: func() any { return make([]byte, 0, 1024) }}
b := bufPool.Get().([]byte)
// ... use b ...
bufPool.Put(b) // boxes the 24-byte slice header: allocates every cycle`,
		After: `type bufferOwner struct { token *[]byte }

var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 1024); return &b }}

func acquireBuffer() bufferOwner {
	p := bufPool.Get().(*[]byte)
	*p = (*p)[:0]
	return bufferOwner{token: p} // carry the cold-miss token with its owner
}

func (o *bufferOwner) release() {
	bufPool.Put(o.token) // return the same token; do not allocate a fresh &slice
	o.token = nil
}`,
		MeasuredWin: `Historic repository microbenchmark evidence, recorded on an
Apple M2 Pro (not remeasured for this documentation change):
BenchmarkPS2114 Get/append/Put measured the value pool at 24.7 ns/op,
24 B/op, 1 alloc/op and the pointer pool at 8.5 ns/op, 0 B/op,
0 allocs/op (~2.9x).

That isolated result is not a general wall-time claim. In a later
owner-reported parallel MoE scratch experiment, the ownership token removed
6–8 allocations and roughly 23 KiB/op but made F32 1.170x slower; a persistent
worker-pool variant made F64 1.121x slower. Both experiments were reverted.
Treat allocation removal as an allocation fact and require paired timing at
the actual call site, especially for parallel consumers.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS2114",
		Doc:  "sync.Pool storing a non-pointer value boxes it on every Put/New",
		Run:  runPS2114,
	},
})

func runPS2114(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				ps2114CheckPut(pass, n)
			case *ast.CompositeLit:
				// sync.Pool{New: func() any { ... }}
				if !ps2114IsSyncPool(pass.TypesInfo.TypeOf(n)) {
					return true
				}
				for _, elt := range n.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "New" {
						if fl, ok := kv.Value.(*ast.FuncLit); ok {
							ps2114CheckNew(pass, fl)
						}
					}
				}
			case *ast.AssignStmt:
				// pool.New = func() any { ... }
				if len(n.Lhs) != 1 || len(n.Rhs) != 1 {
					return true
				}
				fl, ok := n.Rhs[0].(*ast.FuncLit)
				if !ok {
					return true
				}
				sel, ok := n.Lhs[0].(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if fld, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Var); ok &&
					fld.IsField() && fld.Name() == "New" &&
					fld.Pkg() != nil && fld.Pkg().Path() == "sync" {
					ps2114CheckNew(pass, fl)
				}
			}
			return true
		})
	}
	return nil, nil
}

// ps2114CheckPut reports call when it is (*sync.Pool).Put of a value whose
// static type provably boxes with an allocation.
func ps2114CheckPut(pass *analysis.Pass, call *ast.CallExpr) {
	if len(call.Args) != 1 || call.Ellipsis.IsValid() {
		return
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Name() != "Put" || fn.Pkg() == nil || fn.Pkg().Path() != "sync" {
		return
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil || !ps2114IsSyncPool(sig.Recv().Type()) {
		return
	}
	t := ps2114BoxedType(pass, call.Args[0])
	if t == nil {
		return
	}
	pass.Report(analysis.Diagnostic{
		Pos:     call.Pos(),
		End:     call.End(),
		Message: "sync.Pool.Put of non-pointer value (type " + types.TypeString(t, types.RelativeTo(pass.Pkg)) + ") boxes it into an interface, allocating on every Put; pool a pointer instead; " + ps2114OwnershipAdvice,
	})
}

// ps2114CheckNew reports every return in fl (a sync.Pool New function
// literal) whose result provably boxes with an allocation. Returns inside
// nested function literals belong to those literals and are skipped.
func ps2114CheckNew(pass *analysis.Pass, fl *ast.FuncLit) {
	if fl.Body == nil {
		return
	}
	ast.Inspect(fl.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := n.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			return true
		}
		t := ps2114BoxedType(pass, ret.Results[0])
		if t == nil {
			return true
		}
		pass.Report(analysis.Diagnostic{
			Pos:     ret.Results[0].Pos(),
			End:     ret.Results[0].End(),
			Message: "sync.Pool New returns non-pointer value (type " + types.TypeString(t, types.RelativeTo(pass.Pkg)) + "); every pool cycle boxes it, allocating; return a pointer instead; " + ps2114OwnershipAdvice,
		})
		return true
	})
}

// ps2114IsSyncPool reports whether t is sync.Pool or *sync.Pool.
func ps2114IsSyncPool(t types.Type) bool {
	if t == nil {
		return false
	}
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Name() == "Pool" && obj.Pkg() != nil && obj.Pkg().Path() == "sync"
}

// ps2114BoxedType returns e's (defaulted) type when converting a value of
// that type to interface{} provably heap-allocates, or nil when it does not
// or cannot be proven. Silent cases, deliberately:
//   - pointer-like kinds (pointer, unsafe.Pointer, chan, map, func): one
//     word, stored in the interface data word directly;
//   - interface-typed expressions: already boxed, dynamic type unknown;
//   - type parameters: an instantiation may be pointer-like;
//   - zero-size values: boxed against the runtime zero base, no allocation;
//   - untyped nil / invalid types.
func ps2114BoxedType(pass *analysis.Pass, e ast.Expr) types.Type {
	t := pass.TypesInfo.TypeOf(e)
	if t == nil {
		return nil
	}
	t = types.Default(t)
	if _, isTP := t.(*types.TypeParam); isTP {
		return nil
	}
	switch under := t.Underlying().(type) {
	case *types.Pointer, *types.Chan, *types.Map, *types.Signature, *types.Interface, *types.TypeParam:
		return nil
	case *types.Basic:
		switch under.Kind() {
		case types.UnsafePointer, types.UntypedNil, types.Invalid:
			return nil
		}
	}
	if pass.TypesSizes != nil && pass.TypesSizes.Sizeof(t) == 0 {
		return nil
	}
	return t
}
