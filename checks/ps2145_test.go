package checks

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS2145(t *testing.T) {
	t.Parallel()
	t.Run("positive", func(t *testing.T) {
		t.Parallel()
		results := analysistest.Run(t, analysistest.TestData(), PS2145.Analyzer, "ps2145", "ps2145alias")
		for _, result := range results {
			for _, diagnostic := range result.Diagnostics {
				if len(diagnostic.SuggestedFixes) != 0 {
					t.Fatalf("PS2145 is advisory but returned fixes: %#v", diagnostic.SuggestedFixes)
				}
			}
		}
	})
	t.Run("negative", func(t *testing.T) {
		t.Parallel()
		analysistest.Run(t, analysistest.TestData(), PS2145.Analyzer, "ps2145neg")
	})
}

func TestPS2145TargetIntWidth(t *testing.T) {
	t.Parallel()
	const childVariable = "PERFSCAN_PS2145_386_CHILD"
	if os.Getenv(childVariable) == "1" {
		analysistest.Run(t, analysistest.TestData(), PS2145.Analyzer, "ps2145arch32")
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestPS2145TargetIntWidth$", "-test.parallel=1")
	command.Env = append(os.Environ(), "GOOS=linux", "GOARCH=386", "CGO_ENABLED=0", childVariable+"=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("386 analysistest child failed: %v\n%s", err, output)
	}
}

func TestPS2145Deterministic(t *testing.T) {
	t.Parallel()
	var first []string
	for iteration := 0; iteration < 3; iteration++ {
		results := analysistest.Run(t, analysistest.TestData(), PS2145.Analyzer, "ps2145", "ps2145alias")
		var current []string
		for _, result := range results {
			for _, diagnostic := range result.Diagnostics {
				position := result.Pass.Fset.Position(diagnostic.Pos)
				current = append(current, fmt.Sprintf("%s:%d:%d:%s", position.Filename, position.Line, position.Column, diagnostic.Message))
			}
		}
		sort.Strings(current)
		if iteration == 0 {
			first = current
			continue
		}
		if strings.Join(current, "\n") != strings.Join(first, "\n") {
			t.Fatalf("iteration %d diagnostics differ:\nfirst: %q\ncurrent: %q", iteration, first, current)
		}
	}
}

func TestPS2145Documentation(t *testing.T) {
	t.Parallel()
	text := strings.Join(strings.Fields(PS2145.Doc.Text+"\n"+PS2145.Doc.After+"\n"+PS2145.Doc.MeasuredWin), " ")
	for _, want := range []string{
		"os.Open does not prove that a path names a regular file",
		"separate conditional regular-file API",
		"Reuse the same unadvanced handle",
		"invalid immediately after the owner is closed",
		"concurrent file truncation can fault",
		"full consumption of every encoded tensor",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("PS2145 documentation does not contain %q", want)
		}
	}
}
