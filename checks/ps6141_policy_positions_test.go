package checks

import (
	"bytes"
	"encoding/json"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/scanscope"
	"golang.org/x/tools/go/analysis"
)

func TestPS6141PolicyRejectsStaleLoadedPhysicalOffsets(t *testing.T) {
	t.Parallel()
	for _, equalLength := range []bool{false, true} {
		t.Run(map[bool]string{false: "insertedWhitespace", true: "movedWhitespace"}[equalLength], func(t *testing.T) {
			t.Parallel()
			original := strings.Replace(ps6141TwoInputSynthetic, "p:=pack(x)", "if len(x)!=4{panic(\"shape\")};p:=pack(x)", 1)
			loaded, owner := ps6141TypedFixture(t, original)
			c := ps6141TestContract()
			c.ConsumerForm, c.WeightArgument, c.RowsArgument = "twoInputDot", 1, -1
			candidate := ps6141Candidates(loaded, owner, &c)
			if len(candidate) != 1 {
				t.Fatal("loaded candidate prerequisite missing")
			}
			current := strings.Replace(original, ";p:=pack(x)", "; p:=pack(x)", 1)
			if equalLength {
				current = strings.Replace(current, "\n return dot(p,w)", "\nreturn dot(p,w)", 1)
			}
			fresh, freshOwner := ps6141TypedFixture(t, current)
			freshCandidate := ps6141Candidates(fresh, freshOwner, &c)
			if len(freshCandidate) != 1 {
				t.Fatal("fresh candidate prerequisite missing")
			}
			var oldSyntax, freshSyntax bytes.Buffer
			if format.Node(&oldSyntax, loaded.Fset, loaded.Files[0]) != nil || format.Node(&freshSyntax, fresh.Fset, fresh.Files[0]) != nil || !bytes.Equal(oldSyntax.Bytes(), freshSyntax.Bytes()) {
				t.Fatal("control did not preserve canonical syntax")
			}
			oldFile := loaded.Fset.File(candidate[0].quantizer.Pos())
			freshFile := fresh.Fset.File(freshCandidate[0].quantizer.Pos())
			if oldFile.Offset(candidate[0].quantizer.Pos()) == freshFile.Offset(freshCandidate[0].quantizer.Pos()) || equalLength && len(current) != len(original) {
				t.Fatal("control did not change physical site offsets as intended")
			}
			loaded.ReadFile = func(string) ([]byte, error) { return []byte(current), nil }
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
			loaded.ResultOf = map[*analysis.Analyzer]any{scanscope.Key: target}
			policy := ps6141Policy{Schema: 1, Owner: "fixture.owner", Quantizer: c.Quantizer, Consumer: c.Consumer, ConsumerForm: c.ConsumerForm, File: "synthetic.go", QuantizerOffset: oldFile.Offset(candidate[0].quantizer.Pos()), ConsumerOffset: oldFile.Offset(candidate[0].consumer.Pos()), Elements: 4, SourceSHA256: map[string]string{"synthetic.go": ps6141PolicyDigest([]byte(current))}, GoVersion: target.GoVersion, GOOS: target.GOOS, GOARCH: target.GOARCH, ContextSHA256: target.ContextSHA256, TargetPolicy: "architecture-wide-operator-reviewed", Boundary: "complete-conversion-and-consumer", Review: "Synthetic reviewed complete-boundary policy; current hashes but deliberately stale old site offsets."}
			data, err := json.Marshal(policy)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "reviewed-policy.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			c.BenchmarkExemptions = []config.SingleUseQuantizationExemption{{Policy: path, SHA256: ps6141PolicyDigest(data)}}
			if got := len(ps6141Candidates(loaded, owner, &c)); got != 1 {
				t.Fatalf("stale physical offsets exempted candidate: %d", got)
			}
		})
	}
}
