package checks

import (
	"os"
	"testing"

	"go/types"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6124IndependentReview(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source string }{
		{"unreachable_after_return", `func reviewCase() {
    model, _ := model.Load()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    return
    model.DecodeStep(ctx)
}`},
		{"backend_short_declaration_reuse", `func reviewCase() {
    model, _ := model.Load()
    cpu, _ := backend.Get(backend.CPU)
    cpu, err := backend.Get(backend.GPU)
    _ = err
    ctx := backend.NewContext().WithBackend(cpu)
    model.DecodeStep(ctx)
}`},
		{"dead_closure_invocation", `func reviewCase() {
    model, _ := model.Load()
    run := func() {
        cpu, _ := backend.Get(backend.CPU)
        ctx := backend.NewContext().WithBackend(cpu)
        model.DecodeStep(ctx)
    }
    if false { run() }
}`},
		{"invoked_helper_closure_writer", `func mutationHelper() {
    mutate := func() { backend.SetPreference(backend.GPU) }
    mutate()
}
func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    model, _ := model.Load()
    mutationHelper()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    model.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}`},
		{"snapshot_rebinding_after_defer_is_safe", `func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    model, _ := model.Load()
    previous = nil
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    model.DecodeStep(ctx)
}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			files := map[string]string{"ps6124/review.go": "package ps6124\nimport (\"ps6124backend\"; \"ps6124model\")\n" + tc.source}
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
				Name: "PS6124", Doc: "independent semantic review",
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
