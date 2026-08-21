package checks

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6015 implements owner issue #773 by linking accelerator wrappers and route
// evidence to implementation-capability changes across files in a package.
var PS6015 = register(&lint.Check{
	ID:       "PS6015",
	Category: "verify",
	Slug:     "accelerator-route-stale-after-host-kernel",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an accelerator route or its evidence is stale after an implementation-capability change",
		Text: `An accelerator route can become stale without any edit to its
wrapper. A GPU path introduced to replace a scalar/reference fallback may keep
uploading a host tensor, submitting and waiting, and downloading a small result
after the CPU backend later gains a typed, SIMD, or parallel implementation for
the same operation and dtype.

This check implements owner issue #773 as a package-wide join. It reports only
when all four source-auditable facts agree:

  - a host-tensor accelerator wrapper or nearby comment says a slow
    host/reference/scalar fallback motivated the route;
  - another file registers a CPU/host implementation with typed, SIMD,
    vectorized, parallel, AVX, NEON, or optimized-kernel evidence for the same
    canonical operation name and dtype;
  - the wrapper contains ordered host Storage access, upload, kernel
    dispatch/launch, synchronous wait, and download/copy-back calls; and
  - no current same-operation benchmark/promotion harness carries explicit
    route/crossover, hardware, shape, build-mode, both-arm implementation
    capability, and implementation-change invalidation evidence.

A second audit covers the evidence itself. A route benchmark keyed only by
hardware and shape is not timeless: a new SIMD/native kernel on either arm can
reverse a historically correct result. When a matching optimized registration
exists, the benchmark must record its build mode (including GOEXPERIMENT or
build-tag state), control and candidate implementation classes, and an
invalidation/fingerprint dependency that requires remeasurement when either
class changes.

Operation and dtype matching uses normalized identifiers, constant strings,
callee names, and function names. The diagnostic ranks reduction, scatter,
gather, memory-bound, and one-thread-per-output wrappers more strongly. Device
tensor/buffer, graph-node, recorder, and command-buffer contracts stay silent
because persistent residency may reverse the comparison. Asynchronous paths
without a wait and wrappers without the complete round trip also stay silent.

There is NO automatic routing fix. The safe response is a same-binary,
hardware/shape-keyed crossover benchmark and a measured selector bound that
keeps the incumbent outside proven host-winner zones. Large shapes, discrete
GPUs, asynchronous graphs, and persistent device state can still favor the
accelerator.`,
		Before: `// GPU route replaces the slow scalar host fallback.
func VulkanAddBiasBackwardF32(x *Tensor) *Tensor {
	in := vulkan.Upload(x.Storage())
	out := vulkan.DispatchReduction(in)
	vulkan.Wait()
	return NewTensor(vulkan.Download(out))
}
// Elsewhere: RegisterCPU(OpAddBiasBackward, F32, parallelTypedReduction)`,
		After: `// Same-binary evidence, not an unconditional CPU reroute:
BenchmarkAddBiasBackwardRouteCrossover(
	hardware, shapes, buildMode,
	controlImplementation, candidateImplementation,
	invalidateOnCapabilityChange,
)
// Route only the measured host-resident winner zone.`,
		MeasuredWin: `In the Apple-M2/MoltenVK campaign behind issue #773, a
Vulkan AddBiasBackward route still targeted the old scalar column-sum fallback
after the CPU backend gained a bit-exact parallel row-major reduction.
Re-routing only the measured host-resident zone improved six frozen shapes from
3.27x-4.89x at [512,4096] up to 309.9x-451.3x at [1,512]. A six-layer training
step remained non-regressing at 224.50 ms -> 222.65 ms median (1.008x).

A later M2 route campaign reversed an earlier 13%% Metal-training loss after
typed arm64 NEON forward/backward kernels replaced scalar closure kernels under
GOEXPERIMENT=simd: all 84 operation/shape medians exceeded 1.10x, local wins
spanned 1.743x-85.043x, and full training improved 74.553 ms -> 71.823 ms.
That reversal is why route evidence needs capability dependencies rather than
a timeless CPU-loses/GPU-wins conclusion.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6015",
		Doc:  "accelerator fallback rationale is stale after optimized host registration",
		Run:  runPS6015,
	},
})

type ps6015Roundtrip struct {
	upload   []*ast.CallExpr
	kernel   []*ast.CallExpr
	wait     []*ast.CallExpr
	download []*ast.CallExpr
}

func runPS6015(pass *analysis.Pass) (any, error) {
	ps6015AuditRouteEvidence(pass)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			object, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if !ok {
				continue
			}
			sig, ok := object.Type().(*types.Signature)
			if !ok || !ps6013HostContract(sig) || ps6013PersistentContract(sig) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6013AcceleratorText(text) || !ps6015FallbackRationale(text) {
				continue
			}
			operation := ps6015CanonicalOperation(fn.Name.Name)
			dtype := ps6015DType(text)
			if operation == "" || dtype == "" || !ps6015OptimizedRegistration(pass, operation, dtype) {
				continue
			}
			kernel, ok := ps6015SynchronousRoundtrip(pass, fn)
			if !ok || ps6015HasCurrentRouteEvidence(pass, operation, dtype) {
				continue
			}
			priority := ""
			if ps6013LowIntensityText(text) || ps6007ContainsAny(text, "memorybound", "one thread per output", "onethreadperoutput", "small output") {
				priority = " high-priority: the route is reduction/scatter/gather or movement-bound;"
			}
			pass.Reportf(kernel.Pos(), "accelerator route %s/%s still cites a slow host/reference fallback, but this package now registers an optimized host kernel for the same operation and dtype while the wrapper synchronously uploads, submits, waits, and downloads;%s add a same-binary hardware/shape crossover benchmark and preserve the incumbent outside measured winner zones; do not auto-route to CPU", operation, dtype, priority)
		}
	}
	return nil, nil
}

func ps6015AuditRouteEvidence(pass *analysis.Pass) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			route := ps6007ContainsAny(text, "route", "crossover", "selector", "winnerzone", "winner zone")
			hardware := ps6007ContainsAny(text, "hardware", "device", "metal", "vulkan", "cuda", "gpu", "mps")
			shape := ps6007ContainsAny(text, "shape", "extent", "dimensions", "rows", "columns")
			operation, dtype := ps6015CanonicalOperation(fn.Name.Name), ps6015DType(text)
			if !route || !hardware || !shape || operation == "" || dtype == "" ||
				!ps6015AnyOptimizedRegistration(pass, operation, dtype) {
				continue
			}
			if missing := ps6015MissingEvidenceDependencies(text); len(missing) > 0 {
				pass.Reportf(fn.Name.Pos(), "route evidence for %s/%s is not capability-stable; missing %s; record both arm implementation fingerprints and revalidate when either gains a SIMD/native kernel", operation, dtype, strings.Join(missing, ", "))
			}
		}
	}
}

func ps6015MissingEvidenceDependencies(text string) []string {
	normalized := ps6007NormalizeName(text)
	checks := []struct {
		name string
		ok   bool
	}{
		{"build-mode dependency", ps6007ContainsAny(normalized, "buildmode", "goexperiment", "buildtag", "toolchainmode", "experimentmode")},
		{"control implementation capability", ps6007ContainsAny(normalized, "controlimplementation", "controlkernel", "controlcapability", "baselineimplementation", "incumbentimplementation")},
		{"candidate implementation capability", ps6007ContainsAny(normalized, "candidateimplementation", "candidatekernel", "candidatecapability", "challengerimplementation", "newrouteimplementation")},
		{"implementation-change invalidation", ps6007ContainsAny(normalized, "invalidateoncapabilitychange", "invalidateonimplementationchange", "revalidateonkernelchange", "implementationfingerprint", "capabilityfingerprint", "kernelepoch", "implementationepoch")},
	}
	missing := make([]string, 0, len(checks))
	for _, check := range checks {
		if !check.ok {
			missing = append(missing, check.name)
		}
	}
	return missing
}

func ps6015SynchronousRoundtrip(pass *analysis.Pass, fn *ast.FuncDecl) (*ast.CallExpr, bool) {
	hostAccess := false
	var calls ps6015Roundtrip
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		hostAccess = hostAccess || ps6013HostAccessCall(pass, call)
		switch {
		case ps6015UploadCall(pass, call):
			calls.upload = append(calls.upload, call)
		case ps6013KernelCall(pass, call):
			calls.kernel = append(calls.kernel, call)
		case ps6013WaitCall(pass, call):
			calls.wait = append(calls.wait, call)
		case ps6013CopybackCall(pass, call):
			calls.download = append(calls.download, call)
		}
		return true
	})
	if !hostAccess {
		return nil, false
	}
	for _, upload := range calls.upload {
		for _, kernel := range calls.kernel {
			if kernel.Pos() <= upload.Pos() {
				continue
			}
			for _, wait := range calls.wait {
				if wait.Pos() <= kernel.Pos() {
					continue
				}
				for _, download := range calls.download {
					if download.Pos() > wait.Pos() {
						return kernel, true
					}
				}
			}
		}
	}
	return nil, false
}

func ps6015UploadCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	return ps6007ContainsAny(name, "upload", "copytodevice", "copyfromhost", "newbufferwithbytes", "writedevice", "setbytes")
}

func ps6015FallbackRationale(text string) bool {
	fallback := ps6007ContainsAny(text, "fallback", "reference", "scalar", "host path", "hostpath", "cpu path", "cpupath")
	reason := ps6007ContainsAny(text, "slow", "slower", "bottleneck", "replaced", "reason", "avoid", "because")
	return fallback && reason
}

func ps6015OptimizedRegistration(pass *analysis.Pass, operation, dtype string) bool {
	return ps6015FindOptimizedRegistration(pass, operation, dtype, true)
}

func ps6015AnyOptimizedRegistration(pass *analysis.Pass, operation, dtype string) bool {
	return ps6015FindOptimizedRegistration(pass, operation, dtype, false)
}

func ps6015FindOptimizedRegistration(pass *analysis.Pass, operation, dtype string, hostOnly bool) bool {
	for _, file := range pass.Files {
		found := false
		ast.Inspect(file, func(node ast.Node) bool {
			if found {
				return false
			}
			call, ok := node.(*ast.CallExpr)
			if !ok || !ps6015RegistrationCall(pass, call) {
				return true
			}
			text := ps6015NodeText(pass, call)
			host := ps6007ContainsAny(text, "cpu", "host")
			backend := host || ps6007ContainsAny(text, "gpu", "device", "metal", "vulkan", "cuda", "mps", "accelerator")
			optimized := ps6007ContainsAny(text, "optimized", "typed", "simd", "parallel", "vectorized", "avx", "neon", "rowmajor", "native", "amx", "sve")
			if backend && (!hostOnly || host) && optimized && ps6015DType(text) == dtype && ps6015NodeOperation(call, operation) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func ps6015RegistrationCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	return ps6007ContainsAny(name, "registerkernel", "registerop", "registerimplementation", "bindkernel", "setkernel", "addkernel")
}

func ps6015NodeOperation(node ast.Node, operation string) bool {
	matched := false
	ast.Inspect(node, func(current ast.Node) bool {
		if matched {
			return false
		}
		var text string
		switch value := current.(type) {
		case *ast.Ident:
			text = value.Name
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				text = value.Value
			}
		}
		candidate := ps6015CanonicalOperation(text)
		matched = candidate == operation || candidate != "" && operation != "" &&
			(strings.Contains(candidate, operation) || strings.Contains(operation, candidate))
		return !matched
	})
	return matched
}

func ps6015HasCurrentRouteEvidence(pass *analysis.Pass, operation, dtype string) bool {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if ps6015DType(text) != dtype || !strings.Contains(ps6015CanonicalOperation(fn.Name.Name), operation) {
				continue
			}
			route := ps6007ContainsAny(text, "route", "crossover", "selector", "winnerzone", "winner zone")
			hardware := ps6007ContainsAny(text, "hardware", "device", "metal", "vulkan", "cuda", "gpu", "mps")
			shape := ps6007ContainsAny(text, "shape", "extent", "dimensions", "rows", "columns")
			if route && hardware && shape {
				return true
			}
		}
	}
	return false
}

func ps6015FunctionText(pass *analysis.Pass, file *ast.File, fn *ast.FuncDecl) string {
	var text strings.Builder
	text.Grow(256)
	text.WriteByte(' ')
	text.WriteString(strings.ToLower(fn.Name.Name))
	if fn.Doc != nil {
		text.WriteByte(' ')
		text.WriteString(strings.ToLower(fn.Doc.Text()))
	}
	for _, group := range file.Comments {
		if group == fn.Doc || group.Pos() < fn.Body.Pos() || group.End() > fn.Body.End() {
			continue
		}
		text.WriteByte(' ')
		text.WriteString(strings.ToLower(group.Text()))
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			text.WriteByte(' ')
			text.WriteString(strings.ToLower(value.Name))
		case *ast.BasicLit:
			if value.Kind == token.STRING {
				text.WriteByte(' ')
				text.WriteString(strings.ToLower(value.Value))
			}
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, value.Fun); ok {
				text.WriteByte(' ')
				text.WriteString(strings.ToLower(callee.Name()))
				if callee.Pkg() != nil {
					text.WriteByte(' ')
					text.WriteString(strings.ToLower(callee.Pkg().Path()))
				}
			}
		}
		return true
	})
	return text.String()
}

func ps6015NodeText(pass *analysis.Pass, node ast.Node) string {
	var text strings.Builder
	text.Grow(128)
	ast.Inspect(node, func(current ast.Node) bool {
		switch value := current.(type) {
		case *ast.Ident:
			text.WriteByte(' ')
			text.WriteString(strings.ToLower(value.Name))
		case *ast.BasicLit:
			text.WriteByte(' ')
			text.WriteString(strings.ToLower(value.Value))
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, value.Fun); ok {
				text.WriteByte(' ')
				text.WriteString(strings.ToLower(callee.Name()))
			}
		}
		return true
	})
	return text.String()
}

func ps6015DType(text string) string {
	text = strings.ToLower(text)
	switch {
	case ps6007ContainsAny(text, "bfloat16", "bf16"):
		return "bf16"
	case ps6007ContainsAny(text, "float16", "f16", "half"):
		return "f16"
	case ps6007ContainsAny(text, "float32", "f32"):
		return "f32"
	case ps6007ContainsAny(text, "float64", "f64", "double"):
		return "f64"
	case ps6007ContainsAny(text, "int8", "i8"):
		return "i8"
	case ps6007ContainsAny(text, "int32", "i32"):
		return "i32"
	case ps6007ContainsAny(text, "int64", "i64"):
		return "i64"
	}
	return ""
}

func ps6015CanonicalOperation(text string) string {
	name := ps6007NormalizeName(text)
	replacer := strings.NewReplacer(
		"vulkan", "", "metal", "", "cuda", "", "mps", "", "gpu", "", "accelerator", "",
		"backend", "", "wrapper", "", "route", "", "kernel", "", "registration", "", "register", "",
		"implementation", "", "optimized", "", "vectorized", "", "parallel", "", "rowmajor", "",
		"typed", "", "simd", "", "scalar", "", "reference", "", "fallback", "", "host", "", "cpu", "",
		"dispatch", "", "execute", "", "launch", "", "submit", "", "upload", "", "download", "",
		"bfloat16", "", "bf16", "", "float16", "", "f16", "", "float32", "", "f32", "",
		"float64", "", "f64", "", "int8", "", "i8", "", "int32", "", "i32", "", "int64", "", "i64", "",
	)
	name = replacer.Replace(name)
	name = strings.TrimPrefix(name, "op")
	name = strings.TrimSuffix(name, "op")
	if len(name) < 4 {
		return ""
	}
	return name
}
