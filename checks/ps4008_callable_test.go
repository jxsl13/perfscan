package checks

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestPS4008StoredCallableIndirectEffects(t *testing.T) {
	t.Parallel()
	const source = `package sample
func f() {
	base := 1
	p := &base
	mutate := func() { *p = 0 }
	mutate()
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "callable.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("sample", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	index := ps1006BuildAnalysisIndex(pass)
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	defer ps1006ActiveAnalysisIndexes.Delete(pass)

	function := file.Decls[0].(*ast.FuncDecl)
	base := info.Defs[function.Body.List[0].(*ast.AssignStmt).Lhs[0].(*ast.Ident)]
	call := function.Body.List[3].(*ast.ExprStmt).X.(*ast.CallExpr)
	targets, known := ps4008CallableWriteTargets(pass, call.Fun, map[types.Object]bool{base: true}, call.Pos(), make(map[types.Object]bool))
	if !known {
		t.Fatal("stored closure effect was not resolved")
	}
	if !targets[base] {
		t.Fatalf("stored closure indirect write targets %v, want base", targets)
	}
	strideTargets := make(map[types.Object]bool)
	if !ps1006KillCallableWriteTargetsMode(pass, call.Fun, map[types.Object]string{base: "stride"}, strideTargets, call.Pos(), ps4008MayWrite) {
		t.Fatal("stored closure stride effect was not resolved")
	}
	if !strideTargets[base] {
		t.Fatalf("stored closure stride write targets %v, want base", strideTargets)
	}
}

func TestPS4008NestedCallableEffectsAreNearLinear(t *testing.T) {
	t.Parallel()
	const depth = 24
	var source strings.Builder
	source.WriteString("package sample\nfunc f() {\nbase := 1\np := &base\nf0 := func() { *p = 0 }\n")
	for level := 1; level <= depth; level++ {
		fmt.Fprintf(&source, "f%d := func() { f%d(); f%d() }\n", level, level-1, level-1)
	}
	fmt.Fprintf(&source, "f%d()\n}\n", depth)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "nested_callable.go", source.String(), parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("sample", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	index := ps1006BuildAnalysisIndex(pass)
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	defer ps1006ActiveAnalysisIndexes.Delete(pass)

	function := file.Decls[0].(*ast.FuncDecl)
	base := info.Defs[function.Body.List[0].(*ast.AssignStmt).Lhs[0].(*ast.Ident)]
	call := function.Body.List[len(function.Body.List)-1].(*ast.ExprStmt).X.(*ast.CallExpr)
	targets, known := ps4008CallableWriteTargets(pass, call.Fun, map[types.Object]bool{base: true}, call.Pos(), make(map[types.Object]bool))
	if !known || !targets[base] {
		t.Fatalf("nested callable effect unresolved: known=%v targets=%v", known, targets)
	}
	if index.callableVisits > depth+1 {
		t.Fatalf("nested callable summary work grew beyond one visit per literal: got %d, want <= %d", index.callableVisits, depth+1)
	}
}

func TestPS1006StoredCallbackPreservesReadModifyWriteDependency(t *testing.T) {
	t.Parallel()
	const source = `package sample
func keep(target *int) { *target += 0 }
func invoke(callback func()) { callback() }
func f(inner int) {
	base := inner
	callback := func() { keep(&base) }
	invoke(callback)
}`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "stored_callback.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("sample", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	index := ps1006BuildAnalysisIndex(pass)
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	defer ps1006ActiveAnalysisIndexes.Delete(pass)

	function := file.Decls[2].(*ast.FuncDecl)
	inner := info.Uses[function.Type.Params.List[0].Names[0]]
	if inner == nil {
		inner = info.Defs[function.Type.Params.List[0].Names[0]]
	}
	base := info.Defs[function.Body.List[0].(*ast.AssignStmt).Lhs[0].(*ast.Ident)]
	call := function.Body.List[2].(*ast.ExprStmt).X.(*ast.CallExpr)
	output, _, known := ps1006OrderedCallDependencyState(pass, call, inner, ps1006DependencyState{
		deps: map[types.Object]bool{base: true}, strideDeps: map[types.Object]string{base: "stride"},
		pointerAliases: make(map[types.Object]ps1006OrderedPointerAlias),
	})
	if !known || !output.deps[base] || output.strideDeps[base] == "" {
		t.Fatalf("stored callback RMW lost dependency: known=%v derived=%v stride=%q", known, output.deps[base], output.strideDeps[base])
	}
}

func TestPS4008RecursiveReceiverEffectsAreConservative(t *testing.T) {
	t.Parallel()
	const source = `package sample
type box struct{ value int }
func (b *box) direct() { b.direct() }
func (b *box) left() { b.right() }
func (b *box) right() { b.left() }
func (b *box) aliasCycle() { next := b.aliasCycle; next() }
func (b *box) expressionCycle() { (*box).expressionCycle(b) }
func invoke(callback func()) { callback() }
func (b *box) callbackCycle() { invoke(b.callbackCycle) }
func (b *box) readOnly() { _ = b.value }
func (b *box) write() { b.value++ }
type genericBox[T any] struct{ value T }
func (b *genericBox[T]) cycle() { b.cycle() }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "recursive_receiver.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := (&types.Config{}).Check("sample", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	index := ps1006BuildAnalysisIndex(pass)
	ps1006ActiveAnalysisIndexes.Store(pass, index)
	defer ps1006ActiveAnalysisIndexes.Delete(pass)

	want := map[string]bool{
		"direct": true, "left": true, "right": true,
		"aliasCycle": true, "expressionCycle": true, "callbackCycle": true, "cycle": true,
		"readOnly": false, "write": true,
	}
	seen := make(map[string]bool)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil {
			continue
		}
		expected, ok := want[function.Name.Name]
		if !ok {
			continue
		}
		seen[function.Name.Name] = true
		syntax := ps4008CallableSyntaxValue{functionType: function.Type, receiver: function.Recv, receiverArgument: true, body: function.Body}
		receiver := ps4008SyntaxParameterObject(pass, syntax, 0)
		if got := ps4008CallableMayWriteParameter(pass, function.Body, receiver); got != expected {
			t.Errorf("%s receiver may-write = %v, want %v", function.Name.Name, got, expected)
		}
		// Repeating the query exercises the completed per-pass summary rather
		// than re-entering either a direct or mutual cycle.
		if got := ps4008CallableMayWriteParameter(pass, function.Body, receiver); got != expected {
			t.Errorf("cached %s receiver may-write = %v, want %v", function.Name.Name, got, expected)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("receiver method %s was not exercised", name)
		}
	}
}
