package checks

import (
	"go/ast"
	"go/constant"
	"go/token"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6008 implements the source-auditable part of owner issue #759: an
// incumbent-justified accelerator kernel port needs an explicit selector and
// memory-regime manifest before a mechanism is treated as portable.
var PS6008 = register(&lint.Check{
	ID:       "PS6008",
	Category: "verify",
	Slug:     "incumbent-port-missing-selector-regime",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an incumbent-justified GPU kernel port omits selector or memory-regime context",
		Text: `A fast mechanism in an incumbent kernel is not an independently
portable performance feature when the source and target selectors serve
different memory policies. Half operands, accumulator width, staging, tile
shape, and dispatch geometry work as a coupled design. Copying one mechanism
from a direct-quantized kernel does not reproduce the advantage of a target
incumbent that already pays for persistent expanded weights and tuned GEMM.

This check implements the source-auditable boundary from owner issue #759. It
finds real func BenchmarkX(*testing.B) functions with accelerator, kernel, and
incumbent/reference/port context, then looks for a keyed selector-regime
manifest. Recognized type names include SelectorRegimeContext,
KernelPortContext, PortEvidence, PortManifest, and IncumbentContext. The best
manifest in the benchmark must make eleven axes explicit:

  - source input state and source storage/memory state;
  - target input state and target storage/memory state;
  - source fallback and its selection reason;
  - target/production incumbent;
  - bytes cached/materialized outside the timed kernel; and
  - omitted/excluded tile-layout, staging-layout, and dispatch-geometry
    differences.

Individual omission fields or an OmittedCoupledFeatures string are accepted;
an aggregate must say "none" or name each covered category. Keyed field names
are structural evidence even when values are populated dynamically. Stronger
findings are emitted only from constant strings: a clear direct/quantized/
uncached versus persistent/cached/expanded/dense storage-policy mismatch, or
an explicit non-none coupled-feature omission. The check never infers byte
counts, selector reasons, or architectural equivalence from callee names.

This is a pre-implementation inference audit, complementary to benchmarking.
The table establishes whether the source mechanism addresses the target
incumbent's actual regime; output parity establishes semantic reachability;
and an order-alternating benchmark decides realized performance.

There is NO automatic fix. Filling the table requires measurements and source/
target architecture knowledge; changing selectors or porting omitted coupled
features changes design intent.`,
		Before: `func BenchmarkMetalIncumbentKernelPort(b *testing.B) {
    // "llama uses half operands" is not a selector-regime comparison.
    benchmarkHalfOperandPort(b)
}`,
		After: `ctx := SelectorRegimeContext{
    SourceInputState: "...", SourceStorageState: "...",
    TargetInputState: "...", TargetStorageState: "...",
    SourceFallback: "...", SourceSelectionReason: "...",
    TargetIncumbent: "...", CachedBytesOutsideTimedKernel: bytes,
    OmittedTileLayout: "none", OmittedStagingLayout: "none",
    OmittedDispatchGeometry: "none",
}
// Then benchmark the actual target incumbent against the complete port.`,
		MeasuredWin: `In the Metal Q4_K investigation behind issue #759, porting
half threadgroup/simdgroup operands reduced prototype storage from about 20 KB
to 12 KB and preserved finite parity (max relative error 2.947e-4), yet measured
960.125 us versus 385.750 us for the cached-f16 MPS incumbent at M=64, K=2048,
N=5632: only 0.4018x. The source avoided a persistent expanded-weight cache;
the target incumbent already exploited one.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6008",
		Doc:  "incumbent kernel port lacks selector and memory-regime context",
		Run:  runPS6008,
	},
})

type ps6008Field struct {
	value     string
	hasString bool
}

type ps6008Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6008Field
}

type ps6008Axis struct {
	name    string
	present func(map[string]ps6008Field) bool
}

var ps6008Axes = []ps6008Axis{
	{name: "source input state", present: func(f map[string]ps6008Field) bool { return ps6008HasTokens(f, "source", "input") }},
	{name: "source storage state", present: func(f map[string]ps6008Field) bool { return ps6008HasStorage(f, "source") }},
	{name: "target input state", present: func(f map[string]ps6008Field) bool { return ps6008HasTokens(f, "target", "input") }},
	{name: "target storage state", present: func(f map[string]ps6008Field) bool { return ps6008HasStorage(f, "target") }},
	{name: "source fallback", present: func(f map[string]ps6008Field) bool { return ps6008HasTokens(f, "source", "fallback") }},
	{name: "source selection reason", present: func(f map[string]ps6008Field) bool { return ps6008HasReason(f) }},
	{name: "target incumbent", present: func(f map[string]ps6008Field) bool { return ps6008HasIncumbent(f) }},
	{name: "external cached/materialized bytes", present: func(f map[string]ps6008Field) bool { return ps6008HasExternalBytes(f) }},
	{name: "omitted tile/layout differences", present: func(f map[string]ps6008Field) bool { return ps6008HasOmissionAxis(f, "tile") }},
	{name: "omitted staging layout", present: func(f map[string]ps6008Field) bool { return ps6008HasOmissionAxis(f, "staging") }},
	{name: "omitted dispatch geometry", present: func(f map[string]ps6008Field) bool { return ps6008HasOmissionAxis(f, "dispatch") }},
}

func runPS6008(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) || !ps6008PortContext(pass, fn) {
				continue
			}
			manifest, found := ps6008BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "incumbent-justified accelerator kernel port has no selector-regime manifest; missing %s", strings.Join(ps6008Missing(nil), ", "))
				continue
			}
			if missing := ps6008Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "selector-regime manifest for an incumbent-justified accelerator kernel port is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			mismatch := ps6008MemoryMismatch(manifest.fields)
			omissions := ps6008ExplicitOmissions(manifest.fields)
			switch {
			case mismatch && len(omissions) > 0:
				pass.Reportf(manifest.lit.Pos(), "selector-regime manifest explicitly describes different direct-vs-cached source/target memory policies and non-none omitted coupled features (%s); a mechanism-only incumbent port cannot support the claimed gain without a complete target-regime benchmark", strings.Join(omissions, ", "))
			case mismatch:
				pass.Reportf(manifest.lit.Pos(), "selector-regime manifest explicitly describes different direct-vs-cached source/target memory policies; the incumbent mechanism is not independently portable—benchmark the complete target regime")
			case len(omissions) > 0:
				pass.Reportf(manifest.lit.Pos(), "selector-regime manifest declares non-none omitted coupled features (%s); benchmark the complete tile/staging/dispatch design before attributing the incumbent gain to the ported mechanism", strings.Join(omissions, ", "))
			}
		}
	}
	return nil, nil
}

func ps6008BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6008Manifest, bool) {
	var best ps6008Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6008ManifestType(lit.Type) {
			return true
		}
		manifest := ps6008Manifest{lit: lit, fields: ps6008Fields(pass, lit)}
		score := len(ps6008Axes) - len(ps6008Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6008ManifestType(expr ast.Expr) bool {
	var name string
	switch n := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = n.Name
	case *ast.SelectorExpr:
		name = n.Sel.Name
	case *ast.IndexExpr:
		return ps6008ManifestType(n.X)
	case *ast.IndexListExpr:
		return ps6008ManifestType(n.X)
	}
	n := ps6007NormalizeName(name)
	return strings.Contains(n, "selectorregime") || strings.Contains(n, "regimecontext") ||
		strings.Contains(n, "portcontext") || strings.Contains(n, "portevidence") ||
		strings.Contains(n, "portmanifest") || strings.Contains(n, "incumbentcontext")
}

func ps6008Fields(pass *analysis.Pass, lit *ast.CompositeLit) map[string]ps6008Field {
	fields := make(map[string]ps6008Field, len(lit.Elts))
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := ps2110Unparen(kv.Key).(*ast.Ident)
		if !ok {
			continue
		}
		field := ps6008Field{}
		if tv, ok := pass.TypesInfo.Types[ps2110Unparen(kv.Value)]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
			field.value = constant.StringVal(tv.Value)
			field.hasString = true
		}
		fields[ps6007NormalizeName(key.Name)] = field
	}
	return fields
}

func ps6008Missing(fields map[string]ps6008Field) []string {
	missing := make([]string, 0, len(ps6008Axes))
	for _, axis := range ps6008Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6008HasTokens(fields map[string]ps6008Field, tokens ...string) bool {
	for name := range fields {
		matched := true
		for _, token := range tokens {
			if !strings.Contains(name, token) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func ps6008HasStorage(fields map[string]ps6008Field, side string) bool {
	for name := range fields {
		if strings.Contains(name, side) && ps6007ContainsAny(name, "storage", "memory", "cache", "weightstate") {
			return true
		}
	}
	return false
}

func ps6008HasReason(fields map[string]ps6008Field) bool {
	for name := range fields {
		if strings.Contains(name, "reason") && ps6007ContainsAny(name, "source", "selection", "fallback") {
			return true
		}
	}
	return false
}

func ps6008HasIncumbent(fields map[string]ps6008Field) bool {
	for name := range fields {
		if strings.Contains(name, "incumbent") && ps6007ContainsAny(name, "target", "production") {
			return true
		}
	}
	return false
}

func ps6008HasExternalBytes(fields map[string]ps6008Field) bool {
	for name := range fields {
		if strings.Contains(name, "bytes") && ps6007ContainsAny(name, "cached", "materialized", "external", "outside", "resident") {
			return true
		}
	}
	return false
}

func ps6008HasOmissionAxis(fields map[string]ps6008Field, axis string) bool {
	for name, field := range fields {
		if ps6008OmissionName(name) && ps6008OmissionMentions(name, axis) {
			return true
		}
		if ps6008AggregateOmissions(name) && field.hasString {
			value := ps6007NormalizeName(field.value)
			if ps6008NoOmissions(field.value) || ps6008OmissionMentions(value, axis) {
				return true
			}
		}
	}
	return false
}

func ps6008OmissionMentions(text, axis string) bool {
	switch axis {
	case "tile":
		return strings.Contains(text, "tile") || strings.Contains(text, "layout") && !strings.Contains(text, "staging")
	case "staging":
		return strings.Contains(text, "staging")
	case "dispatch":
		return strings.Contains(text, "dispatch") || strings.Contains(text, "geometry")
	default:
		return false
	}
}

func ps6008OmissionName(name string) bool {
	return ps6007ContainsAny(name, "omitted", "excluded", "notported", "difference")
}

func ps6008AggregateOmissions(name string) bool {
	return strings.Contains(name, "coupledfeatures") && ps6008OmissionName(name)
}

func ps6008NoOmissions(value string) bool {
	n := strings.ToLower(strings.TrimSpace(value))
	return n == "" || n == "none" || n == "no" || n == "n/a" || n == "na" || n == "fully ported"
}

func ps6008MemoryMismatch(fields map[string]ps6008Field) bool {
	source, sourceOK := ps6008StoragePolicy(fields, "source")
	target, targetOK := ps6008StoragePolicy(fields, "target")
	if !sourceOK || !targetOK {
		return false
	}
	return source != target
}

func ps6008StoragePolicy(fields map[string]ps6008Field, side string) (string, bool) {
	policy := ""
	for name, field := range fields {
		if field.hasString && strings.Contains(name, side) && ps6007ContainsAny(name, "storage", "memory", "cache", "weightstate") {
			candidate := ps6008MemoryPolicy(field.value)
			if candidate == "" {
				continue
			}
			if policy != "" && policy != candidate {
				return "", false
			}
			policy = candidate
		}
	}
	return policy, policy != ""
}

func ps6008MemoryPolicy(value string) string {
	n := strings.ToLower(value)
	cached := ps6007ContainsAny(n, "cached", "persistent", "expanded", "preconverted", "pre-converted", "dense f16", "resident weight")
	direct := ps6007ContainsAny(n, "direct", "on-the-fly", "on the fly", "uncached", "no cache", "quantized", "compressed", "q4_", "q6_")
	switch {
	case cached && !direct:
		return "cached"
	case direct && !cached:
		return "direct"
	default:
		return ""
	}
}

func ps6008ExplicitOmissions(fields map[string]ps6008Field) []string {
	set := make(map[string]bool)
	for name, field := range fields {
		if !field.hasString || ps6008NoOmissions(field.value) {
			continue
		}
		switch {
		case ps6008AggregateOmissions(name):
			set["coupled features"] = true
		case ps6008OmissionName(name) && strings.Contains(name, "staging"):
			set["staging"] = true
		case ps6008OmissionName(name) && ps6007ContainsAny(name, "dispatch", "geometry"):
			set["dispatch geometry"] = true
		case ps6008OmissionName(name) && ps6008OmissionMentions(name, "tile"):
			set["tile/layout"] = true
		}
	}
	omissions := make([]string, 0, len(set))
	for omission := range set {
		omissions = append(omissions, omission)
	}
	slices.Sort(omissions)
	return omissions
}

func ps6008PortContext(pass *analysis.Pass, fn *ast.FuncDecl) bool {
	accelerator, kernel, port := false, false, false
	classify := func(text string) {
		raw := text
		text = strings.ToLower(text)
		accelerator = accelerator || ps6007ContainsAny(text, "gpu", "metal", "cuda", "mps", "accelerator")
		kernel = kernel || ps6007ContainsAny(text, "kernel", "gemm", "matmul", "simdgroup", "ffn")
		port = port || ps6008PortSignal(raw)
	}
	classify(fn.Name.Name)
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if accelerator && kernel && port {
			return false
		}
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		switch n := node.(type) {
		case *ast.Ident:
			classify(n.Name)
		case *ast.BasicLit:
			if n.Kind == token.STRING {
				classify(n.Value)
			}
		case *ast.CallExpr:
			if callee, _, ok := typedCallee(pass, n.Fun); ok {
				classify(callee.Name())
				if callee.Pkg() != nil {
					classify(callee.Pkg().Path())
				}
			}
		}
		return !(accelerator && kernel && port)
	})
	return accelerator && kernel && port
}

func ps6008PortSignal(text string) bool {
	lower := strings.ToLower(text)
	if ps6007ContainsAny(lower, "incumbent", "reference", "upstream", "sourcekernel", "competitor", "transplant", "llama") ||
		strings.Contains(text, "Port") || strings.HasPrefix(lower, "port") {
		return true
	}
	for _, word := range strings.FieldsFunc(lower, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if word == "port" || word == "ported" || word == "porting" {
			return true
		}
	}
	return false
}
