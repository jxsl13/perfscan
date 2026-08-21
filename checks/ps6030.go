package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6030 implements owner issue #748: nonempty profiler counter streams are
// not claim-grade unless their effective sample density satisfies a
// predeclared floor at every required scope.
var PS6030 = register(&lint.Check{
	ID:       "PS6030",
	Category: "verify",
	Slug:     "profiler-claims-need-sample-density-gates",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a profiler stage claim uses nonempty samples without an effective-density gate",
		Text: `A hardware-counter stream can be nonempty yet far too sparse
for stable stage attribution. Binary samples-greater-than-zero completeness
therefore does not make two captures equally decision-grade.

This check implements owner issue #748. It audits SampleDensityEvidence,
ProfilerDensityGate, StageSamplingDensity, CounterDensityCampaign, or
equivalent manifests. Claim-bearing evidence must record:

  - hardware, workload digest, graph identity, exact output, and exact event
    topology;
  - effective sample counts, active microseconds, and derived samples per
    active microsecond globally and for every named stage;
  - predeclared global and per-stage minimum count/density floors;
  - an explicit density status for every stage;
  - accepted process IDs and their sampling densities;
  - required and actual accepted-process counts;
  - independent-process, identity, contamination, completeness, density, and
    aggregate-only-fully-gated-process statuses;
  - retention of low-density streams as insufficient-density diagnostics;
  - sampling-cadence variance and its disclosure; and
  - median-publication and candidate-selection status.

Constant vectors are checked for equal lengths, valid durations, recomputed
density agreement, threshold/status consistency, accepted-process accounting,
and cadence-variance agreement. Medians and candidates are rejected whenever
any required global/stage scope is below its floor or the predeclared number of
accepted independent processes has not been reached.

There is NO automatic fix. Effective sampling cadence, active stage duration,
process independence, contamination, and exact trace identity are runtime
evidence. Low-density streams should remain visible as diagnostics, never be
silently upgraded to claim-bearing measurements.`,
		Before: `if stage.SampleCount > 0 {
	publish(stage.CounterMean) // technically populated, possibly unstable
}`,
		After: `evidence := SampleDensityEvidence{
	StageNames: []string{"residual+rmsnorm", "q6_k"},
	StageEffectiveSampleCounts: []float64{22, 4},
	StageActiveMicroseconds: []float64{100, 100},
	StageSamplesPerActiveMicrosecond: []float64{0.22, 0.04},
	StageMinimumSampleCounts: []float64{10, 10},
	StageMinimumDensities: []float64{0.10, 0.10},
	StageDensityStatuses: []string{"sufficient", "insufficient-density"},
	LowDensityRetainedDiagnostic: true,
	DensityGatePassed: false,
}`,
		MeasuredWin: `The Apple-M2 traces behind issue #748 retained the same
296-event graph and exact logits digest, yet required global samples varied
from 1,384 to 230. Residual-plus-RMSNorm resolved to 22 versus 4 samples and
Q6_K to 66 versus 8. Effective-density gates prevent the sparse but nonempty
capture from contributing decision-grade stage medians.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6030",
		Doc:  "claim-bearing profiler stages lack effective sample-density floors",
		Run:  runPS6030,
	},
})

type ps6030Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6030Axes = []ps6030Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030HardwareField) }},
	{name: "workload digest", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030WorkloadField) }},
	{name: "graph identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030GraphField) }},
	{name: "global effective sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030GlobalCountsField) }},
	{name: "global active durations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030GlobalDurationsField) }},
	{name: "global sample densities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030GlobalDensitiesField) }},
	{name: "global minimum sample count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030GlobalMinimumCountField) }},
	{name: "global minimum density", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030GlobalMinimumDensityField) }},
	{name: "stage names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageNamesField) }},
	{name: "stage effective sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageCountsField) }},
	{name: "stage active durations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageDurationsField) }},
	{name: "stage sample densities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageDensitiesField) }},
	{name: "stage minimum sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageMinimumCountsField) }},
	{name: "stage minimum densities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageMinimumDensitiesField) }},
	{name: "stage density statuses", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030StageStatusesField) }},
	{name: "accepted process identities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030ProcessIDsField) }},
	{name: "accepted process densities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030ProcessDensitiesField) }},
	{name: "required accepted-process count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030RequiredProcessesField) }},
	{name: "accepted-process count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030AcceptedProcessesField) }},
	{name: "independent-process status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030IndependentField) }},
	{name: "identity gate status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030IdentityGateField) }},
	{name: "contamination gate status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030ContaminationGateField) }},
	{name: "completeness gate status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030CompletenessGateField) }},
	{name: "density gate status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030DensityGateField) }},
	{name: "fully-gated aggregation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030AggregationField) }},
	{name: "low-density diagnostic retention", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030LowDensityRetentionField) }},
	{name: "sampling-cadence variance", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030VarianceField) }},
	{name: "sampling-cadence variance disclosure", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030VarianceDisclosureField) }},
	{name: "exact output/digest status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030ExactOutputField) }},
	{name: "exact event-topology status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030ExactTopologyField) }},
	{name: "median publication status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030PublishedField) }},
	{name: "candidate selection status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6030SelectedField) }},
}

type ps6030Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6030(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6030Context(text) {
				continue
			}
			manifest, found := ps6030BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "profiler sample-density harness has no density-gate manifest; missing %s", strings.Join(ps6030Missing(nil), ", "))
				continue
			}
			if missing := ps6030Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "profiler sample-density evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6030Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "profiler sample-density audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6030Context(text string) bool {
	text = ps6007NormalizeName(text)
	profiler := ps6007ContainsAny(text, "profiler", "profile", "counter", "gpu", "metal")
	density := ps6007ContainsAny(text, "sampledensity", "samplingdensity", "samplesperactive", "densitygate")
	stage := ps6007ContainsAny(text, "stage", "scope", "global")
	return profiler && density && stage
}

func ps6030BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6030Manifest, bool) {
	var best ps6030Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6030ManifestType(lit.Type) {
			return true
		}
		manifest := ps6030Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6030Axes) - len(ps6030Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6030ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6030ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6030ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "sampledensityevidence", "profilerdensitygate", "stagesamplingdensity", "counterdensitycampaign", "effectivedensitygate")
}

func ps6030Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6030Axes))
	for _, axis := range ps6030Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6030HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6030WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && strings.Contains(name, "digest")
}
func ps6030GraphField(name string) bool {
	return strings.Contains(name, "graph") && ps6007ContainsAny(name, "identity", "digest", "id")
}
func ps6030GlobalCountsField(name string) bool {
	return strings.Contains(name, "global") && strings.Contains(name, "effective") && strings.Contains(name, "sample") && strings.Contains(name, "count") && !strings.Contains(name, "minimum")
}
func ps6030GlobalDurationsField(name string) bool {
	return strings.Contains(name, "global") && strings.Contains(name, "active") &&
		ps6007ContainsAny(name, "microsecond", "duration", "us") &&
		!ps6007ContainsAny(name, "samplesperactive", "sampledensity")
}
func ps6030GlobalDensitiesField(name string) bool {
	return strings.Contains(name, "global") && ps6007ContainsAny(name, "samplesperactive", "sampledensity") && !strings.Contains(name, "minimum")
}
func ps6030GlobalMinimumCountField(name string) bool {
	return strings.Contains(name, "global") && strings.Contains(name, "minimum") && strings.Contains(name, "sample") && strings.Contains(name, "count")
}
func ps6030GlobalMinimumDensityField(name string) bool {
	return strings.Contains(name, "global") && strings.Contains(name, "minimum") && strings.Contains(name, "density")
}
func ps6030StageNamesField(name string) bool {
	return strings.Contains(name, "stage") && ps6007ContainsAny(name, "names", "identities", "labels")
}
func ps6030StageCountsField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "effective") && strings.Contains(name, "sample") && strings.Contains(name, "count") && !strings.Contains(name, "minimum")
}
func ps6030StageDurationsField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "active") &&
		ps6007ContainsAny(name, "microsecond", "duration", "us") &&
		!ps6007ContainsAny(name, "samplesperactive", "sampledensity")
}
func ps6030StageDensitiesField(name string) bool {
	return strings.Contains(name, "stage") && ps6007ContainsAny(name, "samplesperactive", "sampledensity") && !strings.Contains(name, "minimum")
}
func ps6030StageMinimumCountsField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "minimum") && strings.Contains(name, "sample") && strings.Contains(name, "count")
}
func ps6030StageMinimumDensitiesField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "minimum") && strings.Contains(name, "densit")
}
func ps6030StageStatusesField(name string) bool {
	return strings.Contains(name, "stage") && strings.Contains(name, "density") && ps6007ContainsAny(name, "statuses", "status", "classifications")
}
func ps6030ProcessIDsField(name string) bool {
	return strings.Contains(name, "acceptedprocess") && ps6007ContainsAny(name, "ids", "identities")
}
func ps6030ProcessDensitiesField(name string) bool {
	return strings.Contains(name, "acceptedprocess") && strings.Contains(name, "densit")
}
func ps6030RequiredProcessesField(name string) bool {
	return strings.Contains(name, "required") && strings.Contains(name, "acceptedprocess") && ps6007ContainsAny(name, "count", "processes")
}
func ps6030AcceptedProcessesField(name string) bool {
	return strings.Contains(name, "acceptedprocess") && strings.Contains(name, "count") && !strings.Contains(name, "required")
}
func ps6030IndependentField(name string) bool {
	return strings.Contains(name, "process") && strings.Contains(name, "independent")
}
func ps6030IdentityGateField(name string) bool {
	return strings.Contains(name, "identity") && strings.Contains(name, "gate")
}
func ps6030ContaminationGateField(name string) bool {
	return strings.Contains(name, "contamination") && strings.Contains(name, "gate")
}
func ps6030CompletenessGateField(name string) bool {
	return strings.Contains(name, "completeness") && strings.Contains(name, "gate")
}
func ps6030DensityGateField(name string) bool {
	return strings.Contains(name, "density") && strings.Contains(name, "gate") && !strings.Contains(name, "aggregate")
}
func ps6030AggregationField(name string) bool {
	return strings.Contains(name, "aggregate") && strings.Contains(name, "fullygated")
}
func ps6030LowDensityRetentionField(name string) bool {
	return strings.Contains(name, "lowdensity") && ps6007ContainsAny(name, "retaineddiagnostic", "diagnosticretained", "retained")
}
func ps6030VarianceField(name string) bool {
	return strings.Contains(name, "samplingcadence") && strings.Contains(name, "variance") && !strings.Contains(name, "disclosed")
}
func ps6030VarianceDisclosureField(name string) bool {
	return strings.Contains(name, "samplingcadence") && strings.Contains(name, "variance") && strings.Contains(name, "disclosed")
}
func ps6030ExactOutputField(name string) bool {
	return ps6007ContainsAny(name, "exactoutput", "outputdigest", "logitsdigest") && ps6007ContainsAny(name, "passed", "matched", "exact", "status")
}
func ps6030ExactTopologyField(name string) bool {
	return strings.Contains(name, "eventtopology") && ps6007ContainsAny(name, "exact", "passed", "matched", "status")
}
func ps6030PublishedField(name string) bool {
	return strings.Contains(name, "median") && ps6007ContainsAny(name, "published", "publication")
}
func ps6030SelectedField(name string) bool {
	return strings.Contains(name, "candidate") && ps6007ContainsAny(name, "selected", "selection")
}

func ps6030Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"independent-process gate", ps6030IndependentField},
		{"identity gate", ps6030IdentityGateField},
		{"contamination gate", ps6030ContaminationGateField},
		{"completeness gate", ps6030CompletenessGateField},
		{"fully-gated process aggregation", ps6030AggregationField},
		{"sampling-cadence variance disclosure", ps6030VarianceDisclosureField},
		{"exact output/digest", ps6030ExactOutputField},
		{"exact event topology", ps6030ExactTopologyField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	globalCounts, _ := ps6016Numbers(fields, ps6030GlobalCountsField)
	globalDurations, _ := ps6016Numbers(fields, ps6030GlobalDurationsField)
	globalDensities, _ := ps6016Numbers(fields, ps6030GlobalDensitiesField)
	stageNames := ps6030Strings(fields, ps6030StageNamesField)
	stageCounts, _ := ps6016Numbers(fields, ps6030StageCountsField)
	stageDurations, _ := ps6016Numbers(fields, ps6030StageDurationsField)
	stageDensities, _ := ps6016Numbers(fields, ps6030StageDensitiesField)
	stageMinimumCounts, _ := ps6016Numbers(fields, ps6030StageMinimumCountsField)
	stageMinimumDensities, _ := ps6016Numbers(fields, ps6030StageMinimumDensitiesField)
	stageStatuses := ps6030Strings(fields, ps6030StageStatusesField)

	globalLengthsOK := ps6030SameLength(globalCounts, globalDurations, globalDensities)
	if !globalLengthsOK {
		warnings = append(warnings, "global count/duration/density vectors have different lengths")
	}
	stageLengthsOK := ps6030SameLength(stageCounts, stageDurations, stageDensities, stageMinimumCounts, stageMinimumDensities) && len(stageNames) == len(stageCounts) && len(stageStatuses) == len(stageCounts)
	if !stageLengthsOK {
		warnings = append(warnings, "stage names/counts/durations/densities/floors/statuses have different lengths")
	}
	globalLow := false
	if globalLengthsOK {
		globalMinimumCount, countOK := ps6016Number(fields, ps6030GlobalMinimumCountField)
		globalMinimumDensity, densityOK := ps6016Number(fields, ps6030GlobalMinimumDensityField)
		for i := range globalCounts {
			if warning := ps6030DensityMismatch("global", i, globalCounts[i], globalDurations[i], globalDensities[i]); warning != "" {
				warnings = append(warnings, warning)
			}
			globalLow = globalLow || countOK && globalCounts[i] < globalMinimumCount || densityOK && globalDensities[i] < globalMinimumDensity
		}
	}
	stageLow := false
	if stageLengthsOK {
		for i := range stageCounts {
			if warning := ps6030DensityMismatch("stage "+stageNames[i], i, stageCounts[i], stageDurations[i], stageDensities[i]); warning != "" {
				warnings = append(warnings, warning)
			}
			low := stageCounts[i] < stageMinimumCounts[i] || stageDensities[i] < stageMinimumDensities[i]
			insufficient := strings.Contains(ps6030StatusName(stageStatuses[i]), "insufficientdensity")
			if low && !insufficient {
				warnings = append(warnings, fmt.Sprintf("stage %q is below its count/density floor but status is %q", stageNames[i], stageStatuses[i]))
			}
			if !low && insufficient {
				warnings = append(warnings, fmt.Sprintf("stage %q meets its count/density floor but status is insufficient-density", stageNames[i]))
			}
			stageLow = stageLow || low
		}
	}
	anyLow := globalLow || stageLow
	if densityPassed, known := ps6026Bool(fields, ps6030DensityGateField); known && densityPassed == anyLow {
		if anyLow {
			warnings = append(warnings, "density gate passes despite a required global/stage scope below its floor")
		} else {
			warnings = append(warnings, "density gate fails although every required global/stage scope meets its floor")
		}
	}
	if retained, known := ps6026Bool(fields, ps6030LowDensityRetentionField); anyLow && known && !retained {
		warnings = append(warnings, "low-density stream is not retained as an insufficient-density diagnostic")
	}
	processIDs := ps6030Strings(fields, ps6030ProcessIDsField)
	processDensities, densitiesOK := ps6016Numbers(fields, ps6030ProcessDensitiesField)
	accepted, acceptedOK := ps6016Number(fields, ps6030AcceptedProcessesField)
	required, requiredOK := ps6016Number(fields, ps6030RequiredProcessesField)
	if acceptedOK && (accepted != float64(len(processIDs)) || densitiesOK && accepted != float64(len(processDensities))) {
		warnings = append(warnings, fmt.Sprintf("accepted-process count %.0f disagrees with process identities/densities", accepted))
	}
	variance, varianceOK := ps6016Number(fields, ps6030VarianceField)
	if densitiesOK && len(processDensities) > 0 {
		minimum, maximum := slices.Min(processDensities), slices.Max(processDensities)
		if minimum <= 0 {
			warnings = append(warnings, "accepted-process sample densities must be positive")
		} else if varianceOK && !ps6025Close(variance, maximum/minimum) {
			warnings = append(warnings, fmt.Sprintf("sampling-cadence variance %.6gx disagrees with max/min density ratio %.6gx", variance, maximum/minimum))
		}
	}
	underfilled := requiredOK && acceptedOK && accepted < required
	if anyLow || underfilled {
		if published, known := ps6026Bool(fields, ps6030PublishedField); known && published {
			warnings = append(warnings, "medians are published before every density floor and accepted-process requirement passes")
		}
		if selected, known := ps6026Bool(fields, ps6030SelectedField); known && selected {
			warnings = append(warnings, "candidate is selected before every density floor and accepted-process requirement passes")
		}
	}
	return warnings
}

func ps6030SameLength(vectors ...[]float64) bool {
	if len(vectors) == 0 {
		return true
	}
	length := len(vectors[0])
	for _, vector := range vectors[1:] {
		if len(vector) != length {
			return false
		}
	}
	return true
}

func ps6030DensityMismatch(scope string, index int, count, duration, density float64) string {
	if duration <= 0 {
		return fmt.Sprintf("%s[%d] active duration must be positive", scope, index)
	}
	calculated := count / duration
	if !ps6025Close(density, calculated) {
		return fmt.Sprintf("%s[%d] density %.6g disagrees with count/active-us %.6g", scope, index, density, calculated)
	}
	return ""
}

func ps6030Strings(fields map[string]ps6016Field, predicate func(string) bool) []string {
	for name, field := range fields {
		if predicate(name) && field.hasStringValues {
			return field.stringValues
		}
	}
	return nil
}

func ps6030StatusName(status string) string {
	return ps6029ModeReplacer.Replace(strings.ToLower(status))
}
