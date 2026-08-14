package checks

import (
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3090 reports a capturing closure handed to a project fan-out helper: the
// closure environment (captured slices and the pointer to a call-local
// completion barrier) is allocated on every dispatch even though the worker
// pool itself is persistent.
//
// Domain check: which functions are fan-out entry points is project vocabulary
// (config.fanOutHelpers). With none listed the check stays silent.
var PS3090 = register(&lint.Check{
	ID:          "PS3090",
	Category:    "indirect",
	Slug:        "fanout-closure-escape",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a capturing closure passed to a fan-out helper allocates per dispatch",
		Text: `A persistent worker pool still allocates on every dispatch when its
task is a capturing function value. parallel(n, func(lo, hi int){ ... }) stores
the closure in a channel task, so every variable the closure captures by
reference — the source and destination slices, and the pointer to a call-local
sync.WaitGroup / atomic counter / barrier — escapes to the heap with it. The
workers are reused; the closure environment and the escaped barrier are not. Per
call the cost is small, but in an inference or training loop with thousands of
dispatches it is material, and it reintroduces GC pressure after the output
buffers themselves have been pooled.

PS3090 flags a closure literal passed directly to a configured fan-out helper
when it captures at least one REFERENCE-typed free variable of the enclosing
function — a slice, map, channel, pointer (including a *WaitGroup barrier),
function, or interface. Those are the payload and barrier escapes the pattern is
about; a closure that captures only value-typed loop bounds is not reported, and
with no fanOutHelpers vocabulary nothing is.

This is advisory only — there is no safe mechanical fix, because removing the
escape means designing a typed task payload (a struct holding the slices and a
reused barrier) with a non-capturing executor, and the safe shape depends on the
pool's cancellation and nesting contract. The recommendation is to benchmark a
typed payload with a non-capturing executor and a reused completion object,
proving with alloc_objects profiles that both the closure and the barrier
disappear.`,
		Before: `parallel(len(dst), func(lo, hi int) { // closure + &wg escape per call
	for i := lo; i < hi; i++ {
		dst[i] = f(src[i])
	}
})`,
		After: `// a typed, non-capturing task payload the pool can reuse
type siluTask struct{ dst, src []float64 }
func (t siluTask) run(lo, hi int) { for i := lo; i < hi; i++ { t.dst[i] = f(t.src[i]) } }
// dispatch t.run without a fresh closure or barrier per call`,
		MeasuredWin: `BenchmarkPS3090 (a two-slice payload dispatched so the task
escapes, Apple M2 Pro): a fresh capturing closure per dispatch is 64 B/op, 1
alloc/op; a typed struct payload carrying the same slice headers is 0 B/op, 0
allocs/op — the whole closure environment disappears. Small per call, but
thousands of dispatches per inference step turn it into steady GC pressure.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3090",
		Doc:  "capturing closure passed to a fan-out helper allocates the closure environment and barrier per dispatch",
		Run:  runPS3090,
	},
})

func runPS3090(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !fanout[astutil.CalleeName(call.Fun)] {
				return true
			}
			for _, arg := range call.Args {
				fl, ok := arg.(*ast.FuncLit)
				if !ok {
					continue
				}
				caps := ps3090RefCaptures(pass, fl)
				if len(caps) == 0 {
					continue
				}
				pass.Report(analysis.Diagnostic{
					Pos: fl.Pos(),
					End: fl.Type.End(),
					Message: "closure passed to fan-out helper " + astutil.CalleeName(call.Fun) +
						" captures " + ps3090List(caps) + " by reference; stored in the async task it" +
						" escapes and allocates per dispatch — pass a typed non-capturing task payload" +
						" (and reuse the completion barrier) to remove the per-call allocation",
				})
			}
			return true
		})
	}
	return nil, nil
}

// ps3090RefCaptures returns the names of the enclosing function's
// reference-typed variables that fl captures (reads or writes) as free
// variables — the closure environment that escapes with the task. Sorted and
// deduplicated.
func ps3090RefCaptures(pass *analysis.Pass, fl *ast.FuncLit) []string {
	seen := map[types.Object]bool{}
	var names []string
	ast.Inspect(fl.Body, func(n ast.Node) bool {
		id, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		obj, ok := pass.TypesInfo.ObjectOf(id).(*types.Var)
		if !ok || seen[obj] {
			return true
		}
		// A package-level variable is shared state, not a per-call escape.
		if obj.Parent() == pass.Pkg.Scope() {
			return true
		}
		// Declared INSIDE the closure (its own param/result/local) — not a
		// capture of the enclosing frame.
		if obj.Pos() >= fl.Pos() && obj.Pos() < fl.End() {
			return true
		}
		if !ps3090IsReference(obj.Type()) {
			return true
		}
		seen[obj] = true
		names = append(names, obj.Name())
		return true
	})
	slices.Sort(names)
	return names
}

// ps3090IsReference reports whether t is a reference-ish type whose capture
// constitutes a real payload/barrier escape: a slice, map, channel, pointer,
// function, or interface.
func ps3090IsReference(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Map, *types.Chan, *types.Pointer, *types.Signature, *types.Interface:
		return true
	}
	return false
}

// ps3090List renders captured names as a readable, quoted, comma-separated list.
func ps3090List(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "'" + n + "'"
	}
	return strings.Join(q, ", ")
}
