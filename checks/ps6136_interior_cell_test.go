package checks

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestPS6136InteriorCellEffects(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body, extra string
		want              bool
	}{
		{"source-method-chain", "g.release()", "", true},
		{"scalar-field-helper", "clear(&g.n)", "func clear(n *int){*n=0}", true},
		{"same-pointer-two-formals", "pair(g,g)", "func pair(a,b *cell){a.release();b.release()}", true},
		{"global-retention", "saved=g", "", false},
		{"interior-retention", "field=&g.n", "", false},
		{"interface-retention", "boxed=g", "", false},
		{"closure", "closure=func(){g.release()}", "", false},
		{"method-value", "closure=g.release", "", false},
		{"asynchronous", "go g.release()", "", false},
		{"deferred", "defer g.release()", "", false},
		{"opaque-pointer", "opaque(g)", "", false},
		{"opaque-field", "opaqueInt(&g.n)", "", false},
		{"whole-cell-write", "*g=cell{}", "", false},
		{"whole-cell-copy", "copy:=*g;boxed=copy", "", false},
		{"returned-pointer", "saved=identity(g)", "func identity(g *cell)*cell{return g}", false},
		{"other-formal-escape", "pair(g,g)", "func pair(a,b *cell){a.release();saved=b}", false},
		{"recursive", "consume(g)", "", false},
		{"mutual-recursion", "relay(g)", "func relay(g *cell){consume(g)}", false},
		{"implicit-other-site-escape", "g.release()", "func (g *cell) escape(){saved=g};func other(o *owner){o.cache.escape()}", false},
		{"pointer-cast", "boxed=unsafe.Pointer(g)", "", false},
		{"linkname-identity", "g.release()", "\n//go:linkname consume elsewhere.consume\n", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := `package cells
import "unsafe"
var _ unsafe.Pointer
type buffer interface{Release()}
type cell struct{b buffer;n int}
type owner struct{blocks []int;cache cell}
var saved *cell
var field *int
var boxed any
var closure func()
func opaque(*cell)
func opaqueInt(*int)
func (g *cell) release(){if g.b!=nil{g.b.Release()};g.b=nil;g.n=0}
func consume(g *cell){` + test.body + `}
func use(o *owner){consume(&o.cache)}
` + test.extra
			pass, pkg := ps6136CellTestPackage(t, source)
			owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
			workspace := ps6136FieldVar(owner, "blocks")
			proved := ps6136InteriorOwnerCells(pass, pkg, owner, workspace)
			if (len(proved) == 1) != test.want {
				t.Fatalf("proved %d addresses, want accepted=%v", len(proved), test.want)
			}
			for node := range proved {
				if _, ok := node.(*ast.UnaryExpr); !ok {
					t.Fatal("approval escaped the exact address node")
				}
			}
			if test.want && !ps6136ClosedObservations(pass, owner, workspace, proved) {
				t.Fatal("closed cell did not satisfy strict owner observations")
			}
		})
	}
}

func ps6136CellTestPackage(t *testing.T, source string) (*analysis.Pass, *ssa.Package) {
	t.Helper()
	fileset := token.NewFileSet()
	file, err := parser.ParseFile(fileset, "cells.go", source, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	pkg, info, err := ssautil.BuildPackage(&types.Config{Importer: importer.Default()}, fileset, types.NewPackage("cells", "cells"), []*ast.File{file}, ssa.SanityCheckFunctions)
	if err != nil {
		t.Fatal(err)
	}
	return &analysis.Pass{Fset: fileset, Files: []*ast.File{file}, TypesInfo: info, Pkg: pkg.Pkg}, pkg
}

func TestPS6136InteriorCellBudgetAndSiblingExposure(t *testing.T) {
	t.Parallel()
	pass, pkg := ps6136CellTestPackage(t, `package cells
type cell struct{n int};type owner struct{blocks []int;cache cell}
func consume(g *cell,o *owner){g.n=0}
func use(o *owner){consume(&o.cache,o)}
`)
	owner := pkg.Pkg.Scope().Lookup("owner").Type().(*types.Named)
	workspace := ps6136FieldVar(owner, "blocks")
	proved := ps6136InteriorOwnerCells(pass, pkg, owner, workspace)
	if len(proved) != 1 {
		t.Fatal("cell-address prerequisite absent")
	}
	if ps6136ClosedObservations(pass, owner, workspace, proved) {
		t.Fatal("cell address approval hid sibling whole-owner argument")
	}
	graph := &ps6136CellUseGraph{pkg: pkg, remaining: 1, active: make(map[ps6136CellUse]bool), done: make(map[ps6136CellUse]bool)}
	if graph.close(ps6136CellUse{value: pkg.Func("consume").Params[0]}, 0) {
		t.Fatal("exhausted pointer effect budget accepted")
	}
}
