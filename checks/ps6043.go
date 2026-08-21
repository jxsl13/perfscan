package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6043 implements owner issue #735: accelerator leaf comparisons must
// declare operations per submission and working-set reuse before speedup.
var PS6043 = register(&lint.Check{
	ID:       "PS6043",
	Category: "verify",
	Slug:     "gpu-leaf-comparison-needs-submission-boundary",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "accelerator leaf benchmarks compare different submission boundaries",
		Text: `Duplicating one GPU operation thousands of times inside a graph
and dividing graph duration by node count measures amortized throughput. It is
not comparable to submitting and waiting once per operation. Reusing one
weight tensor can also turn streaming work into a cache-resident workload.

This check implements owner issue #735. It audits SubmissionBoundaryEvidence,
AcceleratorLeafComparison, GPUAmortizationReport, WorkingSetReuseEvidence, or
equivalent manifests. Both sides must record:

  - hardware, benchmark identity/revision, and workload shape;
  - operations per submission, submissions and waits per timed sample;
  - amortized per-operation and per-submission latency;
  - resident working-set bytes and weight/input reuse mode;
  - warmup/compilation boundary; and
  - independent-process latency distributions.

The manifest must also record boundary compatibility, whether a speedup was
computed, comparison classification, and final decision. Constant evidence is
checked for latency arithmetic, sparse/invalid process samples, and stale
compatibility. Different operations/submission, reuse, or warmup/compile
boundaries must suppress the cross-system speedup and be labeled boundary-
mismatched. There is NO automatic fix because graph construction, residency,
reuse, compilation, and timing distributions are runtime facts.`,
		Before: `speedup := oneSubmitPerOpUS / repeatedGraphAmortizedUS`,
		After: `evidence := SubmissionBoundaryEvidence{
	ControlOperationsPerSubmission: 65536,
	CandidateOperationsPerSubmission: 1,
	ControlAmortizedLatencyUS: 15.88,
	ControlSubmissionLatencyUS: 15.88 * 65536,
	CandidateAmortizedLatencyUS: 160.7335,
	CandidateSubmissionLatencyUS: 160.7335,
	BoundaryCompatible: false,
	SpeedupComputed: false,
	ComparisonClassification: "boundary-mismatched",
}`,
		MeasuredWin: `The Apple-M2-Pro case behind issue #735 reported 15.88
us/op after duplicating one Q4_K matvec 65,536 times per submitted graph. A
one-operation-per-submit diagnostic measured a ten-process median of 157.235
us/op; GoAI measured 160.7335 us/op at that matched boundary. The apparent
roughly 10x deficit was therefore mostly amortization; the matched difference
was about 2.2%.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6043",
		Doc:  "accelerator leaf comparison mixes submission/amortization boundaries",
		Run:  runPS6043,
	},
})

type ps6043Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6043Axes = []ps6043Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6043HardwareField) }},
	{name: "control benchmark identity/revision", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "control", "benchmark") })
	}},
	{name: "candidate benchmark identity/revision", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "candidate", "benchmark") })
	}},
	{name: "workload shape", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6043WorkloadField) }},
	{name: "control operations/submission", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "control", "operationspersubmission") })
	}},
	{name: "candidate operations/submission", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "candidate", "operationspersubmission") })
	}},
	{name: "control submissions/sample", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "control", "submissionspertimedsample") })
	}},
	{name: "candidate submissions/sample", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "candidate", "submissionspertimedsample") })
	}},
	{name: "control waits/sample", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "control", "waitspertimedsample") })
	}},
	{name: "candidate waits/sample", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "candidate", "waitspertimedsample") })
	}},
	{name: "control amortized latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043LatencyField(n, "control", "amortized") })
	}},
	{name: "candidate amortized latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043LatencyField(n, "candidate", "amortized") })
	}},
	{name: "control submission latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043LatencyField(n, "control", "submission") })
	}},
	{name: "candidate submission latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043LatencyField(n, "candidate", "submission") })
	}},
	{name: "control resident working-set bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "control", "residentworkingsetbyte") })
	}},
	{name: "candidate resident working-set bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043SideField(n, "candidate", "residentworkingsetbyte") })
	}},
	{name: "control weight/input reuse", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043ReuseField(n, "control") })
	}},
	{name: "candidate weight/input reuse", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043ReuseField(n, "candidate") })
	}},
	{name: "control warmup/compilation boundary", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043BoundaryField(n, "control") })
	}},
	{name: "candidate warmup/compilation boundary", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043BoundaryField(n, "candidate") })
	}},
	{name: "control independent-process distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043ProcessField(n, "control") })
	}},
	{name: "candidate independent-process distribution", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6043ProcessField(n, "candidate") })
	}},
	{name: "boundary compatibility", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6043CompatibilityField) }},
	{name: "speedup-computed status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6043SpeedupComputedField) }},
	{name: "comparison classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6043ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6043DecisionField) }},
}

type ps6043Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6043(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6043Context(text) {
				continue
			}
			manifest, found := ps6043BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "accelerator leaf comparison has no submission-boundary manifest; missing %s", strings.Join(ps6043Missing(nil), ", "))
				continue
			}
			if missing := ps6043Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "accelerator submission-boundary evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6043Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "accelerator submission-boundary audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6043Context(text string) bool {
	text = ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan", "accelerator")
	leaf := strings.Contains(text, "leaf")
	boundary := ps6007ContainsAny(text, "submissionboundary", "amortization", "workingsetreuse")
	return accelerator && leaf && boundary
}

func ps6043BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6043Manifest, bool) {
	var best ps6043Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6043ManifestType(lit.Type) {
			return true
		}
		manifest := ps6043Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6043Axes) - len(ps6043Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6043ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6043ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6043ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "submissionboundaryevidence", "acceleratorleafcomparison", "gpuamortizationreport", "workingsetreuseevidence", "submissionboundaryreport")
}

func ps6043Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6043Axes))
	for _, axis := range ps6043Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6043HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6043SideField(name, side, detail string) bool {
	return strings.Contains(name, side) && strings.Contains(name, detail)
}
func ps6043WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && strings.Contains(name, "shape")
}
func ps6043LatencyField(name, side, boundary string) bool {
	return strings.Contains(name, side) && strings.Contains(name, boundary) && strings.Contains(name, "latency")
}
func ps6043ReuseField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "reuse") && ps6007ContainsAny(name, "weight", "input", "workingset")
}
func ps6043BoundaryField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "boundary") && ps6007ContainsAny(name, "warmup", "compilation")
}
func ps6043ProcessField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "independentprocess") && ps6007ContainsAny(name, "latency", "latencies", "distribution", "sample")
}
func ps6043CompatibilityField(name string) bool {
	return strings.Contains(name, "boundary") && ps6007ContainsAny(name, "compatible", "compatibility", "matched")
}
func ps6043SpeedupComputedField(name string) bool {
	return strings.Contains(name, "speedup") && ps6007ContainsAny(name, "computed", "valid")
}
func ps6043ClassificationField(name string) bool {
	return strings.Contains(name, "comparison") && strings.Contains(name, "classification")
}
func ps6043DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6043Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	for _, side := range []string{"control", "candidate"} {
		operations, operationsOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, side, "operationspersubmission") })
		amortized, amortizedOK := ps6016Number(fields, func(n string) bool { return ps6043LatencyField(n, side, "amortized") })
		submission, submissionOK := ps6016Number(fields, func(n string) bool { return ps6043LatencyField(n, side, "submission") })
		if operationsOK && operations <= 0 {
			warnings = append(warnings, side+" operations/submission must be positive")
		}
		if operationsOK && amortizedOK && submissionOK && !ps6025Close(submission, amortized*operations) {
			warnings = append(warnings, fmt.Sprintf("%s submission latency %.6g disagrees with amortized*operations %.6g", side, submission, amortized*operations))
		}
		submissions, submissionsOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, side, "submissionspertimedsample") })
		waits, waitsOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, side, "waitspertimedsample") })
		if submissionsOK && submissions <= 0 {
			warnings = append(warnings, side+" submissions/sample must be positive")
		}
		if waitsOK && (waits <= 0 || submissionsOK && waits > submissions) {
			warnings = append(warnings, fmt.Sprintf("%s waits/sample %.6g is invalid for %.6g submissions", side, waits, submissions))
		}
		if samples, ok := ps6016Numbers(fields, func(n string) bool { return ps6043ProcessField(n, side) }); ok {
			if len(samples) < 3 {
				warnings = append(warnings, side+" independent-process distribution has fewer than three samples")
			}
			for _, sample := range samples {
				if sample <= 0 {
					warnings = append(warnings, side+" independent-process distribution contains a non-positive latency")
					break
				}
			}
		}
	}
	controlOperations, controlOperationsOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, "control", "operationspersubmission") })
	candidateOperations, candidateOperationsOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, "candidate", "operationspersubmission") })
	controlBytes, controlBytesOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, "control", "residentworkingsetbyte") })
	candidateBytes, candidateBytesOK := ps6016Number(fields, func(n string) bool { return ps6043SideField(n, "candidate", "residentworkingsetbyte") })
	controlReuse, controlReuseOK := ps6027String(fields, func(n string) bool { return ps6043ReuseField(n, "control") })
	candidateReuse, candidateReuseOK := ps6027String(fields, func(n string) bool { return ps6043ReuseField(n, "candidate") })
	controlBoundary, controlBoundaryOK := ps6027String(fields, func(n string) bool { return ps6043BoundaryField(n, "control") })
	candidateBoundary, candidateBoundaryOK := ps6027String(fields, func(n string) bool { return ps6043BoundaryField(n, "candidate") })
	mismatch := controlOperationsOK && candidateOperationsOK && controlOperations != candidateOperations ||
		controlBytesOK && candidateBytesOK && controlBytes != candidateBytes ||
		controlReuseOK && candidateReuseOK && ps6030StatusName(controlReuse) != ps6030StatusName(candidateReuse) ||
		controlBoundaryOK && candidateBoundaryOK && ps6030StatusName(controlBoundary) != ps6030StatusName(candidateBoundary)
	if compatible, ok := ps6026Bool(fields, ps6043CompatibilityField); ok && compatible == mismatch {
		warnings = append(warnings, fmt.Sprintf("boundary compatibility is %t but operations/reuse/working-set/warmup metadata computes %t", compatible, !mismatch))
	}
	if mismatch {
		if computed, ok := ps6026Bool(fields, ps6043SpeedupComputedField); ok && computed {
			warnings = append(warnings, "speedup is computed across mismatched submission or working-set boundaries")
		}
		if classification, ok := ps6027String(fields, ps6043ClassificationField); ok && !ps6007ContainsAny(ps6030StatusName(classification), "boundarymismatch", "amortizedonly", "incomparable") {
			warnings = append(warnings, fmt.Sprintf("comparison classification %q does not label the boundary mismatch", classification))
		}
		if decision, ok := ps6027String(fields, ps6043DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "accepted") {
			warnings = append(warnings, fmt.Sprintf("final decision %q accepts a boundary-mismatched cross-system comparison", decision))
		}
	}
	return warnings
}
