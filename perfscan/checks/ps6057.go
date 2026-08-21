package checks

import (
	"fmt"
	"go/ast"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6057 implements owner issue #721 for both typed static recorder graphs and
// structured stable-label evidence.
var PS6057 = register(&lint.Check{
	ID:       "PS6057",
	Category: "verify",
	Slug:     "repeated-gpu-producer-elementwise-boundary",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "repeated heavy-producer to tiny-elementwise GPU boundaries are fusion candidates",
		Text: `A command graph may repeatedly end a quantized matmul encoder only
to launch a small residual add, or run paired matmuls before a tiny SwiGLU.
Those producer-to-elementwise seams are useful fusion candidates because an
epilogue can sometimes remove an intermediate buffer boundary and dispatch.

This check implements owner issue #721 through two inputs:

  - typed static recorder graphs: within one function it requires at least two
    adjacent, same-recorder, single-consumer chains from a resolved quantized
    matmul/matvec/GEMM method to residual-add, elementwise-add, bias-add, or
    SwiGLU; paired producer results feeding one SwiGLU are recognized; and
  - StableEncoderBoundaryEvidence, ProducerElementwiseBoundaryReport,
    GPUFusionCandidateInventory, EncoderLabelBoundaryManifest, or equivalent
    manifests: stable labels, per-group counts/times, repeated boundary labels,
    occurrence counts, covered command time, dependency-wait disclosure, and
    end-to-end promotion state are audited.

Different command contexts, reused intermediates, ordinary CPU helpers, fused
method names, non-quantized producers, non-adjacent calls, and a lone boundary
stay silent. Static diagnostics report occurrence count and explicitly mark
covered time unavailable until stable labels are correlated.

Stage-boundary intervals include dependency wait and are not exclusive kernel
times. Occurrence count and covered command time rank experiments only. Every
candidate must pass a before/after end-to-end benchmark, output/allocation
gates appropriate to the repository, and a frozen threshold. There is NO
automatic fix because fusion changes GPU code, numerical order, hazards,
resource lifetimes, and command topology.`,
		Before: `tmp := rec.Q4KMatmul(x, w)
out := rec.ResidualAdd(tmp, residual)

gate := rec.Q4KMatmul(x, wg)
up := rec.Q4KMatmul(x, wu)
act := rec.SwiGLU(gate, up)`,
		After: `// Candidate only:
out := rec.Q4KMatmulResidual(x, w, residual)
act := rec.Q4KPairedSwiGLU(x, wg, wu)
// Retain only after a complete before/after end-to-end gate.`,
		MeasuredWin: `The TinyLlama Q4_K_M trace behind issue #721 contained 340
encoders in one command buffer. Stable groups covered 35.70 ms of a 37.97 ms
command: 66 binary boundaries covered 26.90 ms, 110 Q4_K matmuls 4.37 ms, 22
decode-attention encoders 2.31 ms, and 21 Q6_K matmuls 1.26 ms. The intervals
included dependency wait. They nominated matmul→residual and paired-matmul→
SwiGLU experiments but did not prove a win; an earlier Q4_K SwiGLU fusion was
negative.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6057",
		Doc:  "repeated GPU heavy-producer to elementwise boundary is a fusion candidate",
		Run:  runPS6057,
	},
})

type ps6057SourceProducer struct {
	call   *ast.CallExpr
	result types.Object
	name   string
	recv   string
	typeOf types.Type
}

type ps6057SourceConsumer struct {
	name string
	recv string
}

type ps6057SourceCandidate struct {
	pos       ast.Node
	signature string
}

func runPS6057(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ps6057ReportStatic(pass, fn)
			if !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6057EvidenceContext(text) {
				continue
			}
			manifest, found := ps6057BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU producer/elementwise label inventory has no boundary manifest; missing %s", strings.Join(ps6057Missing(nil), ", "))
				continue
			}
			if missing := ps6057Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU producer/elementwise boundary evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6057Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU producer/elementwise boundary audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6057ReportStatic(pass *analysis.Pass, fn *ast.FuncDecl) {
	var candidates []ps6057SourceCandidate
	ps6032Blocks(fn.Body, func(block *ast.BlockStmt) {
		for index := 0; index < len(block.List); index++ {
			if index+2 < len(block.List) {
				first, firstOK := ps6057ProducerStmt(pass, block.List[index])
				second, secondOK := ps6057ProducerStmt(pass, block.List[index+1])
				if firstOK && secondOK && first.recv != "" && first.recv == second.recv && ps6022CommandType(first.typeOf) && ps6022CommandType(second.typeOf) && ps6032SingleUse(pass, fn.Body, first.result) && ps6032SingleUse(pass, fn.Body, second.result) {
					consumer, ok := ps6057ConsumerStmt(pass, block.List[index+2], []types.Object{first.result, second.result})
					if ok && consumer.recv == first.recv && ps6057PairedConsumer(consumer.name) {
						candidates = append(candidates, ps6057SourceCandidate{pos: first.call, signature: first.name + "+" + second.name + " -> " + consumer.name})
						index += 2
						continue
					}
				}
			}
			if index+1 >= len(block.List) {
				continue
			}
			producer, ok := ps6057ProducerStmt(pass, block.List[index])
			if !ok || producer.recv == "" || !ps6022CommandType(producer.typeOf) || !ps6032SingleUse(pass, fn.Body, producer.result) {
				continue
			}
			consumer, ok := ps6057ConsumerStmt(pass, block.List[index+1], []types.Object{producer.result})
			if ok && consumer.recv == producer.recv {
				candidates = append(candidates, ps6057SourceCandidate{pos: producer.call, signature: producer.name + " -> " + consumer.name})
				index++
			}
		}
	})
	counts := make(map[string]int, len(candidates))
	first := make(map[string]ast.Node, len(candidates))
	for _, candidate := range candidates {
		counts[candidate.signature]++
		if first[candidate.signature] == nil {
			first[candidate.signature] = candidate.pos
		}
	}
	signatures := make([]string, 0, len(counts))
	for signature, count := range counts {
		if count >= 2 {
			signatures = append(signatures, signature)
		}
	}
	slices.Sort(signatures)
	for _, signature := range signatures {
		count := counts[signature]
		pass.Reportf(first[signature].Pos(), "static GPU recorder graph repeats %s %d times with adjacent single-consumer intermediates; rank an epilogue/fusion experiment, but covered command time is unavailable until stable labels are correlated, and retain only after a before/after end-to-end gate because boundary attribution is not a speedup", signature, count)
	}
}

func ps6057ProducerStmt(pass *analysis.Pass, statement ast.Stmt) (ps6057SourceProducer, bool) {
	assign, ok := statement.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return ps6057SourceProducer{}, false
	}
	id, ok := ps2110Unparen(assign.Lhs[0]).(*ast.Ident)
	if !ok || id.Name == "_" {
		return ps6057SourceProducer{}, false
	}
	call, ok := ps2110Unparen(assign.Rhs[0]).(*ast.CallExpr)
	if !ok {
		return ps6057SourceProducer{}, false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() == nil || !ps6057HeavyProducer(fn.Name()) {
		return ps6057SourceProducer{}, false
	}
	result := identObject(pass, id)
	if result == nil || !ps6020DataObject(result) {
		return ps6057SourceProducer{}, false
	}
	recv, recvType := ps6032Receiver(pass, call)
	return ps6057SourceProducer{call: call, result: result, name: fn.Name(), recv: recv, typeOf: recvType}, true
}

func ps6057ConsumerStmt(pass *analysis.Pass, statement ast.Stmt, results []types.Object) (ps6057SourceConsumer, bool) {
	call := ps6032StatementCall(statement)
	if call == nil {
		return ps6057SourceConsumer{}, false
	}
	fn, sig, ok := typedCallee(pass, call.Fun)
	if !ok || sig.Recv() == nil || !ps6057ElementwiseConsumer(fn.Name()) {
		return ps6057SourceConsumer{}, false
	}
	for _, result := range results {
		if !ps6032CallUses(pass, call, result) {
			return ps6057SourceConsumer{}, false
		}
	}
	recv, _ := ps6032Receiver(pass, call)
	return ps6057SourceConsumer{name: fn.Name(), recv: recv}, true
}

func ps6057HeavyProducer(name string) bool {
	name = ps6007NormalizeName(name)
	if strings.Contains(name, "fused") {
		return false
	}
	quantized := ps6007ContainsAny(name, "quant", "kquant", "q4", "q5", "q6", "q8")
	heavy := ps6007ContainsAny(name, "matmul", "matvec", "mulmat", "gemm", "projection")
	return quantized && heavy
}

func ps6057ElementwiseConsumer(name string) bool {
	name = ps6007NormalizeName(name)
	if strings.Contains(name, "fused") {
		return false
	}
	return ps6007ContainsAny(name, "residualadd", "addresidual", "elementwiseadd", "biasadd", "swiglu")
}

func ps6057PairedConsumer(name string) bool {
	return strings.Contains(ps6007NormalizeName(name), "swiglu")
}

type ps6057Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6057Axes = []ps6057Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057HardwareField) }},
	{name: "workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057WorkloadField) }},
	{name: "evidence source", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057SourceField) }},
	{name: "stable encoder labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057StableLabelsField) }},
	{name: "command encoder count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057EncoderCountField) }},
	{name: "command duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057CommandDurationField) }},
	{name: "total encoder interval time", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057IntervalTotalField) }},
	{name: "dependency-wait inclusion", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057WaitField) }},
	{name: "exclusive-kernel-time claim", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057ExclusiveField) }},
	{name: "encoder group labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057GroupLabelsField) }},
	{name: "encoder group occurrence counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057GroupCountsField) }},
	{name: "encoder group covered times", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057GroupTimesField) }},
	{name: "boundary chain labels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057BoundaryLabelsField) }},
	{name: "boundary occurrence counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057BoundaryCountsField) }},
	{name: "boundary covered times", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057BoundaryTimesField) }},
	{name: "repeated boundary total count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057BoundaryTotalCountField) }},
	{name: "repeated boundary covered time", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057BoundaryTotalTimeField) }},
	{name: "covered command-time fraction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057CoverageField) }},
	{name: "fusion experiment recommendation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057RecommendedField) }},
	{name: "end-to-end benchmark requirement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057RequiredField) }},
	{name: "end-to-end benchmark completion", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057CompletedField) }},
	{name: "end-to-end control time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6057EndTimeField(n, "control") })
	}},
	{name: "end-to-end candidate time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6057EndTimeField(n, "candidate") })
	}},
	{name: "end-to-end control/candidate ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057EndRatioField) }},
	{name: "promotion threshold", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057ThresholdField) }},
	{name: "candidate promotion status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057PromotedField) }},
	{name: "prior negative fusion disclosure", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057PriorNegativeField) }},
	{name: "candidate classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6057DecisionField) }},
}

type ps6057Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func ps6057EvidenceContext(text string) bool {
	text = ps6007NormalizeName(text)
	return ps6007ContainsAny(text, "gpu", "metal", "cuda", "vulkan") && ps6007ContainsAny(text, "stableencoderlabel", "producerElementwise", "fusioncandidateinventory", "encoderlabelboundary") && ps6007ContainsAny(text, "boundarycandidate", "producerboundary", "elementwiseboundary")
}

func ps6057BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6057Manifest, bool) {
	var best ps6057Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6057ManifestType(lit.Type) {
			return true
		}
		manifest := ps6057Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6057Axes) - len(ps6057Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6057ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6057ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6057ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "stableencoderboundaryevidence", "producerelementwiseboundaryreport", "gpufusioncandidateinventory", "encoderlabelboundarymanifest", "producerboundaryevidence")
}

func ps6057Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6057Axes))
	for _, axis := range ps6057Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6057HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6057WorkloadField(n string) bool {
	return strings.Contains(n, "workload") && ps6007ContainsAny(n, "identity", "name", "shape")
}
func ps6057SourceField(n string) bool       { return strings.Contains(n, "evidencesource") }
func ps6057StableLabelsField(n string) bool { return strings.Contains(n, "stableencoderlabel") }
func ps6057EncoderCountField(n string) bool {
	return strings.Contains(n, "commandencoder") && strings.Contains(n, "count")
}
func ps6057CommandDurationField(n string) bool { return strings.Contains(n, "commandduration") }
func ps6057IntervalTotalField(n string) bool   { return strings.Contains(n, "totalencoderinterval") }
func ps6057WaitField(n string) bool {
	return strings.Contains(n, "interval") && strings.Contains(n, "dependencywait") && ps6007ContainsAny(n, "include", "disclosed")
}
func ps6057ExclusiveField(n string) bool {
	return strings.Contains(n, "exclusivekerneltime") && ps6007ContainsAny(n, "claimed", "claim")
}
func ps6057GroupLabelsField(n string) bool {
	return strings.Contains(n, "encodergroup") && strings.Contains(n, "label")
}
func ps6057GroupCountsField(n string) bool {
	return strings.Contains(n, "encodergroup") && strings.Contains(n, "occurrencecount")
}
func ps6057GroupTimesField(n string) bool {
	return strings.Contains(n, "encodergroup") && strings.Contains(n, "coveredtime")
}
func ps6057BoundaryLabelsField(n string) bool {
	return strings.Contains(n, "boundarychain") && strings.Contains(n, "label")
}
func ps6057BoundaryCountsField(n string) bool {
	return strings.Contains(n, "boundary") && strings.Contains(n, "occurrencecount") && !strings.Contains(n, "total")
}
func ps6057BoundaryTimesField(n string) bool {
	return strings.Contains(n, "boundary") && strings.Contains(n, "coveredtime") && !strings.Contains(n, "total")
}
func ps6057BoundaryTotalCountField(n string) bool {
	return strings.Contains(n, "repeatedboundarytotalcount")
}
func ps6057BoundaryTotalTimeField(n string) bool {
	return strings.Contains(n, "repeatedboundarycoveredtime")
}
func ps6057CoverageField(n string) bool { return strings.Contains(n, "coveredcommandtimefraction") }
func ps6057RecommendedField(n string) bool {
	return strings.Contains(n, "fusionexperiment") && strings.Contains(n, "recommended")
}
func ps6057RequiredField(n string) bool {
	return strings.Contains(n, "endtoendbenchmark") && strings.Contains(n, "required")
}
func ps6057CompletedField(n string) bool {
	return strings.Contains(n, "endtoendbenchmark") && strings.Contains(n, "completed")
}
func ps6057EndTimeField(n, side string) bool {
	return strings.Contains(n, "endtoend") && strings.Contains(n, side) && ps6007ContainsAny(n, "ns", "time") && !strings.Contains(n, "ratio")
}
func ps6057EndRatioField(n string) bool {
	return strings.Contains(n, "endtoend") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6057ThresholdField(n string) bool {
	return strings.Contains(n, "promotion") && ps6007ContainsAny(n, "threshold", "minimum", "gate")
}
func ps6057PromotedField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "promoted")
}
func ps6057PriorNegativeField(n string) bool {
	return strings.Contains(n, "priornegativefusion") && ps6007ContainsAny(n, "disclosed", "recorded")
}
func ps6057ClassificationField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "classification")
}
func ps6057DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6057Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"stable encoder labels", ps6057StableLabelsField},
		{"dependency-wait inclusion", ps6057WaitField},
		{"fusion experiment recommendation", ps6057RecommendedField},
		{"end-to-end benchmark requirement", ps6057RequiredField},
		{"prior negative fusion disclosure", ps6057PriorNegativeField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	if exclusive, ok := ps6026Bool(fields, ps6057ExclusiveField); ok && exclusive {
		warnings = append(warnings, "dependency-charged encoder intervals are claimed as exclusive kernel time")
	}

	groupLabels, groupLabelsOK := ps6047Strings(fields, ps6057GroupLabelsField)
	groupCounts, groupCountsOK := ps6016Numbers(fields, ps6057GroupCountsField)
	groupTimes, groupTimesOK := ps6016Numbers(fields, ps6057GroupTimesField)
	if groupLabelsOK && groupCountsOK && groupTimesOK && (len(groupLabels) != len(groupCounts) || len(groupLabels) != len(groupTimes)) {
		warnings = append(warnings, "encoder group label/count/time vectors have different lengths")
	}
	encoderCount, encoderCountOK := ps6016Number(fields, ps6057EncoderCountField)
	if groupCountsOK && encoderCountOK && !ps6025Close(ps6057Sum(groupCounts), encoderCount) {
		warnings = append(warnings, fmt.Sprintf("encoder group occurrences %.6g disagree with command encoder count %.6g", ps6057Sum(groupCounts), encoderCount))
	}
	intervalTotal, intervalTotalOK := ps6016Number(fields, ps6057IntervalTotalField)
	if groupTimesOK && intervalTotalOK && !ps6025Close(ps6057Sum(groupTimes), intervalTotal) {
		warnings = append(warnings, fmt.Sprintf("encoder group covered times %.6g disagree with total encoder intervals %.6g", ps6057Sum(groupTimes), intervalTotal))
	}

	boundaryLabels, boundaryLabelsOK := ps6047Strings(fields, ps6057BoundaryLabelsField)
	boundaryCounts, boundaryCountsOK := ps6016Numbers(fields, ps6057BoundaryCountsField)
	boundaryTimes, boundaryTimesOK := ps6016Numbers(fields, ps6057BoundaryTimesField)
	if boundaryLabelsOK && boundaryCountsOK && boundaryTimesOK && (len(boundaryLabels) != len(boundaryCounts) || len(boundaryLabels) != len(boundaryTimes)) {
		warnings = append(warnings, "boundary label/count/time vectors have different lengths")
	}
	boundaryTotalCount, boundaryTotalCountOK := ps6016Number(fields, ps6057BoundaryTotalCountField)
	if boundaryCountsOK && boundaryTotalCountOK && !ps6025Close(ps6057Sum(boundaryCounts), boundaryTotalCount) {
		warnings = append(warnings, fmt.Sprintf("boundary occurrence sum %.6g disagrees with total %.6g", ps6057Sum(boundaryCounts), boundaryTotalCount))
	}
	boundaryTotalTime, boundaryTotalTimeOK := ps6016Number(fields, ps6057BoundaryTotalTimeField)
	if boundaryTimesOK && boundaryTotalTimeOK && !ps6025Close(ps6057Sum(boundaryTimes), boundaryTotalTime) {
		warnings = append(warnings, fmt.Sprintf("boundary covered-time sum %.6g disagrees with total %.6g", ps6057Sum(boundaryTimes), boundaryTotalTime))
	}
	commandDuration, commandDurationOK := ps6016Number(fields, ps6057CommandDurationField)
	if recorded, ok := ps6016Number(fields, ps6057CoverageField); ok && boundaryTotalTimeOK && commandDurationOK && commandDuration > 0 && !ps6025Close(recorded, boundaryTotalTime/commandDuration) {
		warnings = append(warnings, fmt.Sprintf("covered command-time fraction %.6g disagrees with %.6g", recorded, boundaryTotalTime/commandDuration))
	}

	completed, completedOK := ps6026Bool(fields, ps6057CompletedField)
	control, controlOK := ps6016Number(fields, func(n string) bool { return ps6057EndTimeField(n, "control") })
	candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6057EndTimeField(n, "candidate") })
	ratio, ratioOK := 0.0, completedOK && completed && controlOK && candidateOK && candidate > 0
	if ratioOK {
		ratio = control / candidate
		if recorded, ok := ps6016Number(fields, ps6057EndRatioField); ok && !ps6025Close(recorded, ratio) {
			warnings = append(warnings, fmt.Sprintf("end-to-end control/candidate ratio %.6gx disagrees with %.6gx", recorded, ratio))
		}
	}
	threshold, thresholdOK := ps6016Number(fields, ps6057ThresholdField)
	passed := ratioOK && thresholdOK && ratio >= threshold
	if promoted, ok := ps6026Bool(fields, ps6057PromotedField); ok && promoted && !passed {
		warnings = append(warnings, "fusion candidate is promoted without a completed end-to-end benchmark above threshold")
	}
	if decision, ok := ps6027String(fields, ps6057DecisionField); ok && !passed && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
		warnings = append(warnings, fmt.Sprintf("final decision %q treats boundary attribution as a proven performance win", decision))
	}
	return warnings
}

func ps6057Sum(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}
