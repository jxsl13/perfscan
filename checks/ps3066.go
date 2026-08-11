package checks

import (
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS3066 reports three or more consecutive sibling loops over the same
// bound that all index the same buffer.
var PS3066 = register(&lint.Check{
	ID:       "PS3066",
	Category: "indirect",
	Slug:     "consecutive-loops-over-one-buffer",
	Level:    lint.LevelStructured,
	Doc: lint.Documentation{
		Title: "three or more sibling loops over the same bound, all touching one buffer",
		Text: `Each pass streams the whole buffer and evicts it before the next
pass starts. If every index is independent ACROSS the loops, merge them: the
buffer is then touched once and stays in cache through all the stages.
Merging is bit-identical because it changes only WHEN a row is visited,
never how — each row's stages already ran in that order relative to each
other.

Check for a cross-index dependency first: a later loop that needs ALL of an
earlier loop's output cannot merge. The win is the buffer SIZE, not the loop
count — the reference measured -15.9% on a state larger than L2 and -1.8%
on one that was already L1-resident.`,
		Before: `for i := range state { state[i] *= decay }
for i := range state { s += state[i] * k[i] }
for i := range state { state[i] += delta[i] }`,
		After: `for i := range state {
	state[i] *= decay
	s += state[i] * k[i]
	state[i] += delta[i]
}`,
		MeasuredWin: "reference corpus: Kimi delta-attention four passes merged into one, BenchmarkKDA_F64_256x128 8.71→7.33ms (-15.9%); same merge on an L1-resident 64×64 state: -1.8%",
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS3066",
		Doc:  "consecutive sibling loops over one buffer",
		Run:  runPS3066,
	},
})

func runPS3066(pass *analysis.Pass) (any, error) {
	for _, f := range pass.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			block, ok := n.(*ast.BlockStmt)
			if !ok {
				return true
			}
			checkConsecutiveLoops(pass, block)
			return true
		})
	}
	return nil, nil
}

type loopInfo struct {
	stmt    ast.Stmt
	bound   string
	indexed map[string]bool
}

func checkConsecutiveLoops(pass *analysis.Pass, block *ast.BlockStmt) {
	var run []loopInfo
	flush := func() {
		defer func() { run = run[:0] }()
		if len(run) < 3 {
			return
		}
		// Common buffer indexed by every loop in the run.
		common := make(map[string]bool, len(run[0].indexed))
		for b := range run[0].indexed {
			common[b] = true
		}
		for _, li := range run[1:] {
			for b := range common {
				if !li.indexed[b] {
					delete(common, b)
				}
			}
		}
		if len(common) == 0 {
			return
		}
		names := make([]string, 0, len(common))
		for b := range common {
			names = append(names, b)
		}
		pass.Report(analysis.Diagnostic{
			Pos:     run[0].stmt.Pos(),
			End:     run[0].stmt.End(),
			Message: fmt.Sprintf("%d consecutive loops over the same bound stream %s once each; if the per-index work is independent across the loops, merge them so the buffer stays in cache (a later loop needing ALL of an earlier one's output cannot merge)", len(run), strings.Join(names, ", ")),
		})
	}
	for _, stmt := range block.List {
		bound, indexed, ok := loopSignature(stmt)
		if !ok {
			flush()
			continue
		}
		if len(run) > 0 && run[len(run)-1].bound != bound {
			flush()
		}
		run = append(run, loopInfo{stmt: stmt, bound: bound, indexed: indexed})
	}
	flush()
}

// loopSignature renders a loop's bound and collects the identifiers it
// indexes.
func loopSignature(s ast.Stmt) (string, map[string]bool, bool) {
	var bound ast.Expr
	var body *ast.BlockStmt
	switch l := s.(type) {
	case *ast.RangeStmt:
		bound = l.X
		body = l.Body
	case *ast.ForStmt:
		if l.Cond == nil {
			return "", nil, false
		}
		bound = l.Cond
		body = l.Body
	default:
		return "", nil, false
	}
	var b strings.Builder
	_ = printer.Fprint(&b, token.NewFileSet(), bound)
	indexed := map[string]bool{}
	ast.Inspect(body, func(n ast.Node) bool {
		if ix, ok := n.(*ast.IndexExpr); ok {
			if id, ok := ix.X.(*ast.Ident); ok {
				indexed[id.Name] = true
			}
		}
		return true
	})
	return b.String(), indexed, true
}
