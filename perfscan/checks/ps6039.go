package checks

import (
	"fmt"
	"go/ast"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/perfscan/lint"
)

// PS6039 implements owner issue #739: immutable GPU arena comparisons need a
// cache-exceeding multi-resource, equal-work contract.
var PS6039 = register(&lint.Check{
	ID:       "PS6039",
	Category: "verify",
	Slug:     "gpu-arena-needs-cache-exceeding-multi-resource-gate",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a GPU arena claim uses a cache-hot or unequal resource comparison",
		Text: `One hot resource or tiny parameter buffers cannot isolate GPU
resource-binding/allocation granularity from cache residency and storage mode.

This check implements owner issue #739. It audits GPUArenaEvidence,
MultiResourceArenaComparison, ResourceAllocationEvidence,
CacheExceedingArenaGate, or equivalent manifests. The comparison must record:

  - hardware, workload, control/candidate distinct resource counts;
  - control/candidate total resident bytes, cache capacity, and an explicit
    cache-exceeding status;
  - control/candidate storage modes and persistent/transient byte counts;
  - offset alignment and nonzero-offset coverage;
  - identical bytes, kernels, dispatch order/count, hazards, and command-buffer
    count;
  - warm status and iteration count;
  - control/candidate streaming latency, effective bytes/s, allocation bytes,
    and allocation counts;
  - exact nonzero-offset output parity and arena-view lifetime status; and
  - final retain/remove decision.

Constant evidence is checked for unequal work/memory contracts, a working set
that does not exceed cache, stale latency/bandwidth ratios, failed parity/
lifetime, and retention of a slower candidate. There is NO automatic fix:
resource ownership, Metal storage, view offsets, hazards, and cache capacity
are runtime/backend facts.`,
		Before: `BenchmarkOneHotWeight(separateBuffer, arenaView)`,
		After: `evidence := MultiResourceArenaComparison{
	ControlResourceCount: 44, CandidateResourceCount: 1,
	ControlResidentBytes: 285_000_000, CandidateResidentBytes: 285_000_000,
	CacheCapacityBytes: measuredCache,
	WorkingSetExceedsCache: true,
	ControlLatencyNS: 2_124_454, CandidateLatencyNS: 2_156_015,
	ControlCandidateRatio: 0.985361,
	FinalDecision: "removed",
}`,
		MeasuredWin: `The Apple-M2-Pro screen behind issue #739 compared 44
separate shared buffers with one equal-byte aligned-view arena over about 285
MB. Separate buffers measured 2,124,454 ns/op versus 2,156,015 ns/op for the
arena, or 0.985361x control/candidate. Exact nonzero-offset parity and lifetime
passed, but the slower arena was rejected.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6039",
		Doc:  "GPU arena comparison lacks cache-exceeding multi-resource equal-work evidence",
		Run:  runPS6039,
	},
})

type ps6039Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6039Axes = []ps6039Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039HardwareField) }},
	{name: "workload identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039WorkloadField) }},
	{name: "control resource count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "resourcecount") })
	}},
	{name: "candidate resource count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "resourcecount") })
	}},
	{name: "control resident bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "residentbytes") })
	}},
	{name: "candidate resident bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "residentbytes") })
	}},
	{name: "cache capacity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039CacheField) }},
	{name: "cache-exceeding status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039ExceedsField) }},
	{name: "control storage mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "storagemode") })
	}},
	{name: "candidate storage mode", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "storagemode") })
	}},
	{name: "offset alignment", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039AlignmentField) }},
	{name: "nonzero-offset coverage", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039NonzeroField) }},
	{name: "control persistent bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "persistentbytes") })
	}},
	{name: "candidate persistent bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "persistentbytes") })
	}},
	{name: "control transient bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "transientbytes") })
	}},
	{name: "candidate transient bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "transientbytes") })
	}},
	{name: "identical input bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039InputField) }},
	{name: "identical kernels", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039KernelField) }},
	{name: "identical dispatch order/count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039DispatchField) }},
	{name: "identical hazards", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039HazardField) }},
	{name: "identical command-buffer count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039CommandField) }},
	{name: "warm status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039WarmField) }},
	{name: "iteration count", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039IterationsField) }},
	{name: "control latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "latency") })
	}},
	{name: "candidate latency", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "latency") })
	}},
	{name: "control/candidate ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039RatioField) }},
	{name: "control effective bytes/s", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "effectivebytespersecond") })
	}},
	{name: "candidate effective bytes/s", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "effectivebytespersecond") })
	}},
	{name: "control allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "allocationbytes") })
	}},
	{name: "candidate allocation bytes", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "allocationbytes") })
	}},
	{name: "control allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "control", "allocationcount") })
	}},
	{name: "candidate allocation count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6039SideField(n, "candidate", "allocationcount") })
	}},
	{name: "nonzero-offset parity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039ParityField) }},
	{name: "arena-view lifetime", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039LifetimeField) }},
	{name: "final decision", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6039DecisionField) }},
}

type ps6039Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

func runPS6039(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6039Context(text) {
				continue
			}
			manifest, found := ps6039BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "GPU multi-resource arena campaign has no cache-exceeding manifest; missing %s", strings.Join(ps6039Missing(nil), ", "))
				continue
			}
			if missing := ps6039Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU arena comparison evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6039Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "GPU arena comparison audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6039Context(text string) bool {
	text = ps6007NormalizeName(text)
	gpu := ps6007ContainsAny(text, "gpu", "metal", "device")
	arena := ps6007ContainsAny(text, "arena", "multiresource", "allocationgranularity")
	cache := ps6007ContainsAny(text, "cacheexceeding", "residentbytes", "cachecapacity", "streaming")
	return gpu && arena && cache
}

func ps6039BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6039Manifest, bool) {
	var best ps6039Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6039ManifestType(lit.Type) {
			return true
		}
		manifest := ps6039Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6039Axes) - len(ps6039Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6039ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6039ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6039ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "gpuarenaevidence", "multiresourcearenacomparison", "resourceallocationevidence", "cacheexceedingarenagate", "gpuarenacampaign")
}

func ps6039Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6039Axes))
	for _, axis := range ps6039Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6039HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid")
}
func ps6039WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && ps6007ContainsAny(name, "identity", "shape", "name")
}
func ps6039SideField(name, side, detail string) bool {
	return strings.Contains(name, side) && strings.Contains(name, detail)
}
func ps6039CacheField(name string) bool {
	return strings.Contains(name, "cache") && strings.Contains(name, "capacity") && strings.Contains(name, "byte")
}
func ps6039ExceedsField(name string) bool {
	return strings.Contains(name, "workingset") && strings.Contains(name, "exceed") && strings.Contains(name, "cache")
}
func ps6039AlignmentField(name string) bool {
	return strings.Contains(name, "offset") && strings.Contains(name, "alignment")
}
func ps6039NonzeroField(name string) bool {
	return strings.Contains(name, "nonzerooffset") && strings.Contains(name, "covered")
}
func ps6039InputField(name string) bool {
	return strings.Contains(name, "identical") && strings.Contains(name, "input") && strings.Contains(name, "byte")
}
func ps6039KernelField(name string) bool {
	return strings.Contains(name, "identical") && strings.Contains(name, "kernel")
}
func ps6039DispatchField(name string) bool {
	return strings.Contains(name, "identical") && strings.Contains(name, "dispatch") && ps6007ContainsAny(name, "order", "count")
}
func ps6039HazardField(name string) bool {
	return strings.Contains(name, "identical") && strings.Contains(name, "hazard")
}
func ps6039CommandField(name string) bool {
	return strings.Contains(name, "identical") && strings.Contains(name, "commandbuffer") && strings.Contains(name, "count")
}
func ps6039WarmField(name string) bool {
	return strings.Contains(name, "warm") && ps6007ContainsAny(name, "streaming", "status", "passed")
}
func ps6039IterationsField(name string) bool {
	return strings.Contains(name, "iteration") && strings.Contains(name, "count")
}
func ps6039RatioField(name string) bool {
	return strings.Contains(name, "controlcandidate") && ps6007ContainsAny(name, "ratio", "speedup")
}
func ps6039ParityField(name string) bool {
	return strings.Contains(name, "nonzerooffset") && strings.Contains(name, "parity")
}
func ps6039LifetimeField(name string) bool {
	return strings.Contains(name, "arenaview") && strings.Contains(name, "lifetime")
}
func ps6039DecisionField(name string) bool {
	return strings.Contains(name, "final") && strings.Contains(name, "decision")
}

func ps6039Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 12)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"working-set cache exceedance", ps6039ExceedsField},
		{"nonzero-offset coverage", ps6039NonzeroField},
		{"identical input bytes", ps6039InputField},
		{"identical kernels", ps6039KernelField},
		{"identical dispatch order/count", ps6039DispatchField},
		{"identical hazards", ps6039HazardField},
		{"identical command-buffer count", ps6039CommandField},
		{"warm streaming", ps6039WarmField},
		{"nonzero-offset parity", ps6039ParityField},
		{"arena-view lifetime", ps6039LifetimeField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	controlResident, controlResidentOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, "control", "residentbytes") })
	candidateResident, candidateResidentOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, "candidate", "residentbytes") })
	cache, cacheOK := ps6016Number(fields, ps6039CacheField)
	if controlResidentOK && candidateResidentOK && controlResident != candidateResident {
		warnings = append(warnings, fmt.Sprintf("control/candidate resident bytes differ (%.6g vs %.6g)", controlResident, candidateResident))
	}
	if controlResidentOK && cacheOK && controlResident <= cache {
		warnings = append(warnings, fmt.Sprintf("working set %.6g bytes does not exceed %.6g-byte cache capacity", controlResident, cache))
	}
	for _, detail := range []string{"persistentbytes", "transientbytes", "allocationbytes", "allocationcount"} {
		control, controlOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, "control", detail) })
		candidate, candidateOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, "candidate", detail) })
		if controlOK && candidateOK && control != candidate {
			warnings = append(warnings, fmt.Sprintf("control/candidate %s differ (%.6g vs %.6g)", detail, control, candidate))
		}
	}
	controlStorage, controlStorageOK := ps6027String(fields, func(n string) bool { return ps6039SideField(n, "control", "storagemode") })
	candidateStorage, candidateStorageOK := ps6027String(fields, func(n string) bool { return ps6039SideField(n, "candidate", "storagemode") })
	if controlStorageOK && candidateStorageOK && ps6030StatusName(controlStorage) != ps6030StatusName(candidateStorage) {
		warnings = append(warnings, fmt.Sprintf("control/candidate storage modes differ (%q vs %q)", controlStorage, candidateStorage))
	}
	controlLatency, controlLatencyOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, "control", "latency") })
	candidateLatency, candidateLatencyOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, "candidate", "latency") })
	ratio, ratioOK := ps6016Number(fields, ps6039RatioField)
	if controlLatencyOK && candidateLatencyOK && candidateLatency > 0 && ratioOK && !ps6025Close(ratio, controlLatency/candidateLatency) {
		warnings = append(warnings, fmt.Sprintf("control/candidate ratio %.6gx disagrees with latency ratio %.6gx", ratio, controlLatency/candidateLatency))
	}
	for _, side := range []string{"control", "candidate"} {
		latency, latencyOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, side, "latency") })
		bandwidth, bandwidthOK := ps6016Number(fields, func(n string) bool { return ps6039SideField(n, side, "effectivebytespersecond") })
		resident := controlResident
		residentOK := controlResidentOK
		if side == "candidate" {
			resident, residentOK = candidateResident, candidateResidentOK
		}
		if latencyOK && residentOK && latency > 0 && bandwidthOK && !ps6025Close(bandwidth, resident*1e9/latency) {
			warnings = append(warnings, fmt.Sprintf("%s effective bytes/s %.6g disagrees with resident-bytes/latency %.6g", side, bandwidth, resident*1e9/latency))
		}
	}
	if controlLatencyOK && candidateLatencyOK && candidateLatency > controlLatency {
		if decision, ok := ps6027String(fields, ps6039DecisionField); ok && ps6007ContainsAny(ps6030StatusName(decision), "retain", "promote", "ship", "selected") {
			warnings = append(warnings, fmt.Sprintf("final decision %q retains a slower arena candidate (control/candidate %.6gx)", decision, controlLatency/candidateLatency))
		}
	}
	return warnings
}
