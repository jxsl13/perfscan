package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6036 implements owner issue #742: internal stage timestamps and external
// xctrace counters require an exact target-scoped, fail-closed correlation
// contract.
var PS6036 = register(&lint.Check{
	ID:       "PS6036",
	Category: "verify",
	Slug:     "xctrace-stage-counter-correlation-contract",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "xctrace counters are correlated to internal GPU stages without a fail-closed identity contract",
		Text: `Timestamp overlap alone cannot safely join device-wide Apple
Performance Limiter samples to application-owned GPU stages. Export filenames
are unstable, XML references may be declared inside nested composites, and
structurally present counter streams may contain zero samples.

This check implements owner issue #742. It audits XctraceCorrelationEvidence,
StageCounterCorrelation, TargetScopedCounterEvidence, AppleTraceJoinEvidence,
or equivalent manifests. Evidence must record:

  - trace schema, counter set, hardware, benchmark digest, raw-report hashes,
    and physical-capture count;
  - semantic table discovery, recursive nested-reference resolution, and
    ambiguous-table rejection;
  - exact target process and command-buffer identity;
  - target GPU span, application stage span, affine scale, span disagreement,
    tolerance, and alignment status;
  - aligned stage labels, interval starts/ends, sample counts, and aggregates;
  - required counter names and sample counts;
  - missing-stage count and all-stages-reported status;
  - named sub-resolution stages, an explicit aggregation policy, and a ban on
    fabricated interpolation;
  - contamination value/ceiling/status;
  - fresh-process independence, accepted/rejected status, and rejection
    reasons.

Constant evidence is checked for invalid spans, vector mismatch, zero required
streams, missing stages, alignment drift, contamination, interpolation,
provenance-count mismatch, and contradictory acceptance/rejection reasons.
There is NO automatic fix: trace clocks, XML schemas, target identities,
counter semantics, and physical captures are runtime evidence.`,
		Before: `for sample := range performanceLimiterSamples {
	if overlaps(sample.Time, internalStage) { aggregate(sample) }
}`,
		After: `evidence := XctraceCorrelationEvidence{
	SemanticTableDiscoveryPassed: true,
	RecursiveNestedReferencesPassed: true,
	TargetProcessID: pid, TargetCommandBufferID: commandBufferID,
	AlignmentScale: 1.0, SpanDisagreementNS: 1, SpanToleranceNS: 1,
	StageSampleCounts: counts,
	SubResolutionAggregationPolicy: "predeclared logical family",
	FabricatedInterpolation: false,
}`,
		MeasuredWin: `The Apple-M2-Pro validation behind issue #742 used five
one-command-buffer physical reports. Every accepted span aligned at scale 1.0
with at most 1 ns disagreement. The result is a reusable correlation method,
not a workload speed claim.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6036",
		Doc:  "xctrace counter samples lack fail-closed target-scoped stage correlation evidence",
		Run:  runPS6036,
	},
})

type ps6036Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6036Axes = []ps6036Axis{
	{name: "trace schema", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036SchemaField) }},
	{name: "counter set", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036CounterSetField) }},
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036HardwareField) }},
	{name: "benchmark digest", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036DigestField) }},
	{name: "raw-report hashes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036HashesField) }},
	{name: "physical-capture count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036CaptureCountField) }},
	{name: "semantic table-discovery status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036DiscoveryField) }},
	{name: "recursive nested-reference status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ReferencesField) }},
	{name: "ambiguous-table rejection status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036AmbiguousField) }},
	{name: "target process identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ProcessField) }},
	{name: "target command-buffer identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036CommandField) }},
	{name: "exact target-identity status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036IdentityField) }},
	{name: "target GPU span start", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036TargetStartField) }},
	{name: "target GPU span end", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036TargetEndField) }},
	{name: "application stage span", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ApplicationSpanField) }},
	{name: "alignment scale", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ScaleField) }},
	{name: "span disagreement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036DisagreementField) }},
	{name: "span tolerance", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ToleranceField) }},
	{name: "alignment status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036AlignmentField) }},
	{name: "stage labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036StageLabelsField) }},
	{name: "stage interval starts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036StageStartsField) }},
	{name: "stage interval ends", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036StageEndsField) }},
	{name: "stage sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036StageCountsField) }},
	{name: "stage aggregates", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036StageAggregatesField) }},
	{name: "required counter names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036RequiredNamesField) }},
	{name: "required counter sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036RequiredCountsField) }},
	{name: "missing-stage count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036MissingStagesField) }},
	{name: "all-stages-reported status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036AllStagesField) }},
	{name: "sub-resolution stage labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036SubResolutionField) }},
	{name: "sub-resolution aggregation policy", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036AggregationField) }},
	{name: "fabricated-interpolation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036InterpolationField) }},
	{name: "contamination value", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ContaminationValueField) }},
	{name: "contamination ceiling", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ContaminationCeilingField) }},
	{name: "contamination status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ContaminationPassedField) }},
	{name: "fresh-process independence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036IndependenceField) }},
	{name: "report acceptance status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036AcceptedField) }},
	{name: "rejection reasons", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6036ReasonsField) }},
}

type ps6036Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6036(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6036Context(text) {
				continue
			}
			manifest, found := ps6036BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "xctrace stage/counter harness has no target-scoped correlation manifest; missing %s", strings.Join(ps6036Missing(nil), ", "))
				continue
			}
			if missing := ps6036Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "xctrace correlation evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6036Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "xctrace correlation audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6036Context(text string) bool {
	text = ps6007NormalizeName(text)
	xctrace := strings.Contains(text, "xctrace")
	correlation := ps6007ContainsAny(text, "stagecountercorrelation", "targetscopedcounter", "tracejoin", "performanceLimiter")
	gpu := ps6007ContainsAny(text, "gpu", "metal", "commandbuffer")
	return xctrace && correlation && gpu
}

func ps6036BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6036Manifest, bool) {
	var best ps6036Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6036ManifestType(lit.Type) {
			return true
		}
		manifest := ps6036Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6036Axes) - len(ps6036Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6036ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6036ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6036ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "xctracecorrelationevidence", "stagecountercorrelation", "targetscopedcounterevidence", "appletracejoinevidence", "xctracestagejoin")
}

func ps6036Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6036Axes))
	for _, axis := range ps6036Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6036SchemaField(name string) bool {
	return strings.Contains(name, "trace") && strings.Contains(name, "schema")
}
func ps6036CounterSetField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "set")
}
func ps6036HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6036DigestField(name string) bool {
	return strings.Contains(name, "benchmark") && strings.Contains(name, "digest")
}
func ps6036HashesField(name string) bool {
	return strings.Contains(name, "rawreport") && strings.Contains(name, "hash")
}
func ps6036CaptureCountField(name string) bool {
	return strings.Contains(name, "physicalcapture") && strings.Contains(name, "count")
}
func ps6036DiscoveryField(name string) bool {
	return strings.Contains(name, "semantic") && strings.Contains(name, "table") && strings.Contains(name, "discovery")
}
func ps6036ReferencesField(name string) bool {
	return strings.Contains(name, "recursive") && strings.Contains(name, "nested") && ps6007ContainsAny(name, "reference", "ref")
}
func ps6036AmbiguousField(name string) bool {
	return strings.Contains(name, "ambiguous") && strings.Contains(name, "table") && strings.Contains(name, "rejected")
}
func ps6036ProcessField(name string) bool {
	return strings.Contains(name, "target") && strings.Contains(name, "process") && ps6007ContainsAny(name, "id", "identity")
}
func ps6036CommandField(name string) bool {
	return strings.Contains(name, "target") && strings.Contains(name, "commandbuffer") && ps6007ContainsAny(name, "id", "identity")
}
func ps6036IdentityField(name string) bool {
	return strings.Contains(name, "exacttarget") && strings.Contains(name, "identity")
}
func ps6036TargetStartField(name string) bool {
	return strings.Contains(name, "targetgpu") && strings.Contains(name, "span") && strings.Contains(name, "start")
}
func ps6036TargetEndField(name string) bool {
	return strings.Contains(name, "targetgpu") && strings.Contains(name, "span") && strings.Contains(name, "end")
}
func ps6036ApplicationSpanField(name string) bool {
	return strings.Contains(name, "application") && strings.Contains(name, "stage") && strings.Contains(name, "span")
}
func ps6036ScaleField(name string) bool {
	return strings.Contains(name, "alignment") && strings.Contains(name, "scale")
}
func ps6036DisagreementField(name string) bool {
	return strings.Contains(name, "span") && strings.Contains(name, "disagreement")
}
func ps6036ToleranceField(name string) bool {
	return strings.Contains(name, "span") && strings.Contains(name, "tolerance")
}
func ps6036AlignmentField(name string) bool {
	return strings.Contains(name, "alignment") && ps6007ContainsAny(name, "passed", "status", "accepted") && !strings.Contains(name, "scale")
}
func ps6036StageLabelsField(name string) bool {
	return strings.Contains(name, "stage") && ps6007ContainsAny(name, "labels", "names") && !strings.Contains(name, "subresolution")
}
func ps6036StageStartsField(name string) bool {
	return strings.Contains(name, "stageinterval") && strings.Contains(name, "start")
}
func ps6036StageEndsField(name string) bool {
	return strings.Contains(name, "stageinterval") && strings.Contains(name, "end")
}
func ps6036StageCountsField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "sample") && strings.Contains(name, "count")
}
func ps6036StageAggregatesField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "aggregate")
}
func ps6036RequiredNamesField(name string) bool {
	return strings.Contains(name, "requiredcounter") && ps6007ContainsAny(name, "names", "labels")
}
func ps6036RequiredCountsField(name string) bool {
	return strings.Contains(name, "requiredcounter") && strings.Contains(name, "sample") && strings.Contains(name, "count")
}
func ps6036MissingStagesField(name string) bool {
	return strings.Contains(name, "missingstage") && strings.Contains(name, "count")
}
func ps6036AllStagesField(name string) bool {
	return strings.Contains(name, "allstage") && strings.Contains(name, "reported")
}
func ps6036SubResolutionField(name string) bool {
	return strings.Contains(name, "subresolution") && strings.Contains(name, "stage") && ps6007ContainsAny(name, "labels", "names")
}
func ps6036AggregationField(name string) bool {
	return strings.Contains(name, "subresolution") && strings.Contains(name, "aggregation") && strings.Contains(name, "policy")
}
func ps6036InterpolationField(name string) bool {
	return strings.Contains(name, "fabricated") && strings.Contains(name, "interpolation")
}
func ps6036ContaminationValueField(name string) bool {
	return strings.Contains(name, "contamination") && ps6007ContainsAny(name, "value", "observed") && !strings.Contains(name, "ceiling")
}
func ps6036ContaminationCeilingField(name string) bool {
	return strings.Contains(name, "contamination") && strings.Contains(name, "ceiling")
}
func ps6036ContaminationPassedField(name string) bool {
	return strings.Contains(name, "contamination") && ps6007ContainsAny(name, "passed", "status") && !strings.Contains(name, "ceiling")
}
func ps6036IndependenceField(name string) bool {
	return strings.Contains(name, "freshprocess") && strings.Contains(name, "independent")
}
func ps6036AcceptedField(name string) bool {
	return strings.Contains(name, "report") && strings.Contains(name, "accepted")
}
func ps6036ReasonsField(name string) bool {
	return strings.Contains(name, "rejection") && strings.Contains(name, "reason")
}

func ps6036Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"semantic table discovery", ps6036DiscoveryField},
		{"recursive nested-reference resolution", ps6036ReferencesField},
		{"ambiguous-table rejection", ps6036AmbiguousField},
		{"exact target identity", ps6036IdentityField},
		{"span alignment", ps6036AlignmentField},
		{"all-stages reporting", ps6036AllStagesField},
		{"contamination gate", ps6036ContaminationPassedField},
		{"fresh-process independence", ps6036IndependenceField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	start, startOK := ps6016Number(fields, ps6036TargetStartField)
	end, endOK := ps6016Number(fields, ps6036TargetEndField)
	appSpan, appSpanOK := ps6016Number(fields, ps6036ApplicationSpanField)
	scale, scaleOK := ps6016Number(fields, ps6036ScaleField)
	disagreement, disagreementOK := ps6016Number(fields, ps6036DisagreementField)
	tolerance, toleranceOK := ps6016Number(fields, ps6036ToleranceField)
	if startOK && endOK && end <= start || appSpanOK && appSpan <= 0 || scaleOK && scale <= 0 {
		warnings = append(warnings, "target/application spans and affine alignment scale must be positive")
	}
	accepted, acceptedOK := ps6026Bool(fields, ps6036AcceptedField)
	if disagreementOK && toleranceOK && disagreement > tolerance {
		warnings = append(warnings, fmt.Sprintf("span disagreement %.6g ns exceeds declared %.6g ns tolerance", disagreement, tolerance))
		if acceptedOK && accepted {
			warnings = append(warnings, "report is accepted despite alignment drift")
		}
	}
	labels := ps6030Strings(fields, ps6036StageLabelsField)
	starts, _ := ps6016Numbers(fields, ps6036StageStartsField)
	ends, _ := ps6016Numbers(fields, ps6036StageEndsField)
	counts, _ := ps6016Numbers(fields, ps6036StageCountsField)
	aggregates, _ := ps6016Numbers(fields, ps6036StageAggregatesField)
	if len(labels) != len(starts) || len(labels) != len(ends) || len(labels) != len(counts) || len(labels) != len(aggregates) {
		warnings = append(warnings, "stage labels/intervals/sample-counts/aggregates have different lengths")
	} else {
		for i := range labels {
			if ends[i] <= starts[i] || counts[i] < 0 {
				warnings = append(warnings, fmt.Sprintf("stage %q has an invalid interval or negative sample count", labels[i]))
			}
		}
	}
	requiredNames := ps6030Strings(fields, ps6036RequiredNamesField)
	requiredCounts, _ := ps6016Numbers(fields, ps6036RequiredCountsField)
	if len(requiredNames) != len(requiredCounts) {
		warnings = append(warnings, "required counter names and sample counts have different lengths")
	} else if acceptedOK && accepted {
		for i, count := range requiredCounts {
			if count <= 0 {
				warnings = append(warnings, fmt.Sprintf("accepted report has zero samples for required counter %q", requiredNames[i]))
			}
		}
	}
	if missing, ok := ps6016Number(fields, ps6036MissingStagesField); ok && missing > 0 && acceptedOK && accepted {
		warnings = append(warnings, fmt.Sprintf("accepted report is missing %.0f required stages", missing))
	}
	if interpolated, known := ps6026Bool(fields, ps6036InterpolationField); known && interpolated {
		warnings = append(warnings, "sub-resolution stage values use fabricated interpolation")
	}
	if len(ps6030Strings(fields, ps6036SubResolutionField)) > 0 {
		if policy, ok := ps6027String(fields, ps6036AggregationField); !ok || strings.TrimSpace(policy) == "" {
			warnings = append(warnings, "sub-resolution stages lack an explicit logical aggregation policy")
		}
	}
	contamination, contaminationOK := ps6016Number(fields, ps6036ContaminationValueField)
	ceiling, ceilingOK := ps6016Number(fields, ps6036ContaminationCeilingField)
	if contaminationOK && ceilingOK && contamination > ceiling {
		warnings = append(warnings, fmt.Sprintf("contamination %.6g exceeds %.6g ceiling", contamination, ceiling))
		if acceptedOK && accepted {
			warnings = append(warnings, "report is accepted despite contamination")
		}
	}
	hashes := ps6030Strings(fields, ps6036HashesField)
	if captures, ok := ps6016Number(fields, ps6036CaptureCountField); ok && captures != float64(len(hashes)) {
		warnings = append(warnings, fmt.Sprintf("physical-capture count %.0f disagrees with %d raw-report hashes", captures, len(hashes)))
	}
	reasons := ps6030Strings(fields, ps6036ReasonsField)
	if acceptedOK && accepted && len(reasons) > 0 {
		warnings = append(warnings, "accepted report retains rejection reasons")
	}
	if acceptedOK && !accepted && len(reasons) == 0 {
		warnings = append(warnings, "rejected report has no rejection reason")
	}
	return warnings
}
