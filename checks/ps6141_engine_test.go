package checks

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

// Synthetic, complete, typed Go controls; not historical owner source or a
// reproduction of issue #835's measurements. The conversion is deliberately
// simple so source-owned freshness/shape proof is non-vacuous.
const ps6141Synthetic = `package fixture
func pack(x []float32) []int8 {
 p:=make([]int8,len(x))
 for i:=range x {p[i]=int8(x[i])}
 return p
}
func dot(p []int8,rows int) int {if rows!=1{panic("unsupported rows")};sum:=0;for _,v:=range p{sum+=int(v)*int(v)};return sum}
var sink []int8
func owner(x []float32,rows int) int {
 p:=pack(x)
 return dot(p,1)
}`

func TestPS6141StageOneTypedAdmission(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, old, new string
		want           int
	}{
		{"freshSingleRow", "", "", 1},
		{"constantFalse", "p:=pack(x)\n return dot(p,1)", "if false {p:=pack(x);return dot(p,1)};return 0", 0},
		{"constantTrueElse", "p:=pack(x)\n return dot(p,1)", "if true{return 0}else{p:=pack(x);return dot(p,1)}", 0},
		{"falseLoop", "p:=pack(x)\n return dot(p,1)", "for false{p:=pack(x);return dot(p,1)};return 0", 0},
		{"consumerRetains", "if rows!=1", "sink=p;if rows!=1", 0},
		{"consumerRepeats", "return sum}", "for _,v:=range p{sum+=int(v)*int(v)};return sum}", 0},
		{"consumerAmplifiesRows", "return sum}", "return sum*rows*2}", 0},
		{"consumerIgnoresRows", "if rows!=1", "if false", 0},
		{"consumerForwards", "if rows!=1{panic(\"unsupported rows\")};sum:=0;for _,v:=range p{sum+=int(v)*int(v)};return sum", "return dot(p,rows)", 0},
		{"phiStorage", "return dot(p,1)", "if rows>0{p=sink};return dot(p,1)", 0},
		{"multipleConsumers", "return dot(p,1)", "dot(p,1);return dot(p,1)", 0},
		{"multipleRows", "return dot(p,1)", "return dot(p,2)", 0},
		{"dynamicRows", "return dot(p,1)", "return dot(p,rows)", 0},
		{"capture", "return dot(p,1)", "defer func(){sink=p}();return dot(p,1)", 0},
		{"returnedAlias", "return dot(p,1)", "return len(p)", 0},
		{"sliceAlias", "return dot(p,1)", "return dot(p[:],1)", 0},
		{"alternateConsumer", "return dot(p,1)", "if rows>0{return dot(p,1)};return 0", 0},
		{"redirectedStorage", "return dot(p,1)", "p=sink;return dot(p,1)", 0},
		{"unreachable", "p:=pack(x)", "return 0;p:=pack(x)", 0},
		{"cachedStorage", "p:=make([]int8,len(x))", "p:=sink", 0},
		{"wrongGeometry", "p:=make([]int8,len(x))", "p:=make([]int8,len(x)+1)", 0},
		{"allocationFreeScratch", "p:=pack(x)", "p:=sink", 0},
		{"floatRowAPI", "func dot(p []int8,rows int) int {if rows!=1{panic(\"unsupported rows\")};sum:=0;for _,v:=range p{sum+=int(v)*int(v)};return sum}", "func dot(p []int8,rows float64) int {return len(p)*int(rows)}", 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := ps6141Synthetic
			if test.old != "" {
				source = strings.ReplaceAll(source, test.old, test.new)
				if source == ps6141Synthetic {
					t.Fatal("mutation did not change fixture")
				}
			}
			pass, owner := ps6141TypedFixture(t, source)
			contract := ps6141TestContract()
			got := ps6141Candidates(pass, owner, &contract)
			if len(got) != test.want {
				t.Fatalf("candidate count=%d want=%d", len(got), test.want)
			}
			if test.want > 0 && (got[0].quantizer.Pos() == token.NoPos || got[0].consumer.Pos() <= got[0].quantizer.Pos()) {
				t.Fatal("missing actual ordered call witnesses")
			}
		})
	}
}

func TestPS6141StageOneContractGates(t *testing.T) {
	t.Parallel()
	pass, owner := ps6141TypedFixture(t, ps6141Synthetic)
	c := ps6141TestContract()
	if len(ps6141Candidates(pass, owner, &c)) != 1 {
		t.Fatal("positive prerequisite missing")
	}
	c.BenchmarkExemptOwners = []string{"fixture.owner"}
	c.BenchmarkExemptionReason = "reviewed shape-specific conversion amortization campaign"
	if len(ps6141Candidates(pass, owner, &c)) != 0 {
		t.Fatal("reviewed benchmark exemption ignored")
	}
	c = ps6141TestContract()
	c.QuantizationAndDotMeaningReviewed = false
	if len(ps6141Candidates(pass, owner, &c)) != 0 {
		t.Fatal("unreviewed semantic names admitted")
	}
	c = ps6141TestContract()
	c.Consumer = "fixture.missing"
	if len(ps6141Candidates(pass, owner, &c)) != 0 {
		t.Fatal("wrong typed consumer admitted")
	}
}

func ps6141TestContract() config.SingleUseQuantizationContract {
	return config.SingleUseQuantizationContract{Quantizer: "fixture.pack", Consumer: "fixture.dot", PackedArgument: 0, RowsArgument: 1, QuantizationAndDotMeaningReviewed: true}
}

func ps6141TypedFixture(t *testing.T, source string) (*analysis.Pass, *ast.FuncDecl) {
	t.Helper()
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, "synthetic.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection)}
	sizes := types.SizesFor("gc", "amd64")
	pkg, err := (&types.Config{Importer: importer.Default(), Sizes: sizes}).Check("fixture", fs, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	var owner *ast.FuncDecl
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Name.Name == "owner" {
			owner = fn
		}
	}
	if owner == nil {
		t.Fatal("owner missing")
	}
	return &analysis.Pass{Fset: fs, Files: []*ast.File{file}, TypesInfo: info, Pkg: pkg, TypesSizes: sizes}, owner
}
