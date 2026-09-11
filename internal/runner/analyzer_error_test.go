package runner

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/lint"
)

// A failed analyzer must never certify an empty scan or write a partial baseline.
func TestAnalyzerErrorFailsScan(t *testing.T) {
	t.Parallel()
	// Run owns process-global configuration; isolate parallel cases in processes.
	mode := os.Getenv("PERFSCAN_ANALYZER_ERROR_CASE")
	if mode == "" {
		for _, name := range []string{"syntax", "facts"} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				cmd := exec.Command(os.Args[0], "-test.run=^TestAnalyzerErrorFailsScan$")
				cmd.Env = append(os.Environ(), "PERFSCAN_ANALYZER_ERROR_CASE="+name)
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("isolated runner regression: %v: %s", err, output)
				}
			})
		}
		return
	}
	t.Run(mode, func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "sample.go")
		if err := os.WriteFile(path, []byte("package sample\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		check := &lint.Check{ID: "PS9999", Level: lint.LevelIdiomatic, Analyzer: &analysis.Analyzer{
			Name: "PS9999", Run: func(pass *analysis.Pass) (any, error) {
				pass.Reportf(pass.Files[0].Pos(), "partial diagnostic")
				return nil, errors.New("compiler evidence is stale")
			},
		}}
		if mode == "facts" {
			check.Analyzer.FactTypes = []analysis.Fact{new(analyzerErrorFact)}
		}
		var stdout, stderr bytes.Buffer
		baseline := filepath.Join(dir, "must-not-exist.yaml")
		code := Run([]*lint.Check{check}, Options{Patterns: []string{path}, Checks: "PS9999", ExitZero: true, WriteBaseline: true, Baseline: baseline, Stdout: &stdout, Stderr: &stderr})
		if code != 2 || !strings.Contains(stderr.String(), "compiler evidence is stale") || stdout.Len() != 0 {
			t.Fatalf("failed analyzer: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
		}
		if _, err := os.Stat(baseline); !os.IsNotExist(err) {
			t.Fatalf("failed analyzer wrote a baseline: %v", err)
		}
	})
}

type analyzerErrorFact struct{}

func (*analyzerErrorFact) AFact() {}
