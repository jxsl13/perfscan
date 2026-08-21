package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6031 implements owner issue #747: counter completeness gates must follow
// a predeclared semantic policy class instead of treating every exported
// stream as uniformly required.
var PS6031 = register(&lint.Check{
	ID:       "PS6031",
	Category: "verify",
	Slug:     "profiler-counter-completeness-needs-policy-classes",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a profiler completeness gate does not classify counter semantics",
		Text: `Multiplexed profilers may omit one irrelevant, unsupported, or
under-resolved stream while command identity, exact output, topology, and
claim-bearing counters remain valid. Requiring at least one sample from every
exported row makes acceptance depend on unrelated sampling luck.

This check implements owner issue #747. It audits CounterPolicyEvidence,
ProfilerCounterClassification, CounterCompletenessPolicy,
SemanticCounterGate, or equivalent manifests. Every named counter must have:

  - one policy class: required-decision, contamination-sentinel, capability,
    sparse-stage, or optional-diagnostic;
  - sample count, availability status, observed value, and contamination
    ceiling;
  - expected samples, active stage duration, and declared resolution floors;
  - an explicit per-counter gate status and rejection reason.

Required decision counters fail closed when unsampled. Contamination sentinels
fail closed when absent or above their ceiling. Capability counters record
unsupported/unavailable status without rejecting the capture. Sparse-stage
counters require coverage only when both expected samples and active duration
reach their predeclared resolution floors; otherwise they remain explicit
below-resolution diagnostics. Optional diagnostics preserve missingness
without rejecting the capture.

The campaign must also record hardware/workload/graph/output identity,
predeclared required/attempted/accepted/rejected attempt counts, per-attempt
rejection reasons, disclosure of every attempt, a prohibition on retry
selection, and median/candidate publication status. Constant evidence is
checked for vector alignment, class/status/reason contradictions, attempt
accounting, and publication before the accepted-run target.

There is NO automatic fix. Counter semantics, exporter capability, sampling
resolution, contamination ceilings, and independent attempts are runtime
evidence that a source rewrite cannot infer.`,
		Before: `for _, counter := range exportedCounters {
	if counter.Samples == 0 {
		rejectCapture() // treats optional/capability/sparse exactly like required
	}
}`,
		After: `evidence := CounterPolicyEvidence{
	CounterNames: []string{"decision", "occupancy", "capability", "short-stage", "diagnostic"},
	CounterPolicyClasses: []string{
		"required-decision", "contamination-sentinel", "capability",
		"sparse-stage", "optional-diagnostic",
	},
	// class-specific availability, resolution, status, and reason vectors...
	AllAttemptsDisclosed: true,
	RetrySelectionForbidden: true,
}`,
		MeasuredWin: `In the five Apple-M2 attempts behind issue #747, four
otherwise exact captures were rejected for four different sparse streams;
only one report was accepted. The accepted report aligned all 296 events and
had 1,354 samples for every global counter. Semantic classes retain fail-
closed behavior where it proves a claim while avoiding an 80% rejection rate
caused by irrelevant, unsupported, or under-resolved streams.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6031",
		Doc:  "profiler completeness treats counters uniformly instead of using semantic policy classes",
		Run:  runPS6031,
	},
})

type ps6031Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6031Axes = []ps6031Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031HardwareField) }},
	{name: "workload digest", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031WorkloadField) }},
	{name: "graph identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031GraphField) }},
	{name: "exact output/digest status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031ExactOutputField) }},
	{name: "exact event-topology status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031ExactTopologyField) }},
	{name: "counter names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031NamesField) }},
	{name: "counter policy classes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031ClassesField) }},
	{name: "counter sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031SamplesField) }},
	{name: "counter availability statuses", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031AvailabilityField) }},
	{name: "counter observed values", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031ObservedField) }},
	{name: "counter contamination ceilings", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031CeilingsField) }},
	{name: "counter expected samples", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031ExpectedField) }},
	{name: "counter active stage durations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031DurationsField) }},
	{name: "counter minimum expected samples", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031MinimumSamplesField) }},
	{name: "counter minimum active durations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031MinimumDurationsField) }},
	{name: "counter gate statuses", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031StatusesField) }},
	{name: "counter rejection reasons", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031CounterReasonsField) }},
	{name: "required accepted-run count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031RequiredRunsField) }},
	{name: "attempted-run count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031AttemptedRunsField) }},
	{name: "accepted-run count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031AcceptedRunsField) }},
	{name: "rejected-run count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031RejectedRunsField) }},
	{name: "attempt rejection reasons", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031AttemptReasonsField) }},
	{name: "all-attempt disclosure", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031DisclosureField) }},
	{name: "retry-selection prohibition", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031RetryField) }},
	{name: "median publication status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031PublishedField) }},
	{name: "candidate selection status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6031SelectedField) }},
}

type ps6031Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6031(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6031Context(text) {
				continue
			}
			manifest, found := ps6031BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "profiler counter-completeness harness has no semantic policy manifest; missing %s", strings.Join(ps6031Missing(nil), ", "))
				continue
			}
			if missing := ps6031Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "profiler counter-policy evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6031Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "profiler counter-policy audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6031Context(text string) bool {
	text = ps6007NormalizeName(text)
	profiler := ps6007ContainsAny(text, "profiler", "profile", "gpu", "metal")
	counter := strings.Contains(text, "counter")
	policy := ps6007ContainsAny(text, "classification", "policyclass", "semanticpolicy", "completenesspolicy")
	return profiler && counter && policy
}

func ps6031BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6031Manifest, bool) {
	var best ps6031Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6031ManifestType(lit.Type) {
			return true
		}
		manifest := ps6031Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6031Axes) - len(ps6031Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6031ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6031ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6031ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "counterpolicyevidence", "profilercounterclassification", "countercompletenesspolicy", "semanticcountergate", "counterclasscampaign")
}

func ps6031Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6031Axes))
	for _, axis := range ps6031Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6031HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6031WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && strings.Contains(name, "digest")
}
func ps6031GraphField(name string) bool {
	return strings.Contains(name, "graph") && ps6007ContainsAny(name, "identity", "digest", "id")
}
func ps6031ExactOutputField(name string) bool {
	return ps6007ContainsAny(name, "exactoutput", "outputdigest", "logitsdigest") && ps6007ContainsAny(name, "passed", "matched", "exact", "status")
}
func ps6031ExactTopologyField(name string) bool {
	return strings.Contains(name, "eventtopology") && ps6007ContainsAny(name, "exact", "passed", "matched", "status")
}
func ps6031NamesField(name string) bool {
	return strings.Contains(name, "counter") && ps6007ContainsAny(name, "names", "identities", "labels")
}
func ps6031ClassesField(name string) bool {
	return strings.Contains(name, "counter") && ps6007ContainsAny(name, "policyclasses", "semanticclasses", "classifications")
}
func ps6031SamplesField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "sample") && strings.Contains(name, "count") && !ps6007ContainsAny(name, "expected", "minimum")
}
func ps6031AvailabilityField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "availability")
}
func ps6031ObservedField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "observed") && ps6007ContainsAny(name, "values", "measurements")
}
func ps6031CeilingsField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "contamination") && strings.Contains(name, "ceiling")
}
func ps6031ExpectedField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "expected") && strings.Contains(name, "sample") && !strings.Contains(name, "minimum")
}
func ps6031DurationsField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "active") && ps6007ContainsAny(name, "duration", "microsecond") && !strings.Contains(name, "minimum")
}
func ps6031MinimumSamplesField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "minimum") && strings.Contains(name, "expected") && strings.Contains(name, "sample")
}
func ps6031MinimumDurationsField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "minimum") && strings.Contains(name, "active") && ps6007ContainsAny(name, "duration", "microsecond")
}
func ps6031StatusesField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "gate") && strings.Contains(name, "status")
}
func ps6031CounterReasonsField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "rejection") && strings.Contains(name, "reason")
}
func ps6031RequiredRunsField(name string) bool {
	return strings.Contains(name, "required") && strings.Contains(name, "accepted") && strings.Contains(name, "run")
}
func ps6031AttemptedRunsField(name string) bool {
	return strings.Contains(name, "attempted") && strings.Contains(name, "run")
}
func ps6031AcceptedRunsField(name string) bool {
	return strings.Contains(name, "accepted") && strings.Contains(name, "run") && !strings.Contains(name, "required")
}
func ps6031RejectedRunsField(name string) bool {
	return strings.Contains(name, "rejected") && strings.Contains(name, "run")
}
func ps6031AttemptReasonsField(name string) bool {
	return strings.Contains(name, "attempt") && strings.Contains(name, "rejection") && strings.Contains(name, "reason")
}
func ps6031DisclosureField(name string) bool {
	return strings.Contains(name, "allattempt") && strings.Contains(name, "disclosed")
}
func ps6031RetryField(name string) bool {
	return strings.Contains(name, "retry") && strings.Contains(name, "selection") && ps6007ContainsAny(name, "forbidden", "prohibited", "disabled")
}
func ps6031PublishedField(name string) bool {
	return strings.Contains(name, "median") && ps6007ContainsAny(name, "published", "publication")
}
func ps6031SelectedField(name string) bool {
	return strings.Contains(name, "candidate") && ps6007ContainsAny(name, "selected", "selection")
}

func ps6031Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"all-attempt disclosure", ps6031DisclosureField},
		{"retry-selection prohibition", ps6031RetryField},
		{"exact output/digest", ps6031ExactOutputField},
		{"exact event topology", ps6031ExactTopologyField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	vectors := ps6031VectorsFrom(fields)
	if !vectors.aligned() {
		warnings = append(warnings, "counter names/classes/samples/availability/values/ceilings/resolution/status/reason vectors have different lengths")
	} else {
		for i := range vectors.names {
			warnings = append(warnings, vectors.audit(i)...)
		}
	}
	required, requiredOK := ps6016Number(fields, ps6031RequiredRunsField)
	attempted, attemptedOK := ps6016Number(fields, ps6031AttemptedRunsField)
	accepted, acceptedOK := ps6016Number(fields, ps6031AcceptedRunsField)
	rejected, rejectedOK := ps6016Number(fields, ps6031RejectedRunsField)
	if attemptedOK && acceptedOK && rejectedOK && accepted+rejected != attempted {
		warnings = append(warnings, fmt.Sprintf("attempt accounting disagrees (accepted %.0f + rejected %.0f != attempted %.0f)", accepted, rejected, attempted))
	}
	if rejectedOK {
		reasons := ps6030Strings(fields, ps6031AttemptReasonsField)
		if rejected != float64(len(reasons)) {
			warnings = append(warnings, fmt.Sprintf("rejected-run count %.0f disagrees with %d disclosed attempt reasons", rejected, len(reasons)))
		}
	}
	if requiredOK && acceptedOK && accepted < required {
		if published, known := ps6026Bool(fields, ps6031PublishedField); known && published {
			warnings = append(warnings, "medians are published before the predeclared accepted-run count")
		}
		if selected, known := ps6026Bool(fields, ps6031SelectedField); known && selected {
			warnings = append(warnings, "candidate is selected before the predeclared accepted-run count")
		}
	}
	return warnings
}

type ps6031Vectors struct {
	names, classes, availability, statuses, reasons  []string
	samples, observed, ceilings                      []float64
	expected, active, minimumExpected, minimumActive []float64
}

func ps6031VectorsFrom(fields map[string]ps6016Field) ps6031Vectors {
	samples, _ := ps6016Numbers(fields, ps6031SamplesField)
	observed, _ := ps6016Numbers(fields, ps6031ObservedField)
	ceilings, _ := ps6016Numbers(fields, ps6031CeilingsField)
	expected, _ := ps6016Numbers(fields, ps6031ExpectedField)
	active, _ := ps6016Numbers(fields, ps6031DurationsField)
	minimumExpected, _ := ps6016Numbers(fields, ps6031MinimumSamplesField)
	minimumActive, _ := ps6016Numbers(fields, ps6031MinimumDurationsField)
	return ps6031Vectors{
		names: ps6030Strings(fields, ps6031NamesField), classes: ps6030Strings(fields, ps6031ClassesField),
		availability: ps6030Strings(fields, ps6031AvailabilityField), statuses: ps6030Strings(fields, ps6031StatusesField),
		reasons: ps6030Strings(fields, ps6031CounterReasonsField), samples: samples, observed: observed, ceilings: ceilings,
		expected: expected, active: active, minimumExpected: minimumExpected, minimumActive: minimumActive,
	}
}

func (v *ps6031Vectors) aligned() bool {
	length := len(v.names)
	return length == len(v.classes) && length == len(v.availability) && length == len(v.statuses) && length == len(v.reasons) &&
		length == len(v.samples) && length == len(v.observed) && length == len(v.ceilings) && length == len(v.expected) &&
		length == len(v.active) && length == len(v.minimumExpected) && length == len(v.minimumActive)
}

func (v *ps6031Vectors) audit(i int) []string {
	name := v.names[i]
	class := ps6030StatusName(v.classes[i])
	availability := ps6030StatusName(v.availability[i])
	status := ps6030StatusName(v.statuses[i])
	reasonMissing := strings.TrimSpace(v.reasons[i]) == ""
	rejected := strings.Contains(status, "reject")
	var warnings []string
	switch class {
	case "requireddecision", "required":
		if v.samples[i] <= 0 && !rejected {
			warnings = append(warnings, fmt.Sprintf("required-decision counter %q is unsampled but not rejected", name))
		}
		if v.samples[i] <= 0 && reasonMissing {
			warnings = append(warnings, fmt.Sprintf("required-decision counter %q has no rejection reason", name))
		}
	case "contaminationsentinel", "contamination":
		contaminated := v.samples[i] <= 0 || v.observed[i] > v.ceilings[i]
		if contaminated && !rejected {
			warnings = append(warnings, fmt.Sprintf("contamination-sentinel counter %q is missing/above-ceiling but not rejected", name))
		}
		if contaminated && reasonMissing {
			warnings = append(warnings, fmt.Sprintf("contamination-sentinel counter %q has no rejection reason", name))
		}
	case "capability":
		if v.samples[i] <= 0 && !ps6007ContainsAny(availability, "unsupported", "unavailable", "zeroavailability") {
			warnings = append(warnings, fmt.Sprintf("capability counter %q is unsampled without unsupported/unavailable status", name))
		}
		if rejected {
			warnings = append(warnings, fmt.Sprintf("capability counter %q rejects the whole capture", name))
		}
	case "sparsestage", "sparse":
		resolved := v.expected[i] >= v.minimumExpected[i] && v.active[i] >= v.minimumActive[i]
		if resolved && v.samples[i] <= 0 && !rejected {
			warnings = append(warnings, fmt.Sprintf("resolved sparse-stage counter %q is unsampled but not rejected", name))
		}
		if !resolved && !ps6007ContainsAny(status, "belowresolution", "insufficientresolution", "diagnostic") {
			warnings = append(warnings, fmt.Sprintf("under-resolved sparse-stage counter %q lacks below-resolution diagnostic status", name))
		}
		if !resolved && rejected {
			warnings = append(warnings, fmt.Sprintf("under-resolved sparse-stage counter %q rejects the whole capture", name))
		}
	case "optionaldiagnostic", "optional":
		if rejected {
			warnings = append(warnings, fmt.Sprintf("optional-diagnostic counter %q rejects the whole capture", name))
		}
	default:
		warnings = append(warnings, fmt.Sprintf("counter %q has unknown policy class %q", name, v.classes[i]))
	}
	return warnings
}
