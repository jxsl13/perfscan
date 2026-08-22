package checks

import (
	"fmt"
	"go/ast"
	"math"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6050 implements owner issue #728: immutable GPU-read is insufficient to
// recommend private Metal buffers on Apple unified memory.
var PS6050 = register(&lint.Check{
	ID:       "PS6050",
	Category: "verify",
	Slug:     "apple-unified-buffer-private-storage-needs-cost-model",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "private Metal storage is recommended without a unified-memory cost model",
		Text: `Private storage can help some resources, especially textures,
GPU-produced temporaries, or discrete-memory systems. It is not a generic win
for immutable CPU-populated buffers on Apple unified memory: staging, blit,
wait, and transient bytes are real costs.

This check implements owner issue #728. It audits MetalResourceModeEvidence,
UnifiedMemoryBufferReport, SharedPrivateStorageGate,
PrivateBufferCostEvidence, or equivalent manifests. Evidence must record:

  - hardware family, unified/discrete memory, resource type, producer,
    mutability, access pattern/frequency, and post-initialization CPU access;
  - control/candidate storage modes, mandatory staging/blit/wait statuses,
    and private-only feature documentation;
  - payload plus control/candidate transient and persistent Metal bytes;
  - incident shapes/ratios and overall/Q4_K/Q6_K geometric means;
  - primary control/candidate time and ratio;
  - shared/private upload time, ratio, and overhead fraction;
  - leaf and upload allocation bytes/counts;
  - private-mode non-vacuity, odd-tail parity, local benchmark/counter evidence,
    recommendation status, classification, and final decision.

Constant evidence is checked for stale geometric means, ratios, upload
overhead, byte accounting, allocation changes, failed parity/non-vacuity, and
private recommendations contradicted by local evidence or unsupported by a
private-only feature. There is NO automatic fix: textures, GPU-produced data,
discrete memory, platform counters, and ownership lifetimes have different
economics.`,
		Before: `if immutable && gpuReadOnly { copySharedBufferToPrivate() }`,
		After: `evidence := UnifiedMemoryBufferReport{
	MemoryArchitecture: "Apple unified memory",
	ResourceType: "buffer", Producer: "CPU-populated",
	CandidateRequiresStaging: true,
	CandidateRequiresBlit: true, CandidateRequiresWait: true,
	OverallWarmGeomeanSpeedup: 1.005,
	UploadControlCandidateRatio: 0.565,
	PrivateStorageRecommended: false,
}`,
		MeasuredWin: `The Apple-M2-Pro experiment behind issue #728 measured
1.005x warm geomean over 11 Q4_K/Q6_K shapes, while the primary Q4_K shape
regressed from 180,659 to 197,212 ns/op (0.916x). A 6,488,064-byte upload
regressed from 668,883 to 1,184,567 ns/op (0.565x, 77.10% more time) and held
2x transient Metal bytes until blit completion. The candidate was removed.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6050",
		Doc:  "private Metal buffer recommendation lacks unified-memory cost evidence",
		Run:  runPS6050,
	},
})

type ps6050Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6050Axes = []ps6050Axis{
	{name: "hardware family", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050HardwareField) }},
	{name: "memory architecture", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050MemoryField) }},
	{name: "resource type", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050ResourceField) }},
	{name: "resource producer", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050ProducerField) }},
	{name: "mutability", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050MutabilityField) }},
	{name: "access pattern", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050AccessPatternField) }},
	{name: "access frequency", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050AccessFrequencyField) }},
	{name: "post-initialization CPU access", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050CPUAfterInitField) }},
	{name: "control storage mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050StorageField(n, "control") })
	}},
	{name: "candidate storage mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050StorageField(n, "candidate") })
	}},
	{name: "candidate staging requirement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050StagingField) }},
	{name: "candidate blit requirement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050BlitField) }},
	{name: "candidate wait requirement", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050WaitField) }},
	{name: "private-only feature documentation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050FeatureField) }},
	{name: "payload bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050PayloadField) }},
	{name: "control transient Metal bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050MetalBytesField(n, "control", "transient") })
	}},
	{name: "candidate transient Metal bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050MetalBytesField(n, "candidate", "transient") })
	}},
	{name: "control persistent Metal bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050MetalBytesField(n, "control", "persistent") })
	}},
	{name: "candidate persistent Metal bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050MetalBytesField(n, "candidate", "persistent") })
	}},
	{name: "incident shapes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050ShapesField) }},
	{name: "incident ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050RatiosField) }},
	{name: "Q4_K incident ratios", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050QuantRatiosField(n, "q4k") })
	}},
	{name: "Q6_K incident ratios", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050QuantRatiosField(n, "q6k") })
	}},
	{name: "overall warm geometric mean", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050OverallGeomeanField) }},
	{name: "Q4_K geometric mean", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050QuantGeomeanField(n, "q4k") })
	}},
	{name: "Q6_K geometric mean", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050QuantGeomeanField(n, "q6k") })
	}},
	{name: "primary control time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050PrimaryTimeField(n, "control") })
	}},
	{name: "primary candidate time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050PrimaryTimeField(n, "candidate") })
	}},
	{name: "primary ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050PrimaryRatioField) }},
	{name: "shared upload time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050UploadTimeField(n, "shared") })
	}},
	{name: "private upload time", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050UploadTimeField(n, "private") })
	}},
	{name: "upload ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050UploadRatioField) }},
	{name: "upload overhead fraction", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050UploadOverheadField) }},
	{name: "leaf control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "leaf", "control", "byte") })
	}},
	{name: "leaf candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "leaf", "candidate", "byte") })
	}},
	{name: "leaf control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "leaf", "control", "count") })
	}},
	{name: "leaf candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "leaf", "candidate", "count") })
	}},
	{name: "upload control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "upload", "control", "byte") })
	}},
	{name: "upload candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "upload", "candidate", "byte") })
	}},
	{name: "upload control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "upload", "control", "count") })
	}},
	{name: "upload candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6050AllocationField(n, "upload", "candidate", "count") })
	}},
	{name: "private-mode non-vacuity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050NonvacuityField) }},
	{name: "odd-tail parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050ParityField) }},
	{name: "local benchmark evidence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050BenchmarkEvidenceField) }},
	{name: "counter evidence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050CounterEvidenceField) }},
	{name: "private-storage recommendation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050RecommendationField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6050DecisionField) }},
}

type ps6050Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6050(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6050Context(text) {
				continue
			}
			manifest, found := ps6050BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "Apple unified-memory private-buffer campaign has no resource-mode manifest; missing %s", strings.Join(ps6050Missing(nil), ", "))
				continue
			}
			if missing := ps6050Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal resource-mode evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6050Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal resource-mode audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6050Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "metal") && strings.Contains(text, "private") &&
		ps6007ContainsAny(text, "unifiedmemory", "resourcemode", "sharedprivate") && strings.Contains(text, "buffer")
}

func ps6050BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6050Manifest, bool) {
	var best ps6050Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6050ManifestType(lit.Type) {
			return true
		}
		manifest := ps6050Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6050Axes) - len(ps6050Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6050ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6050ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6050ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "metalresourcemodeevidence", "unifiedmemorybufferreport", "sharedprivatestoragegate", "privatebuffercostevidence", "metalstoragecomparison")
}

func ps6050Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6050Axes))
	for _, axis := range ps6050Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6050HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardwarefamily", "hardware", "deviceidentity")
}
func ps6050MemoryField(n string) bool {
	return strings.Contains(n, "memory") && strings.Contains(n, "architecture")
}
func ps6050ResourceField(n string) bool {
	return strings.Contains(n, "resource") && strings.Contains(n, "type")
}
func ps6050ProducerField(n string) bool        { return strings.Contains(n, "producer") }
func ps6050MutabilityField(n string) bool      { return strings.Contains(n, "mutability") }
func ps6050AccessPatternField(n string) bool   { return strings.Contains(n, "accesspattern") }
func ps6050AccessFrequencyField(n string) bool { return strings.Contains(n, "accessfrequency") }
func ps6050CPUAfterInitField(n string) bool {
	return strings.Contains(n, "cpuaccessafterinitialization")
}
func ps6050StorageField(n, side string) bool {
	return strings.Contains(n, side) && strings.Contains(n, "storagemode")
}
func ps6050StagingField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "staging") && strings.Contains(n, "requires")
}
func ps6050BlitField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "blit") && strings.Contains(n, "requires")
}
func ps6050WaitField(n string) bool {
	return strings.Contains(n, "candidate") && strings.Contains(n, "wait") && strings.Contains(n, "requires")
}
func ps6050FeatureField(n string) bool {
	return strings.Contains(n, "privateonlyfeature") && strings.Contains(n, "documented")
}
func ps6050PayloadField(n string) bool {
	return strings.Contains(n, "payload") && strings.Contains(n, "byte")
}
func ps6050MetalBytesField(n, side, lifetime string) bool {
	return strings.Contains(n, side) && strings.Contains(n, lifetime) && strings.Contains(n, "metalbyte")
}
func ps6050ShapesField(n string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, "shape")
}
func ps6050RatiosField(n string) bool {
	return strings.Contains(n, "incident") && strings.Contains(n, "ratio") && !ps6007ContainsAny(n, "q4k", "q6k")
}
func ps6050QuantRatiosField(n, quant string) bool {
	return strings.Contains(n, quant) && strings.Contains(n, "incident") && strings.Contains(n, "ratio")
}
func ps6050OverallGeomeanField(n string) bool {
	return strings.Contains(n, "overallwarm") && strings.Contains(n, "geomean")
}
func ps6050QuantGeomeanField(n, quant string) bool {
	return strings.Contains(n, quant) && strings.Contains(n, "geomean")
}
func ps6050PrimaryTimeField(n, side string) bool {
	return strings.Contains(n, "primary") && strings.Contains(n, side) && ps6007ContainsAny(n, "ns", "time", "latency") && !strings.Contains(n, "ratio")
}
func ps6050PrimaryRatioField(n string) bool {
	return strings.Contains(n, "primary") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6050UploadTimeField(n, mode string) bool {
	return strings.Contains(n, "upload") && strings.Contains(n, mode) && ps6007ContainsAny(n, "ns", "time", "latency") && !strings.Contains(n, "ratio")
}
func ps6050UploadRatioField(n string) bool {
	return strings.Contains(n, "upload") && strings.Contains(n, "controlcandidate") && strings.Contains(n, "ratio")
}
func ps6050UploadOverheadField(n string) bool {
	return strings.Contains(n, "upload") && strings.Contains(n, "overhead")
}
func ps6050AllocationField(n, scope, side, detail string) bool {
	return strings.Contains(n, scope) && strings.Contains(n, side) && strings.Contains(n, "allocation") && strings.Contains(n, detail)
}
func ps6050NonvacuityField(n string) bool {
	return strings.Contains(n, "privatemode") && ps6007ContainsAny(n, "observed", "nonvacuous")
}
func ps6050ParityField(n string) bool {
	return strings.Contains(n, "oddtail") && strings.Contains(n, "parity")
}
func ps6050BenchmarkEvidenceField(n string) bool {
	return strings.Contains(n, "localbenchmark") && strings.Contains(n, "evidence")
}
func ps6050CounterEvidenceField(n string) bool {
	return strings.Contains(n, "counter") && strings.Contains(n, "evidence")
}
func ps6050RecommendationField(n string) bool {
	return strings.Contains(n, "privatestorage") && strings.Contains(n, "recommended")
}
func ps6050ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6050DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6050Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"private-mode non-vacuity", ps6050NonvacuityField}, {"odd-tail parity", ps6050ParityField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	payload, payloadOK := ps6016Number(fields, ps6050PayloadField)
	controlTransient, controlTransientOK := ps6016Number(fields, func(n string) bool { return ps6050MetalBytesField(n, "control", "transient") })
	candidateTransient, candidateTransientOK := ps6016Number(fields, func(n string) bool { return ps6050MetalBytesField(n, "candidate", "transient") })
	controlPersistent, controlPersistentOK := ps6016Number(fields, func(n string) bool { return ps6050MetalBytesField(n, "control", "persistent") })
	candidatePersistent, candidatePersistentOK := ps6016Number(fields, func(n string) bool { return ps6050MetalBytesField(n, "candidate", "persistent") })
	if payloadOK && controlTransientOK && controlTransient != payload {
		warnings = append(warnings, fmt.Sprintf("control transient Metal bytes %.6g disagree with payload %.6g", controlTransient, payload))
	}
	if payloadOK && candidateTransientOK && candidateTransient != 2*payload {
		warnings = append(warnings, fmt.Sprintf("candidate transient Metal bytes %.6g do not account for 2x staging payload %.6g", candidateTransient, 2*payload))
	}
	if controlPersistentOK && candidatePersistentOK && controlPersistent != candidatePersistent {
		warnings = append(warnings, "control/candidate persistent Metal bytes differ")
	}
	shapes, shapesOK := ps6047Strings(fields, ps6050ShapesField)
	ratios, ratiosOK := ps6016Numbers(fields, ps6050RatiosField)
	if shapesOK && ratiosOK && len(shapes) != len(ratios) {
		warnings = append(warnings, "incident shape and ratio vectors have different lengths")
	}
	for _, group := range []struct {
		name    string
		values  func(string) bool
		geomean func(string) bool
	}{
		{"overall", ps6050RatiosField, ps6050OverallGeomeanField},
		{"Q4_K", func(n string) bool { return ps6050QuantRatiosField(n, "q4k") }, func(n string) bool { return ps6050QuantGeomeanField(n, "q4k") }},
		{"Q6_K", func(n string) bool { return ps6050QuantRatiosField(n, "q6k") }, func(n string) bool { return ps6050QuantGeomeanField(n, "q6k") }},
	} {
		values, valuesOK := ps6016Numbers(fields, group.values)
		recorded, recordedOK := ps6016Number(fields, group.geomean)
		if valuesOK && recordedOK {
			calculated, ok := ps6050Geomean(values)
			if !ok {
				warnings = append(warnings, group.name+" geometric mean has non-positive input")
			} else if !ps6025Close(recorded, calculated) {
				warnings = append(warnings, fmt.Sprintf("%s geometric mean %.6gx disagrees with %.6gx", group.name, recorded, calculated))
			}
		}
	}
	primaryControl, primaryControlOK := ps6016Number(fields, func(n string) bool { return ps6050PrimaryTimeField(n, "control") })
	primaryCandidate, primaryCandidateOK := ps6016Number(fields, func(n string) bool { return ps6050PrimaryTimeField(n, "candidate") })
	if primaryControlOK && primaryCandidateOK && primaryCandidate > 0 {
		if recorded, ok := ps6016Number(fields, ps6050PrimaryRatioField); ok && !ps6025Close(recorded, primaryControl/primaryCandidate) {
			warnings = append(warnings, fmt.Sprintf("primary ratio %.6gx disagrees with control/candidate %.6gx", recorded, primaryControl/primaryCandidate))
		}
	}
	sharedUpload, sharedUploadOK := ps6016Number(fields, func(n string) bool { return ps6050UploadTimeField(n, "shared") })
	privateUpload, privateUploadOK := ps6016Number(fields, func(n string) bool { return ps6050UploadTimeField(n, "private") })
	if sharedUploadOK && privateUploadOK && sharedUpload > 0 && privateUpload > 0 {
		if recorded, ok := ps6016Number(fields, ps6050UploadRatioField); ok && !ps6025Close(recorded, sharedUpload/privateUpload) {
			warnings = append(warnings, fmt.Sprintf("upload ratio %.6gx disagrees with shared/private %.6gx", recorded, sharedUpload/privateUpload))
		}
		if recorded, ok := ps6016Number(fields, ps6050UploadOverheadField); ok && !ps6025Close(recorded, privateUpload/sharedUpload-1) {
			warnings = append(warnings, fmt.Sprintf("upload overhead %.6g disagrees with measured %.6g", recorded, privateUpload/sharedUpload-1))
		}
	}
	for _, scope := range []string{"leaf", "upload"} {
		for _, detail := range []string{"byte", "count"} {
			control, controlOK := ps6016Number(fields, func(n string) bool { return ps6050AllocationField(n, scope, "control", detail) })
			candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6050AllocationField(n, scope, "candidate", detail) })
			if controlOK && candidateOK && control != candidate {
				warnings = append(warnings, scope+" control/candidate allocation "+detail+"s differ")
			}
		}
	}
	if recommended, ok := ps6026Bool(fields, ps6050RecommendationField); ok && recommended {
		memory, _ := ps6027String(fields, ps6050MemoryField)
		resource, _ := ps6027String(fields, ps6050ResourceField)
		producer, _ := ps6027String(fields, ps6050ProducerField)
		feature, featureOK := ps6026Bool(fields, ps6050FeatureField)
		benchmark, benchmarkOK := ps6026Bool(fields, ps6050BenchmarkEvidenceField)
		unifiedCPUBuffer := strings.Contains(ps6030StatusName(memory), "unified") && strings.Contains(ps6030StatusName(resource), "buffer") && strings.Contains(ps6030StatusName(producer), "cpu")
		localNegative := ratiosOK && len(ratios) > 0 && slicesMin(ratios) < 1
		if unifiedCPUBuffer && (!featureOK || !feature) && (!benchmarkOK || !benchmark || localNegative) {
			warnings = append(warnings, "private storage is recommended for a CPU-populated unified-memory buffer without supporting local or private-only evidence")
		}
	}
	return warnings
}

func ps6050Geomean(values []float64) (float64, bool) {
	logSum := 0.0
	for _, value := range values {
		if value <= 0 {
			return 0, false
		}
		logSum += math.Log(value)
	}
	return math.Exp(logSum / float64(len(values))), len(values) > 0
}

func slicesMin(values []float64) float64 {
	minimum := values[0]
	for _, value := range values[1:] {
		minimum = min(minimum, value)
	}
	return minimum
}
