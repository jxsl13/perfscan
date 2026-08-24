package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6088(t *testing.T) {
	t.Parallel()
	analysistest.Run(
		t,
		analysistest.TestData(),
		PS6088.Analyzer,
		"ps6088",
		"ps6088initcomma",
		"ps6088initframes",
		"ps6088initorder",
		"ps6088initselect",
		"ps6088initswitch",
		"ps6088iter",
		"ps6088reach",
	)
}

func TestPS6088ConditionSwitchIndexIsReused(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), "switch.go", `package p
func f() {
	switch true {
	case false:
	}
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	statement := file.Decls[0].(*ast.FuncDecl).Body.List[0].(*ast.SwitchStmt)
	condition := statement.Body.List[0].(*ast.CaseClause).List[0]
	pass := &analysis.Pass{Files: []*ast.File{file}}
	t.Cleanup(func() { ps6088SwitchOwners.Delete(pass) })

	if got := ps6088ConditionSwitch(pass, condition); got != statement {
		t.Fatalf("first lookup = %p, want %p", got, statement)
	}
	pass.Files = nil
	if got := ps6088ConditionSwitch(pass, condition); got != statement {
		t.Fatalf("cached lookup = %p, want %p", got, statement)
	}
}

func TestPS6088LocalDomainIndexIsReused(t *testing.T) {
	t.Parallel()

	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "local.go", `package p
func f() {
	limit := 2
	_ = limit
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := new(types.Config).Check("p", fileset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	function := file.Decls[0].(*ast.FuncDecl)
	t.Cleanup(func() { ps6088LocalDomains.Delete(pass) })

	first := ps6088FunctionLocalDomainIndex(pass, function)
	if len(first.definitions) != 1 {
		t.Fatalf("definitions = %d, want 1", len(first.definitions))
	}
	function.Body.List = nil
	second := ps6088FunctionLocalDomainIndex(pass, function)
	if second != first {
		t.Fatalf("cached index = %p, want %p", second, first)
	}
	if len(second.definitions) != 1 {
		t.Fatalf("cached definitions = %d, want 1", len(second.definitions))
	}
}

func TestPS6088StableObjectOwnerIndexIsReused(t *testing.T) {
	t.Parallel()

	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "owners.go", `package p
func first() {
	values := []int{}
	_ = values
}
func second() {
	values := []int{}
	_ = values
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := new(types.Config).Check("p", fileset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	t.Cleanup(func() { ps6088LocalDomains.Delete(pass) })
	first := file.Decls[0].(*ast.FuncDecl).Body.List[1].(*ast.AssignStmt).Rhs[0]
	second := file.Decls[1].(*ast.FuncDecl).Body.List[1].(*ast.AssignStmt).Rhs[0]

	resolved, zero, safe := ps6088StableExpression(pass, first)
	if _, literal := resolved.(*ast.CompositeLit); !literal || zero || !safe {
		t.Fatalf("first stable expression = (%T, %t, %t), want composite, false, true", resolved, zero, safe)
	}
	pass.Files = nil
	resolved, zero, safe = ps6088StableExpression(pass, second)
	if _, literal := resolved.(*ast.CompositeLit); !literal || zero || !safe {
		t.Fatalf("cached-owner stable expression = (%T, %t, %t), want composite, false, true", resolved, zero, safe)
	}
}

func TestPS6088StatementReturnIndexIsReused(t *testing.T) {
	t.Parallel()

	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "statement.go", `package p
func f() { _ = 1 }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := new(types.Config).Check("p", fileset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	statement := file.Decls[0].(*ast.FuncDecl).Body.List[0]
	t.Cleanup(func() { ps6088StatementReturns.Delete(pass) })

	first := ps6088StatementReturnIndexEntry(pass, statement)
	if !ps6088StatementReturnsNormally(pass, statement, false) {
		t.Fatal("first statement-return lookup = false, want true")
	}
	pass.TypesInfo = nil
	second := ps6088StatementReturnIndexEntry(pass, statement)
	if second != first {
		t.Fatalf("cached entry = %p, want %p", second, first)
	}
	if !ps6088StatementReturnsNormally(pass, statement, false) {
		t.Fatal("cached statement-return lookup = false, want true")
	}
}

func TestPS6088ExpressionReturnIndexIsReused(t *testing.T) {
	t.Parallel()

	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "expression.go", `package p
var value = 1
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := new(types.Config).Check("p", fileset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	expression := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0]
	t.Cleanup(func() { ps6088ExpressionReturns.Delete(pass) })

	first := ps6088ExpressionReturnIndexEntry(pass, expression)
	if !ps6088ExpressionReturnsNormally(pass, expression) {
		t.Fatal("first expression-return lookup = false, want true")
	}
	pass.TypesInfo = nil
	second := ps6088ExpressionReturnIndexEntry(pass, expression)
	if second != first {
		t.Fatalf("cached entry = %p, want %p", second, first)
	}
	if !ps6088ExpressionReturnsNormally(pass, expression) {
		t.Fatal("cached expression-return lookup = false, want true")
	}
}

func TestPS6088DirectPanicIndexIsReused(t *testing.T) {
	t.Parallel()

	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "panic.go", `package p
var value = ((((*[2]int)([]int{}))))
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	pkg, err := new(types.Config).Check("p", fileset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{Fset: fileset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
	expression := file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.ValueSpec).Values[0]
	t.Cleanup(func() { ps6088DirectPanics.Delete(pass) })

	if !ps6088DirectExpressionPanics(pass, expression) {
		t.Fatal("first direct-panic lookup = false, want true")
	}
	passIndex, ok := ps6088DirectPanics.Load(pass)
	if !ok {
		t.Fatal("direct-panic pass index is missing")
	}
	panicIndex := passIndex.(*ps6088DirectPanicPassIndex)
	count := func() int {
		entries := 0
		panicIndex.expressions.Range(func(_, _ any) bool {
			entries++
			return true
		})
		return entries
	}
	entries := count()
	for current := expression; ; {
		parenthesized, ok := current.(*ast.ParenExpr)
		if !ok {
			break
		}
		if !ps6088DirectExpressionPanics(pass, parenthesized) {
			t.Fatal("parenthesized direct-panic lookup = false, want true")
		}
		current = parenthesized.X
	}
	if got := count(); got != entries {
		t.Fatalf("cached direct-panic entries = %d, want %d", got, entries)
	}
	pass.TypesInfo = nil
	if !ps6088DirectExpressionPanics(pass, expression) {
		t.Fatal("cached direct-panic lookup = false, want true")
	}
}
