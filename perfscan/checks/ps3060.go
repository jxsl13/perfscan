package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/config"
	"github.com/jxsl13/perfscan/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS3060 reports a serial loop over calls to a function that itself fans
// out — the inner level is parallel, the outer one is not.
var PS3060 = register(&lint.Check{
	ID:          "PS3060",
	Category:    "indirect",
	Slug:        "serial-loop-over-parallel-work",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a serial loop over calls to a function that itself fans out",
		Text: `The items run strictly one after another and each pays its own
fork and join. THE WIN IS NOT ONLY THE WORK, IT IS THE STALLS: the
reference's Muon optimizer spent 62% of its profile in
pthread_cond_wait/signal because each parameter's iterations forked three
times each with nothing overlapping; banding the parameter loop returned
-34.1% with only TWO parameters — far more than two-way overlap of the
work alone explains.

L3 (aggressive): CALL ANY CALLER-SUPPLIED CALLBACK SERIALLY FIRST — a
gradient or loss function belongs to the caller and is not documented as
safe to call from several goroutines; hoist it into a serial pass and fan
out only what follows. Size the test above the fan-out helper's work gate,
or both arms run serially and an overlapping-band mutation passes.`,
		Before: `for _, p := range params {
	newtonSchulz(p) // newtonSchulz fans out internally, 5 forks per param
}`,
		After: `parallel.For(len(params), func(i int) {
	newtonSchulz(params[i]) // outer band overlaps the forks
})`,
		MeasuredWin: "reference corpus: Muon optimizer step -34.1% with only two parameters; 62% of the profile was pthread_cond_wait/signal before",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3060",
		Doc:  "serial loop over internally-parallel calls",
		Run:  runPS3060,
	},
})

func runPS3060(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 {
		return nil, nil
	}
	// Declared functions that fan out internally.
	fanningFuncs := map[string]bool{}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if callsFanOut(fn.Body, fanout) {
				fanningFuncs[fn.Name.Name] = true
			}
		}
	}
	if len(fanningFuncs) == 0 {
		return nil, nil
	}

	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || callsFanOut(fn.Body, fanout) {
				continue
			}
			reportedLoop := map[ast.Node]bool{}
			astutil.WithStack(fn.Body, func(n ast.Node, stack []ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name := astutil.CalleeName(call.Fun)
				if !fanningFuncs[name] {
					return true
				}
				loop, inLoop := astutil.InLoop(stack)
				if !inLoop || reportedLoop[loop] {
					return true
				}
				reportedLoop[loop] = true
				pass.Report(analysis.Diagnostic{
					Pos:     call.Pos(),
					End:     call.End(),
					Message: fmt.Sprintf("%s fans out internally but this loop runs its calls strictly one after another — each pays its own fork and join; band the outer loop (hoist caller-supplied callbacks into a serial pass first, gate with -race)", name),
				})
				return true
			})
		}
	}
	return nil, nil
}
