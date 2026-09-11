package checks

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

func TestPS6126(t *testing.T) {
	t.Parallel()
	path := filepath.Join(analysistest.TestData(), "src", "ps6126", "ps6126.go")
	directory := t.TempDir()
	beforePath := filepath.Join(directory, "before.go")
	beforeSource := `package ps6126
var saved func(int,int)
func parallelWork(workers, grain int, work func(int, int)) { saved = work }
func growing(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte) {
	parallelWork(4,64,func(lo,hi int){ _,_,_,_,_,_,_,_,_,_ = a0,a1,a2,a3,a4,a5,a6,a7,a8,lo+hi })
}
`
	if err := os.WriteFile(beforePath, []byte(beforeSource), 0o600); err != nil {
		t.Fatal(err)
	}
	left, err := closureenv.CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, beforePath)
	if err != nil {
		t.Fatal(err)
	}
	right, err := closureenv.CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, path)
	if err != nil {
		t.Fatal(err)
	}
	pick := func(all []closureenv.Evidence) *closureenv.Evidence {
		for index := range all {
			evidence := &all[index]
			if evidence.Function == "growing" {
				return evidence
			}
		}
		t.Fatal("compiler did not emit growing closure evidence")
		return nil
	}
	before, after := pick(left), pick(right)
	growth, err := closureenv.Compare(before, after)
	if err != nil {
		t.Fatal(err)
	}
	var artifact bytes.Buffer
	if err := closureenv.WriteArtifact(&artifact, growth); err != nil {
		t.Fatal(err)
	}
	analyzer := *PS6126.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6126WithEvidence(pass, []string{artifact.String()}, map[string]bool{"ps6126.parallelWork": true}, false)
	}
	analysistest.Run(t, analysistest.TestData(), &analyzer, "ps6126")
}

func TestPS6126RejectsNonCurrentEvidence(t *testing.T) {
	t.Parallel()
	evidence := closureenv.Evidence{GoVersion: "go0.invalid", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Escapes: true, EnvironmentBytes: 8, SizeClassBytes: 8}
	if closureenv.ValidateCurrent(&evidence) == nil {
		t.Fatal("accepted stale compiler evidence")
	}
}
