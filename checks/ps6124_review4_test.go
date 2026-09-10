package checks

import (
	"go/types"
	"os"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6124IndependentCallableValueReview(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, helpers, action, source string
		want                          bool
	}{
		{name: "aliased_callback_parameter", helpers: `func invoke(cb func()) { next := cb; next() }`, action: `invoke(func(){ backend.SetPreference(backend.GPU) })`, want: true},
		{name: "callback_parameter_inside_invoked_literal", helpers: `func invoke(cb func()) { func(){ cb() }() }`, action: `invoke(func(){ backend.SetPreference(backend.GPU) })`, want: true},
		{name: "all_branches_kill_callback_parameter", helpers: `func invoke(cb func(), pick bool) { if pick { cb = func(){} } else { cb = func(){} }; cb() }`, action: `invoke(func(){ backend.SetPreference(backend.GPU) }, true)`},
		{name: "known_named_benign_callback", helpers: `func invoke(cb func()) { cb() }; func noop() {}`, action: `invoke(noop)`},
		{name: "constructor_excludes_closure_invocation", source: `func reviewCase(construct bool) {
    var loaded *model.Model
    if construct {
        loaded, _ = model.Load()
    } else {
        run := func() {
            cpu, _ := backend.Get(backend.CPU)
            ctx := backend.NewContext().WithBackend(cpu)
            loaded.DecodeStep(ctx)
        }
        run()
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
			files := map[string]string{"ps6124/review.go": "package ps6124\nimport (\"ps6124backend\"; \"ps6124model\")\n" + source}
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
				Name: "PS6124", Doc: "independent callable-value review",
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
