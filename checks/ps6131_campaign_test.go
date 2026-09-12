package checks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/crossover"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// This is a tooling fixture, NOT GoAI performance evidence. The owner's exact
// allocated operation and benchmark bodies run against portable stand-ins for
// Tensor/context/registry/native code. No measured native kernel gain is used.
func TestPS6131OwnerShapedEndToEndCampaign(t *testing.T) {
	t.Parallel()
	ps6131OwnerCampaign(t, "")
}

func TestPS6131RetainedSDKReplaysOldEvidenceAndDetectsCurrentUpgrade(t *testing.T) {
	t.Parallel()
	sdk := os.Getenv("PERFSCAN_TEST_OLD_SDK")
	if sdk == "" {
		t.Skip("set PERFSCAN_TEST_OLD_SDK to a retained different SDK/bin/go for the two-SDK integration regression")
	}
	ps6131OwnerCampaign(t, sdk)
}

func ps6131OwnerCampaign(t *testing.T, sdk string) {
	t.Helper()
	root := t.TempDir()
	helper, err := os.ReadFile("../benchmarkevidence/allocation.go")
	if err != nil {
		t.Fatal(err)
	}
	operation := ps6131PinnedFunction(t, "ps6131_owner_operation.go.txt", "absKernelCPU")
	benchmark := ps6131PinnedFunction(t, "ps6131_owner_benchmarks.go.txt", "BenchmarkAbsF32CPU")
	leaf := ps6131PinnedFunction(t, "ps6131_owner_leaf.go.txt", "absF32")
	files := map[string]string{
		"go.mod":                          "module github.com/jxsl13/perfscan\ngo 1.26\n",
		"benchmarkevidence/allocation.go": string(helper),
		"tensor/tensor.go": `package tensor
type Shape []int
type Dtype int
const(F32 Dtype=iota;F64)
type Tensor struct{kind Dtype;shape Shape;storage Storage}
type Storage struct{f32 []float32;f64 []float64}
func(s *Storage)F32()[]float32{return s.f32}
func(s *Storage)F64()[]float64{return s.f64}
func(t *Tensor)Contiguous()*Tensor{return t}
func(t *Tensor)Dtype()Dtype{return t.kind}
func(t *Tensor)Shape()Shape{return t.shape}
func(t *Tensor)Storage()*Storage{return &t.storage}
func NewOn(_ int,d Dtype,s Shape)*Tensor{n:=1;for _,v:=range s{n*=v};t:=&Tensor{kind:d,shape:append(Shape(nil),s...)};if d==F32{t.storage.f32=make([]float32,n)}else{t.storage.f64=make([]float64,n)};return t}
`,
		"backend/backend.go": `package backend
import "github.com/jxsl13/perfscan/tensor"
type Context struct{}
type Attrs map[string]any
type Op int
const OpAbs Op=1
const CPU=0
type Kernel func(*Context,[]*tensor.Tensor,Attrs)([]*tensor.Tensor,error)
var selected Kernel
func Register(_ Op,k Kernel){selected=k}
func FromName(_ string)(int,bool){return 0,true}
func Get(_ int)(int,bool){return 0,true}
func NewContext()*Context{return &Context{}}
func(c *Context)WithBackend(_ int)*Context{return c}
func(*Context)Device()int{return 0}
func Execute(c *Context,_ Op,in []*tensor.Tensor,a Attrs)([]*tensor.Tensor,error){return selected(c,in,a)}
`,
		"bench/bench.go": `package bench
import "github.com/jxsl13/perfscan/tensor"
func RandF32(shape tensor.Shape,_ int)*tensor.Tensor{t:=tensor.NewOn(0,tensor.F32,shape);for i:=range t.Storage().F32(){t.Storage().F32()[i]=float32(i&1023)-512};return t}
`,
		"cpu/cpu.go": `package cpu
import("fmt";"math";. "unsafe";"github.com/jxsl13/perfscan/backend";"github.com/jxsl13/perfscan/tensor")
const absF32ParallelThreshold=1<<21
func parallel(n int,body func(int,int)){body(0,n)}
// Portable stand-in only; not an ARM64 NEON measurement.
func absF32BlocksNeon(dst,src *float32,blocks int){d,s:=Slice(dst,blocks*16),Slice(src,blocks*16);for i:=range d{d[i]=float32(math.Abs(float64(s[i])))}}
func init(){backend.Register(backend.OpAbs,absKernelCPU)}
` + leaf + "\n" + operation,
		"cpu/cpu_test.go": `package cpu
import("testing";"strconv";"github.com/jxsl13/perfscan/backend";"github.com/jxsl13/perfscan/tensor";"github.com/jxsl13/perfscan/bench")
func inputs(n int)(*backend.Context,[]*tensor.Tensor,backend.Attrs){return backend.NewContext(),[]*tensor.Tensor{bench.RandF32(tensor.Shape{n},37)},nil}
` + benchmark,
	}
	for name, source := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	env := make([]string, 0, len(os.Environ()))
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "GIT_") {
			env = append(env, value)
		}
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fixture git: %v %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	git("config", "core.autocrlf", "false")
	git("add", ".")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "commit", "-m", "pin owner-shaped fixture")
	output := filepath.Join(t.TempDir(), "campaign")
	c := config.DispatchCrossoverContract{DispatchFunction: "github.com/jxsl13/perfscan/cpu.absKernelCPU", ThresholdConstant: "github.com/jxsl13/perfscan/cpu.absF32ParallelThreshold", SerialPolicy: "github.com/jxsl13/perfscan/cpu.absF32", ParallelPolicy: "github.com/jxsl13/perfscan/cpu.parallel", WorkerRunner: "github.com/jxsl13/perfscan/cpu.parallel", LeafKernels: []string{"github.com/jxsl13/perfscan/cpu.absF32BlocksNeon"}, SerialOperation: "github.com/jxsl13/perfscan/cpu.serialCopy", ParallelOperation: "github.com/jxsl13/perfscan/cpu.parallelCopy", OperationInputFactory: "github.com/jxsl13/perfscan/cpu.inputs", DiagnosticFunction: "github.com/jxsl13/perfscan/cpu.TestDispatchDiagnostic", ProductionBenchmark: "BenchmarkAbsF32CPU/n{n}", Campaign: output}
	options := crossover.Options{Repository: root, Commit: git("rev-parse", "HEAD"), Output: output, Package: "./cpu", N: 2, Pairs: 2, Sizes: []int{2048, 2097152}, Procs: []int{2}}
	options.GoBinary = sdk
	c.EvidenceGoBinary = sdk
	report, err := crossover.RecordCampaign(context.Background(), &options, &c, LoadDispatchCrossoverHarness, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Pairs) != 24 || len(report.Original) != 16 {
		t.Fatalf("owner evidence incomplete: native=%d original=%d", len(report.Pairs), len(report.Original))
	}
	pin := func(name string) string {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		return hex.EncodeToString(sum[:])
	}
	if _, err := crossover.VerifyCampaign(context.Background(), root, output, sdk, pin("plan.json"), pin("records.json"), LoadDispatchCrossoverHarness); err != nil {
		t.Fatal(err)
	}
	plan, err := crossover.ReadPlan(output, pin("plan.json"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := crossover.CurrentBuild(context.Background(), root, "./cpu", "", &c, LoadDispatchCrossoverHarness)
	if err != nil {
		t.Fatal(err)
	}
	if sdk != "" && current.GoVersion == plan.Build.GoVersion {
		t.Fatal("two-SDK regression requires different observed compiler versions")
	}
	if crossover.Fresh(current, plan) != (sdk == "") {
		t.Fatal("identical source freshness did not distinguish retained-old vs current observed SDK")
	}
	c.PlanSHA256 = pin("plan.json")
	c.RecordsSHA256 = pin("records.json")
	scan := func(wantStale bool) {
		loaded, err := packages.Load(&packages.Config{Dir: root, Env: append(os.Environ(), "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS="), Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedTypesSizes | packages.NeedImports | packages.NeedDeps}, "./cpu")
		if err != nil || len(loaded) != 1 || len(loaded[0].Errors) != 0 {
			t.Fatalf("analyzer loader: %v", err)
		}
		pkg := loaded[0]
		var diagnostics []analysis.Diagnostic
		pass := &analysis.Pass{Fset: pkg.Fset, Files: pkg.Syntax, Pkg: pkg.Types, TypesInfo: pkg.TypesInfo, TypesSizes: pkg.TypesSizes, Report: func(d analysis.Diagnostic) { diagnostics = append(diagnostics, d) }}
		if _, err := runPS6131Contracts(pass, []config.DispatchCrossoverContract{c}); err != nil {
			t.Fatal(err)
		}
		if (len(diagnostics) == 1) != wantStale {
			t.Fatalf("stale diagnostic=%v want=%v", diagnostics, wantStale)
		}
	}
	scan(sdk != "")
	// These source mutations intentionally share one pinned artifact and are
	// sequential; the surrounding independent campaign test remains parallel.
	for _, tc := range []struct{ name, path, old, new string }{
		{"native_leaf", "cpu/cpu.go", "d[i]=float32(math.Abs(float64(s[i])))", "d[i]=float32(math.Abs(float64(s[i])))*1"},
		{"worker_pool", "cpu/cpu.go", "func parallel(n int,body func(int,int)){body(0,n)}", "func parallel(n int,body func(int,int)){if n>0{body(0,n)}}"},
		{"factory_helper", "cpu/cpu_test.go", "tensor.Shape{n},37", "tensor.Shape{n},38"},
		{"parenthesized_predicate", "cpu/cpu.go", "if len(o) < absF32ParallelThreshold {", "if (len(o) < absF32ParallelThreshold) {"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := files[tc.path]
			updated := strings.Replace(original, tc.old, tc.new, 1)
			if original == updated {
				t.Fatal("mutation does not match owner-shaped fixture")
			}
			path := filepath.Join(root, tc.path)
			if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}()
			changed, err := crossover.CurrentBuild(context.Background(), root, "./cpu", "", &c, LoadDispatchCrossoverHarness)
			if err != nil {
				t.Fatal(err)
			}
			if crossover.Fresh(changed, plan) {
				t.Fatal("changed measured dependency incorrectly qualified stale evidence")
			}
			scan(true)
			if _, err := crossover.VerifyCampaign(context.Background(), root, output, sdk, pin("plan.json"), pin("records.json"), LoadDispatchCrossoverHarness); err != nil {
				t.Fatal("old pinned snapshot should remain verifiable despite mutable checkout change:", err)
			}
		})
	}
	t.Run("parameter_method_spelling_collision", func(t *testing.T) {
		original := files["cpu/cpu.go"]
		updated := strings.Replace(original, "in []*tensor.Tensor", "Dtype []*tensor.Tensor", 1)
		updated = strings.ReplaceAll(updated, "in[", "Dtype[")
		if original == updated {
			t.Fatal("parameter collision did not change")
		}
		path := filepath.Join(root, "cpu/cpu.go")
		if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
		}()
		if _, err := crossover.CurrentBuild(context.Background(), root, "./cpu", "", &c, LoadDispatchCrossoverHarness); err != nil {
			t.Fatal("typed parameter references must not rewrite same-spelled Dtype method:", err)
		}
	})
	for _, tc := range []struct{ name, old, new string }{
		{"rebound_output", "out := tensor.NewOn(ctx.Device(), in[0].Dtype(), in[0].Shape())", "out := tensor.NewOn(ctx.Device(), in[0].Dtype(), in[0].Shape())\nout=tensor.NewOn(ctx.Device(),in[0].Dtype(),in[0].Shape())"},
		{"rebound_storage", "d, o := xc.Storage().F32(), out.Storage().F32()", "d, o := xc.Storage().F32(), out.Storage().F32()\no=make([]float32,len(o))"},
		{"escaped_output_alias", "out := tensor.NewOn(ctx.Device(), in[0].Dtype(), in[0].Shape())", "out := tensor.NewOn(ctx.Device(), in[0].Dtype(), in[0].Shape())\nalias:=out;_=alias"},
		{"escaped_contiguous_alias", "xc := in[0].Contiguous()", "xc := in[0].Contiguous()\nalias:=xc;_=alias"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := files["cpu/cpu.go"]
			updated := strings.Replace(original, tc.old, tc.new, 1)
			if updated == original {
				t.Fatal("storage adversary did not change")
			}
			path := filepath.Join(root, "cpu/cpu.go")
			if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}()
			if _, err := crossover.CurrentBuild(context.Background(), root, "./cpu", "", &c, LoadDispatchCrossoverHarness); err == nil {
				t.Fatal("associated rebound/escaped output or input storage")
			}
		})
	}
	for _, tc := range []struct{ name, path, old, new string }{
		{"wrong_original_operation", "cpu/cpu_test.go", "backend.OpAbs, in, nil", "backend.Op(42), in, nil"},
		{"wrong_original_cpu_context", "cpu/cpu_test.go", "backend.Get(backend.CPU)", "backend.Get(42)"},
		{"wrong_original_shape", "cpu/cpu_test.go", "tensor.Shape{n}, 37", "tensor.Shape{2048}, 37"},
		{"wrong_constructor_dtype", "bench/bench.go", "tensor.NewOn(0,tensor.F32,shape)", "tensor.NewOn(0,tensor.F64,shape)"},
		{"dead_timed_operation", "cpu/cpu_test.go", "for range b.N {", "for range 0 {"},
		{"untimed_operation", "cpu/cpu_test.go", "for range b.N {", "for range 20 {"},
		{"rebound_original_context", "cpu/cpu_test.go", "ctx := backend.NewContext().WithBackend(be)", "ctx := backend.NewContext().WithBackend(be)\nctx=backend.NewContext()"},
		{"rebound_original_input", "cpu/cpu_test.go", "in := []*tensor.Tensor{x}", "in := []*tensor.Tensor{x}\nin[0]=bench.RandF32(tensor.Shape{2048},37)"},
		{"changed_label_extent", "cpu/cpu_test.go", "x := bench.RandF32(tensor.Shape{n}, 37)", "n*=2\nx := bench.RandF32(tensor.Shape{n}, 37)"},
		{"rebound_constructor_output", "bench/bench.go", "for i:=range", "t=tensor.NewOn(0,tensor.F64,shape);for i:=range"},
		{"mutated_constructor_shape", "bench/bench.go", "t:=tensor.NewOn", "shape[0]=2048;t:=tensor.NewOn"},
		{"escaped_constructor_output", "bench/bench.go", "return t", "alias:=t;_=alias;return t"},
		{"fake_registration_sink", "backend/backend.go", "selected=k", "_=k"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := files[tc.path]
			updated := strings.ReplaceAll(original, tc.old, tc.new)
			if updated == original {
				t.Fatal("original benchmark adversary did not change fixture")
			}
			path := filepath.Join(root, tc.path)
			if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}()
			if _, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{Root: root, Pattern: "./cpu"}, &c); err == nil {
				t.Fatal("qualified a misleading original benchmark source route")
			}
		})
	}
	for _, tc := range []struct{ name, path, old, new string }{
		{"typed_test_changed_before_compile", "cpu/cpu_test.go", "for range b.N {", "for range 20 {"},
		{"typed_dependency_changed_before_compile", "bench/bench.go", "float32(i&1023)-512", "float32(i&1023)-511"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := files[tc.path]
			updated := strings.ReplaceAll(original, tc.old, tc.new)
			if updated == original {
				t.Fatal("model/compiler race fixture did not change")
			}
			path := filepath.Join(root, tc.path)
			defer func() {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}()
			observations := 0
			observeThenChange := func(selection *crossover.BuildSelection, contract *config.DispatchCrossoverContract) (*crossover.HarnessModel, error) {
				model, err := LoadDispatchCrossoverHarness(selection, contract)
				if err != nil {
					return nil, err
				}
				observations++
				// prepare intentionally reloads after the initial planning model.
				// Race the final typed observation immediately before compilation.
				if observations == 2 {
					if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
						return nil, err
					}
				}
				return model, nil
			}
			if _, err := crossover.CurrentBuild(context.Background(), root, "./cpu", "", &c, observeThenChange); err == nil || !strings.Contains(err.Error(), "typed model") {
				t.Fatalf("joined typed A to compiled test/dependency B: %v", err)
			}
		})
	}
	ownerRegistry := `type kernelKey struct{op backend.Op;dtype tensor.Dtype}
type Backend struct{table map[kernelKey]backend.Kernel}
var std=&Backend{table:make(map[kernelKey]backend.Kernel)}
func(b *Backend)add(op backend.Op,dtype tensor.Dtype,k backend.Kernel){b.table[kernelKey{op,dtype}]=k}
func init(){reg:=func(op backend.Op,k backend.Kernel){std.add(op,tensor.F32,k);std.add(op,tensor.F64,k)};reg(backend.OpAbs,absKernelCPU)}`
	for _, tc := range []struct{ name, old, new string }{
		{"actual_owner_registration_helper", "", ""},
		{"helper_wrong_op_key", "kernelKey{op,dtype}", "kernelKey{backend.OpAbs,dtype}"},
		{"helper_wrong_dtype_arm", "std.add(op,tensor.F32,k)", "std.add(op,tensor.F64,k)"},
		{"helper_uninvoked_registration", "reg(backend.OpAbs,absKernelCPU)", "_=reg;_=absKernelCPU"},
		{"helper_rebound_registration", "reg(backend.OpAbs,absKernelCPU)", "reg=func(_ backend.Op,_ backend.Kernel){};reg(backend.OpAbs,absKernelCPU)"},
		{"helper_address_escape", "reg(backend.OpAbs,absKernelCPU)", "_= &reg;reg(backend.OpAbs,absKernelCPU)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := files["cpu/cpu.go"]
			updated := strings.Replace(original, "func init(){backend.Register(backend.OpAbs,absKernelCPU)}", ownerRegistry, 1)
			if tc.old != "" {
				updated = strings.Replace(updated, tc.old, tc.new, 1)
			}
			if updated == original {
				t.Fatal("owner registration helper fixture did not change")
			}
			path := filepath.Join(root, "cpu/cpu.go")
			if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.WriteFile(path, []byte(original), 0600); err != nil {
					t.Fatal(err)
				}
			}()
			_, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{Root: root, Pattern: "./cpu"}, &c)
			if (err == nil) != (tc.old == "") {
				t.Fatalf("owner registry source association: %v", err)
			}
		})
	}
	constructorSource := `
func CPU()int{return 0}
func New(dtype Dtype,shape Shape)*Tensor{return NewOn(CPU(),dtype,shape)}
func(s Shape)Numel()int{n:=1;for _,d:=range s{n*=d};return n}
func FromFloat32(shape Shape,data []float32)*Tensor{t:=New(F32,shape);copy(t.storage.F32(),data);return t}
`
	for _, tc := range []struct{ name, old, new string }{
		{"actual_owner_constructor_chain", "", ""},
		{"escaped_storage_alias", "copy(t.storage.F32(),data)", "alias:=t.storage;_=alias;copy(t.storage.F32(),data)"},
		{"range_storage_field_write", "copy(t.storage.F32(),data)", "for _,t.storage=range []Storage{}{};copy(t.storage.F32(),data)"},
		{"mutating_shape_numel", "n:=1;for _,d:=range s", "s[0]=2048;n:=1;for _,d:=range s"},
		{"new_wrapper_changes_shape", "NewOn(CPU(),dtype,shape)", "NewOn(CPU(),dtype,Shape{2048})"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tensorSource := files["tensor/tensor.go"] + constructorSource
			if tc.old != "" {
				tensorSource = strings.Replace(tensorSource, tc.old, tc.new, 1)
			}
			benchSource := `package bench
import "github.com/jxsl13/perfscan/tensor"
func RandF32(shape tensor.Shape,_ int)*tensor.Tensor{return tensor.FromFloat32(shape,make([]float32,shape.Numel()))}`
			for path, source := range map[string]string{"tensor/tensor.go": tensorSource, "bench/bench.go": benchSource} {
				if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
			}
			defer func() {
				for _, path := range []string{"tensor/tensor.go", "bench/bench.go"} {
					if err := os.WriteFile(filepath.Join(root, path), []byte(files[path]), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}()
			_, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{Root: root, Pattern: "./cpu"}, &c)
			if (err == nil) != (tc.old == "") {
				t.Fatalf("constructor storage/shape source association: %v", err)
			}
		})
	}
	t.Run("extra_unreviewed_shape_mutator", func(t *testing.T) {
		tensorSource := files["tensor/tensor.go"] + `
func FromFloat32(shape Shape,_ []float32)*Tensor{shape[0]=2048;return NewOn(0,F32,shape)}`
		benchSource := strings.Replace(files["bench/bench.go"], "for i:=range", "tensor.FromFloat32(shape,nil);for i:=range", 1)
		for path, source := range map[string]string{"tensor/tensor.go": tensorSource, "bench/bench.go": benchSource} {
			if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
		}
		defer func() {
			for _, path := range []string{"tensor/tensor.go", "bench/bench.go"} {
				if err := os.WriteFile(filepath.Join(root, path), []byte(files[path]), 0600); err != nil {
					t.Fatal(err)
				}
			}
		}()
		if _, err := LoadDispatchCrossoverHarness(&crossover.BuildSelection{Root: root, Pattern: "./cpu"}, &c); err == nil {
			t.Fatal("accepted an extra uninspected same-package shape-mutator call")
		}
	})
	stdoutPath := filepath.Join(output, "sample-0.stdout")
	originalStdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stdoutPath, append(originalStdout, []byte("forged extra observation\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := crossover.VerifyCampaign(context.Background(), root, output, sdk, pin("plan.json"), pin("records.json"), LoadDispatchCrossoverHarness); err == nil {
		t.Fatal("accepted changed stdout under completed external record pin")
	}
	if err := os.WriteFile(stdoutPath, originalStdout, 0600); err != nil {
		t.Fatal(err)
	}
}
