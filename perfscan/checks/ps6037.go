package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6037 implements owner issue #741: external GPU profiler captures need
// environment, privacy-safe staging, volume, contamination, and retry-bias
// boundaries.
var PS6037 = register(&lint.Check{
	ID:       "PS6037",
	Category: "verify",
	Slug:     "external-gpu-profiler-capture-policy",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an external GPU-counter capture lacks environment, privacy, volume, or contamination gates",
		Text: `External profiler launches do not necessarily inherit the caller
environment, may encounter macOS privacy boundaries, can make their own
exporter exhaust memory before a streaming parser runs, and may expose device-
wide counter activity from unrelated processes.

This check implements owner issue #741. It audits ExternalProfilerEvidence,
XctraceCapturePolicy, GPUCounterCaptureEvidence, TraceCaptureContract, or
equivalent manifests. A capture policy must record:

  - profiler identity, explicit environment allowlist, and forwarded names;
  - source/staged input paths, source/staged SHA-256, staging status, and a
    trace-safe-path status;
  - command-buffer, capture-byte, and export-byte estimates and hard limits;
  - raw-trace path/status outside the repository and compact-report-only
    publication status;
  - counter names, semantic policies, sample counts, active means, and
    ceilings;
  - required accepted reports, attempted/rejected/accepted counts, rejection
    reasons, independent-report status, median aggregation, and result-emitted
    status.

Constant evidence rejects forwarded variables outside the allowlist, staged
hash mismatch, unsafe staging, capture/export estimates above their cap,
required empty streams, contamination sentinels above their ceiling,
inconsistent retry accounting, publication before the accepted-report target,
non-independent aggregation, repository-local raw traces, or non-compact
publication.

There is NO automatic fix. Environment values, TCC/privacy access, input
staging, external exporter behavior, device-wide counters, and raw artifact
retention are runtime/operational evidence.`,
		Before: `cmd := exec.Command("xctrace", "record", "--launch", benchmark)
// inherited env, Desktop input, unbounded 200-buffer export, device-wide data`,
		After: `evidence := XctraceCapturePolicy{
	EnvironmentAllowlist: []string{"MODEL_PATH"},
	ForwardedEnvironmentNames: []string{"MODEL_PATH"},
	SourceSHA256: sourceHash, StagedSHA256: stagedHash,
	CaptureCommandBufferLimit: 1,
	CounterPolicies: []string{"required", "contamination-sentinel"},
	CounterSampleCounts: counts, CounterActiveMeans: means,
	CounterCeilings: ceilings,
	RawTraceOutsideRepository: true, CompactReportOnlyPublished: true,
}`,
		MeasuredWin: `In issue #741, five independent accepted one-buffer Apple-
M2 traces produced 10,446 samples per counter. Two additional attempts were
rejected for Fragment Occupancy at 2.188452% and 0.121756% against a 0.1%
ceiling. A 200-buffer export expanded to multi-gigabyte XML and was killed;
the bounded short-report policy avoided that unrecoverable exporter boundary.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6037",
		Doc:  "external GPU profiler capture lacks environment, staging, volume, contamination, or retry gates",
		Run:  runPS6037,
	},
})

type ps6037Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6037Axes = []ps6037Axis{
	{name: "profiler identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037ProfilerField) }},
	{name: "environment allowlist", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037AllowlistField) }},
	{name: "forwarded environment names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037ForwardedField) }},
	{name: "source input path", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037SourcePathField) }},
	{name: "staged input path", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037StagedPathField) }},
	{name: "source SHA-256", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037SourceHashField) }},
	{name: "staged SHA-256", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037StagedHashField) }},
	{name: "staging status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037StagingField) }},
	{name: "trace-safe-path status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037SafePathField) }},
	{name: "capture command-buffer limit", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CommandLimitField) }},
	{name: "capture command-buffer count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CommandCountField) }},
	{name: "capture byte estimate", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CaptureEstimateField) }},
	{name: "capture byte limit", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CaptureLimitField) }},
	{name: "export byte estimate", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037ExportEstimateField) }},
	{name: "export byte limit", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037ExportLimitField) }},
	{name: "raw trace path", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037RawPathField) }},
	{name: "raw trace outside-repository status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037RawExternalField) }},
	{name: "compact report path", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CompactPathField) }},
	{name: "compact-only publication status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CompactOnlyField) }},
	{name: "counter names", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CounterNamesField) }},
	{name: "counter policies", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CounterPoliciesField) }},
	{name: "counter sample counts", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CounterCountsField) }},
	{name: "counter active means", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CounterMeansField) }},
	{name: "counter ceilings", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037CounterCeilingsField) }},
	{name: "required accepted-report count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037RequiredField) }},
	{name: "attempted capture count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037AttemptedField) }},
	{name: "rejected capture count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037RejectedField) }},
	{name: "accepted capture count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037AcceptedField) }},
	{name: "capture rejection reasons", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037ReasonsField) }},
	{name: "independent-report status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037IndependentField) }},
	{name: "median aggregation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037MedianField) }},
	{name: "result-emitted status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6037EmittedField) }},
}

type ps6037Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6037(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6037Context(text) {
				continue
			}
			manifest, found := ps6037BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "external GPU-profiler harness has no bounded capture-policy manifest; missing %s", strings.Join(ps6037Missing(nil), ", "))
				continue
			}
			if missing := ps6037Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "external profiler capture evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6037Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "external profiler capture audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6037Context(text string) bool {
	text = ps6007NormalizeName(text)
	external := ps6007ContainsAny(text, "xctrace", "externalprofiler", "tracecapture")
	gpu := ps6007ContainsAny(text, "gpu", "metal", "counter")
	policy := ps6007ContainsAny(text, "capturepolicy", "capturecontract", "environmentallowlist", "exportvolume")
	return external && gpu && policy
}

func ps6037BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6037Manifest, bool) {
	var best ps6037Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6037ManifestType(lit.Type) {
			return true
		}
		manifest := ps6037Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6037Axes) - len(ps6037Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6037ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6037ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6037ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "externalprofilerevidence", "xctracecapturepolicy", "gpucountercaptureevidence", "tracecapturecontract", "boundedcapturepolicy")
}

func ps6037Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6037Axes))
	for _, axis := range ps6037Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6037ProfilerField(name string) bool {
	return strings.Contains(name, "profiler") && ps6007ContainsAny(name, "identity", "name")
}
func ps6037AllowlistField(name string) bool {
	return strings.Contains(name, "environment") && strings.Contains(name, "allowlist")
}
func ps6037ForwardedField(name string) bool {
	return strings.Contains(name, "forwarded") && strings.Contains(name, "environment") && strings.Contains(name, "name")
}
func ps6037SourcePathField(name string) bool {
	return strings.Contains(name, "source") && strings.Contains(name, "input") && strings.Contains(name, "path")
}
func ps6037StagedPathField(name string) bool {
	return strings.Contains(name, "staged") && strings.Contains(name, "input") && strings.Contains(name, "path")
}
func ps6037SourceHashField(name string) bool {
	return strings.Contains(name, "source") && strings.Contains(name, "sha256")
}
func ps6037StagedHashField(name string) bool {
	return strings.Contains(name, "staged") && strings.Contains(name, "sha256")
}
func ps6037StagingField(name string) bool {
	return strings.Contains(name, "staging") && ps6007ContainsAny(name, "passed", "status")
}
func ps6037SafePathField(name string) bool {
	return strings.Contains(name, "tracesafe") && strings.Contains(name, "path")
}
func ps6037CommandLimitField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "commandbuffer") && strings.Contains(name, "limit")
}
func ps6037CommandCountField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "commandbuffer") && strings.Contains(name, "count")
}
func ps6037CaptureEstimateField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "byte") && strings.Contains(name, "estimate")
}
func ps6037CaptureLimitField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "byte") && strings.Contains(name, "limit")
}
func ps6037ExportEstimateField(name string) bool {
	return strings.Contains(name, "export") && strings.Contains(name, "byte") && strings.Contains(name, "estimate")
}
func ps6037ExportLimitField(name string) bool {
	return strings.Contains(name, "export") && strings.Contains(name, "byte") && strings.Contains(name, "limit")
}
func ps6037RawPathField(name string) bool {
	return strings.Contains(name, "rawtrace") && strings.Contains(name, "path")
}
func ps6037RawExternalField(name string) bool {
	return strings.Contains(name, "rawtrace") && strings.Contains(name, "outsiderepository")
}
func ps6037CompactPathField(name string) bool {
	return strings.Contains(name, "compactreport") && strings.Contains(name, "path")
}
func ps6037CompactOnlyField(name string) bool {
	return strings.Contains(name, "compact") && strings.Contains(name, "only") && strings.Contains(name, "published")
}
func ps6037CounterNamesField(name string) bool {
	return strings.Contains(name, "counter") && ps6007ContainsAny(name, "names", "labels")
}
func ps6037CounterPoliciesField(name string) bool {
	return strings.Contains(name, "counter") && ps6007ContainsAny(name, "policies", "classes")
}
func ps6037CounterCountsField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "sample") && strings.Contains(name, "count")
}
func ps6037CounterMeansField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "active") && strings.Contains(name, "mean")
}
func ps6037CounterCeilingsField(name string) bool {
	return strings.Contains(name, "counter") && strings.Contains(name, "ceiling")
}
func ps6037RequiredField(name string) bool {
	return strings.Contains(name, "required") && strings.Contains(name, "acceptedreport")
}
func ps6037AttemptedField(name string) bool {
	return strings.Contains(name, "attempted") && strings.Contains(name, "capture")
}
func ps6037RejectedField(name string) bool {
	return strings.Contains(name, "rejected") && strings.Contains(name, "capture")
}
func ps6037AcceptedField(name string) bool {
	return strings.Contains(name, "accepted") && strings.Contains(name, "capture") && !strings.Contains(name, "required")
}
func ps6037ReasonsField(name string) bool {
	return strings.Contains(name, "capture") && strings.Contains(name, "rejection") && strings.Contains(name, "reason")
}
func ps6037IndependentField(name string) bool {
	return strings.Contains(name, "report") && strings.Contains(name, "independent")
}
func ps6037MedianField(name string) bool {
	return strings.Contains(name, "median") && strings.Contains(name, "aggregation")
}
func ps6037EmittedField(name string) bool {
	return strings.Contains(name, "result") && strings.Contains(name, "emitted")
}

func ps6037Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"input staging", ps6037StagingField},
		{"trace-safe path", ps6037SafePathField},
		{"raw-trace external retention", ps6037RawExternalField},
		{"compact-only publication", ps6037CompactOnlyField},
		{"independent reports", ps6037IndependentField},
		{"median aggregation", ps6037MedianField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	allowlist := ps6030Strings(fields, ps6037AllowlistField)
	forwarded := ps6030Strings(fields, ps6037ForwardedField)
	for _, name := range forwarded {
		if !slices.Contains(allowlist, name) {
			warnings = append(warnings, fmt.Sprintf("forwarded environment variable %q is outside the explicit allowlist", name))
		}
	}
	sourceHash, sourceOK := ps6027String(fields, ps6037SourceHashField)
	stagedHash, stagedOK := ps6027String(fields, ps6037StagedHashField)
	if sourceOK && stagedOK && sourceHash != stagedHash {
		warnings = append(warnings, "source and staged SHA-256 digests differ")
	}
	for _, limit := range []struct {
		label   string
		value   func(string) bool
		maximum func(string) bool
	}{
		{"capture command-buffer", ps6037CommandCountField, ps6037CommandLimitField},
		{"capture byte", ps6037CaptureEstimateField, ps6037CaptureLimitField},
		{"export byte", ps6037ExportEstimateField, ps6037ExportLimitField},
	} {
		value, valueOK := ps6016Number(fields, limit.value)
		maximum, maximumOK := ps6016Number(fields, limit.maximum)
		if valueOK && maximumOK && value > maximum {
			warnings = append(warnings, fmt.Sprintf("%s estimate/count %.6g exceeds %.6g limit", limit.label, value, maximum))
		}
	}
	names := ps6030Strings(fields, ps6037CounterNamesField)
	policies := ps6030Strings(fields, ps6037CounterPoliciesField)
	counts, _ := ps6016Numbers(fields, ps6037CounterCountsField)
	means, _ := ps6016Numbers(fields, ps6037CounterMeansField)
	ceilings, _ := ps6016Numbers(fields, ps6037CounterCeilingsField)
	emitted, emittedOK := ps6026Bool(fields, ps6037EmittedField)
	if len(names) != len(policies) || len(names) != len(counts) || len(names) != len(means) || len(names) != len(ceilings) {
		warnings = append(warnings, "counter names/policies/sample-counts/active-means/ceilings have different lengths")
	} else {
		for i := range names {
			policy := ps6030StatusName(policies[i])
			required := ps6007ContainsAny(policy, "required", "contaminationsentinel", "contamination")
			contamination := ps6007ContainsAny(policy, "contaminationsentinel", "contamination")
			if emittedOK && emitted && required && counts[i] <= 0 {
				warnings = append(warnings, fmt.Sprintf("emitted result has zero samples for required counter %q", names[i]))
			}
			if emittedOK && emitted && contamination && means[i] > ceilings[i] {
				warnings = append(warnings, fmt.Sprintf("emitted result has contamination counter %q at %.6g above %.6g ceiling", names[i], means[i], ceilings[i]))
			}
		}
	}
	required, requiredOK := ps6016Number(fields, ps6037RequiredField)
	attempted, attemptedOK := ps6016Number(fields, ps6037AttemptedField)
	rejected, rejectedOK := ps6016Number(fields, ps6037RejectedField)
	accepted, acceptedOK := ps6016Number(fields, ps6037AcceptedField)
	if attemptedOK && rejectedOK && acceptedOK && accepted+rejected != attempted {
		warnings = append(warnings, fmt.Sprintf("capture accounting disagrees (accepted %.0f + rejected %.0f != attempted %.0f)", accepted, rejected, attempted))
	}
	reasons := ps6030Strings(fields, ps6037ReasonsField)
	if rejectedOK && rejected != float64(len(reasons)) {
		warnings = append(warnings, fmt.Sprintf("rejected capture count %.0f disagrees with %d rejection reasons", rejected, len(reasons)))
	}
	if emittedOK && emitted && requiredOK && acceptedOK && accepted < required {
		warnings = append(warnings, fmt.Sprintf("result is emitted with %.0f accepted reports below required %.0f", accepted, required))
	}
	return warnings
}
