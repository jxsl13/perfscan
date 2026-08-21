package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6045 implements owner issue #733: cold lazy-pipeline profiler traces are
// topology evidence, not timing evidence, until the timed marker is reached.
var PS6045 = register(&lint.Check{
	ID:       "PS6045",
	Category: "verify",
	Slug:     "gpu-profiler-rejects-cold-pipeline-distortion",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a cold GPU profiler trace never reaches the timed region",
		Text: `External profilers can inflate lazy pipeline initialization so
severely that a capture contains encoder/submission tables but never reaches
the benchmark loop. Such a trace is useful for topology debugging and invalid
for timing attribution.

This check implements owner issue #733 for Metal, CUDA, Vulkan, and JIT-backed
graph runtimes. It audits ProfilerInitializationEvidence,
LazyPipelineCaptureReport, TimedRegionTraceGate, ColdLaunchTraceEvidence, or
equivalent manifests. Evidence must record:

  - hardware/OS, source revision, compiler identity, library mode, pipeline-
    cache state, and capture tool/version;
  - standalone and profiled initialization duration, censoring status,
    inflation ratio, and configured maximum;
  - capture duration, timed-region/progress marker text and reached status;
  - warmup, post-warmup activation, in-process trigger, and capture boundary;
  - encoder/submission schema visibility;
  - whether per-kernel timing was accepted and whether distorted
    initialization entered benchmark samples;
  - trace classification and final decision.

Constant evidence is checked for stale inflation arithmetic. Timing must be
rejected when the marker is absent, initialization is censored or exceeds the
gate, or capture starts cold. Cold-launch traces must be topology-only, and
profiled initialization must never enter benchmark samples. There is NO
automatic fix because capture activation, cache state, compiler behavior, and
target progress are runtime/tool facts.`,
		Before: `acceptKernelTimings(externalColdLaunchTrace)`,
		After: `evidence := TimedRegionTraceGate{
	StandaloneInitializationMS: 10.5,
	ProfiledInitializationMS: 120000,
	InitializationInflationRatio: 120000 / 10.5,
	TimedRegionMarker: "generation-loop-start",
	TimedRegionReached: false,
	PerKernelTimingAccepted: false,
	CaptureClassification: "topology-only",
}`,
		MeasuredWin: `In the Apple-M2-Pro case behind issue #733, the pinned
standalone benchmark initialized its embedded Metal library in roughly 10–11
ms. External Instruments captures, including a 120-second capture, stopped in
pipeline initialization before the llama-bench generation marker. Encoder and
submission schemas were visible, but no per-kernel timing distribution was
accepted; a prewarmed in-process capture was the required next boundary.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6045",
		Doc:  "GPU profiler trace accepts cold lazy-pipeline distortion as timing",
		Run:  runPS6045,
	},
})

type ps6045Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6045Axes = []ps6045Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045HardwareField) }},
	{name: "OS identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045OSField) }},
	{name: "source revision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045RevisionField) }},
	{name: "compiler identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045CompilerField) }},
	{name: "library mode", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045LibraryField) }},
	{name: "pipeline-cache state", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045CacheField) }},
	{name: "capture tool/version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045ToolField) }},
	{name: "standalone initialization duration", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6045InitField(n, "standalone") })
	}},
	{name: "profiled initialization duration", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6045InitField(n, "profiled") })
	}},
	{name: "profiled initialization censoring", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045CensoredField) }},
	{name: "initialization inflation ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045InflationField) }},
	{name: "maximum initialization inflation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045MaxInflationField) }},
	{name: "capture duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045DurationField) }},
	{name: "timed-region marker", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045MarkerField) }},
	{name: "timed-region reached status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045ReachedField) }},
	{name: "target progress output", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045ProgressField) }},
	{name: "warmup status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045WarmupField) }},
	{name: "post-warmup activation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045PostWarmupField) }},
	{name: "in-process capture trigger status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045InProcessField) }},
	{name: "capture boundary", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045BoundaryField) }},
	{name: "encoder/submission schema status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045SchemaField) }},
	{name: "per-kernel timing acceptance", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045TimingField) }},
	{name: "profiled-initialization sample mixing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045MixingField) }},
	{name: "capture classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6045DecisionField) }},
}

type ps6045Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6045(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6045Context(text) {
				continue
			}
			manifest, found := ps6045BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU lazy-pipeline capture has no timed-region trace gate; missing %s", strings.Join(ps6045Missing(nil), ", "))
				continue
			}
			if missing := ps6045Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU profiler initialization evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6045Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU profiler initialization audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6045Context(text string) bool {
	text = ps6007NormalizeName(text)
	gpu := ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan", "accelerator")
	profiler := ps6007ContainsAny(text, "profiler", "instruments", "capture")
	distortion := ps6007ContainsAny(text, "lazypipeline", "coldlaunch", "initialization", "timedregiontrace")
	return gpu && profiler && distortion
}

func ps6045BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6045Manifest, bool) {
	var best ps6045Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6045ManifestType(lit.Type) {
			return true
		}
		manifest := ps6045Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6045Axes) - len(ps6045Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6045ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6045ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6045ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "profilerinitializationevidence", "lazypipelinecapturereport", "timedregiontracegate", "coldlaunchtraceevidence", "profilerdistortionevidence")
}

func ps6045Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6045Axes))
	for _, axis := range ps6045Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6045HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6045OSField(name string) bool {
	return ps6007ContainsAny(name, "osidentity", "osversion", "macos")
}
func ps6045RevisionField(name string) bool {
	return ps6007ContainsAny(name, "sourcerevision", "backendrevision", "sourcecommit")
}
func ps6045CompilerField(name string) bool {
	return strings.Contains(name, "compiler") && ps6007ContainsAny(name, "identity", "commit", "version")
}
func ps6045LibraryField(name string) bool {
	return strings.Contains(name, "library") && strings.Contains(name, "mode")
}
func ps6045CacheField(name string) bool {
	return strings.Contains(name, "pipelinecache") && strings.Contains(name, "state")
}
func ps6045ToolField(name string) bool {
	return strings.Contains(name, "capturetool") && ps6007ContainsAny(name, "version", "identity")
}
func ps6045InitField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "initialization") && ps6007ContainsAny(name, "duration", "ms", "ns", "time") && !strings.Contains(name, "ratio")
}
func ps6045CensoredField(name string) bool {
	return strings.Contains(name, "profiledinitialization") && strings.Contains(name, "censored")
}
func ps6045InflationField(name string) bool {
	return strings.Contains(name, "initializationinflation") && strings.Contains(name, "ratio") && !ps6007ContainsAny(name, "maximum", "max", "limit")
}
func ps6045MaxInflationField(name string) bool {
	return strings.Contains(name, "initializationinflation") && strings.Contains(name, "ratio") && ps6007ContainsAny(name, "maximum", "max", "limit")
}
func ps6045DurationField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "duration")
}
func ps6045MarkerField(name string) bool {
	return strings.Contains(name, "timedregion") && strings.Contains(name, "marker") && !strings.Contains(name, "reached")
}
func ps6045ReachedField(name string) bool {
	return strings.Contains(name, "timedregion") && strings.Contains(name, "reached")
}
func ps6045ProgressField(name string) bool {
	return strings.Contains(name, "target") && strings.Contains(name, "progress") && strings.Contains(name, "output")
}
func ps6045WarmupField(name string) bool {
	return strings.Contains(name, "warmup") && ps6007ContainsAny(name, "completed", "passed", "status") && !strings.Contains(name, "activation")
}
func ps6045PostWarmupField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "activatedafterwarmup")
}
func ps6045InProcessField(name string) bool {
	return strings.Contains(name, "inprocess") && strings.Contains(name, "capturetrigger")
}
func ps6045BoundaryField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "boundary")
}
func ps6045SchemaField(name string) bool {
	return strings.Contains(name, "encoder") && strings.Contains(name, "submission") && strings.Contains(name, "schema")
}
func ps6045TimingField(name string) bool {
	return strings.Contains(name, "perkernel") && strings.Contains(name, "timing") && strings.Contains(name, "accepted")
}
func ps6045MixingField(name string) bool {
	return strings.Contains(name, "benchmarksample") && strings.Contains(name, "profiledinitialization")
}
func ps6045ClassificationField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "classification")
}
func ps6045DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6045Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	standalone, standaloneOK := ps6016Number(fields, func(n string) bool { return ps6045InitField(n, "standalone") })
	profiled, profiledOK := ps6016Number(fields, func(n string) bool { return ps6045InitField(n, "profiled") })
	inflation, inflationOK := ps6016Number(fields, ps6045InflationField)
	maxInflation, maxInflationOK := ps6016Number(fields, ps6045MaxInflationField)
	calculatedInflation, calculatedInflationOK := 0.0, standaloneOK && profiledOK && standalone > 0
	if calculatedInflationOK {
		calculatedInflation = profiled / standalone
		if inflationOK && !ps6025Close(inflation, calculatedInflation) {
			warnings = append(warnings, fmt.Sprintf("initialization inflation ratio %.6gx disagrees with profiled/standalone %.6gx", inflation, calculatedInflation))
		}
	}
	reached, reachedOK := ps6026Bool(fields, ps6045ReachedField)
	censored, censoredOK := ps6026Bool(fields, ps6045CensoredField)
	warmup, warmupOK := ps6026Bool(fields, ps6045WarmupField)
	postWarmup, postWarmupOK := ps6026Bool(fields, ps6045PostWarmupField)
	inProcess, inProcessOK := ps6026Bool(fields, ps6045InProcessField)
	timingAccepted, timingAcceptedOK := ps6026Bool(fields, ps6045TimingField)
	mixed, mixedOK := ps6026Bool(fields, ps6045MixingField)
	if mixedOK && mixed {
		warnings = append(warnings, "profiled initialization is mixed into benchmark samples")
	}
	invalidTiming := reachedOK && !reached || censoredOK && censored || calculatedInflationOK && maxInflationOK && calculatedInflation > maxInflation
	if timingAcceptedOK && timingAccepted {
		if reachedOK && !reached {
			warnings = append(warnings, "per-kernel timing is accepted although the timed-region marker was not reached")
		}
		if censoredOK && censored {
			warnings = append(warnings, "per-kernel timing is accepted from censored initialization")
		}
		if calculatedInflationOK && maxInflationOK && calculatedInflation > maxInflation {
			warnings = append(warnings, fmt.Sprintf("per-kernel timing is accepted despite %.6gx initialization inflation exceeding %.6gx", calculatedInflation, maxInflation))
		}
		if warmupOK && !warmup || postWarmupOK && !postWarmup || inProcessOK && !inProcess {
			warnings = append(warnings, "per-kernel timing is accepted without a prewarmed in-process activation boundary")
		}
	}
	boundary, boundaryOK := ps6027String(fields, ps6045BoundaryField)
	cold := boundaryOK && ps6007ContainsAny(ps6030StatusName(boundary), "cold", "externallaunch", "processlaunch")
	if cold {
		if classification, ok := ps6027String(fields, ps6045ClassificationField); ok && !ps6007ContainsAny(ps6030StatusName(classification), "topologyonly", "nontiming", "debug") {
			warnings = append(warnings, fmt.Sprintf("capture classification %q does not label a cold-launch trace topology-only", classification))
		}
	}
	if invalidTiming || cold {
		if decision, ok := ps6027String(fields, ps6045DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "accepttiming", "retain", "promote", "publishlatency") {
			warnings = append(warnings, fmt.Sprintf("final decision %q accepts timing from a distorted cold capture", decision))
		}
	}
	return warnings
}
