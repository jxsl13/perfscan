package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6013 implements owner issue #771: synchronous accelerator wrappers that
// round-trip an otherwise host-resident tensor through device memory.
var PS6013 = register(&lint.Check{
	ID:       "PS6013",
	Category: "verify",
	Slug:     "host-tensor-accelerator-roundtrip",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a synchronous accelerator wrapper round-trips a host-resident tensor",
		Text: `A host-resident tensor wrapper can lose to a typed host loop when it
uploads inputs, creates or initializes the full output, launches a small
movement-bound kernel, waits synchronously, and downloads the whole result
before returning. The kernel may be fast in isolation while PCIe/unified-memory
transitions, command submission, atomics, and the full readback dominate the
public operation.

This check implements owner issue #771 by requiring an ordered conjunction in
one function:

  - its signature consumes and returns host tensor/slice values and contains no
    persistent device-handle contract;
  - a Storage/HostData-style accessor exposes host tensor data;
  - a local slice or output object is allocated from a runtime-sized extent;
  - a Metal/CUDA/GPU/driver kernel Execute/Dispatch/Launch call follows;
  - a device/driver Wait/Synchronize call follows the kernel; and
  - Download/Readback/CopyToHost writes the full result into the same allocated
    output object before return.

Object identity ties the readback to the runtime-sized allocation; source order
ties kernel, wait, and download into a synchronous round trip. Generic
WaitGroup.Wait calls do not count. Functions accepting or returning
DeviceTensor, DeviceBuffer, GPUBuffer, MTLBuffer, graph-node, recorder, or
command-buffer handles are excluded because their state may remain resident.
Paths that enqueue asynchronously, omit the wait, or avoid the full output
copy also stay silent.

Gather, scatter, embedding, reduction, and movement/copy kernels receive a
higher-priority explanation because their low arithmetic intensity rarely
amortizes a full synchronous round trip. The finding still does NOT claim that
CPU execution is faster. It requests an alternating host-vs-device benchmark
over frozen shapes, output parity, allocation/transfer counts, and an explicit
device-residency roadmap.

There is NO automatic fix. A host implementation can change accumulation order,
atomic nondeterminism, dtype behavior, parallelism, and backend selection.
Those choices require project-specific parity tests and measurements.`,
		Before: `out := make([]float32, rows*dim)
indicesBuf := metal.Upload(indices.Storage())
gradBuf := metal.Upload(grad.Storage())
outBuf := metal.Upload(out)
metal.DispatchScatter(indicesBuf, gradBuf, outBuf)
metal.Wait()
metal.Download(outBuf, out)
return NewTensor(out)`,
		After: `// Until tensors remain device-resident across operations:
out := typedHostScatter(indices.Storage(), grad.Storage(), rows, dim)
return NewTensor(out)
// Compare against the round trip on frozen shapes before promotion.`,
		MeasuredWin: `In the Apple-M2 OpEmbedBackward campaign behind issue
#771, replacing upload + zeroed-output upload + atomic scatter + wait + full
download with a deterministic typed host scatter improved every frozen shape
across three independent seven-sample campaigns: 3.93x-6.01x on four broad
shapes and 26.97x-30.76x on the small-table shape. The host path also removed
atomic nondeterminism and exactly preserved reference-order F32 accumulation.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6013",
		Doc:  "synchronous accelerator wrapper round-trips host tensor storage",
		Run:  runPS6013,
	},
})

type ps6013Function struct {
	fn          *ast.FuncDecl
	sig         *types.Signature
	accelerator bool
	hostAccess  []token.Pos
	allocations map[types.Object]token.Pos
	kernels     []*ast.CallExpr
	waits       []*ast.CallExpr
	copybacks   []*ast.CallExpr
}

func runPS6013(pass *analysis.Pass) (any, error) {
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
			scan := ps6013Scan(pass, fn, sig)
			if !scan.accelerator || len(scan.hostAccess) == 0 || len(scan.allocations) == 0 || len(scan.kernels) == 0 || len(scan.waits) == 0 || len(scan.copybacks) == 0 {
				continue
			}
			kernel, allocation, ok := ps6013OrderedRoundtrip(pass, scan)
			if !ok {
				continue
			}
			priority := ""
			if ps6013LowIntensity(fn) || ps6013NodeLowIntensity(kernel) {
				priority = " movement/gather/scatter/reduction kernels have low arithmetic intensity, so this is a high-priority measurement target;"
			}
			pass.Reportf(kernel.Pos(), "host-resident tensor data and runtime-sized output %s cross an accelerator kernel, synchronous wait, and full copy-back in one wrapper;%s benchmark a deterministic typed host path against the complete round trip on frozen shapes with parity and transfer/allocation counts; this is not an automatic CPU-is-faster claim", allocation.Name(), priority)
		}
	}
	return nil, nil
}

func ps6013HostContract(sig *types.Signature) bool {
	hostInput := sig.Recv() != nil && ps6013HostType(sig.Recv().Type())
	for i := 0; i < sig.Params().Len(); i++ {
		if ps6013HostType(sig.Params().At(i).Type()) {
			hostInput = true
			break
		}
	}
	if !hostInput {
		return false
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if ps6013HostType(sig.Results().At(i).Type()) {
			return true
		}
	}
	return false
}

func ps6013HostType(t types.Type) bool {
	text := strings.ToLower(types.TypeString(t, func(*types.Package) string { return "" }))
	if ps6013DeviceTypeText(text) {
		return false
	}
	return strings.Contains(text, "tensor") || strings.HasPrefix(text, "[]")
}

func ps6013PersistentContract(sig *types.Signature) bool {
	if sig.Recv() != nil && ps6013DeviceType(sig.Recv().Type()) {
		return true
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if ps6013DeviceType(sig.Params().At(i).Type()) {
			return true
		}
	}
	for i := 0; i < sig.Results().Len(); i++ {
		if ps6013DeviceType(sig.Results().At(i).Type()) {
			return true
		}
	}
	return false
}

func ps6013DeviceType(t types.Type) bool {
	return ps6013DeviceTypeText(strings.ToLower(types.TypeString(t, func(*types.Package) string { return "" })))
}

func ps6013DeviceTypeText(text string) bool {
	return ps6007ContainsAny(text,
		"devicetensor", "devicebuffer", "gpubuffer", "metalbuffer", "mtlbuffer",
		"graphnode", "graphvalue", "recorder", "commandbuffer",
	)
}

func ps6013Scan(pass *analysis.Pass, fn *ast.FuncDecl, sig *types.Signature) ps6013Function {
	scan := ps6013Function{
		fn:          fn,
		sig:         sig,
		accelerator: ps6013AcceleratorText(fn.Name.Name),
		allocations: make(map[types.Object]token.Pos),
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.Ident:
			scan.accelerator = scan.accelerator || ps6013AcceleratorText(value.Name)
		case *ast.AssignStmt:
			ps6013RecordAssignments(pass, &scan, value.Lhs, value.Rhs)
		case *ast.DeclStmt:
			ps6013RecordDecl(pass, &scan, value)
		case *ast.CallExpr:
			if ps6013HostAccessCall(pass, value) {
				scan.hostAccess = append(scan.hostAccess, value.Pos())
			}
			if ps6013KernelCall(pass, value) {
				scan.kernels = append(scan.kernels, value)
			}
			if ps6013WaitCall(pass, value) {
				scan.waits = append(scan.waits, value)
			}
			if ps6013CopybackCall(pass, value) {
				scan.copybacks = append(scan.copybacks, value)
			}
			if ps6013CallAcceleratorText(pass, value) {
				scan.accelerator = true
			}
		}
		return true
	})
	return scan
}

func ps6013RecordAssignments(pass *analysis.Pass, scan *ps6013Function, lhs, rhs []ast.Expr) {
	if len(lhs) != len(rhs) {
		return
	}
	for i := range lhs {
		id, ok := ps2110Unparen(lhs[i]).(*ast.Ident)
		if !ok || !ps6013RuntimeAllocation(pass, rhs[i]) {
			continue
		}
		if object := identObject(pass, id); object != nil {
			scan.allocations[object] = rhs[i].Pos()
		}
	}
}

func ps6013RecordDecl(pass *analysis.Pass, scan *ps6013Function, decl *ast.DeclStmt) {
	gen, ok := decl.Decl.(*ast.GenDecl)
	if !ok {
		return
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Names) != len(value.Values) {
			continue
		}
		lhs := make([]ast.Expr, len(value.Names))
		for i, name := range value.Names {
			lhs[i] = name
		}
		ps6013RecordAssignments(pass, scan, lhs, value.Values)
	}
}

func ps6013RuntimeAllocation(pass *analysis.Pass, expr ast.Expr) bool {
	call, ok := ps2110Unparen(expr).(*ast.CallExpr)
	if !ok {
		return false
	}
	if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok {
		if builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin); ok && builtin.Name() == "make" && len(call.Args) >= 2 {
			slice, ok := ps2110Unparen(call.Args[0]).(*ast.ArrayType)
			if !ok || slice.Len != nil {
				return false
			}
			for _, extent := range call.Args[1:] {
				if ps6013RuntimeExtent(pass, extent) {
					return true
				}
			}
			return false
		}
	}
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok || !ps6007ContainsAny(ps6007NormalizeName(fn.Name()), "allocoutput", "newoutput", "newtensor", "zerostensor", "emptytensor", "allocbuffer") {
		return false
	}
	for _, arg := range call.Args {
		if ps6013RuntimeExtent(pass, arg) {
			return true
		}
	}
	return false
}

func ps6013RuntimeExtent(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	return !ok || tv.Value == nil || tv.Value.Kind() != constant.Int
}

func ps6013HostAccessCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	return ps6007ContainsAny(name, "storage", "hostdata", "hoststorage", "float32data", "dataf32", "rawdata")
}

func ps6013KernelCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	if ps6013CGOKernelCall(pass, call) {
		return true
	}
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	return ps6007ContainsAny(name, "dispatch", "launch", "kernel", "executekernel", "encodekernel", "runop")
}

func ps6013CGOKernelCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	selector, ok := ps2110Unparen(call.Fun).(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := ps2110Unparen(selector.X).(*ast.Ident)
	if !ok {
		return false
	}
	pkg, ok := pass.TypesInfo.Uses[id].(*types.PkgName)
	if !ok || pkg.Imported().Path() != "C" {
		return false
	}
	name := strings.ToLower(selector.Sel.Name)
	return ps6007ContainsAny(name, "kernel", "dispatch", "launch", "scatter", "gather", "reduce")
}

func ps6013WaitCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	if ps6007ContainsAny(name, "waituntilcompleted", "synchronize", "syncdevice", "blockuntilcomplete") {
		return true
	}
	if name != "wait" && name != "sync" && name != "finish" {
		return false
	}
	return ps6013AcceleratorText(ps6013CallContext(fn, sig))
}

func ps6013CopybackCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	return ps6007ContainsAny(name, "download", "readback", "copytohost", "copyfromdevice", "tohost", "getbytes", "readbuffer")
}

func ps6013CallAcceleratorText(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, sig, ok := typedCallee(pass, call.Fun)
	return ok && ps6013AcceleratorText(ps6013CallContext(fn, sig))
}

func ps6013CallContext(fn *types.Func, sig *types.Signature) string {
	var text strings.Builder
	text.WriteString(fn.Name())
	if fn.Pkg() != nil {
		text.WriteString(fn.Pkg().Path())
	}
	if sig.Recv() != nil {
		text.WriteString(types.TypeString(sig.Recv().Type(), func(*types.Package) string { return "" }))
	}
	return text.String()
}

func ps6013AcceleratorText(text string) bool {
	text = strings.ToLower(text)
	return ps6007ContainsAny(text, "metal", "cuda", "gpu", "mps", "opencl", "accelerator", "device", "driver")
}

func ps6013OrderedRoundtrip(pass *analysis.Pass, scan ps6013Function) (*ast.CallExpr, types.Object, bool) {
	for _, kernel := range scan.kernels {
		if !ps6013PositionBefore(scan.hostAccess, kernel.Pos()) {
			continue
		}
		for _, wait := range scan.waits {
			if wait.Pos() <= kernel.Pos() {
				continue
			}
			for _, copyback := range scan.copybacks {
				if copyback.Pos() <= wait.Pos() {
					continue
				}
				var selected types.Object
				selectedPos := token.NoPos
				for allocation, allocationPos := range scan.allocations {
					if allocationPos >= kernel.Pos() || !ps6013WholeCopyback(pass, copyback, allocation) {
						continue
					}
					if selected == nil || allocationPos < selectedPos || allocationPos == selectedPos && allocation.Pos() < selected.Pos() {
						selected, selectedPos = allocation, allocationPos
					}
				}
				if selected != nil {
					return kernel, selected, true
				}
			}
		}
	}
	return nil, nil, false
}

func ps6013WholeCopyback(pass *analysis.Pass, call *ast.CallExpr, allocation types.Object) bool {
	for _, arg := range call.Args {
		if id, ok := ps2110Unparen(arg).(*ast.Ident); ok && ps6012IdentObject(pass, id) == allocation {
			return true
		}
	}
	mentionsAllocation := false
	hasFullLength := false
	for _, arg := range call.Args {
		mentionsAllocation = mentionsAllocation || ps6012Mentions(pass, arg, allocation)
		candidate, ok := ps2110Unparen(arg).(*ast.CallExpr)
		if !ok || len(candidate.Args) != 1 {
			continue
		}
		id, ok := ps2110Unparen(candidate.Fun).(*ast.Ident)
		if !ok {
			continue
		}
		builtin, ok := pass.TypesInfo.Uses[id].(*types.Builtin)
		hasFullLength = hasFullLength || ok && builtin.Name() == "len" && ps6012Mentions(pass, candidate.Args[0], allocation)
	}
	return mentionsAllocation && hasFullLength
}

func ps6013PositionBefore(positions []token.Pos, limit token.Pos) bool {
	for _, position := range positions {
		if position < limit {
			return true
		}
	}
	return false
}

func ps6013LowIntensity(fn *ast.FuncDecl) bool {
	return ps6013LowIntensityText(fn.Name.Name)
}

func ps6013NodeLowIntensity(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if found {
			return false
		}
		switch value := n.(type) {
		case *ast.Ident:
			found = ps6013LowIntensityText(value.Name)
		case *ast.BasicLit:
			found = ps6013LowIntensityText(value.Value)
		}
		return !found
	})
	return found
}

func ps6013LowIntensityText(text string) bool {
	text = strings.ToLower(text)
	return ps6007ContainsAny(text, "gather", "scatter", "embed", "reduc", "copy", "movement", "transpose")
}
