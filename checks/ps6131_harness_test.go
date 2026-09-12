package checks

import (
	"github.com/jxsl13/perfscan/internal/crossover"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPS6131HarnessRejectsMismatchedFactory(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, factory string
		valid         bool
	}{
		{"exact", "func inputs(n int) []float32 {return make([]float32,n)}", true},
		{"cross_dtype", "func inputs(n int) []float64 {return make([]float64,n)}", false},
		{"not_fixed_shape", "func inputs(n uint) []float32 {return make([]float32,n)}", false},
		{"wrong_tuple", "func inputs(n int) ([]float32,error) {return make([]float32,n),nil}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pass := ps6131SourcePass(t, ps6131SourceFixture+tc.factory)
			c := ps6131TestContract()
			c.OperationInputFactory = "cross.inputs"
			source, ok := ps6131SourceBound(pass, &c)
			if !ok {
				t.Fatal("source route missing")
			}
			model, err := ps6131Harness(pass, source, &c)
			if (err == nil) != tc.valid {
				t.Fatalf("factory qualification: model=%+v err=%v", model, err)
			}
			if tc.valid && (model.OutputExtent != "len(_r0)" || !strings.Contains(model.SerialBody, "if true") || !strings.Contains(model.ParallelBody, "if false")) {
				t.Fatalf("lost typed shape/source join: %+v", model)
			}
		})
	}
}

func TestPS6131GeneratedHarnessCompilesAndExecutes(t *testing.T) {
	t.Parallel()
	fixture := ps6131SourceFixture + "func inputs(n int) []float32 {return make([]float32,n)}"
	pass := ps6131SourcePass(t, fixture)
	c := ps6131TestContract()
	c.OperationInputFactory = "cross.inputs"
	source, ok := ps6131SourceBound(pass, &c)
	if !ok {
		t.Fatal("unbound fixture")
	}
	model, err := ps6131Harness(pass, source, &c)
	if err != nil {
		t.Fatal(err)
	}
	c.SerialOperation = "cross.forcedSerial"
	c.ParallelOperation = "cross.forcedParallel"
	c.DiagnosticFunction = "cross.TestDiagnostic"
	generated, err := crossover.Generate(model, &c)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, data := range map[string][]byte{"go.mod": []byte("module cross\ngo 1.26\nrequire github.com/jxsl13/perfscan v0.0.0\nreplace github.com/jxsl13/perfscan => " + filepath.ToSlash(root) + "\n"), "source.go": []byte(fixture), crossover.HarnessFile: generated} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, scope := range []string{"policy", "forced-operation", "production"} {
		command := exec.Command("go", "test", "-run", "^TestDiagnostic$", "-count=1", "-benchtime=8x")
		// Go's test driver does not forward a non-standard -benchtime spelling;
		// native test flags are forwarded after -args and checked by Measure.
		command.Args = []string{"go", "test", "-run", "^TestDiagnostic$", "-count=1", "-args", "-test.benchtime=8x"}
		command.Dir = dir
		command.Env = append(os.Environ(), "GOWORK=off", "PERFSCAN_CROSSOVER_SIZE=8", "PERFSCAN_CROSSOVER_N=8", "PERFSCAN_CROSSOVER_ARM=parallel", "PERFSCAN_CROSSOVER_SCOPE="+scope)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("generated %s harness failed: %v\n%s", scope, err, output)
		}
	}
}
