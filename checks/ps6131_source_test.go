package checks

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
)

func ps6131PinnedFunction(t *testing.T, file, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, data, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Name.Name == name {
			return string(data[fset.Position(fn.Pos()).Offset:fset.Position(fn.End()).Offset])
		}
	}
	t.Fatalf("missing pinned function %s", name)
	return ""
}

func TestPS6131PinnedOwnerSourceIdentities(t *testing.T) {
	t.Parallel()
	for file, digest := range map[string]string{
		"ps6131_owner_leaf.go.txt":       "3a745c488a6c6f54a86363b2d37927e66c2bebae46632b0d75b6db9293d960e8",
		"ps6131_owner_operation.go.txt":  "234e25273a7d9f23c4f7ba9a20fc9fa34b6fbba69c0e3ff232efee8167b7cd7f",
		"ps6131_owner_scheduler.go.txt":  "f7d4cece7bc0dea547ef2c220d494e7d4f0dc4bad772b76cfab65e7d7fb2baee",
		"ps6131_owner_benchmarks.go.txt": "7a0a7ad43e59841a55cc7360ca376a335a363b98feff538531f6cb223dcf9136",
	} {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/" + file)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != digest {
				t.Fatalf("owner source digest %s", got)
			}
		})
	}
}

func TestPS6131ActualOwnerTensorOperationRoute(t *testing.T) {
	t.Parallel()
	operation := ps6131PinnedFunction(t, "ps6131_owner_operation.go.txt", "absKernelCPU")
	// Only the external parameter/result types are modeled. The production
	// operation body and its dtype switch/serial-break/worker slices stay exact.
	operation = strings.NewReplacer("*backend.Context", "*Context", "*tensor.Tensor", "*Tensor", "backend.Attrs", "Attrs").Replace(operation)
	leaf := ps6131PinnedFunction(t, "ps6131_owner_leaf.go.txt", "absF32")
	source := `package cross
import("math";"fmt")
type Context struct{}
func(*Context)Device()int{return 0}
type Attrs struct{}
type Tensor struct{}
func(*Tensor)Contiguous()*Tensor{return nil}
func(*Tensor)Dtype()int{return 0}
func(*Tensor)Shape()[]int{return nil}
func(*Tensor)Storage()*Storage{return nil}
type Storage struct{}
func(*Storage)F32()[]float32{return nil}
func(*Storage)F64()[]float64{return nil}
var tensor=struct{F32,F64 int;NewOn func(int,int,[]int)*Tensor}{}
const absF32ParallelThreshold=1<<21
func absF32BlocksNeon(dst,src *float32,blocks int)
func parallel(n int,body func(int,int)){body(0,n)}
` + leaf + "\n" + operation
	c := ps6131TestContract()
	c.DispatchFunction = "cross.absKernelCPU"
	c.ThresholdConstant = "cross.absF32ParallelThreshold"
	c.SerialPolicy = "cross.absF32"
	c.ParallelPolicy = "cross.parallel"
	c.WorkerRunner = "cross.parallel"
	c.LeafKernels = []string{"cross.absF32BlocksNeon"}
	s, ok := ps6131SourceBound(ps6131SourcePass(t, source), &c)
	if !ok || s.boundary != 1<<21 || s.dtype != "float32" {
		t.Fatalf("actual owner tensor route unbound: %+v %v", s, ok)
	}
	for _, tc := range []struct{ old, new string }{
		{"absF32(o, d)\n\t\t\tbreak", "absF32(o, d)"},
		{"len(o) < absF32ParallelThreshold", "len(d) < absF32ParallelThreshold"},
		{"absF32(o[lo:hi], d[lo:hi])", "absF32(o[lo:hi], o[lo:hi])"},
	} {
		altered := strings.Replace(source, tc.old, tc.new, 1)
		if altered == source {
			t.Fatalf("bad owner mutation %q", tc.old)
		}
		if _, ok := ps6131SourceBound(ps6131SourcePass(t, altered), &c); ok {
			t.Fatalf("altered owner route accepted: %s", tc.new)
		}
	}
	for _, prefix := range []string{"if false {\n", "return nil,nil\n"} {
		altered := strings.Replace(operation, "{\n\txc :=", "{\n"+prefix+"\txc :=", 1)
		if strings.HasPrefix(prefix, "if false") {
			altered = strings.Replace(altered, "return []*Tensor{out}, nil\n}", "return []*Tensor{out}, nil\n}\nreturn nil,nil\n}", 1)
		}
		if altered == operation {
			t.Fatal("dead-route fixture did not change")
		}
		if _, ok := ps6131SourceBound(ps6131SourcePass(t, strings.Replace(source, operation, altered, 1)), &c); ok {
			t.Fatal("associated unreachable owner dispatch route")
		}
	}
}

const ps6131SourceFixture = `package cross
const measuredBoundary = 1<<18
func leaf(dst,src []float32) { for i:=range dst { dst[i]=-src[i] } }
func worker(n int,work func(int,int)) { work(0,n) }
func serial(dst,src []float32) { leaf(dst,src) }
func parallel(dst,src []float32) { worker(len(src),func(lo,hi int){ leaf(dst[lo:hi],src[lo:hi]) }) }
func serialOperation(src []float32) []float32 { dst:=make([]float32,len(src)); serial(dst,src); return dst }
func parallelOperation(src []float32) []float32 { dst:=make([]float32,len(src)); parallel(dst,src); return dst }
func dispatch(src []float32) []float32 {
 dst:=make([]float32,len(src))
 if len(src)<measuredBoundary { serial(dst,src) } else { parallel(dst,src) }
 return dst
}
`

func ps6131TestContract() config.DispatchCrossoverContract {
	return config.DispatchCrossoverContract{DispatchFunction: "cross.dispatch", ThresholdConstant: "cross.measuredBoundary", SerialPolicy: "cross.serial", ParallelPolicy: "cross.parallel", SerialOperation: "cross.serialOperation", ParallelOperation: "cross.parallelOperation", WorkerRunner: "cross.worker", LeafKernels: []string{"cross.leaf"}, Campaign: "retained-campaign"}
}

func ps6131SourcePass(t *testing.T, source string) *analysis.Pass {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cross.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, Selections: map[*ast.SelectorExpr]*types.Selection{}, Scopes: map[ast.Node]*types.Scope{}}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("cross", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	return &analysis.Pass{Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, TypesSizes: types.SizesFor("gc", "amd64")}
}

func TestPS6131SourceBoundBoundaryAndClosure(t *testing.T) {
	t.Parallel()
	c := ps6131TestContract()
	s, ok := ps6131SourceBound(ps6131SourcePass(t, ps6131SourceFixture), &c)
	if !ok || s.boundary != 1<<18 || s.dtype != "float32" || s.position == token.NoPos {
		t.Fatalf("source association: %+v %v", s, ok)
	}
}

func TestPS6131SourceAssociationRejectsConfigOnlyDependencies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, old, new string }{
		{"deferred_leaf", "func serial(dst,src []float32) { leaf(dst,src) }", "func serial(dst,src []float32) { defer leaf(dst,src) }"},
		{"asynchronous_leaf", "func serial(dst,src []float32) { leaf(dst,src) }", "func serial(dst,src []float32) { go leaf(dst,src) }"},
		{"global_function_value", "func serial(dst,src []float32) { leaf(dst,src) }", "var callable=leaf\nfunc serial(dst,src []float32) { callable(dst,src) }"},
		{"uncalled_callback_argument", "func serial(dst,src []float32) { leaf(dst,src) }", "func unused(work func([]float32,[]float32)){}\nfunc serial(dst,src []float32) { unused(leaf) }"},
		{"neighbor_threshold", "len(src)<measuredBoundary", "len(src)<(measuredBoundary+1)"},
		{"wrong_side", "len(src)<measuredBoundary", "len(src)<=measuredBoundary"},
		{"constant_shape", "len(src)<measuredBoundary", "12<measuredBoundary"},
		{"same_arm", "else { parallel(dst,src) }", "else { serial(dst,src) }"},
		{"wrong_input", "serial(dst,src) } else", "serial(dst,dst) } else"},
		{"partial_result", "make([]float32,len(src))\n if", "make([]float32,12)\n if"},
		{"unused_worker", "worker(len(src),func(lo,hi int){ leaf(dst[lo:hi],src[lo:hi]) })", "leaf(dst,src)"},
		{"uninvoked_callback", "work(0,n)", "_ = work"},
		{"dead_callback", "work(0,n)", "if false { work(0,n) }"},
		{"overwritten_callback", "work(0,n)", "work=func(int,int){}; work(0,n)"},
		{"uninvoked_leaf", "leaf(dst[lo:hi],src[lo:hi])", "_ = func(){ leaf(dst[lo:hi],src[lo:hi]) }"},
		{"operation_not_policy", "serial(dst,src); return dst", "leaf(dst,src); return dst"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			c := ps6131TestContract()
			if _, ok := ps6131SourceBound(ps6131SourcePass(t, strings.Replace(ps6131SourceFixture, tc.old, tc.new, 1)), &c); ok {
				t.Fatal("unbound association accepted")
			}
		})
	}
}
