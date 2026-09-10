package checks

import (
	"go/types"
	"os"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/config"
)

func TestPS6124IndependentControlFlowReview(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source string }{
		{"callback_argument_writer", `func invoke(callback func()) { callback() }
func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    loaded, _ := model.Load()
    invoke(func() { backend.SetPreference(backend.GPU) })
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}`},
		{"nested_invoked_writer", `func mutateHelper() {
    outer := func() {
        inner := func() { backend.SetPreference(backend.GPU) }
        inner()
    }
    outer()
}
func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    loaded, _ := model.Load()
    mutateHelper()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}`},
		{"replaced_uninvoked_writer", `func noop() {}
func mutateHelper() {
    run := func() { backend.SetPreference(backend.GPU) }
    run = noop
    run()
}
func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    loaded, _ := model.Load()
    mutateHelper()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx)
}`},
		{"backend_pointer_write", `func replaceBackend(p **backend.Backend) { *p, _ = backend.Get(backend.GPU) }
func reviewCase() {
    loaded, _ := model.Load()
    cpu, _ := backend.Get(backend.CPU)
    replaceBackend(&cpu)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx)
}`},
		{"goto_bypasses_pin", `func reviewCase(skip bool) {
    var previous []backend.Kind
    if skip { goto construct }
    previous = backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
construct:
    loaded, _ := model.Load()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx) // want "selected Context reaches a source-proven downstream global backend router"
}`},
		{"mutually_exclusive_context_and_route", `func reviewCase(selected bool) {
    loaded, _ := model.Load()
    var ctx *backend.Context
    if selected {
        cpu, _ := backend.Get(backend.CPU)
        ctx = backend.NewContext().WithBackend(cpu)
    } else {
        loaded.DecodeStep(ctx)
    }
}`},
		{"constant_dead_for", `func reviewCase() {
    loaded, _ := model.Load()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    for false { loaded.DecodeStep(ctx) }
}`},
		{"route_after_builtin_panic", `func reviewCase() {
    loaded, _ := model.Load()
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    panic("no route")
    loaded.DecodeStep(ctx)
}`},
		{"dead_writer_does_not_invalidate_pin", `func reviewCase() {
    previous := backend.Preference()
    backend.SetPreference(backend.CPU)
    defer backend.SetPreference(previous...)
    loaded, _ := model.Load()
    if false { backend.SetPreference(backend.GPU) }
    cpu, _ := backend.Get(backend.CPU)
    ctx := backend.NewContext().WithBackend(cpu)
    loaded.DecodeStep(ctx)
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
				Name: "PS6124", Doc: "independent control-flow review",
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
