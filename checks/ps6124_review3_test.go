package checks

import (
	"go/types"
	"os"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6124IndependentCallbackSummaryReview(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, helpers, action, source string
		want                          bool
	}{
		{name: "grouped_parameter_second_writer", helpers: `func second(a, b func()) { b() }`, action: `second(func(){}, func(){ backend.SetPreference(backend.GPU) })`, want: true},
		{name: "grouped_parameter_first_ignored", helpers: `func second(a, b func()) { b() }`, action: `second(func(){ backend.SetPreference(backend.GPU) }, func(){})`},
		{name: "rebound_callback_argument_is_not_invoked", helpers: `func invoke(cb func()) { cb = func(){}; cb() }`, action: `invoke(func(){ backend.SetPreference(backend.GPU) })`},
		{name: "forwarded_callback_writer", helpers: `func invoke(cb func()) { cb() }; func relay(cb func()) { invoke(cb) }`, action: `relay(func(){ backend.SetPreference(backend.GPU) })`, want: true},
		{name: "imported_callback_writer", action: `callbacks.Invoke(func(){ backend.SetPreference(backend.GPU) })`, want: true},
		{name: "method_expression_receiver_offset", helpers: `type Invoker struct{}; func (Invoker) Invoke(cb func()) { cb() }`, action: `Invoker.Invoke(Invoker{}, func(){})`},
		{name: "constant_branch_replaces_writer", helpers: `func helper() { run := func(){ backend.SetPreference(backend.GPU) }; if true { run = func(){} }; run() }`, action: `helper()`},
		{name: "conditional_rebind_can_write", helpers: `func helper(change bool) { run := func(){}; if change { run = func(){ backend.SetPreference(backend.GPU) } }; run() }`, action: `helper(true)`, want: true},
		{name: "direct_literal_invocation_writer", helpers: `func helper() { func(){ backend.SetPreference(backend.GPU) }() }`, action: `helper()`, want: true},
		{name: "construction_excludes_route", source: `func reviewCase(construct bool) {
    var loaded *model.Model
    if construct {
        loaded, _ = model.Load()
    } else {
        cpu, _ := backend.Get(backend.CPU)
        ctx := backend.NewContext().WithBackend(cpu)
        loaded.DecodeStep(ctx)
    }
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			source := tc.source
			if source == "" {
				want := ""
				if tc.want {
					want = ` // want "selected Context reaches a source-proven downstream global backend router"`
				}
				source = tc.helpers + `
func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    loaded, _ := model.Load()
    ` + tc.action + `
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx)` + want + `
}`
			}
			files := map[string]string{
				"ps6124/review.go":             "package ps6124\nimport (\"ps6124backend\"; \"ps6124model\"; \"ps6124callbacks\")\nvar _ = callbacks.Invoke\n" + source,
				"ps6124callbacks/callbacks.go": "package callbacks\nfunc Invoke(cb func()) { cb() }\n",
			}
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
			analyzer := &analysis.Analyzer{
				Name: "PS6124", Doc: "independent callback-summary review",
				FactTypes: []analysis.Fact{new(ps6124RouteFact)},
				Run: func(pass *analysis.Pass) (any, error) {
					if pass.Pkg.Path() == "ps6124" {
						pass.ExportObjectFact = func(types.Object, analysis.Fact) {}
					}
					return runPS6124WithContracts(pass, []config.ScopedBackendRoutingContract{ps6124TestContract("reviewCase")})
				},
			}
			analysistest.Run(t, dir, analyzer, "ps6124")
		})
	}
}
