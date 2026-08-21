package checks

import (
	"go/ast"
	"go/constant"
	"go/types"
	"math"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6007 implements the statically provable part of owner issue #758: a GPU
// residency/fusion proposal declares enough component evidence to prove that
// perfect removal still cannot reach its promotion gate.
var PS6007 = register(&lint.Check{
	ID:       "PS6007",
	Category: "verify",
	Slug:     "removable-pass-ceiling-below-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU residency rewrite's zero-cost removable-pass ceiling is below its promotion gate",
		Text: `Removing conversion or elementwise passes around tuned library GEMMs
can be semantically valid yet have too little leverage to justify the rewrite.
If removable work occupies fraction f of the baseline, making that work free
can improve the whole chain by at most 1/(1-f). A 4% removable share therefore
has a hard ceiling of only 1.0417x, regardless of implementation quality.

This check implements the compile-time evidence boundary from owner issue #758.
Inside a real func BenchmarkX(*testing.B) with GPU/accelerator and GEMM/matmul/
FFN context, it recognizes referenced numeric constants whose names declare:

  - a promotion/speedup gate (promotionGate, minSpeedup, targetSpeedup, etc.);
  - either removable *Share, *Fraction, or *Percent constants; or
  - one baseline constant plus one or more removable component constants in
    the same typed unit or with the same explicit ns/us/ms/micros/cycles suffix.

Multiple removable components are summed. That deliberately overestimates the
available leverage if measurements overlap, so a reported below-gate ceiling
remains conservative. Runtime variables, mixed/implicit units, ambiguous
multiple baselines or gates, non-benchmarks, and source without accelerator-
GEMM context are rejected. The rule never invents timings from call names.

The diagnostic separates three claims that must not be conflated: semantic
reachability and output parity show that a candidate can run; independently
measured removable share bounds its leverage; and an order-alternating
whole-chain benchmark determines the realized result. Passing the first does
not repair a ceiling that fails the second.

There is NO automatic fix. Changing a promotion gate, deleting a candidate, or
expanding the fused region are different engineering decisions. Reject or
resize the proposal before recorder/cache plumbing, then retain the final
whole-chain benchmark for candidates whose ceiling can reach the gate.`,
		Before: `const (
    removablePassShare = 0.04
    promotionGate      = 1.10
)

func BenchmarkMetalFFNResidency(b *testing.B) {
    // Even free removable passes cap the chain at 1/(1-.04) = 1.0417x.
    benchmarkMPSGEMMChain(b, removablePassShare, promotionGate)
}`,
		After: `// Reject or enlarge the proposal before implementation: 1.0417x < 1.10x.
// For a reachable proposal, separately require output parity and an
// order-alternating whole-chain benchmark; neither substitutes for the ceiling.`,
		MeasuredWin: `In the Metal FFN residency investigation behind issue #758
(M=64, D=2048, H=5632; ten alternating warm M2 samples), the baseline median
was 1198.542 us and the residency candidate median was 1158.206 us: 1.0348x,
below the predeclared 1.10x gate despite exact final-output parity and a live
selector mutation. The leverage gate prevented further promotion plumbing.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6007",
		Doc:  "zero-cost removable GPU pass ceiling is below the declared promotion gate",
		Run:  runPS6007,
	},
})

type ps6007Constant struct {
	name  string
	value float64
	typ   types.Type
	id    *ast.Ident
}

func runPS6007(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) || !ps6007GPUChainContext(pass, fn) {
				continue
			}
			gate, share, ok := ps6007CeilingEvidence(pass, fn.Body)
			if !ok {
				continue
			}
			ceiling := 1 / (1 - share)
			if ceiling >= gate.value*(1-1e-12) {
				continue
			}
			pass.Reportf(gate.id.Pos(), "declared removable-pass share %.4f%% has a zero-cost whole-chain ceiling of %.4fx, below the %.4fx promotion gate; semantic parity proves reachability, not leverage—reject or enlarge the proposal before plumbing, then validate any reachable candidate with an order-alternating whole-chain benchmark", share*100, ceiling, gate.value)
		}
	}
	return nil, nil
}

func ps6007CeilingEvidence(pass *analysis.Pass, body *ast.BlockStmt) (ps6007Constant, float64, bool) {
	seen := make(map[*types.Const]bool)
	var gates, baselines, removable, shares, percents []ps6007Constant
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		obj, ok := pass.TypesInfo.Uses[id].(*types.Const)
		if !ok || seen[obj] {
			return true
		}
		seen[obj] = true
		if obj.Val().Kind() != constant.Int && obj.Val().Kind() != constant.Float {
			return true
		}
		value, _ := constant.Float64Val(constant.ToFloat(obj.Val()))
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return true
		}
		evidence := ps6007Constant{name: obj.Name(), value: value, typ: obj.Type(), id: id}
		switch ps6007ConstantRole(obj.Name()) {
		case "gate":
			gates = append(gates, evidence)
		case "baseline":
			baselines = append(baselines, evidence)
		case "removable":
			removable = append(removable, evidence)
		case "share":
			shares = append(shares, evidence)
		case "percent":
			percents = append(percents, evidence)
		}
		return true
	})
	if len(gates) != 1 || gates[0].value <= 1 {
		return ps6007Constant{}, 0, false
	}

	if len(shares)+len(percents) > 0 {
		var share float64
		for _, component := range shares {
			if component.value < 0 || component.value >= 1 {
				return ps6007Constant{}, 0, false
			}
			share += component.value
		}
		for _, component := range percents {
			if component.value < 0 || component.value >= 100 {
				return ps6007Constant{}, 0, false
			}
			share += component.value / 100
		}
		if share > 0 && share < 1 {
			return gates[0], share, true
		}
		return ps6007Constant{}, 0, false
	}

	if len(baselines) != 1 || len(removable) == 0 || baselines[0].value <= 0 {
		return ps6007Constant{}, 0, false
	}
	base := baselines[0]
	var total float64
	for _, component := range removable {
		if component.value < 0 || !ps6007SameUnit(base, component) {
			return ps6007Constant{}, 0, false
		}
		total += component.value
	}
	if total <= 0 || total >= base.value {
		return ps6007Constant{}, 0, false
	}
	return gates[0], total / base.value, true
}

func ps6007ConstantRole(name string) string {
	n := ps6007NormalizeName(name)
	if strings.Contains(n, "promotiongate") || strings.Contains(n, "speedupgate") ||
		strings.Contains(n, "minspeedup") || strings.Contains(n, "minimumspeedup") ||
		strings.Contains(n, "targetspeedup") {
		return "gate"
	}
	removable := strings.Contains(n, "removable") || strings.Contains(n, "removedpass") || strings.Contains(n, "removedwork")
	if removable && strings.Contains(n, "percent") {
		return "percent"
	}
	if removable && (strings.Contains(n, "share") || strings.Contains(n, "fraction")) {
		return "share"
	}
	if removable {
		return "removable"
	}
	if strings.Contains(n, "baseline") {
		return "baseline"
	}
	return ""
}

func ps6007NormalizeName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

func ps6007SameUnit(base, component ps6007Constant) bool {
	if types.Identical(base.typ, component.typ) && !ps6007Untyped(base.typ) {
		return true
	}
	baseUnit := ps6007NameUnit(base.name)
	return baseUnit != "" && baseUnit == ps6007NameUnit(component.name)
}

func ps6007Untyped(t types.Type) bool {
	basic, ok := t.(*types.Basic)
	return ok && basic.Info()&types.IsUntyped != 0
}

func ps6007NameUnit(name string) string {
	n := ps6007NormalizeName(name)
	for _, unit := range []string{
		"nanoseconds", "microseconds", "milliseconds",
		"nanos", "micros", "millis", "cycles", "ticks",
		"ns", "us", "ms",
	} {
		if strings.HasSuffix(n, unit) {
			return unit
		}
	}
	return ""
}

func ps6007GPUChainContext(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	accelerator := false
	gemm := false
	classify := func(text string) {
		text = strings.ToLower(text)
		accelerator = accelerator || ps6007ContainsAny(text, "gpu", "metal", "cuda", "mps", "accelerator")
		gemm = gemm || ps6007ContainsAny(text, "gemm", "matmul", "matrixmultiply", "matrix_multiply", "ffn")
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if accelerator && gemm {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			classify(n.Name)
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, n.Fun); ok {
				classify(callee.Name())
				if callee.Pkg() != nil {
					classify(callee.Pkg().Path())
				}
			}
		}
		return !(accelerator && gemm)
	})
	return accelerator && gemm
}

func ps6007ContainsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
