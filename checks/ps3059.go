package checks

import (
	"fmt"
	"go/ast"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/astutil"
	"github.com/jxsl13/perfscan/lint"
)

// PS3059 reports a serial nest whose writes land through a base DERIVED
// from the outermost loop variable — PS3034's blind spot.
var PS3059 = register(&lint.Check{
	ID:          "PS3059",
	Category:    "indirect",
	Slug:        "serial-nest-derived-base",
	Level:       lint.LevelAggressive,
	NeedsConfig: true,
	Vocab:       []string{"fanOutHelpers"},
	Doc: lint.Documentation{
		Title: "a serial nest writing through a base derived from the outermost loop variable",
		Text: `PS3034 asks whether each write names the outermost loop
variable — and a nest that hoists its row offset into a local
(obase := b*out; dst[obase+j] = …) does not. The reference missed two large
finds this way (a 66.3% cut and a 6.2x), both in files whose other hot loop
had fanned out since it was written.

Widening PS3034 to follow derived names was tried and REVERTED in the
reference: it broke fixtures and inflated counts without flagging the
target. A separate check with the narrow condition — every write through a
derived base, at least one not naming the variable directly — is what
works.

L3 (aggressive): check the band owns disjoint output, and gate with BOTH a
bit-exact digest and -race — an accumulated destination fires both on
overlap, a pure permutation only -race.`,
		Before: `for b := 0; b < batch; b++ {
	obase := b * out
	for j := 0; j < out; j++ {
		dst[obase+j] = src[j*batch+b] // derived base: PS3034 is blind here
	}
}`,
		After: `parallel.For(batch, func(b int) { // digest + -race gated
	obase := b * out
	for j := 0; j < out; j++ {
		dst[obase+j] = src[j*batch+b]
	}
})`,
		MeasuredWin: "goai reference: GGUF weight transpose -66.3% (154.2→49.8ms) once banded; KAN fused spline 6.2x — both missed by PS3034's direct-name condition",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3059",
		Doc:  "serial nest writing through a derived base",
		Run:  runPS3059,
	},
})

func runPS3059(pass *analysis.Pass) (any, error) {
	fanout := config.Current().FanOutHelpers
	if len(fanout) == 0 || !packageHasFanOut(pass, fanout) {
		return nil, nil
	}
	for _, f := range pass.Files {
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || callsFanOut(fn.Body, fanout) {
				continue
			}
			topLevelNests(fn.Body, func(nest ast.Node) {
				outerVar := outermostLoopVar(nest)
				if outerVar == "" {
					return
				}
				writes := nestWrites(nest)
				if len(writes) == 0 {
					return
				}
				derived := derivedFromVar(nest, outerVar)
				sawIndirect := false
				for _, w := range writes {
					if indexNames(w, outerVar) {
						continue
					}
					viaDerived := false
					for name := range derived {
						if name != outerVar && indexNames(w, name) {
							viaDerived = true
							break
						}
					}
					if !viaDerived {
						return // a write outside the derivation chain: not provably disjoint
					}
					sawIndirect = true
				}
				if !sawIndirect {
					return // PS3034's territory
				}
				body := astutil.LoopBody(nest)
				pass.Report(analysis.Diagnostic{
					Pos:     nest.Pos(),
					End:     body.Lbrace,
					Message: fmt.Sprintf("every write in this serial nest lands through a base derived from outer variable %s — bandable, and a fan-out helper exists in this package; check the band owns disjoint output, then gate with BOTH a bit-exact digest and -race", outerVar),
				})
			})
		}
	}
	return nil, nil
}
