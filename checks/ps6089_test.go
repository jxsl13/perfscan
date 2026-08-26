package checks

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/cfg"
)

func TestPS6089(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6089.Analyzer, "ps6089")
}

func TestPS6089CallableEffectWorkIsLinearAndBounded(t *testing.T) {
	t.Parallel()

	work := func(depth int) int {
		nested := strings.Repeat("identity(", depth) + "slot" + strings.Repeat(")", depth)
		source := `package p
type Command struct{}
type State struct{ command *Command }
func identity(value **Command) **Command { return value }
func sink(value **Command) {}
func benchmark() {
	state := &State{}
	slot := &state.command
	mutate := func() { sink(` + nested + `) }
	mutate()
}`
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "callable.go", source, 0)
		if err != nil {
			t.Fatalf("parse nested callable fixture: %v", err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check nested callable fixture: %v", err)
		}
		var body *ast.BlockStmt
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name.Name == "benchmark" {
				body = function.Body
				break
			}
		}
		index := ps6089ReceiverAliases(&analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}, body)
		if !index.complete || index.buildWork > 64*(depth*6+64) {
			t.Fatalf("nested callable summary exceeded its linear budget: depth=%d work=%d complete=%v", depth, index.buildWork, index.complete)
		}
		return index.buildWork
	}

	const size = 32
	first := work(size)
	second := work(2 * size)
	if second > 2*first+64 {
		t.Fatalf("doubling nested callable depth grew summary work superlinearly: %d -> %d", first, second)
	}

	object := types.NewVar(token.NoPos, nil, "slot", types.NewPointer(types.NewStruct(nil, nil)))
	used := 0
	_, complete := ps6089DeduplicateReceiverEffects([]ps6089ReceiverWrite{{id: []types.Object{object, object}}}, func(amount int) bool {
		used += amount
		return used <= 1
	})
	if complete {
		t.Fatal("effect deduplication must fail closed when its shared budget is exhausted")
	}
}

func TestPS6089WideAggregateWorkIsLinearAndBounded(t *testing.T) {
	t.Parallel()

	work := func(width int) (int, bool) {
		var fields strings.Builder
		var values strings.Builder
		var definitions strings.Builder
		var calls strings.Builder
		for index := range width {
			fmt.Fprintf(&fields, "F%d **Command\n", index)
			fmt.Fprintf(&values, "F%d: slot,\n", index)
			fmt.Fprintf(&definitions, "mutate%d := holder.replace\n", index)
			fmt.Fprintf(&calls, "mutate%d()\n", index)
		}
		source := `package p
type Command struct{}
type Mutator struct {` + fields.String() + `}
func (m Mutator) replace() {}
func benchmark() {
	command := &Command{}
	slot := &command
	holder := Mutator{` + values.String() + `}
` + definitions.String() + calls.String() + `}`
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "wide.go", source, 0)
		if err != nil {
			t.Fatalf("parse wide aggregate fixture: %v", err)
		}
		info := &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Defs:       make(map[*ast.Ident]types.Object),
			Uses:       make(map[*ast.Ident]types.Object),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
		}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check wide aggregate fixture: %v", err)
		}
		var body *ast.BlockStmt
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Name.Name == "benchmark" {
				body = function.Body
				break
			}
		}
		index := ps6089ReceiverAliases(&analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}, body)
		if index.buildWork > 64*(width*12+128) {
			t.Fatalf("wide aggregate summary exceeded its shared linear budget: width=%d work=%d complete=%v", width, index.buildWork, index.complete)
		}
		return index.buildWork, index.complete
	}

	first, firstComplete := work(64)
	second, secondComplete := work(128)
	if !firstComplete || !secondComplete {
		t.Fatalf("wide aggregate summaries must complete at regression sizes: complete=%v/%v", firstComplete, secondComplete)
	}
	if second > 2*first+256 {
		t.Fatalf("doubling aggregate width grew summary work superlinearly: %d -> %d", first, second)
	}
}

func TestPS6089RecursiveDefinedPointerDepthIsBounded(t *testing.T) {
	t.Parallel()

	name := types.NewTypeName(token.NoPos, nil, "RecursivePointer", nil)
	recursive := types.NewNamed(name, nil, nil)
	recursive.SetUnderlying(types.NewPointer(recursive))
	if depth := ps6089PointerDepth(recursive); depth < 1<<20 {
		t.Fatalf("recursive defined pointer must remain conservative, got depth %d", depth)
	}
}

func TestPS6089LifecycleEventWorkIsLinearAndBounded(t *testing.T) {
	t.Parallel()

	work := func(events int) int {
		command := types.NewVar(token.NoPos, nil, "command", types.NewPointer(types.NewStruct(nil, nil)))
		id := []types.Object{command}
		block := &cfg.Block{Live: true}
		flow := &ps6089LifecycleFlow{
			blocks:        make(map[token.Pos]*cfg.Block),
			postDominates: make(map[ps6089PositionPair]bool),
			postKnown:     make(map[ps6089PositionPair]bool),
			reaches:       make(map[ps6089PositionPair]bool),
			reachKnown:    make(map[ps6089PositionPair]bool),
		}
		entry := token.Pos(1)
		flow.blocks[entry] = block
		var waited []ps6089OrderedReceiver
		var created []ps6089OrderedReceiver
		for index := range events {
			waitPosition := token.Pos(index + 2)
			createPosition := token.Pos(events + index + 2)
			flow.blocks[waitPosition] = block
			flow.blocks[createPosition] = block
			waited = append(waited, ps6089OrderedReceiver{id: id, order: int(waitPosition), pos: waitPosition})
			created = append(created, ps6089OrderedReceiver{id: id, order: int(createPosition), pos: createPosition})
		}
		candidatePosition := token.Pos(2*events + 2)
		candidate := ps6089OrderedReceiver{id: id, order: int(candidatePosition), pos: candidatePosition}
		flow.blocks[candidatePosition] = block
		var committed []ps6089OrderedReceiver
		for index := range events {
			position := token.Pos(2*events + index + 3)
			flow.blocks[position] = block
			committed = append(committed, ps6089OrderedReceiver{id: id, order: int(position), pos: position})
		}
		aliases := &ps6089ReceiverAliasIndex{flow: flow, writes: &ps6089ReceiverWritePath{}, complete: true, limit: 16 * (events + 1)}
		result, used := ps6089LifecycleSequenceWork(&analysis.Pass{}, flow, aliases, entry, created, committed, waited, nil, candidate)
		if result {
			t.Fatal("waits ordered before every commit cannot complete a lifecycle")
		}
		limit := 16 * (len(created) + len(committed) + len(waited) + 1)
		if used > limit {
			t.Fatalf("lifecycle event search exceeded its linear budget: used=%d limit=%d", used, limit)
		}
		return used
	}

	const size = 64
	first := work(size)
	second := work(2 * size)
	if second > 2*first+16 {
		t.Fatalf("doubling lifecycle events grew search work superlinearly: %d -> %d", first, second)
	}
}

func TestPS6089NamedCallScanStopsAtLifecycleBudget(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		var elements strings.Builder
		for range size {
			elements.WriteString("0,")
		}
		source := fmt.Sprintf("package p\nfunc f(){ _ = [%d]int{%s} }", size, elements.String())
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "budget.go", source, 0)
		if err != nil {
			t.Fatalf("parse named-call budget fixture: %v", err)
		}
		info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection)}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check named-call budget fixture: %v", err)
		}
		statement := file.Decls[0].(*ast.FuncDecl).Body.List[0]
		budget := &ps6089LifecycleBudget{limit: size / 2, complete: true}
		if ps6089NamedCallsReturnBudget(&analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}, statement, nil, budget) {
			t.Fatal("an exhausted AST scan must fail closed")
		}
		if budget.complete || budget.work > budget.limit {
			t.Fatalf("AST scan did not stop at its lifecycle budget: complete=%v work=%d limit=%d", budget.complete, budget.work, budget.limit)
		}
		return budget.work
	}

	first := work(64)
	second := work(128)
	if second > 2*first+2 {
		t.Fatalf("doubling AST size grew charged named-call work superlinearly: %d -> %d", first, second)
	}
}

func TestPS6089LifecycleCFGBudgetIsSharedAndBounded(t *testing.T) {
	t.Parallel()

	work := func(size int) int {
		blocks := make([]*cfg.Block, size)
		flow := &ps6089LifecycleFlow{
			blocks:        make(map[token.Pos]*cfg.Block),
			postDominates: make(map[ps6089PositionPair]bool),
			postKnown:     make(map[ps6089PositionPair]bool),
			reaches:       make(map[ps6089PositionPair]bool),
			reachKnown:    make(map[ps6089PositionPair]bool),
		}
		for index := range blocks {
			blocks[index] = &cfg.Block{Live: true}
			if index > 0 {
				blocks[index-1].Succs = []*cfg.Block{blocks[index]}
			}
		}
		budget := &ps6089LifecycleBudget{limit: 32 * size, complete: true}
		for query := range size {
			before := token.Pos(query + 1)
			after := token.Pos(size + query + 1)
			flow.blocks[before] = blocks[0]
			flow.blocks[after] = blocks[len(blocks)-1]
			_ = ps6089PositionPostDominatesBudget(&analysis.Pass{}, flow, before, after, budget)
			if !budget.complete {
				break
			}
		}
		if budget.complete {
			t.Fatal("distinct multi-block postdomination queries must consume the shared fail-closed budget")
		}
		if budget.work > budget.limit {
			t.Fatalf("CFG traversal exceeded its shared budget: work=%d limit=%d", budget.work, budget.limit)
		}
		return budget.work
	}

	first := work(64)
	second := work(128)
	if second > 2*first+32 {
		t.Fatalf("doubling CFG size grew charged lifecycle work superlinearly: %d -> %d", first, second)
	}
}

func TestPS6089CallableFanoutStopsAtSharedBudget(t *testing.T) {
	t.Parallel()

	work := func(size int) (int, bool) {
		var declarations strings.Builder
		var effects strings.Builder
		var calls strings.Builder
		for index := range size {
			fmt.Fprintf(&declarations, "var command%d *Command\n", index)
			fmt.Fprintf(&effects, "command%d = &Command{}\n_ = command%d\n", index, index)
			calls.WriteString("mutate()\n")
		}
		source := `package p
type Command struct{}
func benchmark() {
` + declarations.String() + `
mutate := func() {
` + effects.String() + `}
` + calls.String() + `}`
		files := token.NewFileSet()
		file, err := parser.ParseFile(files, "fanout.go", source, 0)
		if err != nil {
			t.Fatalf("parse callable fanout fixture: %v", err)
		}
		info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection)}
		pkg, err := (&types.Config{}).Check("p", files, []*ast.File{file}, info)
		if err != nil {
			t.Fatalf("type-check callable fanout fixture: %v", err)
		}
		body := file.Decls[1].(*ast.FuncDecl).Body
		index := ps6089ReceiverAliases(&analysis.Pass{Fset: files, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}, body)
		limit := 64 * (size*8 + 128)
		if index.buildWork > limit {
			t.Fatalf("callable fanout exceeded its shared budget: size=%d work=%d limit=%d", size, index.buildWork, limit)
		}
		return index.buildWork, index.complete
	}

	first, firstComplete := work(64)
	second, secondComplete := work(128)
	if !firstComplete || !secondComplete {
		t.Fatalf("deduplicated callable fanout must complete at regression sizes: complete=%v/%v, work=%d/%d", firstComplete, secondComplete, first, second)
	}
	if second > 2*first+256 {
		t.Fatalf("doubling callable fanout grew charged work superlinearly: %d -> %d", first, second)
	}
}

func TestPS6089ReceiverAliasWorkIsLinear(t *testing.T) {
	t.Parallel()

	work := func(writes int) int {
		state := types.NewVar(token.NoPos, nil, "state", types.NewPointer(types.NewStruct(nil, nil)))
		aliasObject := types.NewVar(token.NoPos, nil, "slot", types.NewPointer(state.Type()))
		block := &cfg.Block{Live: true}
		definition := token.Pos(writes + 10)
		use := definition + 1
		flow := &ps6089LifecycleFlow{blocks: make(map[token.Pos]*cfg.Block)}
		flow.blocks[definition] = block
		flow.blocks[use] = block
		node := &ps6089ReceiverWritePath{}
		for index := 0; index < writes; index++ {
			position := token.Pos(index + 1)
			flow.blocks[position] = block
			node.writes = append(node.writes, ps6089ReceiverWrite{id: []types.Object{state}, pos: position})
		}
		index := &ps6089ReceiverAliasIndex{
			aliases: map[types.Object]ps6089ReceiverAlias{
				aliasObject: {id: []types.Object{state}, definition: definition},
			},
			flow:     flow,
			writes:   &ps6089ReceiverWritePath{children: map[types.Object]*ps6089ReceiverWritePath{state: node}},
			limit:    16 * (writes + 2),
			complete: true,
		}
		for range 4 {
			result := index.canonical([]types.Object{aliasObject}, use)
			if len(result) != 1 || result[0] != state {
				t.Fatalf("receiver alias stopped resolving before the budget: %v", result)
			}
		}
		if !index.complete || index.work > index.limit {
			t.Fatalf("receiver alias work did not complete within the linear budget: complete=%v, got %d, limit %d", index.complete, index.work, index.limit)
		}
		return index.work
	}

	const size = 64
	first := work(size)
	second := work(2 * size)
	if second > 2*first+32 {
		t.Fatalf("doubling source writes grew alias work superlinearly: %d -> %d", first, second)
	}

	pathWork := func(fields int) int {
		path := make([]types.Object, fields)
		for index := range path {
			path[index] = types.NewVar(token.NoPos, nil, "field", types.NewPointer(types.NewStruct(nil, nil)))
		}
		aliasObject := types.NewVar(token.NoPos, nil, "alias", types.NewPointer(types.NewStruct(nil, nil)))
		index := &ps6089ReceiverAliasIndex{
			aliases: map[types.Object]ps6089ReceiverAlias{
				aliasObject: {id: path, definition: 1},
			},
			flow:     &ps6089LifecycleFlow{blocks: make(map[token.Pos]*cfg.Block)},
			writes:   &ps6089ReceiverWritePath{},
			limit:    16 * (fields + 2),
			complete: true,
		}
		for range 4 {
			result := index.canonical([]types.Object{aliasObject}, 2)
			if len(result) != fields || !slices.Equal(result, path) {
				t.Fatalf("receiver path stopped resolving before the budget: got %d fields, want %d", len(result), fields)
			}
		}
		if !index.complete || index.work > index.limit {
			t.Fatalf("receiver path work did not complete within the linear budget: complete=%v, got %d, limit %d", index.complete, index.work, index.limit)
		}
		return index.work
	}

	first = pathWork(size)
	second = pathWork(2 * size)
	if second > 2*first+32 {
		t.Fatalf("doubling receiver path length grew alias work superlinearly: %d -> %d", first, second)
	}
}
