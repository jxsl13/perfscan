package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6052 implements owner issue #726: replacing repeated tiny immutable Metal
// parameter buffers with an arena is only safe and profitable when the arena's
// storage and in-flight lifetime are amortized beyond one command recorder.
var PS6052 = register(&lint.Check{
	ID:       "PS6052",
	Category: "verify",
	Slug:     "metal-parameter-arena-needs-amortized-lifetime-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "tiny immutable Metal parameter buffers are replaced by a recorder-local arena without an end-to-end gate",
		Text: `Repeated newBufferWithBytes calls for small immutable shape and
scalar records can dominate Metal host recording. Combining them into aligned
suballocations is promising, but allocating a large arena for every command
recorder can move more memory-management work than it removes.

This check implements owner issue #726. It audits ParameterArenaEvidence,
ImmutableParameterBufferReport, RecorderArenaAmortizationGate,
MetalParameterArenaCampaign, or equivalent manifests. Evidence must record:

  - hardware/toolchain, the repeated-buffer count and maximum record size;
  - arena capacity, live payload, utilization, alignment, overflow fallback,
    async commit/wait lifetime, and profiling-recorder coverage;
  - whether the arena is allocated per recorder and which completion-aware
    pool, long-lived in-flight-epoch ring, or validated setBytes alternative
    is documented;
  - independent alternating host-recording and complete-workload samples,
    their medians and control/candidate ratios;
  - paired complete-workload ratios, Go allocation counts, exact output
    parity, performance threshold/verdict, recommendation, and decision.

Constant evidence is checked for stale medians, ratios, paired regression,
utilization, allocation or output mismatches, unsafe arena lifetime claims,
and promotion of a candidate that loses the complete-workload gate. There is
NO automatic fix: completion ownership, command-buffer concurrency, driver
setBytes limits, and end-to-end cost are runtime/backend facts. The safe output
is an amortized design candidate that remains default-off until validated.`,
		Before: `for each immutable parameter record:
    buffer = device.newBufferWithBytes(record)`,
		After: `arena = completionAwarePool.acquire(inFlightEpoch)
offset = arena.copyAligned(record, 256) // with checked overflow fallback`,
		MeasuredWin: `The Apple-M2-Pro experiment behind issue #726 reduced a
32-buffer host-recording leaf from 85,799.5 ns to 28,103.5 ns (3.053x) with
unchanged 1 alloc/op. A naive 256 KiB arena allocated for every recorder lost
the complete two-token TinyLlama Q4_K_M gate: 75.218 ms versus 80.078 ms,
0.939x, with a paired median 3.33% candidate regression. It was rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6052",
		Doc:  "Metal immutable-parameter arena lacks amortized lifetime and end-to-end evidence",
		Run:  runPS6052,
	},
})

type ps6052Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6052Axes = []ps6052Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052HardwareField) }},
	{name: "toolchain identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052ToolchainField) }},
	{name: "candidate default-off status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052DefaultOffField) }},
	{name: "immutable buffer count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052BufferCountField) }},
	{name: "maximum immutable record bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052RecordBytesField) }},
	{name: "arena capacity bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052ArenaBytesField) }},
	{name: "arena live payload bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052PayloadBytesField) }},
	{name: "arena utilization", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052UtilizationField) }},
	{name: "alignment bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052AlignmentBytesField) }},
	{name: "suballocation alignment verification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052AlignmentVerifiedField) }},
	{name: "overflow fallback verification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052OverflowField) }},
	{name: "async commit/wait lifetime verification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052AsyncLifetimeField) }},
	{name: "profiling-recorder coverage", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052ProfilingField) }},
	{name: "per-recorder allocation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052PerRecorderField) }},
	{name: "completion-aware pool design", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052PoolField) }},
	{name: "long-lived in-flight ring design", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052RingField) }},
	{name: "in-flight epoch count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052EpochField) }},
	{name: "setBytes driver validation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052SetBytesField) }},
	{name: "host process count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052CountField(n, "host", "process") })
	}},
	{name: "host iterations per process", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052CountField(n, "host", "iteration") })
	}},
	{name: "host alternating order", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052AlternatingField(n, "host") })
	}},
	{name: "host control latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052LatencyField(n, "host", "control") })
	}},
	{name: "host candidate latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052LatencyField(n, "host", "candidate") })
	}},
	{name: "host control median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052MedianField(n, "host", "control") })
	}},
	{name: "host candidate median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052MedianField(n, "host", "candidate") })
	}},
	{name: "host control/candidate ratio", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052RatioField(n, "host") })
	}},
	{name: "host control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052AllocationField(n, "host", "control") })
	}},
	{name: "host candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052AllocationField(n, "host", "candidate") })
	}},
	{name: "complete workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052WorkloadField) }},
	{name: "complete workload token count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052TokenCountField) }},
	{name: "complete workload process count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052CountField(n, "endtoend", "process") })
	}},
	{name: "complete workload iterations per process", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052CountField(n, "endtoend", "iteration") })
	}},
	{name: "complete workload alternating order", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052AlternatingField(n, "endtoend") })
	}},
	{name: "complete workload control latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052LatencyField(n, "endtoend", "control") })
	}},
	{name: "complete workload candidate latencies", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052LatencyField(n, "endtoend", "candidate") })
	}},
	{name: "complete workload control median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052MedianField(n, "endtoend", "control") })
	}},
	{name: "complete workload candidate median", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052MedianField(n, "endtoend", "candidate") })
	}},
	{name: "complete workload control/candidate ratio", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6052RatioField(n, "endtoend") })
	}},
	{name: "paired candidate/control ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052PairedRatiosField) }},
	{name: "median paired candidate/control ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052PairedMedianField) }},
	{name: "exact output parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052ParityField) }},
	{name: "output mismatch count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052MismatchField) }},
	{name: "complete workload performance minimum", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052MinimumField) }},
	{name: "complete workload performance verdict", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052VerdictField) }},
	{name: "per-recorder candidate recommendation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052CandidateRecommendedField) }},
	{name: "amortized design recommendation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052AmortizedRecommendedField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6052DecisionField) }},
}

type ps6052Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6052(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6052Context(text) {
				continue
			}
			manifest, found := ps6052BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "Metal immutable-parameter arena has no amortization manifest; missing %s", strings.Join(ps6052Missing(nil), ", "))
				continue
			}
			if missing := ps6052Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal parameter-arena evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6052Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal parameter-arena audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6052Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "metal") &&
		ps6007ContainsAny(text, "immutableparameter", "newbufferwithbytes", "parameterbuffer") &&
		ps6007ContainsAny(text, "arena", "recorder", "suballocation")
}

func ps6052BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6052Manifest, bool) {
	var best ps6052Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6052ManifestType(lit.Type) {
			return true
		}
		manifest := ps6052Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6052Axes) - len(ps6052Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6052ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6052ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6052ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "parameterarenaevidence", "immutableparameterbufferreport", "recorderarenaamortizationgate", "metalparameterarenacampaign", "parameterarenaamortizationevidence")
}

func ps6052Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6052Axes))
	for _, axis := range ps6052Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6052HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6052ToolchainField(n string) bool {
	return ps6007ContainsAny(n, "toolchain", "compileridentity")
}
func ps6052DefaultOffField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "defaultoff")
}
func ps6052BufferCountField(n string) bool {
	return strings.Contains(n, "immutable") && strings.Contains(n, "buffer") && strings.Contains(n, "count")
}
func ps6052RecordBytesField(n string) bool {
	return strings.Contains(n, "immutable") && strings.Contains(n, "record") && strings.Contains(n, "byte")
}
func ps6052ArenaBytesField(n string) bool {
	return strings.Contains(n, "arena") && strings.Contains(n, "capacity") && strings.Contains(n, "byte")
}
func ps6052PayloadBytesField(n string) bool {
	return strings.Contains(n, "arena") && strings.Contains(n, "payload") && strings.Contains(n, "byte")
}
func ps6052UtilizationField(n string) bool {
	return strings.Contains(n, "arena") && strings.Contains(n, "utilization")
}
func ps6052AlignmentBytesField(n string) bool {
	return strings.Contains(n, "alignment") && strings.Contains(n, "byte")
}
func ps6052AlignmentVerifiedField(n string) bool {
	return strings.Contains(n, "suballocation") && strings.Contains(n, "alignment") && strings.Contains(n, "verified")
}
func ps6052OverflowField(n string) bool {
	return strings.Contains(n, "overflow") && strings.Contains(n, "fallback") && strings.Contains(n, "verified")
}
func ps6052AsyncLifetimeField(n string) bool {
	return strings.Contains(n, "async") && strings.Contains(n, "commit") && strings.Contains(n, "wait") && strings.Contains(n, "lifetime")
}
func ps6052ProfilingField(n string) bool {
	return strings.Contains(n, "profiling") && strings.Contains(n, "recorder") && ps6007ContainsAny(n, "covered", "coverage", "verified")
}
func ps6052PerRecorderField(n string) bool {
	return strings.Contains(n, "arena") && strings.Contains(n, "perrecorder")
}
func ps6052PoolField(n string) bool {
	return strings.Contains(n, "completionaware") && strings.Contains(n, "pool")
}
func ps6052RingField(n string) bool {
	return strings.Contains(n, "longlived") && strings.Contains(n, "ring")
}
func ps6052EpochField(n string) bool {
	return strings.Contains(n, "inflight") && strings.Contains(n, "epoch") && strings.Contains(n, "count")
}
func ps6052SetBytesField(n string) bool {
	return strings.Contains(n, "setbytes") && strings.Contains(n, "driver") && strings.Contains(n, "validated")
}
func ps6052CountField(n, scope, detail string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, detail) && strings.Contains(n, "count")
}
func ps6052AlternatingField(n, scope string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, "alternating") && strings.Contains(n, "order")
}
func ps6052LatencyField(n, scope, side string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, side) && ps6007ContainsAny(n, "latencies", "times")
}
func ps6052MedianField(n, scope, side string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, side) && strings.Contains(n, "median")
}
func ps6052RatioField(n, scope string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio") && !strings.Contains(n, "paired")
}
func ps6052AllocationField(n, scope, side string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, "count")
}
func ps6052WorkloadField(n string) bool {
	return strings.Contains(n, "endtoend") && strings.Contains(n, "workload")
}
func ps6052TokenCountField(n string) bool {
	return strings.Contains(n, "endtoend") && strings.Contains(n, "token") && strings.Contains(n, "count")
}
func ps6052PairedRatiosField(n string) bool {
	return strings.Contains(n, "paired") && strings.Contains(n, "candidatecontrol") && strings.Contains(n, "ratios") && !strings.Contains(n, "median")
}
func ps6052PairedMedianField(n string) bool {
	return strings.Contains(n, "median") && strings.Contains(n, "paired") && strings.Contains(n, "candidatecontrol") && strings.Contains(n, "ratio")
}
func ps6052ParityField(n string) bool {
	return strings.Contains(n, "exactoutput") && strings.Contains(n, "parity")
}
func ps6052MismatchField(n string) bool {
	return strings.Contains(n, "output") && strings.Contains(n, "mismatch") && strings.Contains(n, "count")
}
func ps6052MinimumField(n string) bool {
	return strings.Contains(n, "endtoend") && strings.Contains(n, "performance") && ps6007ContainsAny(n, "minimum", "threshold", "gate")
}
func ps6052VerdictField(n string) bool {
	return strings.Contains(n, "endtoend") && strings.Contains(n, "performance") && strings.Contains(n, "verdict")
}
func ps6052CandidateRecommendedField(n string) bool {
	return strings.Contains(n, "perrecorder") && strings.Contains(n, "candidate") && strings.Contains(n, "recommended")
}
func ps6052AmortizedRecommendedField(n string) bool {
	return strings.Contains(n, "amortized") && strings.Contains(n, "design") && strings.Contains(n, "recommended")
}
func ps6052ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6052DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6052Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 20)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"candidate default-off", ps6052DefaultOffField},
		{"suballocation alignment verification", ps6052AlignmentVerifiedField},
		{"overflow fallback verification", ps6052OverflowField},
		{"async commit/wait lifetime verification", ps6052AsyncLifetimeField},
		{"profiling-recorder coverage", ps6052ProfilingField},
		{"host alternating order", func(n string) bool { return ps6052AlternatingField(n, "host") }},
		{"complete-workload alternating order", func(n string) bool { return ps6052AlternatingField(n, "endtoend") }},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}

	arenaBytes, arenaOK := ps6016Number(fields, ps6052ArenaBytesField)
	payloadBytes, payloadOK := ps6016Number(fields, ps6052PayloadBytesField)
	if arenaOK && payloadOK {
		if payloadBytes > arenaBytes {
			warnings = append(warnings, fmt.Sprintf("arena payload %.0f bytes exceeds %.0f-byte capacity", payloadBytes, arenaBytes))
		}
		if recorded, ok := ps6016Number(fields, ps6052UtilizationField); ok && arenaBytes > 0 && !ps6025Close(recorded, payloadBytes/arenaBytes) {
			warnings = append(warnings, fmt.Sprintf("arena utilization %.6g disagrees with %.6g", recorded, payloadBytes/arenaBytes))
		}
	}
	if alignment, ok := ps6016Number(fields, ps6052AlignmentBytesField); ok && (alignment < 256 || !ps6052PowerOfTwo(alignment)) {
		warnings = append(warnings, fmt.Sprintf("alignment %.0f bytes is not a power of two of at least 256", alignment))
	}

	hostRatio, hostRatioOK := ps6052AuditSamples(fields, "host", &warnings)
	_ = hostRatio
	_ = hostRatioOK
	e2eRatio, e2eRatioOK := ps6052AuditSamples(fields, "endtoend", &warnings)

	for _, scope := range []string{"host"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6052AllocationField(n, scope, "control") })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6052AllocationField(n, scope, "candidate") })
		if controlOK && candidateOK && control != candidate {
			warnings = append(warnings, scope+" control/candidate allocation counts differ")
		}
	}

	controls, controlsOK := ps6016Numbers(fields, func(n string) bool { return ps6052LatencyField(n, "endtoend", "control") })
	candidates, candidatesOK := ps6016Numbers(fields, func(n string) bool { return ps6052LatencyField(n, "endtoend", "candidate") })
	paired, pairedOK := ps6016Numbers(fields, ps6052PairedRatiosField)
	pairedMedian, pairedMedianOK := 0.0, false
	if controlsOK && candidatesOK && pairedOK {
		if len(controls) != len(candidates) || len(controls) != len(paired) {
			warnings = append(warnings, "complete-workload control/candidate/paired vectors have different lengths")
		} else {
			for index := range paired {
				if controls[index] > 0 && !ps6025Close(paired[index], candidates[index]/controls[index]) {
					warnings = append(warnings, fmt.Sprintf("paired candidate/control ratio %d is %.6g, want %.6g", index, paired[index], candidates[index]/controls[index]))
					break
				}
			}
			if len(paired) > 0 {
				pairedMedian, pairedMedianOK = ps6016Median(paired), true
				if recorded, ok := ps6016Number(fields, ps6052PairedMedianField); ok && !ps6025Close(recorded, pairedMedian) {
					warnings = append(warnings, fmt.Sprintf("median paired candidate/control ratio %.6g disagrees with %.6g", recorded, pairedMedian))
				}
			}
		}
	}

	parity, parityOK := ps6026Bool(fields, ps6052ParityField)
	mismatches, mismatchesOK := ps6016Number(fields, ps6052MismatchField)
	if parityOK && mismatchesOK && parity != (mismatches == 0) {
		warnings = append(warnings, fmt.Sprintf("exact output parity is %t but mismatch count is %.0f", parity, mismatches))
	}
	minimum, minimumOK := ps6016Number(fields, ps6052MinimumField)
	performancePassed := e2eRatioOK && minimumOK && e2eRatio >= minimum && parityOK && parity
	if value, ok := ps6027String(fields, ps6052VerdictField); ok {
		normalized := ps6030StatusName(value)
		claimsPass := ps6007ContainsAny(normalized, "pass", "accept") && !ps6007ContainsAny(normalized, "fail", "reject")
		claimsFail := ps6007ContainsAny(normalized, "fail", "reject")
		if performancePassed && !claimsPass || !performancePassed && !claimsFail {
			warnings = append(warnings, fmt.Sprintf("complete-workload performance verdict %q disagrees with computed gate", value))
		}
	}

	pool, poolOK := ps6026Bool(fields, ps6052PoolField)
	ring, ringOK := ps6026Bool(fields, ps6052RingField)
	setBytes, setBytesOK := ps6026Bool(fields, ps6052SetBytesField)
	hasAmortizedAlternative := poolOK && pool || ringOK && ring || setBytesOK && setBytes
	if recommended, ok := ps6026Bool(fields, ps6052AmortizedRecommendedField); ok && recommended && !hasAmortizedAlternative {
		warnings = append(warnings, "amortized design is recommended without a documented completion-aware pool, in-flight ring, or validated setBytes path")
	}
	perRecorder, perRecorderOK := ps6026Bool(fields, ps6052PerRecorderField)
	if recommended, ok := ps6026Bool(fields, ps6052CandidateRecommendedField); ok && recommended {
		if !performancePassed || perRecorderOK && perRecorder {
			warnings = append(warnings, "per-recorder arena is recommended despite failed complete-workload or non-amortized lifetime gates")
		}
	}
	if decision, ok := ps6027String(fields, ps6052DecisionField); ok && (!performancePassed || perRecorderOK && perRecorder) && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
		warnings = append(warnings, fmt.Sprintf("final decision %q retains a per-recorder arena that failed the complete-workload/amortization gate", decision))
	}
	if pairedMedianOK && pairedMedian > 1 && performancePassed {
		warnings = append(warnings, "complete-workload gate passes despite a paired median candidate regression")
	}
	return warnings
}

func ps6052AuditSamples(fields map[string]ps6016Field, scope string, warnings *[]string) (float64, bool) {
	controls, controlsOK := ps6016Numbers(fields, func(n string) bool { return ps6052LatencyField(n, scope, "control") })
	candidates, candidatesOK := ps6016Numbers(fields, func(n string) bool { return ps6052LatencyField(n, scope, "candidate") })
	count, countOK := ps6016Number(fields, func(n string) bool { return ps6052CountField(n, scope, "process") })
	if controlsOK && candidatesOK {
		if len(controls) != len(candidates) {
			*warnings = append(*warnings, scope+" control/candidate latency vectors have different lengths")
			return 0, false
		}
		if countOK && float64(len(controls)) != count {
			*warnings = append(*warnings, fmt.Sprintf("%s process count %.0f disagrees with %d samples", scope, count, len(controls)))
		}
		if len(controls) > 0 {
			controlMedian := ps6016Median(controls)
			candidateMedian := ps6016Median(candidates)
			if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6052MedianField(n, scope, "control") }); ok && !ps6025Close(recorded, controlMedian) {
				*warnings = append(*warnings, fmt.Sprintf("%s control median %.6g disagrees with %.6g", scope, recorded, controlMedian))
			}
			if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6052MedianField(n, scope, "candidate") }); ok && !ps6025Close(recorded, candidateMedian) {
				*warnings = append(*warnings, fmt.Sprintf("%s candidate median %.6g disagrees with %.6g", scope, recorded, candidateMedian))
			}
			if candidateMedian > 0 {
				ratio := controlMedian / candidateMedian
				if recorded, ok := ps6016Number(fields, func(n string) bool { return ps6052RatioField(n, scope) }); ok && !ps6025Close(recorded, ratio) {
					*warnings = append(*warnings, fmt.Sprintf("%s control/candidate ratio %.6g disagrees with %.6g", scope, recorded, ratio))
				}
				return ratio, true
			}
		}
	}
	return 0, false
}

func ps6052PowerOfTwo(value float64) bool {
	integer := int64(value)
	return value == float64(integer) && integer > 0 && integer&(integer-1) == 0
}
