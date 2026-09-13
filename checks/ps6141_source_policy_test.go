package checks

import (
	"encoding/json"
	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/scanscope"
	"go/ast"
	"golang.org/x/tools/go/analysis"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ps6141TestAttachSourcePolicy(t *testing.T, pass *analysis.Pass, owner *ast.FuncDecl, source string, c *config.SingleUseQuantizationContract, elements int64) {
	t.Helper()
	pass.ReadFile = func(string) ([]byte, error) { return []byte(source), nil }
	before := ps6141Candidates(pass, owner, c)
	if len(before) != 1 {
		t.Fatal("genuine pinned source policy prerequisite missing")
	}
	file := pass.Fset.File(before[0].quantizer.Pos())
	target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
	pass.ResultOf = map[*analysis.Analyzer]any{scanscope.Key: target}
	policy := ps6141Policy{Schema: 1, Owner: "fixture.owner", Quantizer: c.Quantizer, Consumer: c.Consumer, ConsumerForm: c.ConsumerForm, File: "synthetic.go", QuantizerOffset: file.Offset(before[0].quantizer.Pos()), ConsumerOffset: file.Offset(before[0].consumer.Pos()), Elements: elements, SourceSHA256: map[string]string{"synthetic.go": ps6141PolicyDigest([]byte(source))}, GoVersion: target.GoVersion, GOOS: target.GOOS, GOARCH: target.GOARCH, ContextSHA256: target.ContextSHA256, TargetPolicy: "architecture-wide-operator-reviewed", Boundary: "complete-conversion-and-consumer", Review: "Synthetic policy applicability only; not executed benchmark evidence."}
	data, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "policy.json")
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	c.BenchmarkExemptions = []config.SingleUseQuantizationExemption{{Policy: path, SHA256: ps6141PolicyDigest(data)}}
}

func TestPS6141SourceSummaryPolicyBinding(t *testing.T) {
	t.Parallel()
	for _, destination := range []bool{false, true} {
		for _, negative := range []bool{false, true} {
			t.Run(map[bool]string{false: "return", true: "destination"}[destination]+map[bool]string{false: "Exact", true: "OtherShape"}[negative], func(t *testing.T) {
				t.Parallel()
				source := strings.ReplaceAll(ps6141LanesSynthetic, "p:=pack(x);return dot(p,w)", "if len(x)!=32{panic(\"shape\")};p:=pack(x);return dot(p,w)")
				c := ps6141TestContract()
				c.ConsumerForm = "sourceSummary"
				c.WeightArgument = 1
				c.RowsArgument = -1
				if destination {
					source = strings.ReplaceAll(source, "p:=pack(x);return dot(p,w)", "p:=make(Packed,len(x)/32);fill(p,x);return dot(p,w)")
					c.ProducerForm = "destination"
					c.Quantizer = "fixture.fill"
					c.DestinationArgument = 0
					c.FloatInputArgument = 1
				}
				pass, owner := ps6141TypedFixture(t, source)
				pass.ReadFile = func(string) ([]byte, error) { return []byte(source), nil }
				before := ps6141Candidates(pass, owner, &c)
				if len(before) != 1 {
					t.Fatal("genuine policy prerequisite missing")
				}
				file := pass.Fset.File(before[0].quantizer.Pos())
				target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
				pass.ResultOf = map[*analysis.Analyzer]any{scanscope.Key: target}
				policy := ps6141Policy{Schema: 1, Owner: "fixture.owner", Quantizer: c.Quantizer, Consumer: c.Consumer, ConsumerForm: c.ConsumerForm, File: "synthetic.go", QuantizerOffset: file.Offset(before[0].quantizer.Pos()), ConsumerOffset: file.Offset(before[0].consumer.Pos()), Elements: 32, SourceSHA256: map[string]string{"synthetic.go": ps6141PolicyDigest([]byte(source))}, GoVersion: target.GoVersion, GOOS: target.GOOS, GOARCH: target.GOARCH, ContextSHA256: target.ContextSHA256, TargetPolicy: "architecture-wide-operator-reviewed", Boundary: "complete-conversion-and-consumer", Review: "Synthetic operator-reviewed shape applicability; not executed benchmark evidence."}
				if negative {
					policy.Elements = 64
				}
				data, err := json.Marshal(policy)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(t.TempDir(), "policy.json")
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				c.BenchmarkExemptions = []config.SingleUseQuantizationExemption{{Policy: path, SHA256: ps6141PolicyDigest(data)}}
				want := 0
				if negative {
					want = 1
				}
				if got := len(ps6141Candidates(pass, owner, &c)); got != want {
					t.Fatalf("candidates=%d want=%d", got, want)
				}
			})
		}
	}
}
