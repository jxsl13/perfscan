package checks

import (
	"crypto/sha256"
	"fmt"
	"go/types"
	"os"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6124TypedRouteAndScopedLifetime(t *testing.T) {
	t.Parallel()
	contracts := []config.ScopedBackendRoutingContract{
		ps6124TestContract("baseline"),
		ps6124TestContract("fixed"),
		ps6124TestContract("late"),
		ps6124TestContract("unrelated"),
		ps6124TestContract("conditionalMutation"),
		ps6124TestContract("helperMutation"),
		ps6124TestContract("snapshotOverwrite"),
		ps6124TestContract("indirectUnknown"),
		ps6124TestContract("capturedReassign"),
		ps6124TestContract("tupleReuse"),
		ps6124TestContract("deadRoute"),
		ps6124TestContract("parallelPin"),
		ps6124TestContract("snapshotAliasWrite"),
		ps6124TestContract("contextReassign"),
		ps6124TestContract("ignoredCallbackWriter"),
		ps6124TestContract("nestedUninvokedWriter"),
		ps6124TestContract("liveWriterDeadSecond"),
		ps6124TestContract("noopThenWriter"),
	}
	analyzer := &analysis.Analyzer{
		Name: "PS6124", Doc: "PS6124 test",
		FactTypes: []analysis.Fact{new(ps6124RouteFact)},
		Run:       func(pass *analysis.Pass) (any, error) { return runPS6124WithContracts(pass, contracts) },
	}
	analysistest.Run(t, analysistest.TestData(), analyzer, "ps6124")
}

func TestPS6124CallableDefinitionKills(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, helpers, action string
		want                  bool
	}{
		{"alias_killed", `func invoke(cb func()) { next := cb; next = func(){}; next() }`, `invoke(func(){ backend.SetPreference(backend.GPU) })`, false},
		{"captured_parameter_killed", `func invoke(cb func()) { cb = func(){}; func(){ cb() }() }`, `invoke(func(){ backend.SetPreference(backend.GPU) })`, false},
		{"captured_parameter_killed_before_named_literal_call", `func invoke(cb func()) { run:=func(){cb()}; cb=func(){}; run() }`, `invoke(func(){ backend.SetPreference(backend.GPU) })`, false},
		{"conditional_capture_preserves_incoming", `func invoke(cb func(), replace bool) { run:=func(){cb()}; if replace { cb=func(){} }; run() }`, `invoke(func(){ backend.SetPreference(backend.GPU) }, true)`, true},
		{"named_writer_through_helper", `func invoke(cb func()) { cb() }; func namedWriter(){ backend.SetPreference(backend.GPU) }; func helper(){ invoke(namedWriter) }`, `helper()`, true},
		{"named_writer_direct_argument", `func invoke(cb func()) { cb() }; func namedWriter(){ backend.SetPreference(backend.GPU) }`, `invoke(namedWriter)`, true},
		{"named_noop_through_helper", `func invoke(cb func()) { cb() }; func namedNoop(){}; func helper(){ invoke(namedNoop) }`, `helper()`, false},
		{"both_branches_kill_literal", `func helper(pick bool) { run := func(){ backend.SetPreference(backend.GPU) }; if pick { run=func(){} } else { run=func(){} }; run() }`, `helper(true)`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			want := ""
			if tc.want {
				want = ` // want "selected Context reaches a source-proven downstream global backend router"`
			}
			source := "package ps6124\nimport (\"ps6124backend\"; \"ps6124model\")\n" + tc.helpers + `
func reviewCase() {
 previous := backend.Preference(); backend.SetPreference(backend.CPU); defer backend.SetPreference(previous...)
 loaded, _ := model.Load(); ` + tc.action + `
 cpu, _ := backend.Get(backend.CPU); ctx := backend.NewContext().WithBackend(cpu)
 loaded.DecodeStep(ctx)` + want + `
}`
			files := map[string]string{"ps6124/review.go": source}
			for _, path := range []string{"ps6124backend/backend.go", "ps6124model/model.go", "ps6124linear/linear.go"} {
				data, err := os.ReadFile("testdata/src/" + path)
				if err != nil {
					t.Fatal(err)
				}
				files[path] = string(data)
			}
			dir, cleanup, err := analysistest.WriteFiles(files)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(cleanup)
			analyzer := &analysis.Analyzer{Name: "PS6124", Doc: "callable definition kills", FactTypes: []analysis.Fact{new(ps6124RouteFact)}, Run: func(pass *analysis.Pass) (any, error) {
				if pass.Pkg.Path() == "ps6124" {
					pass.ExportObjectFact = func(types.Object, analysis.Fact) {}
				}
				return runPS6124WithContracts(pass, []config.ScopedBackendRoutingContract{ps6124TestContract("reviewCase")})
			}}
			analysistest.Run(t, dir, analyzer, "ps6124")
		})
	}
}

func TestPS6124PinnedOwnerBeforeAfter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		file, digest string
		pinned       bool
	}{
		{"ps6124_owner_before.go.txt", "1a1da7b002de31ed39298613420f283af522694c189b6af97def98f2a35c35e4", false},
		{"ps6124_owner_after.go.txt", "dd54c68fb119358b52f716864ef32fd21d375d3cc82586b61f590efa68a514a6", true},
	} {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile("testdata/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != tc.digest {
				t.Fatalf("digest=%s want=%s", got, tc.digest)
			}
			text := string(data)
			hasPin := strings.Contains(text, "preference := backend.Preference()\n\tbackend.SetPreference(backend.CPU)\n\tdefer backend.SetPreference(preference...)")
			if hasPin != tc.pinned {
				t.Fatalf("scoped pin=%v want=%v", hasPin, tc.pinned)
			}
			for _, route := range []string{"nlp.QuantLlamaFromGGUF", "backend.Get(backend.CPU)", "WithBackend(cpu)", "model.DecodeStep(ctx"} {
				if !strings.Contains(text, route) {
					t.Errorf("owner fixture lacks %q", route)
				}
			}
		})
	}
}

func TestPS6124MetadataAndContractValidation(t *testing.T) {
	t.Parallel()
	if PS6124.AutoFix || !PS6124.NeedsConfig || PS6124.Level != 3 || len(PS6124.Vocab) != 1 || PS6124.Vocab[0] != "scopedBackendRoutingContracts" {
		t.Fatalf("PS6124 metadata drift: %+v", PS6124)
	}
	contract := ps6124TestContract("baseline")
	if !contract.Valid() {
		t.Fatal("complete PS6124 contract is invalid")
	}
	contract.ProcessGlobalWriterIsolation = false
	if contract.Valid() {
		t.Fatal("contract without reviewed process-global writer isolation is valid")
	}
}

func TestPS6124ConfigCompileCopiesNestedVocabulary(t *testing.T) {
	t.Parallel()
	contract := ps6124TestContract("baseline")
	compiled := (config.Config{ScopedBackendRoutingContracts: []config.ScopedBackendRoutingContract{contract}}).Compile()
	contract.ConstructionCallables[0] = "example.com/changed.Load"
	contract.GlobalRouters[0] = "example.com/changed.Default"
	if compiled.ScopedBackendRoutingContracts[0].ConstructionCallables[0] != "ps6124model.Load" || compiled.ScopedBackendRoutingContracts[0].GlobalRouters[0] != "ps6124backend.Default" {
		t.Fatal("compiled PS6124 nested vocabulary aliases caller storage")
	}
}

func ps6124TestContract(site string) config.ScopedBackendRoutingContract {
	return config.ScopedBackendRoutingContract{
		Name: "test routing", ConfiguredSite: "ps6124." + site,
		BackendGet: "ps6124backend.Get", ContextWithBackend: "ps6124backend.Context.WithBackend",
		Preference: "ps6124backend.Preference", SetPreference: "ps6124backend.SetPreference",
		SelectedBackend:       "ps6124backend.CPU",
		ConstructionCallables: []string{"ps6124model.Load"},
		GlobalRouters:         []string{"ps6124backend.Default"}, ProcessGlobalWriterIsolation: true,
	}
}
