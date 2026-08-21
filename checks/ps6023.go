package checks

import (
	"fmt"
	"go/ast"
	"math"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6023 implements owner issue #763: asynchronous accelerator evidence must
// separate device time from the fixed host API/command-buffer boundary and
// sweep working sets across cache regimes.
var PS6023 = register(&lint.Check{
	ID:       "PS6023",
	Category: "verify",
	Slug:     "host-floor-masks-device-gain",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "an asynchronous accelerator benchmark can hide a device gain behind host fixed cost",
		Text: `Recorder creation, command encoding, submission, synchronization,
and host bookkeeping add a fixed floor around asynchronous accelerator work.
At small or medium working sets that floor can make a stable device-side gain
look like noise. Cache thresholds add a second trap: one benchmark size can
accept or reject the same optimization depending on whether its footprint
crosses a cache level.

This check implements owner issue #763. It finds real BenchmarkX(*testing.B)
functions with accelerator, asynchronous command/recorder, device-vs-host
timing, and cache/working-set context. The best HostDeviceFloorEvidence,
AcceleratorTimingSweep, CacheThresholdEvidence, DeviceHostBoundaryGate,
AsyncFloorGate, or equivalent keyed manifest must retain:

  - hardware and exact workload geometry;
  - control and candidate storage/memory policy;
  - warmups, interleaving, samples per campaign, and campaign count;
  - paired device-interval and host-API-boundary ratios/distributions;
  - an unchanged-control distribution;
  - working-set bytes plus a cache/context threshold sweep;
  - the configured host noise band and promotion ratio; and
  - finite-output and exactness/parity status.

When these values are compile-time constants, the check reports "host floor
masks device gain" whenever device improvement exceeds the noise band while
the corresponding host-boundary ratio remains inside it. It also reports a
cache/working-set series that crosses the promotion threshold, because a
single chosen size would reverse the decision. Sequence lengths must agree and
contain at least two regimes.

There is NO automatic fix. Device timestamps, working-set accounting, cache
boundaries, hardware, and a valid unchanged control are measured evidence that
source rewriting cannot invent. Keep both timing views: device intervals
explain the mechanism; the host boundary establishes realized API leverage.`,
		Before: `BenchmarkMetalF16KV(b) // one ctx512 host-boundary number: ~1.03x`,
		After: `evidence := HostDeviceFloorEvidence{
	Hardware: "Apple M2 Pro", Shape: "sq=1,h=32,kv=4,dk=64",
	ControlStorage: "f32 KV", CandidateStorage: "IEEE f16 KV",
	Warmups: 8, Interleaved: true, SamplesPerCampaign: 31, Campaigns: 5,
	ContextSizes: []int{128, 512, 1024, 2048},
	WorkingSetBytes: measuredBytes,
	DeviceSpeedups: deviceRatios, HostBoundarySpeedups: hostRatios,
	UnchangedControlRatios: controls,
	NoiseBand: 0.03, PromotionRatio: 1.03,
	ExactParity: true, FiniteOutput: true,
}`,
		MeasuredWin: `In the Apple-M2 f16-KV campaign behind issue #763, ctx512
device intervals improved 1.192x-1.290x while the conventional host boundary
showed only 1.022x-1.104x. The device gain grew with footprint: 1.009x-1.017x
at ctx128, 1.416x-1.594x at ctx1024, and 1.525x-1.751x at ctx2048. A single
ctx512 host benchmark would have obscured both the mechanism and cache trend.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6023",
		Doc:  "host command-buffer floor masks a stable device gain or one size hides a cache threshold",
		Run:  runPS6023,
	},
})

type ps6023Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

type ps6023Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6023Axes = []ps6023Axis{
	{name: "hardware", present: func(f map[string]ps6016Field) bool { return ps6023Has(f, "hardware", "device") }},
	{name: "workload geometry", present: func(f map[string]ps6016Field) bool { return ps6023Has(f, "shape", "geometry") }},
	{name: "control storage policy", present: func(f map[string]ps6016Field) bool { return ps6023Storage(f, "control") }},
	{name: "candidate storage policy", present: func(f map[string]ps6016Field) bool { return ps6023Storage(f, "candidate") }},
	{name: "warmup count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return strings.Contains(n, "warmup") })
	}},
	{name: "interleaved sampling", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6007ContainsAny(n, "interleaved", "alternating", "pairedorder") })
	}},
	{name: "samples per campaign", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023SamplesField) }},
	{name: "campaign count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return strings.Contains(n, "campaign") && !strings.Contains(n, "sample") })
	}},
	{name: "device interval ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023DeviceRatiosField) }},
	{name: "host API-boundary ratios", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023HostRatiosField) }},
	{name: "unchanged-control distribution", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023ControlField) }},
	{name: "working-set bytes", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023WorkingSetField) }},
	{name: "cache/context threshold sweep", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023SweepField) }},
	{name: "host noise band", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023NoiseField) }},
	{name: "promotion ratio", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023PromotionField) }},
	{name: "exactness/parity status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023ExactField) }},
	{name: "finite-output status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6023FiniteField) }},
}

func runPS6023(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6006Benchmark(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6023Context(text) {
				continue
			}
			manifest, found := ps6023BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "asynchronous accelerator host/device cache benchmark has no host-floor evidence manifest; missing %s", strings.Join(ps6023Missing(nil), ", "))
				continue
			}
			if missing := ps6023Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "host/device cache-threshold evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6023Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "host/device cache-threshold audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6023Context(text string) bool {
	normalized := ps6007NormalizeName(text)
	accelerator := ps6007ContainsAny(normalized, "gpu", "metal", "mps", "cuda", "vulkan", "accelerator", "device")
	async := ps6007ContainsAny(normalized, "recorder", "commandbuffer", "submit", "wait", "synchronize", "asynchronous", "async")
	timing := ps6007ContainsAny(normalized, "devicehost", "hostdevice") ||
		strings.Contains(normalized, "host") && ps6007ContainsAny(normalized, "devicetime", "deviceinterval", "gputime", "timestamp")
	cache := ps6007ContainsAny(normalized, "cache", "workingset", "footprint", "contextsize", "threshold")
	return accelerator && async && timing && cache
}

func ps6023BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6023Manifest, bool) {
	var best ps6023Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6023ManifestType(lit.Type) {
			return true
		}
		manifest := ps6023Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6023Axes) - len(ps6023Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6023ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6023ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6023ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return strings.Contains(name, "hostdevicefloor") ||
		strings.Contains(name, "acceleratortimingsweep") ||
		strings.Contains(name, "cachethresholdevidence") ||
		strings.Contains(name, "devicehostboundary") ||
		strings.Contains(name, "asyncfloorgate") ||
		strings.Contains(name, "hostfloor")
}

func ps6023Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6023Axes))
	for _, axis := range ps6023Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6023Has(fields map[string]ps6016Field, alternatives ...string) bool {
	return ps6016HasName(fields, func(name string) bool { return ps6007ContainsAny(name, alternatives...) })
}

func ps6023Storage(fields map[string]ps6016Field, side string) bool {
	return ps6016HasName(fields, func(name string) bool {
		return strings.Contains(name, side) && ps6007ContainsAny(name, "storage", "memory", "dtype", "cache")
	})
}

func ps6023SamplesField(name string) bool {
	return strings.Contains(name, "sample") && ps6007ContainsAny(name, "campaign", "percampaign", "count")
}

func ps6023DeviceRatiosField(name string) bool {
	return ps6007ContainsAny(name, "device", "gpu") && ps6007ContainsAny(name, "ratio", "speedup", "distribution") && !strings.Contains(name, "control")
}

func ps6023HostRatiosField(name string) bool {
	return strings.Contains(name, "host") && ps6007ContainsAny(name, "boundary", "api") && ps6007ContainsAny(name, "ratio", "speedup", "distribution")
}

func ps6023ControlField(name string) bool {
	return strings.Contains(name, "control") && ps6007ContainsAny(name, "unchanged", "ratio", "distribution", "sample")
}

func ps6023WorkingSetField(name string) bool {
	return strings.Contains(name, "bytes") && ps6007ContainsAny(name, "workingset", "footprint", "resident", "cache")
}

func ps6023SweepField(name string) bool {
	return ps6007ContainsAny(name, "contextsizes", "cachesweep", "thresholdsweep", "workingsetsizes", "footprintsizes")
}

func ps6023NoiseField(name string) bool {
	return strings.Contains(name, "noise") && ps6007ContainsAny(name, "band", "ratio", "threshold", "floor")
}

func ps6023PromotionField(name string) bool {
	return strings.Contains(name, "promotion") && ps6007ContainsAny(name, "ratio", "gate", "minimum", "threshold")
}

func ps6023ExactField(name string) bool {
	return ps6007ContainsAny(name, "exact", "parity", "bitexact") && ps6007ContainsAny(name, "passed", "status", "gate", "result")
}

func ps6023FiniteField(name string) bool {
	return strings.Contains(name, "finite") && ps6007ContainsAny(name, "output", "passed", "status", "gate")
}

func ps6023Audit(fields map[string]ps6016Field) []string {
	var warnings []string
	device, deviceOK := ps6016Numbers(fields, ps6023DeviceRatiosField)
	host, hostOK := ps6016Numbers(fields, ps6023HostRatiosField)
	controls, controlsOK := ps6016Numbers(fields, ps6023ControlField)
	workingSets, workingOK := ps6016Numbers(fields, ps6023WorkingSetField)
	sweep, sweepOK := ps6016Numbers(fields, ps6023SweepField)
	noise, noiseOK := ps6016Number(fields, ps6023NoiseField)
	promotion, promotionOK := ps6016Number(fields, ps6023PromotionField)

	for _, series := range []struct {
		label  string
		values []float64
	}{
		{"device ratios", device},
		{"host-boundary ratios", host},
		{"unchanged controls", controls},
		{"working-set bytes", workingSets},
		{"cache/context sweep", sweep},
	} {
		if series.values != nil && len(series.values) < 2 {
			warnings = append(warnings, series.label+" contains fewer than two cache/working-set regimes")
		}
	}
	if deviceOK && hostOK && len(device) != len(host) {
		warnings = append(warnings, fmt.Sprintf("device/host series lengths differ (%d vs %d)", len(device), len(host)))
	}
	if deviceOK && workingOK && len(device) != len(workingSets) {
		warnings = append(warnings, fmt.Sprintf("device/working-set series lengths differ (%d vs %d)", len(device), len(workingSets)))
	}
	if deviceOK && sweepOK && len(device) != len(sweep) {
		warnings = append(warnings, fmt.Sprintf("device/cache-sweep series lengths differ (%d vs %d)", len(device), len(sweep)))
	}
	if controlsOK && deviceOK && len(controls) != len(device) {
		warnings = append(warnings, fmt.Sprintf("unchanged-control/device series lengths differ (%d vs %d)", len(controls), len(device)))
	}
	if noiseOK {
		band := noise
		if band >= 1 {
			band--
		}
		if band < 0 || band >= 1 {
			warnings = append(warnings, fmt.Sprintf("host noise band %.4g is invalid", noise))
		} else if deviceOK && hostOK && len(device) == len(host) {
			for i := range device {
				if device[i] > 1 && device[i]-1 > band && math.Abs(host[i]-1) <= band {
					warnings = append(warnings, fmt.Sprintf("host floor masks device gain at sweep index %d (device %.4gx, host %.4gx inside ±%.2f%% noise)", i, device[i], host[i], band*100))
					break
				}
			}
		}
	}
	if promotionOK && promotion > 0 {
		for _, series := range []struct {
			name   string
			values []float64
			ok     bool
		}{{"device", device, deviceOK}, {"host boundary", host, hostOK}} {
			if !series.ok || len(series.values) < 2 {
				continue
			}
			minimum, maximum := slices.Min(series.values), slices.Max(series.values)
			if minimum < promotion && maximum >= promotion {
				warnings = append(warnings, fmt.Sprintf("%s sweep crosses %.4gx promotion ratio (%.4gx..%.4gx); one benchmark size reverses the decision", series.name, promotion, minimum, maximum))
			}
		}
	}
	for name, field := range fields {
		if (ps6023ExactField(name) || ps6023FiniteField(name) || ps6007ContainsAny(name, "interleaved", "alternating")) && field.hasBool && !field.boolVal {
			warnings = append(warnings, name+" is explicitly false")
		}
	}
	for _, count := range []struct {
		label     string
		predicate func(string) bool
	}{
		{"warmup count", func(name string) bool { return strings.Contains(name, "warmup") }},
		{"samples per campaign", ps6023SamplesField},
		{"campaign count", func(name string) bool { return strings.Contains(name, "campaign") && !strings.Contains(name, "sample") }},
	} {
		if value, ok := ps6016Number(fields, count.predicate); ok && value <= 0 {
			warnings = append(warnings, count.label+" is not positive")
		}
	}
	return warnings
}
