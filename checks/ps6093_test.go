package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/cfg"
)

func TestPS6093(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6093.Analyzer, "ps6093")
}

func TestPS6093ProofPathBudget(t *testing.T) {
	t.Parallel()
	blocks := make([]cfg.Block, 128)
	for index := range len(blocks) - 1 {
		blocks[index].Succs = []*cfg.Block{&blocks[index+1]}
	}
	paths := ps6093ProofPaths{remaining: 8}
	if cycles, complete := ps6093AccessCyclesThroughProof(&paths, &cfg.Block{}, &blocks[0]); complete || cycles {
		t.Fatal("path scan exceeded its budget and accepted an incomplete proof")
	}
	if paths.remaining != 0 {
		t.Fatalf("remaining budget = %d, want 0", paths.remaining)
	}
}

func TestPS6093ProofWorkBudget(t *testing.T) {
	t.Parallel()
	paths := ps6093ProofPaths{remaining: 2}
	for range 2 {
		if !ps6093ProofWork(&paths) {
			t.Fatal("proof work exhausted before its configured limit")
		}
	}
	if ps6093ProofWork(&paths) {
		t.Fatal("proof work exceeded its configured limit")
	}
	if paths.remaining != 0 {
		t.Fatalf("remaining budget = %d, want 0", paths.remaining)
	}
}

func TestPS6093BoundSafetyBudget(t *testing.T) {
	t.Parallel()
	statements := make([]ast.Stmt, 128)
	for index := range statements {
		statements[index] = &ast.EmptyStmt{}
	}
	paths := ps6093ProofPaths{remaining: 8}
	_, complete := ps6093ObjectUnsafeBudgeted(&analysis.Pass{}, &ast.BlockStmt{List: statements}, nil, &paths, nil, token.NoPos, token.Pos(^uint(0)>>1))
	if complete {
		t.Fatal("bound safety scan accepted an incomplete budgeted traversal")
	}
	if paths.remaining != 0 || paths.spent != 8 {
		t.Fatalf("budget state = remaining %d, spent %d; want 0, 8", paths.remaining, paths.spent)
	}
}

func TestPS6093PackageEffectBudget(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		t.Helper()
		statements := make([]ast.Stmt, size)
		calls := make([]*ast.CallExpr, size)
		block := &cfg.Block{Live: true}
		for index := range statements {
			position := token.Pos(4*index + 2)
			call := &ast.CallExpr{
				Fun:    &ast.Ident{NamePos: position, Name: "effect"},
				Lparen: position + 1,
				Rparen: position + 2,
			}
			calls[index] = call
			statements[index] = &ast.ExprStmt{X: call}
			block.Nodes = append(block.Nodes, call)
		}
		body := &ast.BlockStmt{Lbrace: 1, List: statements, Rbrace: token.Pos(4*size + 2)}
		parents := ps6087Parents(body)
		paths := ps6093ProofPaths{
			blocks:    make(map[token.Pos]*cfg.Block),
			nodeOrder: make(map[token.Pos]int),
			remaining: 8 * size,
		}
		ps6093IndexBlockNodes(&paths, block)
		pass := &analysis.Pass{TypesInfo: &types.Info{Types: make(map[ast.Expr]types.TypeAndValue)}}
		stableEffects := 0
		stable := func(node ast.Node) bool {
			stableEffects++
			before := ps6093BlockExecutesBefore(&paths, block, node, calls[len(calls)-1], parents)
			after := ps6093BlockExecutesAfter(&paths, block, node, calls[0], parents)
			return before && after
		}
		if !ps6093PackageEffectsStable(pass, body, parents, &paths, nil, stable) {
			t.Fatalf("size %d complete same-block effect scan rejected a stable proof", size)
		}
		if stableEffects != size {
			t.Fatalf("size %d stable effects = %d, want %d", size, stableEffects, size)
		}
		if ps6093PackageEffectsStable(pass, body, parents, &paths, nil, stable) {
			t.Fatalf("size %d exhausted package-effect scan accepted an incomplete proof", size)
		}
		if paths.remaining != 0 || paths.spent != 8*size {
			t.Fatalf("size %d budget state = remaining %d, spent %d; want 0, %d", size, paths.remaining, paths.spent, 8*size)
		}
		return paths.spent
	}

	small := work(64)
	large := work(128)
	if large != 2*small {
		t.Fatalf("doubling package-effect input changed bounded work from %d to %d; want %d", small, large, 2*small)
	}
}

func TestPS6093DominanceQueryBudget(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		t.Helper()
		entry := &cfg.Block{Live: true}
		before := &cfg.Block{Live: true}
		after := &cfg.Block{Live: true}
		entry.Succs = []*cfg.Block{before}
		before.Succs = []*cfg.Block{after}
		graph := &cfg.CFG{Blocks: []*cfg.Block{entry, before, after}}
		paths := ps6093ProofPaths{
			dominance: make(map[ps6093BlockPair]bool),
			blocks:    make(map[token.Pos]*cfg.Block, size+1),
			remaining: size + 2,
		}
		for index := range size {
			paths.blocks[token.Pos(index+1)] = before
		}
		afterPosition := token.Pos(size + 1)
		paths.blocks[afterPosition] = after
		for index := range size {
			dominates, complete := ps6093GraphPositionDominates(graph, &paths, token.Pos(index+1), afterPosition)
			if !complete || !dominates {
				t.Fatalf("size %d cached dominance query %d unexpectedly declined", size, index)
			}
		}
		if paths.remaining != 0 || paths.spent != size+2 {
			t.Fatalf("size %d dominance work = remaining %d, spent %d; want 0, %d", size, paths.remaining, paths.spent, size+2)
		}
		if _, complete := ps6093GraphPositionDominates(graph, &paths, 1, afterPosition); complete {
			t.Fatalf("size %d exhausted dominance query reported a complete proof", size)
		}
		return paths.spent
	}

	small := work(64)
	large := work(128)
	if large-small != 64 {
		t.Fatalf("doubling dominance queries changed bounded work from %d to %d; want delta 64", small, large)
	}
	if ps6093ProofBudget(128, 128) != 2*ps6093ProofBudget(64, 64) {
		t.Fatal("shared proof budget does not scale linearly with syntax and CFG size")
	}
}

func TestPS6093ConditionGuaranteeBudget(t *testing.T) {
	t.Parallel()

	expr := &ast.BinaryExpr{
		X:  &ast.Ident{Name: "i"},
		Op: token.LSS,
		Y:  &ast.Ident{Name: "n"},
	}
	paths := ps6093ProofPaths{remaining: 1}
	proved, complete := ps6093ConditionGuarantees(&analysis.Pass{}, &paths, expr, true,
		func(*ast.BinaryExpr, bool) bool { return true },
		func(*ast.BinaryExpr) (bool, bool) { return false, ps6093ProofWork(&paths) },
	)
	if complete || proved {
		t.Fatal("condition proof accepted an incomplete package guard")
	}
	if !paths.exhausted {
		t.Fatal("condition proof did not mark the shared budget exhausted")
	}
}

func TestPS6093NestedLoopAndSliceScanBudget(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		t.Helper()
		statements := make([]ast.Stmt, size)
		for index := range statements {
			statements[index] = &ast.EmptyStmt{}
		}
		body := &ast.BlockStmt{List: statements}
		paths := ps6093ProofPaths{remaining: 4 * size}
		pass := &analysis.Pass{TypesInfo: &types.Info{Uses: make(map[*ast.Ident]types.Object)}}
		object := types.NewVar(token.NoPos, nil, "slice", types.NewSlice(types.Typ[types.Byte]))
		for range size {
			_, _ = ps6093ObjectUnsafeBudgeted(pass, body, nil, &paths, object, token.NoPos, token.Pos(^uint(0)>>1))
			_, _ = ps6093HeaderEscapesBudgeted(pass, body, nil, &paths, object)
		}
		if paths.remaining != 0 || paths.spent != 4*size {
			t.Fatalf("size %d repeated scan work = remaining %d, spent %d; want 0, %d", size, paths.remaining, paths.spent, 4*size)
		}
		if _, complete := ps6093ObjectUnsafeBudgeted(pass, body, nil, &paths, object, token.NoPos, token.Pos(^uint(0)>>1)); complete {
			t.Fatalf("size %d exhausted nested-loop scan reported complete", size)
		}
		if _, complete := ps6093HeaderEscapesBudgeted(pass, body, nil, &paths, object); complete {
			t.Fatalf("size %d exhausted slice-header scan reported complete", size)
		}
		return paths.spent
	}

	small := work(64)
	large := work(128)
	if large != 2*small {
		t.Fatalf("doubling repeated scan input changed bounded work from %d to %d; want %d", small, large, 2*small)
	}
}
