package checks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/internal/crossover"
)

func TestPS6131PerArmPreflightRejectsRestorationAliasingAndProductionMismatch(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, serial, parallel, extra, scope string }{
		{"serial_mutation_parallel_restores", "leaf(dst,src);src[0]++", "src[0]--;worker(len(src),func(lo,hi int){leaf(dst[lo:hi],src[lo:hi])})", "", "forced-operation"},
		{"policy_serial_mutation_parallel_restores", "leaf(dst,src);src[0]++", "src[0]--;worker(len(src),func(lo,hi int){leaf(dst[lo:hi],src[lo:hi])})", "", "policy"},
		{"parallel_overwrites_serial_storage", "leaf(dst,src);shared=dst", "worker(len(src),func(lo,hi int){leaf(dst[lo:hi],src[lo:hi])});for i:=range dst {dst[i]++;shared[i]=dst[i]}", "var shared []float32\n", "forced-operation"},
		{"policy_parallel_overwrites_serial_storage", "leaf(dst,src);shared=dst", "worker(len(src),func(lo,hi int){leaf(dst[lo:hi],src[lo:hi])});for i:=range dst {dst[i]++;shared[i]=dst[i]}", "var shared []float32\n", "policy"},
		{"production_third_call_diverges", "leaf(dst,src);calls++;if calls==3 {dst[0]++}", "worker(len(src),func(lo,hi int){leaf(dst[lo:hi],src[lo:hi])});calls++;if calls==3 {dst[0]++}", "var calls int\n", "production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := strings.Replace(ps6131SourceFixture, "func serial(dst,src []float32) { leaf(dst,src) }", "func serial(dst,src []float32) { "+tc.serial+" }", 1)
			fixture = strings.Replace(fixture, "func parallel(dst,src []float32) { worker(len(src),func(lo,hi int){ leaf(dst[lo:hi],src[lo:hi]) }) }", "func parallel(dst,src []float32) { "+tc.parallel+" }", 1)
			fixture += tc.extra + "func inputs(n int) []float32 {return make([]float32,n)}\n"
			pass := ps6131SourcePass(t, fixture)
			c := ps6131TestContract()
			c.OperationInputFactory = "cross.inputs"
			c.DiagnosticFunction = "cross.TestDiagnostic"
			source, ok := ps6131SourceBound(pass, &c)
			if !ok {
				t.Fatal("adversarial fixture lost source route")
			}
			model, err := ps6131Harness(pass, source, &c)
			if err != nil {
				t.Fatal(err)
			}
			harness, err := crossover.Generate(model, &c)
			if err != nil {
				t.Fatal(err)
			}
			root, err := filepath.Abs("..")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for name, data := range map[string][]byte{"go.mod": []byte("module cross\ngo 1.26\nrequire github.com/jxsl13/perfscan v0.0.0\nreplace github.com/jxsl13/perfscan => " + filepath.ToSlash(root) + "\n"), "source.go": []byte(fixture), crossover.HarnessFile: harness} {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("go", "test", "-run", "^TestDiagnostic$", "-count=1", "-args", "-test.benchtime=8x")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOWORK=off", "PERFSCAN_CROSSOVER_SIZE=8", "PERFSCAN_CROSSOVER_N=8", "PERFSCAN_CROSSOVER_ARM=parallel", "PERFSCAN_CROSSOVER_SCOPE="+tc.scope)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("accepted per-arm mutation/alias/divergence: %s", out)
			}
			// testing.Benchmark retains b.Fatal's log privately. Require the
			// helper's explicit failed/exited status, not an unrelated compiler
			// failure; positive generated-harness executions test all scopes.
			if !strings.Contains(string(out), "diagnostic benchmark failed or exited") {
				t.Fatalf("failed without exercising benchmark integrity rejection: %v %s", err, out)
			}
		})
	}
}
