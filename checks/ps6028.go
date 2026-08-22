package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6028 implements owner issue #750: timestamped GPU intervals provide an
// exact timing-contamination gate even when multiplexed hardware counters are
// absent.
var PS6028 = register(&lint.Check{
	ID:       "PS6028",
	Category: "verify",
	Slug:     "gpu-window-needs-exact-foreign-overlap-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU exclusivity claim lacks exact foreign-interval overlap evidence",
		Text: `Multiplexed hardware-counter streams can randomly omit a
contamination witness even when timestamped GPU intervals, command identity,
and output identity are exact. Rejecting every timing report with one absent
diagnostic counter makes acceptance depend on exporter sampling luck. Treating
that missing witness as zero is unsafe.

This check implements owner issue #750. It audits GPUExclusivityEvidence,
ForeignIntervalOverlapEvidence, ExactGPUOverlapGate, GPUWindowContamination,
or equivalent manifests. A deterministic timing-exclusivity gate must record:

  - hardware, workload digest, graph identity, and evidence-schema version;
  - target process and command-buffer identity plus the target's full GPU span;
  - the number of inspected intervals on the same GPU;
  - expected graph events and matched GPU intervals;
  - foreign positive-overlap interval count, union overlap duration without
    double counting, and sorted unique foreign-process identities;
  - explicit all-same-GPU inspection, exact target-identity exclusion,
    positive-duration intersection, boundary-touch exclusion, and interval-
    union status;
  - whether an exclusive GPU window is required and whether that gate passed;
  - exact output/digest and serialized-evidence status; and
  - whether the claim needs counter semantics, whether the sampled-counter
    gate remains available/passed, and whether missing counters were imputed.

Constant evidence is rejected when the target span is invalid, counts or
union duration disagree, process identities are unsorted or duplicated,
event topology differs, target exclusion/intersection/union statuses fail,
positive foreign overlap is accepted as exclusive, a clean required window is
rejected, a counter-semantic claim lacks its counter gate, or any missing
counter is imputed as zero. Boundary contact with zero positive duration is a
clean timing window.

There is NO automatic fix. Interval identity, timestamp domains, command-
buffer ownership, device filtering, output parity, and counter semantics are
runtime evidence that source rewriting cannot invent. Sampled-counter gates
remain necessary for claims specifically about counter behavior; exact
foreign-interval overlap independently protects timing exclusivity.`,
		Before: `if fragmentOccupancySamples == 0 {
	rejectTimingReport() // exporter sparsity, not proven contamination
}`,
		After: `evidence := GPUExclusivityEvidence{
	TargetProcessID: processID, TargetCommandBufferID: commandBufferID,
	TargetGPUSpanStartNS: start, TargetGPUSpanEndNS: end,
	EverySameGPUIntervalInspected: true,
	ExactTargetProcessCommandExcluded: true,
	PositiveDurationIntersectionOnly: true,
	BoundaryTouchExcluded: true,
	ForeignUnionOverlapNS: unionPositiveOverlap(foreignIntervals),
	SortedForeignProcessIDs: sortedUniqueProcessIDs,
	RequireExclusiveGPUWindow: true,
	ExclusiveGPUWindowPassed: unionOverlapNS == 0,
}`,
		MeasuredWin: `In the Apple-M2 campaign behind issue #750, five
independent traces of the exact TinyLlama Q4_K_M graph all retained the exact
logits digest and 296 matching GPU intervals. Every trace had zero foreign
same-GPU overlap even though hardware-counter sample counts varied across
processes. The exact interval gate therefore established a deterministic
timing-contamination boundary without imputing absent counters as zero.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6028",
		Doc:  "GPU timing exclusivity lacks exact foreign-interval overlap evidence",
		Run:  runPS6028,
	},
})

type ps6028Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6028Axes = []ps6028Axis{
	{name: "hardware/device identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028HardwareField) }},
	{name: "workload digest", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028WorkloadField) }},
	{name: "graph identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028GraphField) }},
	{name: "evidence schema version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028SchemaField) }},
	{name: "target process identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028TargetProcessField) }},
	{name: "target command-buffer identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028TargetCommandField) }},
	{name: "target GPU span start", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028StartField) }},
	{name: "target GPU span end", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028EndField) }},
	{name: "same-GPU interval count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028InspectedCountField) }},
	{name: "all same-GPU intervals inspected", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028AllInspectedField) }},
	{name: "expected GPU event count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ExpectedEventsField) }},
	{name: "matched GPU interval count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028MatchedEventsField) }},
	{name: "foreign overlap interval count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ForeignCountField) }},
	{name: "foreign union-overlap duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028UnionField) }},
	{name: "sorted foreign process identities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ForeignProcessesField) }},
	{name: "exact target exclusion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028TargetExclusionField) }},
	{name: "positive-duration intersection status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028PositiveDurationField) }},
	{name: "boundary-touch exclusion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028BoundaryField) }},
	{name: "union de-duplication status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028UnionDedupField) }},
	{name: "foreign process sorting status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ProcessSortStatusField) }},
	{name: "exclusive-window requirement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ExclusiveRequiredField) }},
	{name: "exclusive-window result", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ExclusivePassedField) }},
	{name: "exact output/digest status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ExactOutputField) }},
	{name: "serialized evidence status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028SerializedField) }},
	{name: "counter-semantic claim status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028CounterClaimField) }},
	{name: "sampled-counter gate retention", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028CounterRetainedField) }},
	{name: "sampled-counter gate result", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028CounterPassedField) }},
	{name: "missing-counter imputation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6028ImputationField) }},
}

type ps6028Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6028(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6028Context(text) {
				continue
			}
			manifest, found := ps6028BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU interval exclusivity harness has no exact foreign-overlap manifest; missing %s", strings.Join(ps6028Missing(nil), ", "))
				continue
			}
			if missing := ps6028Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU foreign-overlap evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6028Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU foreign-overlap audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6028Context(text string) bool {
	text = ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(text, "gpu", "metal", "mps", "cuda", "vulkan", "accelerator", "device")
	interval := ps6007ContainsAny(text, "interval", "timestamp", "timespan", "window")
	overlap := ps6007ContainsAny(text, "foreignoverlap", "exclusivity", "exclusivegpu", "contamination", "overlapgate")
	return accelerator && interval && overlap
}

func ps6028BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6028Manifest, bool) {
	var best ps6028Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6028ManifestType(lit.Type) {
			return true
		}
		manifest := ps6028Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6028Axes) - len(ps6028Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6028ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6028ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6028ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "gpuexclusivityevidence", "foreignintervaloverlap", "exactgpuoverlapgate", "gpuwindowcontamination", "foreignoverlapgate")
}

func ps6028Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6028Axes))
	for _, axis := range ps6028Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6028HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "gpudevice", "deviceidentity", "deviceid") && !strings.Contains(name, "target")
}

func ps6028WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && ps6007ContainsAny(name, "digest", "identity", "id")
}

func ps6028GraphField(name string) bool {
	return strings.Contains(name, "graph") && ps6007ContainsAny(name, "digest", "identity", "id")
}

func ps6028SchemaField(name string) bool {
	return strings.Contains(name, "schema") && ps6007ContainsAny(name, "version", "revision")
}

func ps6028TargetProcessField(name string) bool {
	return strings.Contains(name, "target") && strings.Contains(name, "process") && ps6007ContainsAny(name, "id", "identity")
}

func ps6028TargetCommandField(name string) bool {
	return strings.Contains(name, "target") && ps6007ContainsAny(name, "commandbuffer", "command") && ps6007ContainsAny(name, "id", "identity") && !strings.Contains(name, "excluded")
}

func ps6028StartField(name string) bool {
	return strings.Contains(name, "target") && ps6007ContainsAny(name, "gpuspan", "gpuwindow", "interval") && strings.Contains(name, "start")
}

func ps6028EndField(name string) bool {
	return strings.Contains(name, "target") && ps6007ContainsAny(name, "gpuspan", "gpuwindow", "interval") && strings.Contains(name, "end")
}

func ps6028InspectedCountField(name string) bool {
	return strings.Contains(name, "samegpu") && strings.Contains(name, "interval") && strings.Contains(name, "count")
}

func ps6028AllInspectedField(name string) bool {
	return strings.Contains(name, "samegpu") && strings.Contains(name, "interval") && ps6007ContainsAny(name, "every", "all") && strings.Contains(name, "inspected")
}

func ps6028ExpectedEventsField(name string) bool {
	return strings.Contains(name, "expected") && strings.Contains(name, "gpu") && ps6007ContainsAny(name, "eventcount", "events")
}

func ps6028MatchedEventsField(name string) bool {
	return strings.Contains(name, "matched") && strings.Contains(name, "gpu") && ps6007ContainsAny(name, "intervalcount", "intervals", "eventcount")
}

func ps6028ForeignCountField(name string) bool {
	return strings.Contains(name, "foreign") && strings.Contains(name, "overlap") && strings.Contains(name, "count")
}

func ps6028UnionField(name string) bool {
	return strings.Contains(name, "foreign") && strings.Contains(name, "union") && strings.Contains(name, "overlap") && ps6007ContainsAny(name, "ns", "duration", "time")
}

func ps6028ForeignProcessesField(name string) bool {
	return strings.Contains(name, "foreign") && strings.Contains(name, "process") && ps6007ContainsAny(name, "id", "identity") && !ps6007ContainsAny(name, "sortedstatus", "sortpassed")
}

func ps6028TargetExclusionField(name string) bool {
	return strings.Contains(name, "exacttarget") && strings.Contains(name, "process") && strings.Contains(name, "command") && ps6007ContainsAny(name, "excluded", "exclusion")
}

func ps6028PositiveDurationField(name string) bool {
	return strings.Contains(name, "positive") && strings.Contains(name, "duration") && ps6007ContainsAny(name, "intersection", "overlap")
}

func ps6028BoundaryField(name string) bool {
	return strings.Contains(name, "boundary") && strings.Contains(name, "touch") && ps6007ContainsAny(name, "excluded", "nonoverlap", "ignored")
}

func ps6028UnionDedupField(name string) bool {
	return strings.Contains(name, "union") && strings.Contains(name, "overlap") && ps6007ContainsAny(name, "deduplicated", "nodoublecount", "merged")
}

func ps6028ProcessSortStatusField(name string) bool {
	return strings.Contains(name, "foreign") && strings.Contains(name, "process") && ps6007ContainsAny(name, "sortedstatus", "sortpassed", "sortedunique")
}

func ps6028ExclusiveRequiredField(name string) bool {
	return strings.Contains(name, "exclusive") && strings.Contains(name, "gpu") && ps6007ContainsAny(name, "require", "required", "requirement")
}

func ps6028ExclusivePassedField(name string) bool {
	return strings.Contains(name, "exclusive") && strings.Contains(name, "gpu") && ps6007ContainsAny(name, "passed", "result", "status")
}

func ps6028ExactOutputField(name string) bool {
	return ps6007ContainsAny(name, "outputdigest", "exactoutput", "logitsdigest", "exactlogits") && ps6007ContainsAny(name, "passed", "exact", "matched", "status")
}

func ps6028SerializedField(name string) bool {
	return ps6007ContainsAny(name, "serializedevidence", "evidenceserialized", "jsonserialized", "serialization") && ps6007ContainsAny(name, "passed", "status", "serialized")
}

func ps6028CounterClaimField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "semantic") && strings.Contains(name, "claim")
}

func ps6028CounterRetainedField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "gate") && ps6007ContainsAny(name, "retained", "available")
}

func ps6028CounterPassedField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "gate") && ps6007ContainsAny(name, "passed", "result", "status")
}

func ps6028ImputationField(name string) bool {
	return strings.Contains(name, "missingcounter") && ps6007ContainsAny(name, "imputedzero", "zeroimputed", "treatedaszero")
}

func ps6028Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	start, startOK := ps6016Number(fields, ps6028StartField)
	end, endOK := ps6016Number(fields, ps6028EndField)
	inspected, inspectedOK := ps6016Number(fields, ps6028InspectedCountField)
	expected, expectedOK := ps6016Number(fields, ps6028ExpectedEventsField)
	matched, matchedOK := ps6016Number(fields, ps6028MatchedEventsField)
	foreignCount, foreignCountOK := ps6016Number(fields, ps6028ForeignCountField)
	union, unionOK := ps6016Number(fields, ps6028UnionField)
	required, requiredOK := ps6026Bool(fields, ps6028ExclusiveRequiredField)
	passed, passedOK := ps6026Bool(fields, ps6028ExclusivePassedField)
	counterClaim, counterClaimOK := ps6026Bool(fields, ps6028CounterClaimField)
	counterPassed, counterPassedOK := ps6026Bool(fields, ps6028CounterPassedField)

	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"all same-GPU intervals inspected", ps6028AllInspectedField},
		{"exact target process/command-buffer exclusion", ps6028TargetExclusionField},
		{"positive-duration intersection", ps6028PositiveDurationField},
		{"boundary-touch exclusion", ps6028BoundaryField},
		{"union-overlap de-duplication", ps6028UnionDedupField},
		{"foreign process sorting", ps6028ProcessSortStatusField},
		{"exact output/digest", ps6028ExactOutputField},
		{"evidence serialization", ps6028SerializedField},
		{"sampled-counter gate retention", ps6028CounterRetainedField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	if startOK && endOK && end <= start {
		warnings = append(warnings, fmt.Sprintf("target GPU span is invalid (start %.6g, end %.6g)", start, end))
	}
	if inspectedOK && inspected < 1 {
		warnings = append(warnings, "same-GPU interval count must include at least the target interval")
	}
	if foreignCountOK && foreignCount < 0 || unionOK && union < 0 {
		warnings = append(warnings, "foreign overlap count and union duration must be non-negative")
	}
	if inspectedOK && foreignCountOK && foreignCount > inspected-1 {
		warnings = append(warnings, fmt.Sprintf("foreign overlap count %.0f exceeds non-target same-GPU interval capacity %.0f", foreignCount, inspected-1))
	}
	if foreignCountOK && unionOK && (foreignCount == 0) != (union == 0) {
		warnings = append(warnings, fmt.Sprintf("foreign overlap count %.0f and union duration %.6g disagree about positive overlap", foreignCount, union))
	}
	if expectedOK && matchedOK && expected != matched {
		warnings = append(warnings, fmt.Sprintf("expected GPU events %.0f differ from matched GPU intervals %.0f", expected, matched))
	}
	if requiredOK && required && passedOK && unionOK {
		if union > 0 && passed {
			warnings = append(warnings, fmt.Sprintf("exclusive GPU window passes despite %.6g ns of positive foreign union overlap", union))
		}
		if union == 0 && !passed {
			warnings = append(warnings, "exclusive GPU window fails despite zero positive foreign overlap")
		}
	}
	if counterClaimOK && counterClaim && counterPassedOK && !counterPassed {
		warnings = append(warnings, "counter-semantic claim proceeds while its sampled-counter gate fails")
	}
	if imputed, known := ps6026Bool(fields, ps6028ImputationField); known && imputed {
		warnings = append(warnings, "missing hardware counter is imputed as zero")
	}
	warnings = append(warnings, ps6028ProcessWarnings(fields, foreignCount, foreignCountOK)...)
	return warnings
}

func ps6028ProcessWarnings(fields map[string]ps6016Field, foreignCount float64, foreignCountOK bool) []string {
	for name, field := range fields {
		if !ps6028ForeignProcessesField(name) {
			continue
		}
		if field.hasNumbers {
			if !slices.IsSorted(field.numbers) {
				return []string{"foreign process identities are not sorted"}
			}
			if ps6028HasAdjacentDuplicate(field.numbers) {
				return []string{"foreign process identities are not unique"}
			}
			if foreignCountOK && foreignCount == 0 && len(field.numbers) != 0 {
				return []string{"foreign process identities are nonempty for zero overlapping intervals"}
			}
		}
		if field.hasStringValues {
			if !slices.IsSorted(field.stringValues) {
				return []string{"foreign process identities are not sorted"}
			}
			if ps6028HasAdjacentDuplicate(field.stringValues) {
				return []string{"foreign process identities are not unique"}
			}
			if foreignCountOK && foreignCount == 0 && len(field.stringValues) != 0 {
				return []string{"foreign process identities are nonempty for zero overlapping intervals"}
			}
		}
	}
	return nil
}

func ps6028HasAdjacentDuplicate[S ~[]E, E comparable](values S) bool {
	for i := 1; i < len(values); i++ {
		if values[i] == values[i-1] {
			return true
		}
	}
	return false
}
