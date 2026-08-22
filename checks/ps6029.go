package checks

import (
	"fmt"
	"go/ast"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// PS6029 implements owner issue #749: claim-bearing timing/topology and
// multiplexed contamination witnesses may be acquired independently, but only
// under an exact, predeclared join contract.
var PS6029 = register(&lint.Check{
	ID:       "PS6029",
	Category: "verify",
	Slug:     "profiler-claim-witness-split-pass-contract",
	Level:    lint.LevelStructured,
	AutoFix:  false,
	Doc: lint.Documentation{
		Title: "a profiler split-pass campaign lacks an exact claim/witness join contract",
		Text: `Stable GPU interval timing can remain claim-bearing while a
multiplexed hardware-counter export randomly omits a contamination witness.
The missing witness correctly prevents that pass from proving cleanliness, but
it need not discard an independently acquired exact timing/topology pass.

This check implements owner issue #749. It audits SplitPassProfilerEvidence,
ClaimWitnessJoinEvidence, ContaminationWitnessProtocol,
SplitPassMeasurementProtocol, or equivalent manifests. The protocol must
record:

  - hardware, workload digest, graph identity, build identity, and a thermal/
    process contract;
  - split-pass or strict-one-pass mode and availability of strict mode;
  - claim and witness signal identities;
  - predeclared required, attempted, accepted, and rejected counts for claim
    and witness passes, with pass IDs and rejection reasons;
  - exact join status for workload, graph, hardware, build, and thermal/process
    identity;
  - independent claim acquisition, witness acquisition, and witness
    aggregation;
  - an explicit prohibition on imputing a missing witness as zero;
  - exact output/digest and event-topology status; and
  - whether medians were published or a candidate selected.

Constant evidence is checked for failed join/identity/independence gates,
overlapping claim/witness pass IDs in split mode, inconsistent accounting,
missing rejection disclosure, missing-witness zero imputation, and publication
or selection before both predeclared accepted-pass counts are satisfied.
Strict-one-pass mode remains valid when the exporter is reliable.

There is NO automatic fix. A source rewrite cannot invent independent process
captures, counter witnesses, join identities, rejection reasons, or exact
outputs. Acquire the evidence independently and join only exact contracts.`,
		Before: `report := profileOnce()
if report.FragmentOccupancySamples == 0 {
	reject(report.Timing) // timing and witness share one failure-prone pass
}`,
		After: `evidence := SplitPassProfilerEvidence{
	ProtocolMode: "split-pass",
	ClaimSignal: "exact GPU intervals + topology",
	WitnessSignal: "contamination counters",
	JoinWorkloadDigestMatched: true,
	JoinGraphIdentityMatched: true,
	JoinHardwareMatched: true,
	JoinBuildMatched: true,
	JoinThermalProcessContractMatched: true,
	MissingWitnessImputedZero: false,
	// independently bounded claim/witness pass counts and rejection reasons...
}`,
		MeasuredWin: `In the Apple-M2 campaign behind issue #749, exact
TinyLlama Q4_K_M timing retained the expected logits digest and 296 internal
plus 296 GPU events, while multiplexed Fragment Occupancy and Texture Sample
Limiter witnesses were absent in separate attempts. The bounded campaign
reached only three accepted attempts of five, so it correctly published no
stage medians and selected no implementation candidate.`,
	},
	Analyzer: &analysis.Analyzer{
		Name: "PS6029",
		Doc:  "profiler claim and contamination-witness passes lack an exact split-pass join contract",
		Run:  runPS6029,
	},
})

type ps6029Axis struct {
	name    string
	present func(map[string]ps6016Field) bool
}

var ps6029Axes = []ps6029Axis{
	{name: "hardware identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029HardwareField) }},
	{name: "workload digest", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029WorkloadField) }},
	{name: "graph identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029GraphField) }},
	{name: "build identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029BuildField) }},
	{name: "thermal/process contract", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ThermalField) }},
	{name: "protocol mode", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ModeField) }},
	{name: "strict one-pass availability", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029StrictAvailableField) }},
	{name: "claim signal identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ClaimSignalField) }},
	{name: "witness signal identity", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029WitnessSignalField) }},
	{name: "required claim-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "claim", "required") })
	}},
	{name: "attempted claim-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "claim", "attempt") })
	}},
	{name: "accepted claim-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "claim", "accepted") })
	}},
	{name: "rejected claim-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "claim", "rejected") })
	}},
	{name: "claim pass identities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ClaimIDsField) }},
	{name: "claim rejection reasons", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029ReasonsField(n, "claim") })
	}},
	{name: "required witness-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "witness", "required") })
	}},
	{name: "attempted witness-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "witness", "attempt") })
	}},
	{name: "accepted witness-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "witness", "accepted") })
	}},
	{name: "rejected witness-pass count", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029CountField(n, "witness", "rejected") })
	}},
	{name: "witness pass identities", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029WitnessIDsField) }},
	{name: "witness rejection reasons", present: func(f map[string]ps6016Field) bool {
		return ps6016HasName(f, func(n string) bool { return ps6029ReasonsField(n, "witness") })
	}},
	{name: "workload-digest join status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029JoinWorkloadField) }},
	{name: "graph-identity join status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029JoinGraphField) }},
	{name: "hardware join status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029JoinHardwareField) }},
	{name: "build join status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029JoinBuildField) }},
	{name: "thermal/process join status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029JoinThermalField) }},
	{name: "claim-pass independence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ClaimIndependentField) }},
	{name: "witness-pass independence", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029WitnessIndependentField) }},
	{name: "independent witness aggregation", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029WitnessAggregationField) }},
	{name: "missing-witness imputation status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ImputationField) }},
	{name: "exact output/digest status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ExactOutputField) }},
	{name: "exact event-topology status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029ExactTopologyField) }},
	{name: "median publication status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029PublishedField) }},
	{name: "candidate selection status", present: func(f map[string]ps6016Field) bool { return ps6016HasName(f, ps6029SelectedField) }},
}

type ps6029Manifest struct {
	lit    *ast.CompositeLit
	fields map[string]ps6016Field
}

var ps6029ModeReplacer = strings.NewReplacer("-", "", " ", "", "_", "")

func runPS6029(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !ps6021Harness(pass, fn) {
				continue
			}
			text := ps6015FunctionText(pass, file, fn)
			if !ps6029Context(text) {
				continue
			}
			manifest, found := ps6029BestManifest(pass, fn.Body)
			if !found {
				pass.Reportf(fn.Name.Pos(), "profiler claim/witness campaign has no split-pass join manifest; missing %s", strings.Join(ps6029Missing(nil), ", "))
				continue
			}
			if missing := ps6029Missing(manifest.fields); len(missing) > 0 {
				pass.Reportf(manifest.lit.Pos(), "profiler split-pass evidence is incomplete; missing %s", strings.Join(missing, ", "))
				continue
			}
			if warnings := ps6029Audit(manifest.fields); len(warnings) > 0 {
				pass.Reportf(manifest.lit.Pos(), "profiler split-pass audit: %s", strings.Join(warnings, "; "))
			}
		}
	}
	return nil, nil
}

func ps6029Context(text string) bool {
	text = ps6007NormalizeName(text)
	profiler := ps6007ContainsAny(text, "profiler", "profile", "gpu", "metal", "counter")
	passes := ps6007ContainsAny(text, "splitpass", "claimwitness", "witnesspass", "claimpass")
	contamination := ps6007ContainsAny(text, "contamination", "witness", "multiplex", "counter")
	return profiler && passes && contamination
}

func ps6029BestManifest(pass *analysis.Pass, body *ast.BlockStmt) (ps6029Manifest, bool) {
	var best ps6029Manifest
	bestScore := -1
	ast.Inspect(body, func(node ast.Node) bool {
		if _, nested := node.(*ast.FuncLit); nested {
			return false
		}
		lit, ok := node.(*ast.CompositeLit)
		if !ok || !ps6029ManifestType(lit.Type) {
			return true
		}
		manifest := ps6029Manifest{lit: lit, fields: ps6016Fields(pass, lit)}
		score := len(ps6029Axes) - len(ps6029Missing(manifest.fields))
		if score > bestScore {
			best, bestScore = manifest, score
		}
		return true
	})
	return best, bestScore >= 0
}

func ps6029ManifestType(expr ast.Expr) bool {
	var name string
	switch value := ps2110Unparen(expr).(type) {
	case *ast.Ident:
		name = value.Name
	case *ast.SelectorExpr:
		name = value.Sel.Name
	case *ast.IndexExpr:
		return ps6029ManifestType(value.X)
	case *ast.IndexListExpr:
		return ps6029ManifestType(value.X)
	}
	name = ps6007NormalizeName(name)
	return ps6007ContainsAny(name, "splitpassprofilerevidence", "claimwitnessjoin", "contaminationwitnessprotocol", "splitpassmeasurementprotocol", "profilerjoincontract")
}

func ps6029Missing(fields map[string]ps6016Field) []string {
	missing := make([]string, 0, len(ps6029Axes))
	for _, axis := range ps6029Axes {
		if fields == nil || !axis.present(fields) {
			missing = append(missing, axis.name)
		}
	}
	return missing
}

func ps6029HardwareField(name string) bool {
	return ps6007ContainsAny(name, "hardware", "deviceidentity", "gpuid") && !strings.Contains(name, "join")
}

func ps6029WorkloadField(name string) bool {
	return strings.Contains(name, "workload") && strings.Contains(name, "digest") && !strings.Contains(name, "join")
}

func ps6029GraphField(name string) bool {
	return strings.Contains(name, "graph") && ps6007ContainsAny(name, "identity", "digest", "id") && !strings.Contains(name, "join")
}

func ps6029BuildField(name string) bool {
	return strings.Contains(name, "build") && ps6007ContainsAny(name, "identity", "digest", "id") && !strings.Contains(name, "join")
}

func ps6029ThermalField(name string) bool {
	return strings.Contains(name, "thermal") && strings.Contains(name, "process") && strings.Contains(name, "contract") && !strings.Contains(name, "join")
}

func ps6029ModeField(name string) bool {
	return strings.Contains(name, "protocol") && strings.Contains(name, "mode")
}

func ps6029StrictAvailableField(name string) bool {
	return strings.Contains(name, "strict") && strings.Contains(name, "onepass") && ps6007ContainsAny(name, "available", "supported")
}

func ps6029ClaimSignalField(name string) bool {
	return strings.Contains(name, "claim") && strings.Contains(name, "signal")
}

func ps6029WitnessSignalField(name string) bool {
	return strings.Contains(name, "witness") && strings.Contains(name, "signal")
}

func ps6029CountField(name, side, kind string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "pass") && strings.Contains(name, kind) && ps6007ContainsAny(name, "count", "passes", "required", "attempted", "accepted", "rejected")
}

func ps6029ClaimIDsField(name string) bool {
	return strings.Contains(name, "claim") && strings.Contains(name, "pass") && ps6007ContainsAny(name, "ids", "identities") && !strings.Contains(name, "join")
}

func ps6029WitnessIDsField(name string) bool {
	return strings.Contains(name, "witness") && strings.Contains(name, "pass") && ps6007ContainsAny(name, "ids", "identities") && !strings.Contains(name, "join")
}

func ps6029ReasonsField(name, side string) bool {
	return strings.Contains(name, side) && strings.Contains(name, "rejection") && strings.Contains(name, "reason")
}

func ps6029JoinWorkloadField(name string) bool {
	return strings.Contains(name, "join") && strings.Contains(name, "workload") && strings.Contains(name, "matched")
}

func ps6029JoinGraphField(name string) bool {
	return strings.Contains(name, "join") && strings.Contains(name, "graph") && strings.Contains(name, "matched")
}

func ps6029JoinHardwareField(name string) bool {
	return strings.Contains(name, "join") && ps6007ContainsAny(name, "hardware", "device") && strings.Contains(name, "matched")
}

func ps6029JoinBuildField(name string) bool {
	return strings.Contains(name, "join") && strings.Contains(name, "build") && strings.Contains(name, "matched")
}

func ps6029JoinThermalField(name string) bool {
	return strings.Contains(name, "join") && strings.Contains(name, "thermal") && strings.Contains(name, "process") && strings.Contains(name, "matched")
}

func ps6029ClaimIndependentField(name string) bool {
	return strings.Contains(name, "claim") && strings.Contains(name, "pass") && strings.Contains(name, "independent")
}

func ps6029WitnessIndependentField(name string) bool {
	return strings.Contains(name, "witness") && strings.Contains(name, "pass") && strings.Contains(name, "independent")
}

func ps6029WitnessAggregationField(name string) bool {
	return strings.Contains(name, "witness") && strings.Contains(name, "aggregation") && strings.Contains(name, "independent")
}

func ps6029ImputationField(name string) bool {
	return strings.Contains(name, "missingwitness") && ps6007ContainsAny(name, "imputedzero", "zeroimputed", "treatedaszero")
}

func ps6029ExactOutputField(name string) bool {
	return ps6007ContainsAny(name, "exactoutput", "outputdigest", "logitsdigest") && ps6007ContainsAny(name, "passed", "matched", "exact", "status")
}

func ps6029ExactTopologyField(name string) bool {
	return strings.Contains(name, "eventtopology") && ps6007ContainsAny(name, "exact", "passed", "matched", "status")
}

func ps6029PublishedField(name string) bool {
	return strings.Contains(name, "median") && ps6007ContainsAny(name, "published", "publication")
}

func ps6029SelectedField(name string) bool {
	return strings.Contains(name, "candidate") && ps6007ContainsAny(name, "selected", "selection")
}

func ps6029Audit(fields map[string]ps6016Field) []string {
	warnings := make([]string, 0, 14)
	for _, status := range []struct {
		name      string
		predicate func(string) bool
	}{
		{"strict one-pass availability", ps6029StrictAvailableField},
		{"workload-digest join", ps6029JoinWorkloadField},
		{"graph-identity join", ps6029JoinGraphField},
		{"hardware join", ps6029JoinHardwareField},
		{"build join", ps6029JoinBuildField},
		{"thermal/process join", ps6029JoinThermalField},
		{"claim-pass independence", ps6029ClaimIndependentField},
		{"witness-pass independence", ps6029WitnessIndependentField},
		{"independent witness aggregation", ps6029WitnessAggregationField},
		{"exact output/digest", ps6029ExactOutputField},
		{"exact event topology", ps6029ExactTopologyField},
	} {
		if value, known := ps6026Bool(fields, status.predicate); known && !value {
			warnings = append(warnings, status.name+" is explicitly false")
		}
	}
	if imputed, known := ps6026Bool(fields, ps6029ImputationField); known && imputed {
		warnings = append(warnings, "missing contamination witness is imputed as zero")
	}
	claim := ps6029Counts(fields, "claim")
	witness := ps6029Counts(fields, "witness")
	warnings = append(warnings, ps6029CountWarnings("claim", claim)...)
	warnings = append(warnings, ps6029CountWarnings("witness", witness)...)
	if claim.rejectedOK && claim.rejected > 0 && ps6029SequenceLength(fields, func(n string) bool { return ps6029ReasonsField(n, "claim") }) == 0 {
		warnings = append(warnings, "rejected claim passes have no disclosed reasons")
	}
	if witness.rejectedOK && witness.rejected > 0 && ps6029SequenceLength(fields, func(n string) bool { return ps6029ReasonsField(n, "witness") }) == 0 {
		warnings = append(warnings, "rejected witness passes have no disclosed reasons")
	}
	underfilled := claim.requiredOK && claim.acceptedOK && claim.accepted < claim.required ||
		witness.requiredOK && witness.acceptedOK && witness.accepted < witness.required
	if underfilled {
		if published, known := ps6026Bool(fields, ps6029PublishedField); known && published {
			warnings = append(warnings, "medians are published before both predeclared accepted-pass counts are satisfied")
		}
		if selected, known := ps6026Bool(fields, ps6029SelectedField); known && selected {
			warnings = append(warnings, "candidate is selected before both predeclared accepted-pass counts are satisfied")
		}
	}
	if mode, ok := ps6027String(fields, ps6029ModeField); ok {
		normalized := ps6029ModeReplacer.Replace(strings.ToLower(mode))
		if !ps6007ContainsAny(normalized, "splitpass", "strictonepass", "onepassstrict") {
			warnings = append(warnings, fmt.Sprintf("protocol mode %q is neither split-pass nor strict-one-pass", mode))
		} else if strings.Contains(normalized, "splitpass") && ps6029PassIDsOverlap(fields) {
			warnings = append(warnings, "split-pass claim and witness pass identities overlap")
		}
	}
	return warnings
}

type ps6029CountSet struct {
	required, attempted, accepted, rejected         float64
	requiredOK, attemptedOK, acceptedOK, rejectedOK bool
}

func ps6029Counts(fields map[string]ps6016Field, side string) ps6029CountSet {
	required, requiredOK := ps6016Number(fields, func(n string) bool { return ps6029CountField(n, side, "required") })
	attempted, attemptedOK := ps6016Number(fields, func(n string) bool { return ps6029CountField(n, side, "attempt") })
	accepted, acceptedOK := ps6016Number(fields, func(n string) bool { return ps6029CountField(n, side, "accepted") })
	rejected, rejectedOK := ps6016Number(fields, func(n string) bool { return ps6029CountField(n, side, "rejected") })
	return ps6029CountSet{required, attempted, accepted, rejected, requiredOK, attemptedOK, acceptedOK, rejectedOK}
}

func ps6029CountWarnings(side string, counts ps6029CountSet) []string {
	var warnings []string
	if counts.requiredOK && counts.required < 1 {
		warnings = append(warnings, side+" required-pass count must be positive")
	}
	if counts.attemptedOK && counts.acceptedOK && counts.rejectedOK && counts.accepted+counts.rejected != counts.attempted {
		warnings = append(warnings, fmt.Sprintf("%s pass accounting disagrees (accepted %.0f + rejected %.0f != attempted %.0f)", side, counts.accepted, counts.rejected, counts.attempted))
	}
	if counts.requiredOK && counts.attemptedOK && counts.required > counts.attempted {
		warnings = append(warnings, fmt.Sprintf("%s requires %.0f accepted passes but permits only %.0f attempts", side, counts.required, counts.attempted))
	}
	return warnings
}

func ps6029SequenceLength(fields map[string]ps6016Field, predicate func(string) bool) int {
	for name, field := range fields {
		if !predicate(name) {
			continue
		}
		if field.hasStringValues {
			return len(field.stringValues)
		}
		if field.hasNumbers {
			return len(field.numbers)
		}
	}
	return 0
}

func ps6029PassIDsOverlap(fields map[string]ps6016Field) bool {
	claim := ps6029StringValues(fields, ps6029ClaimIDsField)
	witness := ps6029StringValues(fields, ps6029WitnessIDsField)
	for _, id := range witness {
		if slices.Contains(claim, id) {
			return true
		}
	}
	return false
}

func ps6029StringValues(fields map[string]ps6016Field, predicate func(string) bool) []string {
	for name, field := range fields {
		if predicate(name) && field.hasStringValues {
			return field.stringValues
		}
	}
	return nil
}
