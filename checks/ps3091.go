package checks

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3091 reports a single-slot cache for an expensive compiled artifact — a
// graph, pipeline, shader, plan, kernel or executable — that recompiles
// whenever a stored signature changes. Alternating among a small stable set of
// shapes then recompiles on nearly every dispatch even though the working set
// is bounded.
//
// Domain check: which calls produce an expensive compiled resource is project
// vocabulary (config.compiledResourceFuncs), because a bare "last-value
// memoization" of a cheap value is fine and only a costly compile makes the
// single slot a thrash. With no vocabulary the check stays silent.
var PS3091 = register(&lint.Check{
	ID:          "PS3091",
	Category:    "indirect",
	Slug:        "single-slot-compile-cache",
	Level:       lint.LevelStructured,
	NeedsConfig: true,
	Vocab:       []string{"compiledResourceFuncs"},
	Doc: lint.Documentation{
		Title: "a one-entry compiled-graph/pipeline cache thrashes under alternating signatures; key a bounded cache by the full signature",
		Text: `A dispatch path that keeps ONE compiled artifact next to ONE
last-seen signature and rebuilds it whenever the signature changes —

  if sig != lastSig {
      lastGraph = compileGraph(sig)   // an expensive compile / bridge call
      lastSig = sig
  }

— is a single-entry cache. It is free when one shape repeats, but a workload
that alternates among a few shapes (batch sizes, sequence lengths, conv
configs) misses on nearly every call and pays the full compile every dispatch,
even though the working set is small and bounded. A bounded cache keyed by the
COMPLETE compile-affecting signature (e.g. a 16-entry LRU) turns the thrash back
into hits; the lookup overhead is negligible next to a multi-millisecond
compile. Single-shape microbenchmarks hide this — measure it with a multi-shape
A/B benchmark.

The match is deliberately narrow, to stay off cheap memoization:
  - an if whose condition is an inequality (sig != lastSig);
  - its body assigns a PERSISTENT target (a package-level var or a struct
    field, i.e. state that survives the call) from a call whose name is in
    config.compiledResourceFuncs — the opt-in that marks an EXPENSIVE compile;
  - its body also assigns to the persistent operand of the condition (the
    "remember the last signature" update) — this distinguishes the cache from a
    plain lazy init (if x == nil { x = ... }, which has no signature to store).

This is advisory only — no auto-fix: replacing the slot with a bounded,
signature-keyed cache is a design change (eviction policy, key composition, and
for accelerator pipelines device/driver identity when artifacts outlive a
device context) only the author can make.`,
		Before: `if sig != lastSig {
	release(lastGraph)
	lastGraph = compileGraph(sig) // recompiles on every alternation
	lastSig = sig
}`,
		After: `g, ok := graphCache.Get(sig) // bounded LRU keyed by the full signature
if !ok {
	g = compileGraph(sig)
	graphCache.Add(sig, g)
}`,
		MeasuredWin: `Reported by a GoAI Metal reproduction (issue #559): a
single-slot Conv2D/attention MPSGraph cache cycling five shapes per iteration
forced a 15-30 ms compile on nearly every call. A bounded 16-entry cache keyed
by the full signature took Conv2D 13.3 ms -> 1.85 ms (7.2x) and attention 21.5
ms -> 4.8 ms (4.4x), with single-shape performance unchanged (bounded-lookup
overhead negligible).`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3091",
		Doc:  "single-slot compiled-resource cache that recompiles when a stored signature changes",
		Run:  runPS3091,
	},
})

func runPS3091(pass *analysis.Pass) (any, error) {
	resources := config.Current().CompiledResourceFuncs
	if len(resources) == 0 {
		return nil, nil
	}
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			ifst, ok := n.(*ast.IfStmt)
			if !ok {
				return true
			}
			cond, ok := ifst.Cond.(*ast.BinaryExpr)
			if !ok || cond.Op != token.NEQ {
				return true
			}
			call := ps3091CompileAssign(pass, ifst.Body, resources)
			if call == nil {
				return true
			}
			if !ps3091UpdatesCondOperand(pass, ifst.Body, cond) {
				return true
			}
			pass.Report(analysis.Diagnostic{
				Pos: ifst.Pos(),
				End: ifst.Cond.End(),
				Message: "single-slot cache: " + astutil.CalleeName(call.Fun) +
					" rebuilds an expensive compiled resource whenever the stored signature changes; a workload alternating among a few shapes recompiles on nearly every dispatch — key a bounded cache by the full signature instead",
			})
			return true
		})
	}
	return nil, nil
}

// ps3091CompileAssign returns the compiled-resource call assigned to a
// persistent target somewhere in body, or nil. It is the "lastGraph =
// compile(sig)" half of the pattern.
func ps3091CompileAssign(pass *analysis.Pass, body *ast.BlockStmt, resources map[string]bool) *ast.CallExpr {
	var found *ast.CallExpr
	ast.Inspect(body, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range as.Rhs {
			call, ok := rhs.(*ast.CallExpr)
			if !ok || !resources[astutil.CalleeName(call.Fun)] {
				continue
			}
			// The target the compiled resource is stored into must be
			// persistent (survives the call), or it is not a cache.
			if i < len(as.Lhs) && ps3091IsPersistent(pass, as.Lhs[i]) {
				found = call
				return false
			}
		}
		return true
	})
	return found
}

// ps3091UpdatesCondOperand reports whether body assigns to the PERSISTENT
// operand of cond — the "lastSig = sig" update that marks a stored signature
// (as opposed to a lazy init, whose condition operand is never re-stored).
func ps3091UpdatesCondOperand(pass *analysis.Pass, body *ast.BlockStmt, cond *ast.BinaryExpr) bool {
	var targets []types.Object
	for _, op := range []ast.Expr{cond.X, cond.Y} {
		if obj := ps3091VarObj(pass, op); obj != nil && ps3091IsPersistent(pass, op) {
			targets = append(targets, obj)
		}
	}
	if len(targets) == 0 {
		return false
	}
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range as.Lhs {
			lobj := ps3091VarObj(pass, lhs)
			for _, t := range targets {
				if lobj != nil && lobj == t {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// ps3091VarObj returns the variable object an lvalue expression refers to: an
// identifier or a selector (x.field / pkg.Var).
func ps3091VarObj(pass *analysis.Pass, e ast.Expr) types.Object {
	switch x := e.(type) {
	case *ast.Ident:
		if v, ok := pass.TypesInfo.ObjectOf(x).(*types.Var); ok {
			return v
		}
	case *ast.SelectorExpr:
		if v, ok := pass.TypesInfo.ObjectOf(x.Sel).(*types.Var); ok {
			return v
		}
	}
	return nil
}

// ps3091IsPersistent reports whether an lvalue names state that survives the
// current call: a package-level variable, or a struct field (a selector whose
// selected symbol is a field). A function-local variable is NOT persistent.
func ps3091IsPersistent(pass *analysis.Pass, e ast.Expr) bool {
	switch x := e.(type) {
	case *ast.Ident:
		v, ok := pass.TypesInfo.ObjectOf(x).(*types.Var)
		return ok && v.Parent() == pass.Pkg.Scope()
	case *ast.SelectorExpr:
		if sel, ok := pass.TypesInfo.Selections[x]; ok {
			return sel.Kind() == types.FieldVal
		}
		// A qualified package-level variable (pkg.Var) is persistent too.
		if v, ok := pass.TypesInfo.ObjectOf(x.Sel).(*types.Var); ok {
			return v.Parent() != nil && v.Parent() == v.Pkg().Scope()
		}
	}
	return false
}
