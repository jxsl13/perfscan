package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6049 implements owner issue #729: xctrace Metal Performance Limiters
// extraction must stream, discover tables semantically, and fail closed on ROI.
var PS6049 = register(&lint.Check{
	ID:       "PS6049",
	Category: "verify",
	Slug:     "xctrace-metal-limiters-needs-streaming-roi",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an xctrace Metal counter probe uses an empty instrument or ambiguous ROI",
		Text: `The CLI-selectable Metal GPU Counters instrument can expose
Performance Limiters that are absent from the public counter-sample-buffer
surface. A default Game Performance capture, table-order assumption, or
in-memory XML load can silently produce empty, wrong, or hostile evidence.

This check implements owner issue #729. It audits XctraceMetalLimiterEvidence,
StreamingCounterProbeReport, MetalCounterROIGate,
PerformanceLimiterCaptureEvidence, or equivalent manifests. Evidence must
record:

  - tool/OS/device identity and explicit Metal GPU Counters plus Metal
    Application instrument selection;
  - fixed command/arguments, iterations, buffers/iteration, capture duration,
    and external raw-output location;
  - enumeration of every counter-info table, semantic required-name matching,
    unique-table selection, and independence from profile numbers/order;
  - raw value bytes, streaming parser status, peak parser memory, and complete
    id/ref resolution;
  - observed/expected/final selected command-buffer pairs, ID pairing,
    completion integrity, and calibration/warmup exclusion;
  - continuous wall ROI, union-of-lifetimes ROI, and duty cycle;
  - counter names plus aligned sample-count/min/mean/max vectors;
  - device-wide contamination acknowledgment, compact JSON provenance, raw
    retention policy, classification, and final decision.

Constant evidence is checked for count/ROI arithmetic, ambiguous semantic
tables, unresolved references, incomplete pairs, zero sample counts, invalid
counter summaries, and unbounded parser memory. There is NO automatic fix
because xctrace schemas, command-buffer identity, device activity, and counter
samples are external runtime/tool evidence.`,
		Before: `xctrace record --template "Game Performance" // Counter Set: (null)
xml.Unmarshal(hugeCounterValueXML, &allRows)`,
		After: `report := StreamingCounterProbeReport{
	MetalGPUCountersInstrumentSelected: true,
	CounterInfoTablesEnumerated: 2,
	SelectedTableMatchedRequiredNames: true,
	StreamingParserUsed: true,
	ObservedCommandBufferPairs: 10041,
	ExpectedTimedCommandBufferPairs: 10000,
	SelectedFinalCommandBufferPairs: 10000,
	WallROIDurationNS: wall, UnionROIDurationNS: active,
	DutyCycle: active / wall,
}`,
		MeasuredWin: `The Apple-M2-Pro prototype behind issue #729 streamed a
358 MB XML export and selected exactly 10,000 timed buffers from 10,041
observed. Q4_K duty cycle was 96.44% with 19.65% buffer-read and 18.74% ALU
limiters; Q6_K duty cycle was 96.72% with 25.51% ALU and 13.61% buffer-read.
Semantic table discovery avoided a schema-only sibling table whose order and
profile labels were not stable.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6049",
		Doc:  "xctrace Metal limiter probe lacks streaming semantic ROI evidence",
		Run:  runPS6049,
	},
})

type ps6049Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6049Axes = []ps6049Axis{
	{name: "tool identity/version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ToolField) }},
	{name: "OS identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049OSField) }},
	{name: "device identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049DeviceField) }},
	{name: "Metal GPU Counters availability", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049AvailableField) }},
	{name: "explicit Metal GPU Counters selection", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049CountersSelectedField) }},
	{name: "Metal Application selection", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ApplicationSelectedField) }},
	{name: "null counter-set avoidance", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049NullAvoidedField) }},
	{name: "fixed command", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049CommandField) }},
	{name: "fixed arguments", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ArgumentsField) }},
	{name: "fixed iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049IterationsField) }},
	{name: "buffers per iteration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049BuffersField) }},
	{name: "capture duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049CaptureDurationField) }},
	{name: "external raw-output location", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049RawLocationField) }},
	{name: "counter-info table count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049TableCountField) }},
	{name: "all counter-info tables enumerated", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049EnumeratedField) }},
	{name: "required counter names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049RequiredNamesField) }},
	{name: "semantic table matches", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049SemanticMatchesField) }},
	{name: "unique semantic selection", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049UniqueField) }},
	{name: "profile/order independence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049OrderIndependentField) }},
	{name: "raw counter-value bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049RawBytesField) }},
	{name: "streaming parser status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049StreamingField) }},
	{name: "peak parser memory", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049PeakMemoryField) }},
	{name: "id/ref resolution", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ReferencesField) }},
	{name: "observed command-buffer pairs", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ObservedPairsField) }},
	{name: "expected timed command-buffer pairs", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ExpectedPairsField) }},
	{name: "selected final command-buffer pairs", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049SelectedPairsField) }},
	{name: "command-buffer ID pairing", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049IDPairingField) }},
	{name: "completion integrity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049CompletionsField) }},
	{name: "calibration/warmup exclusion", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ExclusionField) }},
	{name: "wall ROI duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049WallField) }},
	{name: "union ROI duration", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049UnionField) }},
	{name: "command-buffer duty cycle", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049DutyField) }},
	{name: "counter names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049CounterNamesField) }},
	{name: "counter sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049SampleCountsField) }},
	{name: "counter minimums", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049MinimumsField) }},
	{name: "counter means", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049MeansField) }},
	{name: "counter maximums", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049MaximumsField) }},
	{name: "device-wide contamination acknowledgment", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ContaminationField) }},
	{name: "compact JSON identity/hash", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049JSONField) }},
	{name: "raw retention policy", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049RetentionField) }},
	{name: "raw cleanup status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049CleanupField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6049DecisionField) }},
}

type ps6049Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6049(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6049Context(text) {
				continue
			}
			manifest, found := ps6049BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "xctrace Metal limiter probe has no streaming ROI manifest; missing %s", strings.Join(ps6049Missing(nil), ", "))
				continue
			}
			if missing := ps6049Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "xctrace Metal limiter evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6049Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "xctrace Metal limiter audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6049Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "xctrace") && strings.Contains(text, "metal") &&
		ps6007ContainsAny(text, "limiter", "counter") && ps6007ContainsAny(text, "streaming", "roi", "probe")
}

func ps6049BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6049Manifest, bool) {
	var best ps6049Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6049ManifestType(lit.Type) {
			return true
		}
		manifest := ps6049Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6049Axes) - len(ps6049Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6049ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6049ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6049ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "xctracemetallimiterevidence", "streamingcounterprobereport", "metalcounterroigate", "performancelimitercaptureevidence", "xctracelimiterreport")
}

func ps6049Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6049Axes))
	for _, axis := range ps6049Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6049ToolField(n string) bool {
	return strings.Contains(n, "xctrace") && ps6007ContainsAny(n, "version", "identity")
}
func ps6049OSField(n string) bool { return ps6007ContainsAny(n, "osidentity", "osversion", "macos") }
func ps6049DeviceField(n string) bool {
	return strings.Contains(n, "device") && strings.Contains(n, "identity")
}
func ps6049AvailableField(n string) bool {
	return strings.Contains(n, "metalgpucountersinstrument") && strings.Contains(n, "available")
}
func ps6049CountersSelectedField(n string) bool {
	return strings.Contains(n, "metalgpucountersinstrument") && strings.Contains(n, "selected")
}
func ps6049ApplicationSelectedField(n string) bool {
	return strings.Contains(n, "metalapplicationinstrument") && strings.Contains(n, "selected")
}
func ps6049NullAvoidedField(n string) bool {
	return strings.Contains(n, "nullcounterset") && strings.Contains(n, "avoided")
}
func ps6049CommandField(n string) bool {
	return strings.Contains(n, "fixed") && strings.Contains(n, "command")
}
func ps6049ArgumentsField(n string) bool {
	return strings.Contains(n, "fixed") && strings.Contains(n, "argument")
}
func ps6049IterationsField(n string) bool {
	return strings.Contains(n, "fixediteration") && strings.Contains(n, "count")
}
func ps6049BuffersField(n string) bool { return strings.Contains(n, "buffersperiteration") }
func ps6049CaptureDurationField(n string) bool {
	return strings.Contains(n, "capture") && strings.Contains(n, "duration")
}
func ps6049RawLocationField(n string) bool {
	return strings.Contains(n, "raw") && strings.Contains(n, "external") && ps6007ContainsAny(n, "location", "directory", "path")
}
func ps6049TableCountField(n string) bool {
	return strings.Contains(n, "counterinfotable") && strings.Contains(n, "count")
}
func ps6049EnumeratedField(n string) bool {
	return strings.Contains(n, "counterinfotable") && strings.Contains(n, "enumerated")
}
func ps6049RequiredNamesField(n string) bool {
	return strings.Contains(n, "requiredcounter") && strings.Contains(n, "name")
}
func ps6049SemanticMatchesField(n string) bool {
	return strings.Contains(n, "semantic") && strings.Contains(n, "table") && strings.Contains(n, "match")
}
func ps6049UniqueField(n string) bool {
	return strings.Contains(n, "unique") && strings.Contains(n, "semantic") && strings.Contains(n, "table")
}
func ps6049OrderIndependentField(n string) bool {
	return strings.Contains(n, "profile") && strings.Contains(n, "order") && strings.Contains(n, "independent")
}
func ps6049RawBytesField(n string) bool {
	return strings.Contains(n, "rawcountervalue") && strings.Contains(n, "byte")
}
func ps6049StreamingField(n string) bool {
	return strings.Contains(n, "streamingparser") && ps6007ContainsAny(n, "used", "status", "passed")
}
func ps6049PeakMemoryField(n string) bool {
	return strings.Contains(n, "peakparsermemory") && strings.Contains(n, "byte")
}
func ps6049ReferencesField(n string) bool {
	return strings.Contains(n, "idref") && strings.Contains(n, "resolved")
}
func ps6049ObservedPairsField(n string) bool { return strings.Contains(n, "observedcommandbufferpair") }
func ps6049ExpectedPairsField(n string) bool {
	return strings.Contains(n, "expectedtimedcommandbufferpair")
}
func ps6049SelectedPairsField(n string) bool {
	return strings.Contains(n, "selectedfinalcommandbufferpair")
}
func ps6049IDPairingField(n string) bool {
	return strings.Contains(n, "commandbufferid") && strings.Contains(n, "paired")
}
func ps6049CompletionsField(n string) bool {
	return strings.Contains(n, "selectedcompletion") && strings.Contains(n, "present")
}
func ps6049ExclusionField(n string) bool {
	return strings.Contains(n, "calibration") && strings.Contains(n, "warmup") && strings.Contains(n, "excluded")
}
func ps6049WallField(n string) bool {
	return strings.Contains(n, "wallroi") && strings.Contains(n, "duration")
}
func ps6049UnionField(n string) bool {
	return strings.Contains(n, "unionroi") && strings.Contains(n, "duration")
}
func ps6049DutyField(n string) bool { return strings.Contains(n, "dutycycle") }
func ps6049CounterNamesField(n string) bool {
	return strings.Contains(n, "selectedcounter") && strings.Contains(n, "name")
}
func ps6049SampleCountsField(n string) bool {
	return strings.Contains(n, "counter") && strings.Contains(n, "samplecount")
}
func ps6049MinimumsField(n string) bool {
	return strings.Contains(n, "counter") && strings.Contains(n, "minimum")
}
func ps6049MeansField(n string) bool {
	return strings.Contains(n, "counter") && strings.Contains(n, "mean")
}
func ps6049MaximumsField(n string) bool {
	return strings.Contains(n, "counter") && strings.Contains(n, "maximum")
}
func ps6049ContaminationField(n string) bool {
	return strings.Contains(n, "devicewide") && strings.Contains(n, "contamination") && strings.Contains(n, "acknowledged")
}
func ps6049JSONField(n string) bool {
	return strings.Contains(n, "compactjson") && ps6007ContainsAny(n, "hash", "identity", "path")
}
func ps6049RetentionField(n string) bool {
	return strings.Contains(n, "raw") && strings.Contains(n, "retentionpolicy")
}
func ps6049CleanupField(n string) bool {
	return strings.Contains(n, "raw") && strings.Contains(n, "cleanup")
}
func ps6049ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6049DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6049Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 16)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"Metal GPU Counters availability", ps6049AvailableField}, {"explicit Metal GPU Counters selection", ps6049CountersSelectedField},
		{"Metal Application selection", ps6049ApplicationSelectedField}, {"null counter-set avoidance", ps6049NullAvoidedField},
		{"all counter-info table enumeration", ps6049EnumeratedField}, {"unique semantic table selection", ps6049UniqueField},
		{"profile/order independence", ps6049OrderIndependentField}, {"streaming parser", ps6049StreamingField},
		{"id/ref resolution", ps6049ReferencesField}, {"command-buffer ID pairing", ps6049IDPairingField},
		{"selected completion integrity", ps6049CompletionsField}, {"calibration/warmup exclusion", ps6049ExclusionField},
		{"device-wide contamination acknowledgment", ps6049ContaminationField}, {"raw cleanup", ps6049CleanupField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	tableCount, tableCountOK := ps6016Number(fields, ps6049TableCountField)
	semanticMatches, semanticMatchesOK := ps6016Number(fields, ps6049SemanticMatchesField)
	if tableCountOK && tableCount <= 0 {
		warnings = append(warnings, "counter-info table count must be positive")
	}
	if semanticMatchesOK && semanticMatches != 1 {
		warnings = append(warnings, fmt.Sprintf("semantic counter-info discovery found %.0f matching tables, want exactly one", semanticMatches))
	}
	rawBytes, rawBytesOK := ps6016Number(fields, ps6049RawBytesField)
	peakMemory, peakMemoryOK := ps6016Number(fields, ps6049PeakMemoryField)
	if rawBytesOK && peakMemoryOK && rawBytes > 0 && peakMemory >= rawBytes {
		warnings = append(warnings, fmt.Sprintf("peak parser memory %.6g bytes is not bounded below raw %.6g-byte export", peakMemory, rawBytes))
	}
	iterations, iterationsOK := ps6016Number(fields, ps6049IterationsField)
	buffers, buffersOK := ps6016Number(fields, ps6049BuffersField)
	expected, expectedOK := ps6016Number(fields, ps6049ExpectedPairsField)
	observed, observedOK := ps6016Number(fields, ps6049ObservedPairsField)
	selected, selectedOK := ps6016Number(fields, ps6049SelectedPairsField)
	if iterationsOK && buffersOK && expectedOK && expected != iterations*buffers {
		warnings = append(warnings, fmt.Sprintf("expected timed pairs %.0f disagree with iterations*buffers %.0f", expected, iterations*buffers))
	}
	if expectedOK && selectedOK && selected != expected {
		warnings = append(warnings, fmt.Sprintf("selected final pairs %.0f disagree with expected %.0f", selected, expected))
	}
	if observedOK && selectedOK && observed < selected {
		warnings = append(warnings, fmt.Sprintf("only %.0f command-buffer pairs observed for %.0f selected", observed, selected))
	}
	wall, wallOK := ps6016Number(fields, ps6049WallField)
	union, unionOK := ps6016Number(fields, ps6049UnionField)
	duty, dutyOK := ps6016Number(fields, ps6049DutyField)
	if wallOK && unionOK && wall > 0 {
		if union > wall {
			warnings = append(warnings, "union ROI duration exceeds continuous wall ROI")
		}
		if dutyOK && !ps6025Close(duty, union/wall) {
			warnings = append(warnings, fmt.Sprintf("duty cycle %.6g disagrees with union/wall %.6g", duty, union/wall))
		}
	}
	names, namesOK := ps6047Strings(fields, ps6049CounterNamesField)
	counts, countsOK := ps6016Numbers(fields, ps6049SampleCountsField)
	minimums, minimumsOK := ps6016Numbers(fields, ps6049MinimumsField)
	means, meansOK := ps6016Numbers(fields, ps6049MeansField)
	maximums, maximumsOK := ps6016Numbers(fields, ps6049MaximumsField)
	if namesOK && countsOK && minimumsOK && meansOK && maximumsOK {
		length := len(names)
		if len(counts) != length || len(minimums) != length || len(means) != length || len(maximums) != length {
			warnings = append(warnings, "counter name/count/min/mean/max vectors have different lengths")
		} else {
			if slices.ContainsFunc(counts, func(value float64) bool { return value <= 0 }) {
				warnings = append(warnings, "one or more selected counters have zero samples")
			}
			for index := range names {
				if minimums[index] > means[index] || means[index] > maximums[index] {
					warnings = append(warnings, fmt.Sprintf("counter %q has invalid min/mean/max ordering", names[index]))
					break
				}
			}
		}
	}
	return warnings
}
