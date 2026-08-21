package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6047 implements owner issue #731: packed quant loads need alignment,
// pipeline-selection, parity, incident-shape, and compiler-delta evidence.
var PS6047 = register(&lint.Check{
	ID:       "PS6047",
	Category: "verify",
	Slug:     "metal-quant-packed-load-needs-alignment-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a packed Metal quant-unpack experiment omits alignment or compiler evidence",
		Text: `A smaller native kernel can produce a real but sub-threshold leaf
gain. Packed device loads are only valid when the row/block alignment supports
their width; a 210-byte Q6_K stride gives odd rows two-byte, not four-byte,
alignment.

This check implements owner issue #731. It audits QuantUnpackEvidence,
PackedLoadExperiment, MetalQuantCompilerReport, Q6KLoadScheduleGate, or
equivalent manifests. Evidence must record:

  - hardware/OS/Go/toolchain/native target and quant format;
  - row stride, minimum alignment, selected device-load width, assembled word
    width, direct-32-bit validity, and load schedule;
  - distinct control/candidate pipeline identities, candidate selection, and
    fallback exclusion;
  - mixed-sign odd-tail exact parity;
  - incident shapes, control/candidate latency and ratio vectors, allocation
    bytes/count vectors, minimum speedup, and all-cells direction;
  - same-AIR provenance, control/candidate native text bytes, byte/fraction
    deltas, spill-section bytes, GPU-stats/register availability;
  - promotion threshold/verdict, classification, and final decision.

Constant evidence is checked for unsafe word width, stale table ratios,
allocation differences, text-size arithmetic, and contradictory promotion.
A code-size win remains reportable even when every incident cell improves but
the minimum speedup misses the keep threshold. There is NO automatic fix
because alignment, compiler output, pipeline identity, parity, and timings are
backend/runtime facts.`,
		Before: `uint packed = *(device uint *)(row + offset); // assumes 4-byte alignment`,
		After: `ushort lo = *(device ushort *)(row + offset);
ushort hi = *(device ushort *)(row + offset + 2);
uint packed = uint(lo) | (uint(hi) << 16);`,
		MeasuredWin: `The Apple-M2-Pro Q6_K experiment behind issue #731 used
aligned 16-bit loads because the 210-byte row stride only guarantees two-byte
alignment. Four incident shapes improved by roughly 1.2–3.7%, below the 1.10x
gate. Native text shrank from 2,738 to 2,440 bytes (298 bytes, 10.9%), neither
object spilled, and register counts remained unknown because GPU stats were
empty. The candidate was removed while preserving the compiler result.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6047",
		Doc:  "packed quant load experiment lacks alignment/compiler gate",
		Run:  runPS6047,
	},
})

type ps6047Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6047Axes = []ps6047Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047HardwareField) }},
	{name: "OS identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047OSField) }},
	{name: "Go version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047GoField) }},
	{name: "toolchain build", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047ToolchainField) }},
	{name: "native target", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047TargetField) }},
	{name: "quant format", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047FormatField) }},
	{name: "row stride", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047StrideField) }},
	{name: "minimum row alignment", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047AlignmentField) }},
	{name: "selected load width", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047LoadWidthField) }},
	{name: "assembled word width", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047WordWidthField) }},
	{name: "direct 32-bit load validity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047Direct32Field) }},
	{name: "load schedule", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047ScheduleField) }},
	{name: "control pipeline identity", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047PipelineField(n, "control") })
	}},
	{name: "candidate pipeline identity", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047PipelineField(n, "candidate") })
	}},
	{name: "candidate pipeline selection", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047SelectedField) }},
	{name: "fallback exclusion", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047FallbackField) }},
	{name: "mixed-sign odd-tail parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047ParityField) }},
	{name: "incident shapes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047ShapesField) }},
	{name: "control latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047LatenciesField(n, "control") })
	}},
	{name: "candidate latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047LatenciesField(n, "candidate") })
	}},
	{name: "incident speedups", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047SpeedupsField) }},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047AllocationField(n, "control", "byte") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047AllocationField(n, "candidate", "byte") })
	}},
	{name: "control allocation counts", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047AllocationField(n, "control", "count") })
	}},
	{name: "candidate allocation counts", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047AllocationField(n, "candidate", "count") })
	}},
	{name: "minimum incident speedup", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047MinimumField) }},
	{name: "all-cells direction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047AllImprovedField) }},
	{name: "same AIR library provenance", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047SameAIRField) }},
	{name: "control native text bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047TextField(n, "control") })
	}},
	{name: "candidate native text bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047TextField(n, "candidate") })
	}},
	{name: "native text byte delta", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047TextDeltaField) }},
	{name: "native text reduction fraction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047TextReductionField) }},
	{name: "control spill-section bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047SpillField(n, "control") })
	}},
	{name: "candidate spill-section bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6047SpillField(n, "candidate") })
	}},
	{name: "GPU stats empty status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047StatsField) }},
	{name: "register counts known status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047RegistersField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047ThresholdField) }},
	{name: "promotion verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047VerdictField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6047DecisionField) }},
}

type ps6047Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6047(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6047Context(text) {
				continue
			}
			manifest, found := ps6047BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "packed quant-unpack campaign has no alignment/compiler manifest; missing %s", strings.Join(ps6047Missing(nil), ", "))
				continue
			}
			if missing := ps6047Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "packed quant-unpack evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6047Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "packed quant-unpack audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6047Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "metal") && ps6007ContainsAny(text, "quant", "q6k") &&
		ps6007ContainsAny(text, "packedload", "quantunpack", "loadschedule")
}

func ps6047BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6047Manifest, bool) {
	var best ps6047Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6047ManifestType(lit.Type) {
			return true
		}
		manifest := ps6047Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6047Axes) - len(ps6047Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6047ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6047ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6047ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "quantunpackevidence", "packedloadexperiment", "metalquantcompilerreport", "q6kloadschedulegate", "quantunpackreport")
}

func ps6047Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6047Axes))
	for _, axis := range ps6047Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6047HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6047OSField(n string) bool { return ps6007ContainsAny(n, "osidentity", "osversion", "macos") }
func ps6047GoField(n string) bool { return strings.Contains(n, "go") && strings.Contains(n, "version") }
func ps6047ToolchainField(n string) bool {
	return strings.Contains(n, "toolchain") && ps6007ContainsAny(n, "build", "version", "identity")
}
func ps6047TargetField(n string) bool {
	return strings.Contains(n, "native") && strings.Contains(n, "target")
}
func ps6047FormatField(n string) bool {
	return strings.Contains(n, "quant") && strings.Contains(n, "format")
}
func ps6047StrideField(n string) bool {
	return strings.Contains(n, "row") && strings.Contains(n, "stride")
}
func ps6047AlignmentField(n string) bool {
	return strings.Contains(n, "row") && strings.Contains(n, "alignment")
}
func ps6047LoadWidthField(n string) bool {
	return strings.Contains(n, "selected") && strings.Contains(n, "loadwidth")
}
func ps6047WordWidthField(n string) bool {
	return strings.Contains(n, "assembled") && strings.Contains(n, "wordwidth")
}
func ps6047Direct32Field(n string) bool {
	return strings.Contains(n, "direct32") && strings.Contains(n, "valid")
}
func ps6047ScheduleField(n string) bool {
	return strings.Contains(n, "load") && strings.Contains(n, "schedule")
}
func ps6047PipelineField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "pipeline") && strings.Contains(n, "identity")
}
func ps6047SelectedField(n string) bool {
	return strings.Contains(n, "candidatepipeline") && strings.Contains(n, "selected")
}
func ps6047FallbackField(n string) bool {
	return strings.Contains(n, "fallback") && strings.Contains(n, "excluded")
}
func ps6047ParityField(n string) bool {
	return strings.Contains(n, "mixedsign") && strings.Contains(n, "oddtail") && strings.Contains(n, "parity")
}
func ps6047ShapesField(n string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, "shape")
}
func ps6047LatenciesField(n, side string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, side) && ps6007ContainsAny(n, "latencies", "times")
}
func ps6047SpeedupsField(n string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, "speedup") && !strings.Contains(n, "minimum")
}
func ps6047AllocationField(n, side, detail string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, detail)
}
func ps6047MinimumField(n string) bool {
	return strings.Contains(n, "minimum") && strings.Contains(n, "incident") && strings.Contains(n, "speedup")
}
func ps6047AllImprovedField(n string) bool {
	return strings.Contains(n, "allincident") && strings.Contains(n, "improved")
}
func ps6047SameAIRField(n string) bool {
	return strings.Contains(n, "sameair") && ps6007ContainsAny(n, "library", "provenance")
}
func ps6047TextField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "nativetext") && strings.Contains(n, "byte") && !strings.Contains(n, "delta")
}
func ps6047TextDeltaField(n string) bool {
	return strings.Contains(n, "nativetext") && strings.Contains(n, "delta") && strings.Contains(n, "byte")
}
func ps6047TextReductionField(n string) bool {
	return strings.Contains(n, "nativetext") && strings.Contains(n, "reduction") && ps6007ContainsAny(n, "fraction", "ratio", "percent")
}
func ps6047SpillField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "spillsection") && strings.Contains(n, "byte")
}
func ps6047StatsField(n string) bool {
	return strings.Contains(n, "gpustats") && strings.Contains(n, "empty")
}
func ps6047RegistersField(n string) bool {
	return strings.Contains(n, "registercount") && strings.Contains(n, "known")
}
func ps6047ThresholdField(n string) bool {
	return strings.Contains(n, "promotion") && ps6007ContainsAny(n, "threshold", "gate", "minimum")
}
func ps6047VerdictField(n string) bool {
	return strings.Contains(n, "promotion") && ps6007ContainsAny(n, "verdict", "result")
}
func ps6047ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6047DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6047Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"candidate pipeline selection", ps6047SelectedField}, {"fallback exclusion", ps6047FallbackField},
		{"mixed-sign odd-tail parity", ps6047ParityField}, {"same AIR library provenance", ps6047SameAIRField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	alignment, alignmentOK := ps6016Number(fields, ps6047AlignmentField)
	loadWidth, loadWidthOK := ps6016Number(fields, ps6047LoadWidthField)
	if alignmentOK && loadWidthOK && loadWidth > alignment {
		warnings = append(warnings, fmt.Sprintf("selected %.0f-byte load exceeds %.0f-byte minimum row alignment", loadWidth, alignment))
	}
	if direct32, ok := ps6026Bool(fields, ps6047Direct32Field); ok && alignmentOK && direct32 != (alignment >= 4) {
		warnings = append(warnings, fmt.Sprintf("direct-32-bit validity is %t but %.0f-byte alignment computes %t", direct32, alignment, alignment >= 4))
	}
	controlPipeline, controlPipelineOK := ps6027String(fields, func(n string) bool { return ps6047PipelineField(n, "control") })
	candidatePipeline, candidatePipelineOK := ps6027String(fields, func(n string) bool { return ps6047PipelineField(n, "candidate") })
	if controlPipelineOK && candidatePipelineOK && ps6030StatusName(controlPipeline) == ps6030StatusName(candidatePipeline) {
		warnings = append(warnings, "control and candidate pipeline identities are identical")
	}
	shapes, shapesOK := ps6047Strings(fields, ps6047ShapesField)
	controls, controlsOK := ps6016Numbers(fields, func(n string) bool { return ps6047LatenciesField(n, "control") })
	candidates, candidatesOK := ps6016Numbers(fields, func(n string) bool { return ps6047LatenciesField(n, "candidate") })
	ratios, ratiosOK := ps6016Numbers(fields, ps6047SpeedupsField)
	if shapesOK && controlsOK && candidatesOK && ratiosOK {
		if len(shapes) != len(controls) || len(shapes) != len(candidates) || len(shapes) != len(ratios) {
			warnings = append(warnings, "incident shape/latency/speedup vectors have different lengths")
		} else {
			for index := range shapes {
				if candidates[index] > 0 && !ps6025Close(ratios[index], controls[index]/candidates[index]) {
					warnings = append(warnings, fmt.Sprintf("incident speedup for %q is %.6gx, want %.6gx", shapes[index], ratios[index], controls[index]/candidates[index]))
					break
				}
			}
		}
	}
	for _, detail := range []string{"byte", "count"} {
		control, controlOK := ps6016Numbers(fields, func(n string) bool { return ps6047AllocationField(n, "control", detail) })
		candidate, candidateOK := ps6016Numbers(fields, func(n string) bool { return ps6047AllocationField(n, "candidate", detail) })
		if controlOK && candidateOK && !slices.Equal(control, candidate) {
			warnings = append(warnings, "control/candidate incident allocation "+detail+" vectors differ")
		}
	}
	minimum, minimumOK := ps6016Number(fields, ps6047MinimumField)
	if ratiosOK && len(ratios) > 0 {
		calculated := slices.Min(ratios)
		if minimumOK && !ps6025Close(minimum, calculated) {
			warnings = append(warnings, fmt.Sprintf("minimum incident speedup %.6gx disagrees with %.6gx", minimum, calculated))
		}
		allImproved := calculated > 1
		if recorded, ok := ps6026Bool(fields, ps6047AllImprovedField); ok && recorded != allImproved {
			warnings = append(warnings, fmt.Sprintf("all-incident-shapes-improved status is %t but ratios compute %t", recorded, allImproved))
		}
		minimum, minimumOK = calculated, true
	}
	controlText, controlTextOK := ps6016Number(fields, func(n string) bool { return ps6047TextField(n, "control") })
	candidateText, candidateTextOK := ps6016Number(fields, func(n string) bool { return ps6047TextField(n, "candidate") })
	if controlTextOK && candidateTextOK && controlText > 0 {
		delta := candidateText - controlText
		if recorded, ok := ps6016Number(fields, ps6047TextDeltaField); ok && !ps6025Close(recorded, delta) {
			warnings = append(warnings, fmt.Sprintf("native text delta %.6g bytes disagrees with candidate-control %.6g", recorded, delta))
		}
		reduction := (controlText - candidateText) / controlText
		if recorded, ok := ps6016Number(fields, ps6047TextReductionField); ok && !ps6025Close(recorded, reduction) {
			warnings = append(warnings, fmt.Sprintf("native text reduction %.6g disagrees with %.6g", recorded, reduction))
		}
	}
	statsEmpty, statsEmptyOK := ps6026Bool(fields, ps6047StatsField)
	registersKnown, registersKnownOK := ps6026Bool(fields, ps6047RegistersField)
	if statsEmptyOK && registersKnownOK && statsEmpty && registersKnown {
		warnings = append(warnings, "register counts are marked known although GPU stats are empty")
	}
	threshold, thresholdOK := ps6016Number(fields, ps6047ThresholdField)
	if minimumOK && thresholdOK {
		passed := minimum >= threshold
		if verdict, ok := ps6027String(fields, ps6047VerdictField); ok {
			normalized := ps6030StatusName(verdict)
			verdictPass := ps6007ContainsAny(normalized, "pass", "promote", "accept") && !ps6007ContainsAny(normalized, "fail", "reject")
			verdictFail := ps6007ContainsAny(normalized, "fail", "reject")
			if passed && !verdictPass || !passed && !verdictFail {
				warnings = append(warnings, fmt.Sprintf("promotion verdict %q disagrees with minimum %.6gx versus %.6gx threshold", verdict, minimum, threshold))
			}
		}
		if decision, ok := ps6027String(fields, ps6047DecisionField); ok && !passed && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
			warnings = append(warnings, fmt.Sprintf("final decision %q retains a packed-load candidate below the keep threshold", decision))
		}
	}
	return warnings
}

func ps6047Strings(fields map[string]ps6016Field, predicate func(string) bool) ([]string, bool) {
	for name, field := range fields {
		if predicate(name) && field.hasStringValues {
			return field.stringValues, true
		}
	}
	return nil, false
}
