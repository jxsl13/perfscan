package checks

import (
	"fmt"
	"go/ast"
	"math"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6025 implements owner issue #753: summed stage busy time is not a GPU
// trace completeness measure; structural boundary span and omissions are.
var PS6025 = register(&lint.Check{
	ID:       "PS6025",
	Category: "verify",
	Slug:     "gpu-busy-share-is-not-trace-completeness",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU trace treats summed stage busy time as structural completeness",
		Text: `A GPU command can contain every expected encoder while the sum
of encoder durations is materially shorter than the command interval.
Inter-encoder gaps, scheduler time, and boundary work are real elapsed time,
not necessarily missing samples. Calling summed duration "coverage" creates a
flaky completeness gate and can mis-rank a change that alters scheduling gaps
rather than kernel work.

This check implements owner issue #753. It audits GPUTraceEvidence,
ProfilerTraceEvidence, CommandSpanEvidence, TraceCompletenessEvidence, or
equivalent manifests in real benchmark/test harnesses. Evidence must keep
these concepts separate:

  - hardware and workload identity;
  - the runtime command-buffer span;
  - first-event-start to last-event-end boundary span;
  - the sum of individual stage/encoder durations;
  - summed busy share, named as a diagnostic rather than coverage;
  - preserved inter-stage/scheduler gap duration;
  - expected event identity/order plus expected and observed counts;
  - explicit omission and trace-error counters;
  - a boundary-span agreement tolerance or status; and
  - profiled-versus-ordinary semantic parity.

For constant manifests, PS6025 verifies boundary/command agreement, expected
versus observed counts, zero omissions/errors, parity, the recorded busy share
against sum/command, and the recorded gap against boundary-minus-sum. It also
reports coverage/completeness fields derived from summed stage time. A
minimum busy-share floor at or above 75% is treated as a tight decision gate:
if a structurally complete trace falls below it, the diagnostic explains that
the floor would reject valid evidence. A small sanity floor may remain, but
busy share is diagnostic—not the completeness proof.

There is NO automatic fix. Trace clocks, event identity, omitted samples,
overlap, and scheduler gaps are runtime evidence a syntax rewrite cannot
invent.`,
		Before: `evidence := GPUTraceEvidence{
	CommandSpanNS: 100000,
	SummedStageCoverage: 0.80,
	MinimumCoverage: 0.80,
}
// Reject below "80% coverage".`,
		After: `evidence := GPUTraceEvidence{
	Hardware: "Apple M2 Pro", Workload: "TinyLlama position-1 decode",
	CommandSpanNS: commandSpan,
	FirstToLastEventSpanNS: eventBoundarySpan,
	SummedStageDurationNS: summedBusy,
	BusyShare: summedBusy / commandSpan, // diagnostic only
	InterStageGapNS: eventBoundarySpan - summedBusy,
	ExpectedEventIDs: expectedIDs,
	ExpectedEventCount: 340, ObservedEventCount: 340,
	MPSOmissions: 0, OverflowOmissions: 0, UnsupportedOmissions: 0,
	TraceErrorCount: 0, BoundarySpanTolerance: 0.02,
	ProfiledOrdinaryParity: true,
}`,
		MeasuredWin: `Across repeated Apple-M2-Pro traces of the same
340-event TinyLlama position-1 decode command, every successful trace had
exact logits, zero MPS/overflow/unsupported omissions, and a first-to-last
event span matching the command interval. Summed-event/command busy share
still varied from 0.7923 to 0.8727. An 80% summed-duration "coverage" floor
therefore rejected one valid rerun; boundary-span plus explicit omission
checks did not.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6025",
		Doc:  "GPU summed-stage busy share is not command-trace completeness",
		Run:  runPS6025,
	},
})

type ps6025Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6025Axes = []ps6025Axis{
	{name: "hardware", present: func(f map[string]ps6016Field) bool { return ps6025Has(f, "hardware", "device") }},
	{name: "workload", present: func(f map[string]ps6016Field) bool { return ps6025Has(f, "workload", "model", "shape", "campaign") }},
	{name: "runtime command span", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025CommandSpanField) }},
	{name: "first-to-last event boundary span", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025BoundarySpanField) }},
	{name: "summed stage duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025SummedDurationField) }},
	{name: "diagnostic busy share", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025BusyShareField) }},
	{name: "inter-stage/scheduler gap duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025GapField) }},
	{name: "expected event identity/order", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025ExpectedIdentityField) }},
	{name: "expected event count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025ExpectedCountField) }},
	{name: "observed event count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025ObservedCountField) }},
	{name: "explicit omission counters", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025OmissionField) }},
	{name: "trace-error counter", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025ErrorField) }},
	{name: "boundary-span agreement tolerance/status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025BoundaryGateField) }},
	{name: "profiled-versus-ordinary parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6025ParityField) }},
}

type ps6025Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6025(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6011Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6025Context(text) {
				continue
			}
			manifest, found := ps6025BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU trace harness has no command-span completeness manifest; missing %s", strings.Join(ps6025Missing(nil), ", "))
				continue
			}
			if missing := ps6025Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU trace completeness evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6025Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU trace completeness audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6025Context(text string) bool {
	text = ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(text, "gpu", "metal", "mps", "cuda", "vulkan", "accelerator", "device")
	trace := ps6007ContainsAny(text, "trace", "profiler", "profiled")
	command := ps6007ContainsAny(text, "command", "commandbuffer", "runtimeinterval")
	event := ps6007ContainsAny(text, "event", "encoder", "stage")
	return accelerator && trace && command && event
}

func ps6025BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6025Manifest, bool) {
	var best ps6025Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6025ManifestType(lit.Type) {
			return true
		}
		manifest := ps6025Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6025Axes) - len(ps6025Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6025ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6025ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6025ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "gputraceevidence", "profilertraceevidence", "commandspanevidence", "tracecompleteness", "tracecoverageevidence", "eventspanevidence", "stagebusyevidence")
}

func ps6025Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6025Axes))
	for _, axis := range ps6025Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6025Has(fields map[string]ps6016Field, alternatives ...string) bool {
	return ps6016HasName(fields, func(name string) bool { return ps6007ContainsAny(name, alternatives...) })
}

func ps6025CommandSpanField(name string) bool {
	return strings.Contains(name, "command") && ps6007ContainsAny(name, "span", "duration", "interval") &&
		!ps6007ContainsAny(name, "boundary", "firsttolast", "busy", "gap", "tolerance", "agreement")
}

func ps6025BoundarySpanField(name string) bool {
	return ps6007ContainsAny(name, "firsttolast", "eventboundary", "boundaryspan", "eventspan", "encoderspan") &&
		ps6007ContainsAny(name, "span", "duration", "interval") &&
		!ps6007ContainsAny(name, "tolerance", "agreement", "passed", "status", "gate", "maximumdifference")
}

func ps6025SummedDurationField(name string) bool {
	return ps6007ContainsAny(name, "summed", "sum", "total") &&
		ps6007ContainsAny(name, "stage", "event", "encoder") &&
		ps6007ContainsAny(name, "duration", "time", "busy") &&
		!ps6007ContainsAny(name, "share", "coverage", "threshold", "minimum", "floor")
}

func ps6025BusyShareField(name string) bool {
	return strings.Contains(name, "busy") && ps6007ContainsAny(name, "share", "ratio") &&
		!ps6007ContainsAny(name, "minimum", "threshold", "floor", "required")
}

func ps6025GapField(name string) bool {
	return strings.Contains(name, "gap") && ps6007ContainsAny(name, "interstage", "scheduler", "event", "encoder", "stage") &&
		ps6007ContainsAny(name, "duration", "time", "span", "ns")
}

func ps6025ExpectedIdentityField(name string) bool {
	return strings.Contains(name, "expected") && ps6007ContainsAny(name, "event", "encoder", "stage") &&
		ps6007ContainsAny(name, "id", "name", "identity", "order", "sequence")
}

func ps6025ExpectedCountField(name string) bool {
	return strings.Contains(name, "expected") && ps6007ContainsAny(name, "event", "encoder", "stage") && strings.Contains(name, "count")
}

func ps6025ObservedCountField(name string) bool {
	return ps6007ContainsAny(name, "observed", "profiled", "recorded", "actual") &&
		ps6007ContainsAny(name, "event", "encoder", "stage") && strings.Contains(name, "count")
}

func ps6025OmissionField(name string) bool {
	return ps6007ContainsAny(name, "omission", "omitted", "missedevent", "missingevent")
}

func ps6025ErrorField(name string) bool {
	return strings.Contains(name, "error") && ps6007ContainsAny(name, "count", "counter", "trace", "profile")
}

func ps6025BoundaryGateField(name string) bool {
	return ps6007ContainsAny(name, "boundaryspan", "spanagreement", "commandspan") &&
		ps6007ContainsAny(name, "tolerance", "passed", "status", "gate", "maximumdifference")
}

func ps6025ParityField(name string) bool {
	return ps6007ContainsAny(name, "parity", "exactness", "identical") &&
		ps6007ContainsAny(name, "profiled", "ordinary", "output", "logit", "passed", "status")
}

func ps6025BusyFloorField(name string) bool {
	return strings.Contains(name, "busy") && ps6007ContainsAny(name, "share", "ratio") &&
		ps6007ContainsAny(name, "minimum", "threshold", "floor", "required")
}

func ps6025CoverageMislabel(name string) bool {
	return ps6007ContainsAny(name, "coverage", "completeness") &&
		ps6007ContainsAny(name, "summedstage", "summedevent", "summedencoder", "stagebusy", "eventbusy", "encoderbusy")
}

func ps6025Audit(fields map[string]ps6016Field) []string {
	var mislabeled []string
	for name := range fields {
		if ps6025CoverageMislabel(name) {
			mislabeled = append(mislabeled, name)
		}
	}
	slices.Sort(mislabeled)
	warnings := make([]string, 0, len(mislabeled)+8)
	for _, name := range mislabeled {
		warnings = append(warnings, name+" labels summed busy time as coverage/completeness; name it BusyShare and keep it diagnostic")
	}

	command, commandOK := ps6016Number(fields, ps6025CommandSpanField)
	boundary, boundaryOK := ps6016Number(fields, ps6025BoundarySpanField)
	summed, summedOK := ps6016Number(fields, ps6025SummedDurationField)
	busy, busyOK := ps6025Ratio(fields, ps6025BusyShareField)
	gap, gapOK := ps6016Number(fields, ps6025GapField)
	expected, expectedOK := ps6016Number(fields, ps6025ExpectedCountField)
	observed, observedOK := ps6016Number(fields, ps6025ObservedCountField)
	omissions, omissionsOK := ps6025CounterSum(fields, ps6025OmissionField)
	errors, errorsOK := ps6025CounterSum(fields, ps6025ErrorField)
	tolerance, toleranceOK := ps6025Tolerance(fields)
	floor, floorOK := ps6025Ratio(fields, ps6025BusyFloorField)

	if commandOK && command <= 0 {
		warnings = append(warnings, "runtime command span is not positive")
	}
	if boundaryOK && boundary <= 0 {
		warnings = append(warnings, "first-to-last event boundary span is not positive")
	}
	if summedOK && summed < 0 {
		warnings = append(warnings, "summed stage duration is negative")
	}
	spanComplete := false
	if commandOK && command > 0 && boundaryOK && boundary > 0 && toleranceOK && tolerance >= 0 {
		difference := math.Abs(boundary-command) / command
		spanComplete = difference <= tolerance
		if !spanComplete {
			warnings = append(warnings, fmt.Sprintf("event boundary span differs from command span by %.2f%%, above %.2f%% tolerance", difference*100, tolerance*100))
		}
	}
	countsComplete := expectedOK && observedOK && expected == observed
	if expectedOK && observedOK && expected != observed {
		warnings = append(warnings, fmt.Sprintf("expected/observed event counts differ (%.0f vs %.0f)", expected, observed))
	}
	if omissionsOK && omissions != 0 {
		warnings = append(warnings, fmt.Sprintf("explicit omission counters total %.0f", omissions))
	}
	if errorsOK && errors != 0 {
		warnings = append(warnings, fmt.Sprintf("trace-error counters total %.0f", errors))
	}
	for name, field := range fields {
		if ps6025ParityField(name) && field.hasBool && !field.boolVal {
			warnings = append(warnings, name+" is explicitly false")
		}
	}
	calculatedBusy, calculatedBusyOK := 0.0, commandOK && command > 0 && summedOK
	if calculatedBusyOK {
		calculatedBusy = summed / command
		if busyOK && !ps6025Close(busy, calculatedBusy) {
			warnings = append(warnings, fmt.Sprintf("recorded busy share %.4f disagrees with summed-stage/command ratio %.4f", busy, calculatedBusy))
		}
	}
	if boundaryOK && summedOK && boundary >= summed {
		calculatedGap := boundary - summed
		if gapOK && !ps6025Close(gap, calculatedGap) {
			warnings = append(warnings, fmt.Sprintf("recorded inter-stage gap %.4g disagrees with boundary-minus-summed duration %.4g", gap, calculatedGap))
		}
	}
	structurallyComplete := spanComplete && countsComplete && omissionsOK && omissions == 0 && errorsOK && errors == 0
	if floorOK && floor >= 0.75 {
		if structurallyComplete && calculatedBusyOK && calculatedBusy < floor {
			warnings = append(warnings, fmt.Sprintf("tight %.2f%% busy-share floor rejects a structurally complete %.2f%% trace; busy share is diagnostic, not completeness", floor*100, calculatedBusy*100))
		} else {
			warnings = append(warnings, fmt.Sprintf("%.2f%% busy-share floor is a tight universal completeness gate; use boundary span, identities/counts, and omissions instead", floor*100))
		}
	}
	return warnings
}

func ps6025CounterSum(fields map[string]ps6016Field, predicate func(string) bool) (float64, bool) {
	total, found := 0.0, false
	for name, field := range fields {
		if predicate(name) && field.hasNumber {
			total += field.number
			found = true
		}
	}
	return total, found
}

func ps6025Ratio(fields map[string]ps6016Field, predicate func(string) bool) (float64, bool) {
	for name, field := range fields {
		if !predicate(name) || !field.hasNumber {
			continue
		}
		value := field.number
		if strings.Contains(name, "percent") || strings.Contains(name, "pct") {
			value /= 100
		}
		return value, true
	}
	return 0, false
}

func ps6025Tolerance(fields map[string]ps6016Field) (float64, bool) {
	for name, field := range fields {
		if !ps6025BoundaryGateField(name) || !field.hasNumber {
			continue
		}
		value := field.number
		if strings.Contains(name, "percent") || strings.Contains(name, "pct") {
			value /= 100
		}
		return value, true
	}
	return 0, false
}

func ps6025Close(left, right float64) bool {
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return math.Abs(left-right) <= 1e-6*scale
}
