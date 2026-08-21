package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6021 implements owner issue #765: an exact-output gate is not generally
// valid when a vendor GEMM can change reduction scheduling with result shape.
var PS6021 = register(&lint.Check{
	ID:       "PS6021",
	Category: "verify",
	Slug:     "fused-gemm-exact-output-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a fused floating-point vendor GEMM uses bit-exact output as a mandatory promotion gate",
		Text: `Concatenating bit-exact operands and replacing several GEMMs with
one wider GEMM can change output bits even when every dot product keeps the
same K and identical values. Vendor libraries may choose tiling, accumulation,
or reduction schedules from the result width, column offset, or overall shape.
An exact-output gate therefore rejects a numerically sound shape fusion for a
reason that does not isolate operand correctness.

This check implements owner issue #765. It examines real BenchmarkX(*testing.B)
and TestX(*testing.T) harnesses only when their source identifies all of these:

  - fused, grouped, mixed, combined, or concatenated floating-point work;
  - a GEMM, matmul, or projection whose result shape/width/columns change; and
  - a vendor/accelerator implementation such as MPS, Metal, CUDA/cuBLAS,
    Vulkan, GPU, or another explicitly named vendor library.

Within that narrow context, an ExactOutputRequired/BitExactOutput-style true
manifest field, an explicit requireExactOutput/assertBitExactOutput helper, or
a fatal !slices.Equal/bytes.Equal/reflect.DeepEqual output comparison is a
mandatory exact-output gate. The diagnostic separately audits the replacement
evidence: exact operand/expanded-storage bytes, finite bounded numerical error
(for example segment NRMSE plus a tolerance), and end-to-end semantic or
quality behavior.

Exact comparisons of expanded weights/storage are not output gates. Integer
GEMMs and comments documenting a custom shape-invariant fixed reduction order
stay silent. Setting ExactOutputRequired to false while retaining exact operand
storage, finite error bounds, and semantic gates is the intended manifest.

There is NO automatic fix. The correct tolerance and semantic metric depend on
dtype, model, hardware, and the operation's contract. Keep exact storage/input
validation strict, then predeclare finite error and trained end-to-end quality
gates before benchmarking the shape rewrite.`,
		Before: `gate := FusedGEMMGate{
	ExactOutputRequired: true,
}
if !slices.Equal(separateOutput, wideOutput) {
	t.Fatal("fusion changed output bits")
}`,
		After: `gate := FusedGEMMGate{
	ExactOutputRequired: false,
	ExpandedOperandStorageExact: true,
	FiniteOutput: true,
	SegmentNRMECeiling: 1e-3,
	TrainedSemanticParity: true,
}`,
		MeasuredWin: `In the Apple-M2 MPS campaign behind issue #765, concatenated
f16 expansion bytes were exact, yet one wider GEMM changed output bits solely
through result-column-dependent scheduling. Segment NRMSE was 2.29e-4 to
8.88e-4 at M64 and 0 to 8.97e-4 at M512. The fused ten-layer projection stage
still improved 1.7308x and 1.2160x respectively across ten interleaved samples.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6021",
		Doc:  "fused vendor GEMM shape rewrite incorrectly requires bit-exact floating-point output",
		Run:  runPS6021,
	},
})

func runPS6021(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6021Context(text) || ps6021FixedReduction(text) {
				continue
			}
			parents := ps6019Parents(fn.Body)
			gate, ok := ps6021ExactOutputGate(pass, fn.Body, parents)
			if !ok {
				continue
			}
			missing := ps6021MissingEvidence(text)
			detail := ""
			if len(missing) > 0 {
				detail = "; missing replacement evidence: " + strings.Join(missing, ", ")
			}
			pass.Reportf(gate.Pos(), "fused floating-point vendor GEMM shape rewrite makes bit-exact output a mandatory promotion gate%s; exact output is not a generic shape-fusion invariant—require exact operand/storage validation separately, then finite bounded numerical error and end-to-end semantic/quality gates", detail)
		}
	}
	return nil, nil
}

func ps6021Harness(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	if fn.Recv != nil {
		return false
	}
	want := ""
	switch {
	case strings.HasPrefix(fn.Name.Name, "Benchmark"):
		want = "B"
	case strings.HasPrefix(fn.Name.Name, "Test"):
		want = "T"
	default:
		return false
	}
	object, ok := pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok {
		return false
	}
	sig, ok := object.Type().(*types.Signature)
	if !ok || sig.Params().Len() != 1 || sig.Results().Len() != 0 || sig.Variadic() {
		return false
	}
	pointer, ok := types.Unalias(sig.Params().At(0).Type()).(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := types.Unalias(pointer.Elem()).(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && named.Obj().Name() == want
}

func ps6021Context(text string) bool {
	normalized := ps6007NormalizeName(text)
	fusion := ps6007ContainsAny(normalized, "fused", "fusion", "grouped", "combined", "mixed", "concatenated", "widegemm")
	gemm := ps6007ContainsAny(normalized, "gemm", "matmul", "projection")
	shape := ps6007ContainsAny(normalized, "shape", "width", "wide", "column", "segment", "concat")
	vendor := ps6007ContainsAny(normalized, "mps", "metal", "cuda", "cublas", "vulkan", "gpu", "accelerator", "vendor")
	floating := ps6007ContainsAny(normalized, "f16", "float16", "bf16", "bfloat16", "f32", "float32", "f64", "float64", "floatingpoint")
	return fusion && gemm && shape && vendor && floating
}

func ps6021FixedReduction(text string) bool {
	normalized := ps6007NormalizeName(text)
	return ps6007ContainsAny(normalized,
		"customfixedreductionorder",
		"shapeinvariantreductionorder",
		"documentedfixedreductionschedule",
		"integergemmexactcontract",
	)
}

func ps6021ExactOutputGate(pass *analysis.Pass, body *ast.BlockStmt, parents map[ast.Node]ast.Node) (ast.Node, bool) {
	var gate ast.Node
	ast.Inspect(body, func(node ast.Node) bool {
		if gate != nil {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch value := node.(type) {
		case *ast.KeyValueExpr:
			key, ok := ps2110Unparen(value.Key).(*ast.Ident)
			if ok && ps6021ExactOutputField(key.Name) && ps6021RequiredValue(pass, value.Value) {
				gate = value
				return false
			}
		case *ast.CallExpr:
			if ps6021ExplicitExactHelper(pass, value) || ps6021FatalExactComparison(pass, value, parents) {
				gate = value
				return false
			}
		}
		return true
	})
	return gate, gate != nil
}

func ps6021ExactOutputField(name string) bool {
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name,
		"exactoutputrequired",
		"requireexactoutput",
		"bitexactoutput",
		"outputbitexact",
		"exactresultrequired",
		"requireexactresult",
	)
}

func ps6021RequiredValue(pass *analysis.Pass, expr ast.Expr) bool {
	tv, ok := pass.TypesInfo.Types[ps2110Unparen(expr)]
	if !ok || tv.Value == nil {
		return true
	}
	switch tv.Value.Kind() {
	case constant.Bool:
		return constant.BoolVal(tv.Value)
	case constant.String:
		value := ps6007NormalizeName(constant.StringVal(tv.Value))
		return ps6007ContainsAny(value, "required", "exact", "bitexact", "strict")
	}
	return true
}

func ps6021ExplicitExactHelper(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok {
		return false
	}
	name := ps6007NormalizeName(fn.Name())
	return ps6007ContainsAny(name,
		"requireexactoutput",
		"assertexactoutput",
		"assertbitexactoutput",
		"requirebitexactresult",
	)
}

func ps6021FatalExactComparison(pass *analysis.Pass, call *ast.CallExpr, parents map[ast.Node]ast.Node) bool {
	fn, _, ok := typedCallee(pass, call.Fun)
	if !ok || !ps6021EqualityFunction(fn.Pkg().Path(), fn.Name()) || !ps6021OutputArgs(call) {
		return false
	}
	negated := false
	for parent := parents[call]; parent != nil; parent = parents[parent] {
		if unary, ok := parent.(*ast.UnaryExpr); ok && unary.Op == token.NOT {
			negated = true
		}
		if statement, ok := parent.(*ast.IfStmt); ok {
			return negated && call.Pos() >= statement.Cond.Pos() && call.End() <= statement.Cond.End() && ps6021FatalBody(pass, statement.Body)
		}
		if _, boundary := parent.(ast.Stmt); boundary {
			return false
		}
	}
	return false
}

func ps6021EqualityFunction(pkgPath, name string) bool {
	if name == "DeepEqual" {
		return pkgPath == "reflect"
	}
	return name == "Equal" && (pkgPath == "slices" || pkgPath == "bytes")
}

func ps6021OutputArgs(call *ast.CallExpr) bool {
	var output, storage bool
	for _, arg := range call.Args {
		ast.Inspect(arg, func(node ast.Node) bool {
			id, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			name := ps6007NormalizeName(id.Name)
			output = output || ps6007ContainsAny(name, "output", "result", "logit", "projection", "got", "want", "candidate", "reference")
			storage = storage || ps6007ContainsAny(name, "storage", "operand", "weight", "input", "expanded")
			return true
		})
	}
	return output && !storage
}

func ps6021FatalBody(pass *analysis.Pass, body *ast.BlockStmt) bool {
	fatal := false
	ast.Inspect(body, func(node ast.Node) bool {
		if fatal {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if id, ok := ps2110Unparen(call.Fun).(*ast.Ident); ok && id.Name == "panic" {
			fatal = true
			return false
		}
		if fn, _, ok := typedCallee(pass, call.Fun); ok {
			fatal = ps6007ContainsAny(ps6007NormalizeName(fn.Name()), "fatal", "failnow")
		}
		return !fatal
	})
	return fatal
}

func ps6021MissingEvidence(text string) []string {
	normalized := ps6007NormalizeName(text)
	exactStorage := ps6007ContainsAny(normalized,
		"operandstorageexact", "expandedstorageexact", "exactoperand", "exactweightbytes", "bitexactstorage", "exactexpandedbytes",
	)
	finite := strings.Contains(normalized, "finite")
	errorBound := ps6007ContainsAny(normalized, "nrmse", "relativeerror", "maxerror", "errorbound", "tolerance", "numericalceiling")
	semantic := ps6007ContainsAny(normalized,
		"semanticparity", "trainedsemantic", "trainedmodel", "tokenparity", "generatedtoken", "qualitygate", "perplexity", "endtoendquality",
	)
	var missing []string
	if !exactStorage {
		missing = append(missing, "exact operand/expanded-storage bytes")
	}
	if !finite || !errorBound {
		missing = append(missing, "finite bounded numerical error")
	}
	if !semantic {
		missing = append(missing, "end-to-end semantic/quality gate")
	}
	return missing
}
