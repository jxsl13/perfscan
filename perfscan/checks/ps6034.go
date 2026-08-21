package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6034 implements owner issue #744: a kernel-throughput claim must separate
// host recording, device interval, and synchronized wall time across cold/warm
// states and both one-submit and amortized command depths.
var PS6034 = register(&lint.Check{
	ID:       "PS6034",
	Category: "verify",
	Slug:     "metal-throughput-needs-depth-and-warmup-decomposition",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a Metal kernel-throughput claim conflates recording, submission, GPU time, or warm-up",
		Text: `A one-submit accelerator microbenchmark can invert the apparent
winner when Objective-C encoding, command submission, synchronization, and GPU
power-state ramp-up dominate a small kernel. Synchronized wall time alone does
not identify an arithmetic-kernel deficit.

This check implements owner issue #744. It audits MetalMeasurementEvidence,
CommandDepthTimingEvidence, HostGPUWallDecomposition, SustainedWarmupEvidence,
or equivalent manifests. Every sample row must retain aligned vectors for:

  - exact shape and implementation labels;
  - command depth;
  - host recording, GPUStartTime-to-GPUEndTime, and synchronized wall
    nanoseconds per dispatch;
  - warm-up duration and dispatch count;
  - cold/warm state; and
  - fresh-process sample identity.

The manifest must predeclare a sustained warm-up minimum and amortized command
depth, include both depth one and the amortized depth for every shape/
implementation group, and record fresh-process pair count, resident-input,
exact-shape, timestamp, synchronization, alternation, process independence,
exact-output, DVFS stability, and submission-amortization disclosure statuses.

Constant evidence is checked for vector alignment, non-positive or impossible
timings, duplicate process identities, under-warmed/cold samples, and missing
one-submit/amortized cells. Any of those facts blocks a kernel-throughput claim.

There is NO automatic fix. Device timestamp domains, power state, command
submission, synchronization, resident resources, and independent process
sampling are runtime evidence.`,
		Before: `BenchmarkQ4KOneSubmit(b) // one short synchronized wall number`,
		After: `evidence := MetalMeasurementEvidence{
	SampleShapeLabels: []string{"K2048,N2048", "K2048,N2048"},
	SampleCommandDepths: []int{1, 1024},
	HostRecordingNSPerDispatch: recording,
	GPUIntervalNSPerDispatch: gpuIntervals,
	SynchronizedWallNSPerDispatch: wall,
	WarmupDurationNS: []int64{1e9, 1e9},
	ColdWarmStates: []string{"warm", "warm"},
	FreshProcessSampleIDs: processIDs,
}`,
		MeasuredWin: `On Apple M2 Pro in issue #744, GoAI Q4_K measured 21.176
us synchronized wall and 16.102 us GPU per dispatch, with 4.409 us attributable
to recording. Without sustained warm-up the GPU interval varied from roughly
14 to 29 us and falsely suggested a 1.37x shader deficit. After equal one-
second warm-up and 1,024 dispatches per command, the remaining target was
command construction rather than Q4_K arithmetic.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6034",
		Doc:  "Metal kernel-throughput evidence lacks host/GPU/wall, command-depth, or warm-up separation",
		Run:  runPS6034,
	},
})

type ps6034Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6034Axes = []ps6034Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034HardwareField) }},
	{name: "sample shape labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ShapesField) }},
	{name: "sample implementation labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ImplementationsField) }},
	{name: "sample command depths", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034DepthsField) }},
	{name: "host recording time per dispatch", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034RecordingField) }},
	{name: "GPU interval time per dispatch", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034GPUField) }},
	{name: "synchronized wall time per dispatch", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034WallField) }},
	{name: "warm-up durations", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034WarmupDurationField) }},
	{name: "warm-up dispatch counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034WarmupDispatchField) }},
	{name: "cold/warm states", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034StatesField) }},
	{name: "fresh-process sample identities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ProcessIDsField) }},
	{name: "minimum sustained warm-up", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034MinimumWarmupField) }},
	{name: "minimum amortized command depth", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034MinimumDepthField) }},
	{name: "fresh-process pair count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034PairsField) }},
	{name: "resident-input status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ResidentField) }},
	{name: "exact-shape status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ExactShapeField) }},
	{name: "GPU timestamp status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034TimestampField) }},
	{name: "synchronization status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034SynchronizationField) }},
	{name: "alternating-order status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034AlternatingField) }},
	{name: "fresh-process independence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034IndependenceField) }},
	{name: "exact-output status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ExactOutputField) }},
	{name: "DVFS stability status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034DVFSField) }},
	{name: "submission-amortization disclosure", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034DisclosureField) }},
	{name: "kernel-throughput claim status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6034ClaimField) }},
}

type ps6034Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6034(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6034Context(text) {
				continue
			}
			manifest, found := ps6034BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "Metal command-depth/warm-up harness has no decomposed timing manifest; missing %s", strings.Join(ps6034Missing(nil), ", "))
				continue
			}
			if missing := ps6034Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal depth/warm-up evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6034Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal depth/warm-up audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6034Context(text string) bool {
	text = ps6007NormalizeName(text)
	metal := ps6007ContainsAny(text, "metal", "applegpu")
	depth := ps6007ContainsAny(text, "commanddepth", "dispatchdepth", "amortizeddepth", "onesubmit")
	warmup := ps6007ContainsAny(text, "warmup", "sustainedwarm", "dvfs", "coldwarm")
	timing := ps6007ContainsAny(text, "hostgpuwall", "recordinggpuwall", "gpuinterval", "synchronizedwall")
	return metal && depth && warmup && timing
}

func ps6034BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6034Manifest, bool) {
	var best ps6034Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6034ManifestType(lit.Type) {
			return true
		}
		manifest := ps6034Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6034Axes) - len(ps6034Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6034ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6034ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6034ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "metalmeasurementevidence", "commanddepthtimingevidence", "hostgpuwalldecomposition", "sustainedwarmupevidence", "depthwarmupcampaign")
}

func ps6034Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6034Axes))
	for _, axis := range ps6034Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6034HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6034ShapesField(name string) bool {
	return strings.Contains(name, "sample") && strings.Contains(name, "shape") && ps6007ContainsAny(name, "labels", "names", "identities")
}
func ps6034ImplementationsField(name string) bool {
	return strings.Contains(name, "sample") && strings.Contains(name, "implementation") && ps6007ContainsAny(name, "labels", "names", "identities")
}
func ps6034DepthsField(name string) bool {
	return strings.Contains(name, "sample") && strings.Contains(name, "commanddepth")
}
func ps6034RecordingField(name string) bool {
	return strings.Contains(name, "host") && strings.Contains(name, "recording") && strings.Contains(name, "perdispatch")
}
func ps6034GPUField(name string) bool {
	return strings.Contains(name, "gpuinterval") && strings.Contains(name, "perdispatch")
}
func ps6034WallField(name string) bool {
	return strings.Contains(name, "synchronizedwall") && strings.Contains(name, "perdispatch")
}
func ps6034WarmupDurationField(name string) bool {
	return strings.Contains(name, "warmup") && ps6007ContainsAny(name, "duration", "ns", "window") && !strings.Contains(name, "minimum")
}
func ps6034WarmupDispatchField(name string) bool {
	return strings.Contains(name, "warmup") && strings.Contains(name, "dispatch") && strings.Contains(name, "count")
}
func ps6034StatesField(name string) bool {
	return ps6007ContainsAny(name, "coldwarmstates", "warmstates", "powerstates")
}
func ps6034ProcessIDsField(name string) bool {
	return strings.Contains(name, "freshprocess") && strings.Contains(name, "sample") && ps6007ContainsAny(name, "ids", "identities")
}
func ps6034MinimumWarmupField(name string) bool {
	return strings.Contains(name, "minimum") && strings.Contains(name, "sustained") && strings.Contains(name, "warmup")
}
func ps6034MinimumDepthField(name string) bool {
	return strings.Contains(name, "minimum") && strings.Contains(name, "amortized") && strings.Contains(name, "depth")
}
func ps6034PairsField(name string) bool {
	return strings.Contains(name, "freshprocess") && strings.Contains(name, "pair") && strings.Contains(name, "count")
}
func ps6034ResidentField(name string) bool {
	return strings.Contains(name, "resident") && ps6007ContainsAny(name, "input", "resource", "weight")
}
func ps6034ExactShapeField(name string) bool {
	return strings.Contains(name, "exactshape") && ps6007ContainsAny(name, "passed", "status", "matched")
}
func ps6034TimestampField(name string) bool {
	return strings.Contains(name, "gpu") && strings.Contains(name, "timestamp") && ps6007ContainsAny(name, "passed", "used", "status")
}
func ps6034SynchronizationField(name string) bool {
	return strings.Contains(name, "synchron") && ps6007ContainsAny(name, "passed", "status", "complete")
}
func ps6034AlternatingField(name string) bool {
	return strings.Contains(name, "alternating") && ps6007ContainsAny(name, "order", "passed", "status")
}
func ps6034IndependenceField(name string) bool {
	return strings.Contains(name, "freshprocess") && strings.Contains(name, "independent")
}
func ps6034ExactOutputField(name string) bool {
	return strings.Contains(name, "exactoutput") && ps6007ContainsAny(name, "passed", "digest", "status")
}
func ps6034DVFSField(name string) bool {
	return strings.Contains(name, "dvfs") && ps6007ContainsAny(name, "stable", "passed", "status")
}
func ps6034DisclosureField(name string) bool {
	return strings.Contains(name, "submission") && strings.Contains(name, "amortization") && strings.Contains(name, "disclosed")
}
func ps6034ClaimField(name string) bool {
	return strings.Contains(name, "kernelthroughput") && ps6007ContainsAny(name, "claimed", "claim", "status")
}

func ps6034Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"resident-input gate", ps6034ResidentField},
		{"exact-shape gate", ps6034ExactShapeField},
		{"GPU timestamp gate", ps6034TimestampField},
		{"synchronization gate", ps6034SynchronizationField},
		{"alternating-order gate", ps6034AlternatingField},
		{"fresh-process independence", ps6034IndependenceField},
		{"exact-output gate", ps6034ExactOutputField},
		{"DVFS stability gate", ps6034DVFSField},
		{"submission-amortization disclosure", ps6034DisclosureField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	shapes := ps6030Strings(fields, ps6034ShapesField)
	implementations := ps6030Strings(fields, ps6034ImplementationsField)
	depths, _ := ps6016Numbers(fields, ps6034DepthsField)
	recording, _ := ps6016Numbers(fields, ps6034RecordingField)
	gpu, _ := ps6016Numbers(fields, ps6034GPUField)
	wall, _ := ps6016Numbers(fields, ps6034WallField)
	warmup, _ := ps6016Numbers(fields, ps6034WarmupDurationField)
	warmupDispatches, _ := ps6016Numbers(fields, ps6034WarmupDispatchField)
	states := ps6030Strings(fields, ps6034StatesField)
	processIDs := ps6030Strings(fields, ps6034ProcessIDsField)
	length := len(depths)
	aligned := length == len(shapes) && length == len(implementations) && length == len(recording) && length == len(gpu) && length == len(wall) && length == len(warmup) && length == len(warmupDispatches) && length == len(states) && length == len(processIDs)
	if !aligned {
		warnings = append(warnings, "shape/implementation/depth/recording/GPU/wall/warm-up/state/process vectors have different lengths")
		return warnings
	}
	minimumWarmup, minimumWarmupOK := ps6016Number(fields, ps6034MinimumWarmupField)
	minimumDepth, minimumDepthOK := ps6016Number(fields, ps6034MinimumDepthField)
	claimed, claimedOK := ps6026Bool(fields, ps6034ClaimField)
	groups := make(map[string][]float64, len(depths))
	seenProcesses := make(map[string]bool, len(processIDs))
	for i := range depths {
		label := shapes[i] + "/" + implementations[i]
		groups[label] = append(groups[label], depths[i])
		if recording[i] <= 0 || gpu[i] <= 0 || wall[i] <= 0 {
			warnings = append(warnings, "sample "+strconv.Itoa(i)+" has non-positive recording/GPU/wall time")
		}
		if wall[i] < gpu[i] {
			warnings = append(warnings, fmt.Sprintf("sample %d synchronized wall %.6g ns is below GPU interval %.6g ns", i, wall[i], gpu[i]))
		}
		if claimedOK && claimed && minimumWarmupOK && warmup[i] < minimumWarmup {
			warnings = append(warnings, fmt.Sprintf("sample %d warm-up %.6g ns is below sustained %.6g ns minimum", i, warmup[i], minimumWarmup))
		}
		if claimedOK && claimed && !strings.Contains(ps6030StatusName(states[i]), "warm") {
			warnings = append(warnings, fmt.Sprintf("sample %d is labeled %q during a kernel-throughput claim", i, states[i]))
		}
		if seenProcesses[processIDs[i]] {
			warnings = append(warnings, fmt.Sprintf("fresh-process sample identity %q is duplicated", processIDs[i]))
		}
		seenProcesses[processIDs[i]] = true
	}
	if claimedOK && claimed && minimumDepthOK {
		for label, groupDepths := range groups {
			if !slices.Contains(groupDepths, 1) || !ps6034HasAmortizedDepth(groupDepths, minimumDepth) {
				warnings = append(warnings, fmt.Sprintf("shape/implementation %q lacks both depth 1 and amortized depth >= %.0f", label, minimumDepth))
			}
		}
	}
	if pairs, ok := ps6016Number(fields, ps6034PairsField); ok && pairs < 1 {
		warnings = append(warnings, "fresh-process pair count must be positive")
	}
	return warnings
}

func ps6034HasAmortizedDepth(depths []float64, minimum float64) bool {
	for _, depth := range depths {
		if depth >= minimum {
			return true
		}
	}
	return false
}
