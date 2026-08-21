package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6048 implements owner issue #730: Metal native artifact analysis must be
// version-pinned and fail closed on absent statistics/disassembly support.
var PS6048 = register(&lint.Check{
	ID:       "PS6048",
	Category: "verify",
	Slug:     "metal-native-artifact-analysis-fails-closed",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "Metal native artifact analysis guesses unavailable compiler statistics",
		Text: `Offline Metal translation can provide actionable native text,
metadata, hash, and spill-section evidence while exposing no valid AGX2
register counts or disassembly. Missing capabilities are unknown, not zero.

This check implements owner issue #730. It audits MetalNativeArtifactEvidence,
NativeCompilerArtifactReport, AGXArtifactCapabilityGate,
OfflineMetalTranslationEvidence, or equivalent manifests. Evidence must record:

  - hardware/OS, component build, compiler and binary versions, mounted
    toolchain location, and locator source;
  - air-arch command/result, SDK and deployment targets;
  - exact source/function/pipeline identities and hashes;
  - ordered commands and tool hashes for MSL -> AIR -> MetalLib -> pipeline ->
    air-nt native translation, including save-temps/extraction status;
  - native object hash and text, metadata, statistics, and spill/scratch bytes;
  - semantic statistics validation and metadata/schema separation;
  - explicit register, spill-statistic, and disassembly capability states;
  - requested performance-statistics/max-register options, artifact-change and
    effectiveness status;
  - classification and final decision.

Constant evidence is checked for missing provenance, invalid section values,
register/spill claims without a nonempty semantically validated statistics
section, metadata decoded with the wrong schema, and options called effective
without changed output or emitted statistics. There is NO automatic fix:
toolchain installation, native translation, schema semantics, and target
capabilities are external compiler facts.`,
		Before: `registers := decodeGPUStatsSchema(nativeSection("__GPU_METADATA"))`,
		After: `report := MetalNativeArtifactEvidence{
	ArchitectureResolutionCommand: "air-arch -default",
	ResolvedDeviceArchitecture: "applegpu_g14s",
	NativeTextBytes: 2738,
	NativeStatisticsBytes: 0,
	RegisterCountCapability: "unknown-missing-statistics",
	DisassemblyCapability: "unsupported-agx2",
	MetadataDecodedWithStatisticsSchema: false,
}`,
		MeasuredWin: `The Apple-M2-Pro workflow behind issue #730 used Metal
Toolchain build 17F109 / Apple metal 32023.883 and resolved applegpu_g14s with
air-arch. Real native text sections measured 3,122 and 2,738 bytes with no
spill/scratch section. GPU statistics remained empty; option variants were
byte-identical and AGX2 disassembly was unsupported, so register counts stayed
explicitly unknown.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6048",
		Doc:  "Metal native compiler artifact analysis does not fail closed",
		Run:  runPS6048,
	},
})

type ps6048Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6048Axes = []ps6048Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048HardwareField) }},
	{name: "OS identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048OSField) }},
	{name: "toolchain component build", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ComponentField) }},
	{name: "compiler version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048CompilerField) }},
	{name: "binary versions", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048BinaryVersionsField) }},
	{name: "mounted toolchain path", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ToolchainPathField) }},
	{name: "toolchain locator source", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048LocatorField) }},
	{name: "architecture resolution command", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ArchCommandField) }},
	{name: "resolved device architecture", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ArchitectureField) }},
	{name: "SDK version", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048SDKField) }},
	{name: "deployment target", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048DeploymentField) }},
	{name: "source identity/hash", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048SourceField) }},
	{name: "function identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048FunctionField) }},
	{name: "pipeline descriptor hash", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048PipelineField) }},
	{name: "translation commands", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048CommandsField) }},
	{name: "tool hashes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ToolHashesField) }},
	{name: "MSL-to-AIR status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048AIRField) }},
	{name: "MetalLib status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048MetalLibField) }},
	{name: "pipeline-script status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048PipelineStatusField) }},
	{name: "native translation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048NativeStatusField) }},
	{name: "native extraction/save-temps status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ExtractionField) }},
	{name: "native object hash", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048NativeHashField) }},
	{name: "native text bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048TextField) }},
	{name: "native metadata bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048MetadataBytesField) }},
	{name: "native statistics bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048StatsBytesField) }},
	{name: "native spill/scratch bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048SpillBytesField) }},
	{name: "spill/scratch section status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048SpillSectionField) }},
	{name: "statistics semantic validation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048StatsValidationField) }},
	{name: "metadata/statistics schema separation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048MetadataMisuseField) }},
	{name: "register-count capability", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048RegisterField) }},
	{name: "spill-statistic capability", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048SpillCapabilityField) }},
	{name: "disassembly capability", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048DisassemblyField) }},
	{name: "performance-statistics option requested", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048PerfRequestedField) }},
	{name: "performance-statistics option effective", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048PerfEffectiveField) }},
	{name: "max-register option", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048MaxRegistersField) }},
	{name: "max-register option effective", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048MaxEffectiveField) }},
	{name: "option artifact change", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ArtifactChangedField) }},
	{name: "classification", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048ClassificationField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6048DecisionField) }},
}

type ps6048Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6048(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6048Context(text) {
				continue
			}
			manifest, found := ps6048BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "Metal native artifact analysis has no fail-closed provenance manifest; missing %s", strings.Join(ps6048Missing(nil), ", "))
				continue
			}
			if missing := ps6048Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal native artifact evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6048Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "Metal native artifact audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6048Context(text string) bool {
	text = ps6007NormalizeName(text)
	return strings.Contains(text, "metal") && strings.Contains(text, "native") &&
		ps6007ContainsAny(text, "artifact", "compiler") && ps6007ContainsAny(text, "failclosed", "capability", "offlinetranslation")
}

func ps6048BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6048Manifest, bool) {
	var best ps6048Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6048ManifestType(lit.Type) {
			return true
		}
		manifest := ps6048Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6048Axes) - len(ps6048Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6048ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6048ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6048ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "metalnativeartifactevidence", "nativecompilerartifactreport", "agxartifactcapabilitygate", "offlinemetaltranslationevidence", "metalartifactreport")
}

func ps6048Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6048Axes))
	for _, axis := range ps6048Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6048HardwareField(n string) bool {
	return ps6007ContainsAny(n, "hardware", "deviceidentity", "gpuid")
}
func ps6048OSField(n string) bool { return ps6007ContainsAny(n, "osidentity", "osversion", "macos") }
func ps6048ComponentField(n string) bool {
	return strings.Contains(n, "toolchaincomponent") && strings.Contains(n, "build")
}
func ps6048CompilerField(n string) bool {
	return strings.Contains(n, "compiler") && strings.Contains(n, "version")
}
func ps6048BinaryVersionsField(n string) bool {
	return strings.Contains(n, "binary") && strings.Contains(n, "version")
}
func ps6048ToolchainPathField(n string) bool {
	return strings.Contains(n, "mountedtoolchain") && strings.Contains(n, "path")
}
func ps6048LocatorField(n string) bool {
	return strings.Contains(n, "toolchainlocator") && ps6007ContainsAny(n, "source", "command")
}
func ps6048ArchCommandField(n string) bool {
	return strings.Contains(n, "architecture") && strings.Contains(n, "resolutioncommand")
}
func ps6048ArchitectureField(n string) bool { return strings.Contains(n, "resolveddevicearchitecture") }
func ps6048SDKField(n string) bool {
	return strings.Contains(n, "sdk") && strings.Contains(n, "version")
}
func ps6048DeploymentField(n string) bool {
	return strings.Contains(n, "deployment") && strings.Contains(n, "target")
}
func ps6048SourceField(n string) bool {
	return strings.Contains(n, "source") && strings.Contains(n, "hash")
}
func ps6048FunctionField(n string) bool {
	return strings.Contains(n, "function") && strings.Contains(n, "identity")
}
func ps6048PipelineField(n string) bool {
	return strings.Contains(n, "pipeline") && strings.Contains(n, "descriptor") && strings.Contains(n, "hash")
}
func ps6048CommandsField(n string) bool {
	return strings.Contains(n, "translation") && strings.Contains(n, "command")
}
func ps6048ToolHashesField(n string) bool {
	return strings.Contains(n, "tool") && strings.Contains(n, "hash")
}
func ps6048AIRField(n string) bool { return strings.Contains(n, "msltoair") }
func ps6048MetalLibField(n string) bool {
	return strings.Contains(n, "metallib") && strings.Contains(n, "passed")
}
func ps6048PipelineStatusField(n string) bool {
	return strings.Contains(n, "pipelinescript") && strings.Contains(n, "passed")
}
func ps6048NativeStatusField(n string) bool {
	return strings.Contains(n, "nativetranslation") && strings.Contains(n, "passed")
}
func ps6048ExtractionField(n string) bool {
	return strings.Contains(n, "native") && ps6007ContainsAny(n, "extraction", "savetemps") && strings.Contains(n, "passed")
}
func ps6048NativeHashField(n string) bool {
	return strings.Contains(n, "nativeobject") && strings.Contains(n, "hash")
}
func ps6048TextField(n string) bool {
	return strings.Contains(n, "nativetext") && strings.Contains(n, "byte")
}
func ps6048MetadataBytesField(n string) bool {
	return strings.Contains(n, "nativemetadata") && strings.Contains(n, "byte")
}
func ps6048StatsBytesField(n string) bool {
	return strings.Contains(n, "nativestatistics") && strings.Contains(n, "byte")
}
func ps6048SpillBytesField(n string) bool {
	return strings.Contains(n, "nativespillscratch") && strings.Contains(n, "byte")
}
func ps6048SpillSectionField(n string) bool {
	return strings.Contains(n, "spillscratchsection") && ps6007ContainsAny(n, "present", "status")
}
func ps6048StatsValidationField(n string) bool {
	return strings.Contains(n, "statisticssection") && strings.Contains(n, "semanticallyvalidated")
}
func ps6048MetadataMisuseField(n string) bool {
	return strings.Contains(n, "metadata") && strings.Contains(n, "statisticsschema")
}
func ps6048RegisterField(n string) bool {
	return strings.Contains(n, "registercount") && strings.Contains(n, "capability")
}
func ps6048SpillCapabilityField(n string) bool {
	return strings.Contains(n, "spillstatistic") && strings.Contains(n, "capability")
}
func ps6048DisassemblyField(n string) bool {
	return strings.Contains(n, "disassembly") && strings.Contains(n, "capability")
}
func ps6048PerfRequestedField(n string) bool {
	return strings.Contains(n, "performancestatistics") && strings.Contains(n, "requested")
}
func ps6048PerfEffectiveField(n string) bool {
	return strings.Contains(n, "performancestatistics") && strings.Contains(n, "effective")
}
func ps6048MaxRegistersField(n string) bool {
	return strings.Contains(n, "maxregister") && strings.Contains(n, "requested")
}
func ps6048MaxEffectiveField(n string) bool {
	return strings.Contains(n, "maxregister") && strings.Contains(n, "effective")
}
func ps6048ArtifactChangedField(n string) bool {
	return strings.Contains(n, "optionartifact") && strings.Contains(n, "changed")
}
func ps6048ClassificationField(n string) bool {
	return ps6007ContainsAny(n, "classification", "resultclass")
}
func ps6048DecisionField(n string) bool {
	return strings.Contains(n, "final") && strings.Contains(n, "decision")
}

func ps6048Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name string
		pred func(string) bool
	}{
		{"MSL-to-AIR translation", ps6048AIRField}, {"MetalLib construction", ps6048MetalLibField},
		{"pipeline-script construction", ps6048PipelineStatusField}, {"native translation", ps6048NativeStatusField},
		{"native extraction/save-temps", ps6048ExtractionField},
	} {
		if value, known := ps6026Bool(fields, status.pred); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	if command, ok := ps6027String(fields, ps6048ArchCommandField); ok && !strings.Contains(ps6030StatusName(command), "airarchdefault") {
		warnings = append(warnings, fmt.Sprintf("architecture resolution command %q does not use air-arch -default", command))
	}
	commands, commandsOK := ps6047Strings(fields, ps6048CommandsField)
	toolHashes, toolHashesOK := ps6047Strings(fields, ps6048ToolHashesField)
	if commandsOK && len(commands) < 4 {
		warnings = append(warnings, "offline translation records fewer than four commands")
	}
	if toolHashesOK && len(toolHashes) == 0 {
		warnings = append(warnings, "tool hash manifest is empty")
	}
	for _, section := range []struct {
		name string
		pred func(string) bool
	}{
		{"native text", ps6048TextField}, {"native metadata", ps6048MetadataBytesField},
		{"native statistics", ps6048StatsBytesField}, {"native spill/scratch", ps6048SpillBytesField},
	} {
		if value, ok := ps6016Number(fields, section.pred); ok && value < 0 {
			warnings = append(warnings, section.name+" bytes must not be negative")
		}
	}
	statsBytes, statsBytesOK := ps6016Number(fields, ps6048StatsBytesField)
	statsValidated, statsValidatedOK := ps6026Bool(fields, ps6048StatsValidationField)
	metadataMisused, metadataMisusedOK := ps6026Bool(fields, ps6048MetadataMisuseField)
	if metadataMisusedOK && metadataMisused {
		warnings = append(warnings, "native metadata is decoded with the statistics schema")
	}
	validStats := statsBytesOK && statsBytes > 0 && statsValidatedOK && statsValidated
	for _, capability := range []struct {
		name string
		pred func(string) bool
	}{
		{"register-count", ps6048RegisterField}, {"spill-statistic", ps6048SpillCapabilityField},
	} {
		if value, ok := ps6027String(fields, capability.pred); ok {
			normalized := ps6030StatusName(value)
			claimed := ps6007ContainsAny(normalized, "known", "available", "reported", "zero") && !ps6007ContainsAny(normalized, "unknown", "unavailable", "unsupported")
			if claimed && !validStats {
				warnings = append(warnings, fmt.Sprintf("%s capability %q claims a value without nonempty validated statistics", capability.name, value))
			}
		}
	}
	spillBytes, spillBytesOK := ps6016Number(fields, ps6048SpillBytesField)
	spillPresent, spillPresentOK := ps6026Bool(fields, ps6048SpillSectionField)
	if spillBytesOK && spillPresentOK && spillPresent != (spillBytes > 0) {
		warnings = append(warnings, fmt.Sprintf("spill/scratch section presence is %t but byte count is %.6g", spillPresent, spillBytes))
	}
	artifactChanged, artifactChangedOK := ps6026Bool(fields, ps6048ArtifactChangedField)
	for _, option := range []struct {
		name string
		pred func(string) bool
	}{
		{"performance-statistics", ps6048PerfEffectiveField}, {"max-register", ps6048MaxEffectiveField},
	} {
		if effective, ok := ps6026Bool(fields, option.pred); ok && effective && artifactChangedOK && !artifactChanged && !validStats {
			warnings = append(warnings, option.name+" option is marked effective without changed artifact or emitted statistics")
		}
	}
	return warnings
}
