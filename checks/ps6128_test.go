package checks

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6128FullTypedNativeChain(t *testing.T) {
	t.Parallel()
	contract := ps6128TestContract()
	analyzer := &analysis.Analyzer{Name: "PS6128", Doc: "test", Run: func(pass *analysis.Pass) (any, error) {
		return runPS6128WithContracts(pass, []config.NativeGenerationDispatchContract{contract})
	}}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6128")
}

func TestPS6128AdversarialSuppressions(t *testing.T) {
	t.Parallel()
	base := `package casepkg
import "strings"
var gemm func(int)
func brandString() string { return "" }
func amxSupported() bool { brand := brandString(); return strings.Contains(brand,"Apple M1") || strings.Contains(brand,"Apple M2") || strings.Contains(brand,"Apple M3") }
func init(){ if amxSupported(){ gemm=compute } }
func parallelWork(work func()){ work() }
func compute(k int){ parallelWork(func(){ tile(k) }) }
func caller(k int){ gemm(k) }
func tile(k int)
`
	assembly := "#include \"textflag.h\"\n#define AMX_LDY_R5 WORD $0x00201025\nTEXT ·tile(SB), NOSPLIT, $0-8\n MOVD $0x5000000000000000, R16\n ORR R16, R0, R5\n LSR $2, R3, R15\n AMX_LDY_R5\n EOR $0x1000000000000000, R5, R5\n RET\n"
	for _, tc := range []struct {
		name, source, asm string
		allRequired       bool
	}{
		{"overwritten_dispatch", strings.Replace(base, "func caller", "func overwrite(){ gemm=nil }\nfunc caller", 1), assembly, false},
		{"unused_dispatch", strings.Replace(base, "func caller(k int){ gemm(k) }", "", 1), assembly, false},
		{"uninvoked_callback", strings.Replace(base, "func parallelWork(work func()){ work() }", "func parallelWork(work func()){}", 1), assembly, false},
		{"overwritten_callback", strings.Replace(base, "func parallelWork(work func()){ work() }", "func parallelWork(work func()){ work=func(){}; work() }", 1), assembly, false},
		{"dead_callback", strings.Replace(base, "func parallelWork(work func()){ work() }", "func parallelWork(work func()){ if false { work() } }", 1), assembly, false},
		{"unused_closure_callback", strings.Replace(base, "func parallelWork(work func()){ work() }", "func parallelWork(work func()){ _ = func(){ work() } }", 1), assembly, false},
		{"earlier_generation_rejection", strings.Replace(base, "brand := brandString(); return", "brand := brandString(); if strings.Contains(brand,\"Apple M1\"){ return false }; return", 1), assembly, false},
		{"constant_contains_source", strings.Replace(strings.ReplaceAll(base, "strings.Contains(brand,", "strings.Contains(\"Apple M2\","), "brand := brandString();", "brand := brandString(); _ = brand;", 1), assembly, false},
		{"source_overwritten", strings.Replace(base, "brand := brandString(); return", "brand := brandString(); brand=\"Apple M2\"; return", 1), assembly, false},
		{"assigned_predicate", strings.Replace(base, "return strings.Contains(brand,\"Apple M1\") || strings.Contains(brand,\"Apple M2\") || strings.Contains(brand,\"Apple M3\")", "enabled := strings.Contains(brand,\"Apple M1\") || strings.Contains(brand,\"Apple M2\") || strings.Contains(brand,\"Apple M3\"); return enabled", 1), assembly, false},
		{"narrow_guard", strings.Replace(base, "strings.Contains(brand,\"Apple M1\") || ", "", 1), assembly, false},
		{"descriptor_only_in_comment", base, strings.Replace(assembly, " MOVD $0x5000000000000000, R16", " // 0x5000000000000000", 1), false},
		{"pair_only", base, strings.Replace(assembly, "0x5000000000000000", "0x4000000000000000", 1), false},
		{"descriptor_not_consumed", base, strings.Replace(assembly, "ORR R16, R0, R5", "ORR R17, R0, R5", 1), false},
		{"descriptor_source_overwritten", base, strings.Replace(assembly, "ORR R16, R0, R5", "MOVD $0x4000000000000000, R16\n ORR R16, R0, R5", 1), false},
		{"descriptor_after_return", base, strings.Replace(assembly, " MOVD $0x5000000000000000, R16", " RET\n MOVD $0x5000000000000000, R16", 1), false},
		{"missing_native_source", base, "#include \"textflag.h\"\nTEXT ·other(SB), NOSPLIT, $0-0\n RET\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"casepkg/case.go": tc.source, "casepkg/kernel.s": tc.asm})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			contract := ps6128TestContract()
			for _, field := range []*string{&contract.CapabilityPredicate, &contract.DispatchVariable, &contract.InstalledFunction, &contract.WorkerRunner, &contract.NativeSymbol} {
				*field = strings.Replace(*field, "ps6128.", "casepkg.", 1)
			}
			if tc.allRequired {
				contract.RequiredGenerations = []string{"Apple M1", "Apple M2", "Apple M3"}
			}
			analyzer := &analysis.Analyzer{Name: "PS6128", Doc: "negative", Run: func(pass *analysis.Pass) (any, error) {
				return runPS6128WithContracts(pass, []config.NativeGenerationDispatchContract{contract})
			}}
			analysistest.Run(t, dir, analyzer, "casepkg")
		})
	}
}

func TestPS6128MetadataAndContract(t *testing.T) {
	t.Parallel()
	if PS6128.AutoFix || !PS6128.NeedsConfig || PS6128.Level != 3 || len(PS6128.Vocab) != 1 || PS6128.Vocab[0] != "nativeGenerationDispatchContracts" {
		t.Fatalf("metadata drift: %+v", PS6128)
	}
	c := ps6128TestContract()
	if !c.Valid() {
		t.Fatal("complete contract invalid")
	}
	c.DescriptorMeaningReviewed = false
	if c.Valid() {
		t.Fatal("unreviewed descriptor contract valid")
	}
	original := config.Config{NativeGenerationDispatchContracts: []config.NativeGenerationDispatchContract{ps6128TestContract()}}
	compiled := original.Compile()
	original.NativeGenerationDispatchContracts[0].RequiredGenerations[0] = "changed"
	if compiled.NativeGenerationDispatchContracts[0].RequiredGenerations[0] != "Apple M2" {
		t.Fatal("compiled contract aliases input")
	}
}

func TestPS6128PinnedOwnerReplay(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, digest string }{
		{"ps6128_owner_driver.go.txt", "062cf2f894b211c6accf092549b56e4d18ecb964571739aeca4e2dea8aa00c1d"},
		{"ps6128_owner_kernel.s.txt", "be4dac66ab978a75687c01feec325b36a02640a37d5108242764a252bdd48aa0"},
		{"ps6128_corsix_ldst.md.txt", "f5507faa909d145200bb3bbf78548e09e53f83c3d8cb84c19f7ed0b61fbdb018"},
		{"ps6128_owner_worker.go.txt", "9187c83c62d510017cef9e8d2feb07d26bea58979b0e96ac9ec6c4252ad91888"},
		{"ps6128_owner_caller.go.txt", "6a8987717cf3342057f429f66566a8343616517444e03a01373cb642b3103eb1"},
		{"ps6128_corsix_LICENSE.txt", "6b4c0566bde3293566462060100f1eca4760c8e192351a099c08c0389f016aa0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/" + tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != tc.digest {
				t.Fatalf("%s digest=%s", tc.name, got)
			}
		})
	}
}

func TestPS6128PinnedOwnerBeforeAndRepairShapedReplay(t *testing.T) {
	t.Parallel()
	driverBytes, err := os.ReadFile("testdata/ps6128_owner_driver.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	assemblyBytes, err := os.ReadFile("testdata/ps6128_owner_kernel.s.txt")
	if err != nil {
		t.Fatal(err)
	}
	workerBytes, err := os.ReadFile("testdata/ps6128_owner_worker.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	callerBytes, err := os.ReadFile("testdata/ps6128_owner_caller.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	worker := ps6128PinnedFunction(t, workerBytes, "parallelWork")
	caller := ps6128PinnedFunction(t, callerBytes, "gemmF32")
	driver := strings.Replace(string(driverBytes), "//go:build darwin && goexperiment.simd\n\n", "", 1)
	driver = strings.Replace(driver, "\t\"syscall\"\n", "\t\"ownersysctl\"\n", 1)
	driver = strings.Replace(driver, "\t\"sync/atomic\"\n", "\t\"sync\"\n\t\"sync/atomic\"\n\t\"time\"\n", 1)
	driver = strings.ReplaceAll(driver, "syscall.Sysctl", "ownersysctl.Sysctl")
	driver += "\n" + worker + "\n" + caller + `
var gemmF32AMX, gemmF32Accel func(A, B, C []float32, m, k, n int) bool
var gemmF32GemvAccel func(w,x,y []float32,k,n int)
func getF32Raw(n int) *[]float32 { value:=make([]float32,n); return &value }
func putF32(*[]float32) {}
func gemmF32BandNeonCols(A, B, bias, C []float32, lo, hi, k, n, colLo, colHi int) {}
func gemmF32RowsScalar(A,B,C []float32, lo,hi,k,n,jlo,jhi int) {}
func getF32Pack(n int) *[]float32 { value:=make([]float32,n); return &value }
func putF32Pack(*[]float32) {}
func packBPanels(B,packed []float32, lo,hi,k,n int) {}
func gemmF32BandNeon(A,B,packed,C []float32, lo,hi,k,n int) {}
const parThreshold=1; const poolDenseMaxWork=1; const poolDenseGapNs=1; const parWorkPerWorker=1; const poolSpinPolls=1; const poolStealEvery=1
var poolDense atomic.Bool; var poolLastBarrierEnd atomic.Int64
type poolBarrier struct { pending atomic.Int64; wg sync.WaitGroup }
type poolTask struct { body func(int,int); lo,hi int; b *poolBarrier }
var poolChs []chan poolTask; var poolPark []chan struct{}
`
	assembly := strings.Replace(string(assemblyBytes), "//go:build darwin && goexperiment.simd\n\n", "", 1)
	for _, tc := range []struct {
		name, source string
		finding      bool
	}{
		{"before", strings.Replace(driver, "func amxSupported() bool {", "func amxSupported() bool { // want \"native dispatch accepts Apple M1\"", 1), true},
		{"repair_shaped_guard", strings.Replace(driver, "\treturn strings.Contains(brand, \"Apple M1\") ||\n\t\tstrings.Contains(brand, \"Apple M2\") ||", "\treturn strings.Contains(brand, \"Apple M2\") ||", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir, cleanup, err := analysistest.WriteFiles(map[string]string{"owner987/owner_driver.go": tc.source, "owner987/gemm_amx_arm64.s": assembly, "ownersysctl/sysctl.go": "package ownersysctl\nfunc Sysctl(string)(string,error){return \"\",nil}\n"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			contract := config.NativeGenerationDispatchContract{Name: "pinned AMX", CapabilityPredicate: "owner987.amxSupported", DispatchVariable: "owner987.gemmF32AMX", InstalledFunction: "owner987.gemmF32AMXCompute", WorkerRunner: "owner987.parallelWork", WorkerArgument: 2, MainLoopArgument: 4, NativeMainLoopArgument: 3, NativeMainLoopParameter: "k", NativeMainLoopOffset: 24, NativeSymbol: "owner987.gemmF32TileAMX32x32", AssemblyFile: "gemm_amx_arm64.s", DescriptorLiteral: "0x5000000000000000", RequiredGenerations: []string{"Apple M2", "Apple M3"}, WorkerExecutesSynchronously: true, DescriptorMeaningReviewed: true}
			analyzer := &analysis.Analyzer{Name: "PS6128", Doc: "owner replay", Run: func(pass *analysis.Pass) (any, error) {
				return runPS6128WithContracts(pass, []config.NativeGenerationDispatchContract{contract})
			}}
			analysistest.Run(t, dir, analyzer, "owner987")
		})
	}
}

func ps6128PinnedFunction(t *testing.T, source []byte, name string) string {
	t.Helper()
	files := token.NewFileSet()
	parsed, err := parser.ParseFile(files, name+".go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Name.Name == name {
			start, end := files.Position(fn.Pos()).Offset, files.Position(fn.End()).Offset
			return string(source[start:end])
		}
	}
	t.Fatalf("pinned function %s absent", name)
	return ""
}

func ps6128TestContract() config.NativeGenerationDispatchContract {
	return config.NativeGenerationDispatchContract{
		Name: "AMX quad load", CapabilityPredicate: "ps6128.amxSupported", DispatchVariable: "ps6128.gemm",
		InstalledFunction: "ps6128.compute", WorkerRunner: "ps6128.parallelWork", WorkerArgument: 0, MainLoopArgument: 0, NativeMainLoopArgument: 0, NativeMainLoopParameter: "k", NativeMainLoopOffset: 0,
		NativeSymbol: "ps6128.tile", AssemblyFile: "kernel.s", DescriptorLiteral: "0x5000000000000000",
		RequiredGenerations: []string{"Apple M2", "Apple M3"}, WorkerExecutesSynchronously: true, DescriptorMeaningReviewed: true,
	}
}
