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

func TestPS6080RequiredRoleSeeds(t *testing.T) {
	t.Parallel()
	for mask := range 64 {
		t.Run(fmt.Sprint(mask), func(t *testing.T) {
			t.Parallel()
			functions := make(map[*types.Func]*ps6080Function)
			roles := []ps6080Role{ps6080StorageRole, ps6080DecodeRole, ps6080MatmulRole}
			for index, role := range roles {
				if mask&(1<<index) == 0 {
					continue
				}
				object := types.NewFunc(token.NoPos, nil, fmt.Sprint(index), nil)
				functions[object] = &ps6080Function{
					object: object, roles: role, backend: mask&(1<<(index+3)) != 0,
					// Incoming edges cannot disqualify a possible seed before the
					// later indirect-reference analysis has populated its facts.
					cpuIncoming: true,
				}
			}
			want := mask&7 == 7 && mask&(1<<4) == 0 && mask&(1<<5) == 0
			if got := ps6080HasRequiredRoleSeeds(functions); got != want {
				t.Fatalf("mask=%06b has required seeds=%v want=%v", mask, got, want)
			}
		})
	}
	t.Run("one declaration may seed multiple roles", func(t *testing.T) {
		t.Parallel()
		functions := map[*types.Func]*ps6080Function{nil: {roles: ps6080StorageRole | ps6080DecodeRole | ps6080MatmulRole}}
		if !ps6080HasRequiredRoleSeeds(functions) {
			t.Fatal("combined role declaration must remain eligible for full analysis")
		}
	})
}

func TestPS6080ImpossibleSurfaceWithRecursiveCallbacks(t *testing.T) {
	t.Parallel()
	const recursive = `
func walk(f func(), n int) { if n == 0 { f(); return }; walk(f,n-1); walk(f,n-1) }
func unrelated0(){}
func unrelated1(){}
func unrelated2(){}
`
	for _, source := range []string{
		"package p\n" + recursive,
		"package p\nfunc quantBlockByteSize()int{return 1}\nfunc portableDecode(){}\n" + recursive,
		"package p\nfunc quantBlockByteSize()int{return 1}\nfunc QMatMul(){}\n" + recursive,
		"package p\nfunc portableDecode(){}\nfunc QMatMul(){}\n" + recursive,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			if got := ps6080SurfacePreflightDiagnostics(t, source); len(got) != 0 {
				t.Fatalf("missing role surface produced diagnostics: %v", got)
			}
		})
	}
}

func TestPS6080SurfacePreflightPreservesLayeredGap(t *testing.T) {
	t.Parallel()
	const source = `package p
type Quant uint8
const (Q4 Quant=iota; IQ2; Q8)
func quantBlockByteSize(q Quant)int {switch q{case Q4,IQ2,Q8:return 32};return 0}
func portableDecode(q Quant)bool {switch q{case Q4,IQ2,Q8:return true};return false}
func qMatMulSupported(q Quant)bool {return q==Q4||q==Q8}
func qMatMulRow(q Quant)bool {switch q{case Q4,Q8:return true};return false}
func QMatMul(q Quant)bool {if qMatMulSupported(q){return qMatMulRow(q)};return false}
`
	got := ps6080SurfacePreflightDiagnostics(t, source)
	if len(got) != 1 || !strings.Contains(got[0], "quant variant IQ2") {
		t.Fatalf("live layered gap lost: %v", got)
	}
}

func ps6080SurfacePreflightDiagnostics(t *testing.T, source string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "surface.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{
		Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object),
		Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection),
		Instances: make(map[*ast.Ident]types.Instance),
	}
	sizes := types.SizesFor("gc", "arm64")
	pkg, err := (&types.Config{Sizes: sizes}).Check("preflight/probe", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics []string
	pass := &analysis.Pass{
		Analyzer: PS6080.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg,
		TypesInfo: info, TypesSizes: sizes,
		Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d.Message) },
	}
	if _, err := runPS6080(pass); err != nil {
		t.Fatal(err)
	}
	return diagnostics
}
