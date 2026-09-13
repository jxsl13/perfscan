package checks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"github.com/jxsl13/perfscan/internal/scanscope"
	"golang.org/x/tools/go/analysis"
)

func ps6141PolicyDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func TestPS6141PinnedReviewedSiteShapePolicy(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"exact", "unknownTarget", "differentOS", "differentArch", "differentSDK", "differentContext", "cpuSpecific", "wrongShape", "otherSite", "otherOwner", "wrongBoundary", "emptyReview", "wrongSource", "missingSource", "wrongPin", "emptyFile", "malformed", "trailing", "duplicate", "caseAlias", "unknownField", "guardOtherFormal", "guardRebound", "guardAliased", "guardSkipped", "nonpositiveShape", "staleLoadedAST"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			source := strings.Replace(ps6141TwoInputSynthetic, "p:=pack(x)", "if len(x)!=4{panic(\"shape\")};p:=pack(x)", 1)
			switch name {
			case "guardOtherFormal":
				source = strings.ReplaceAll(source, "x []float32,w", "x,y []float32,w")
				source = strings.Replace(source, "len(x)!=4", "len(y)!=4", 1)
			case "guardRebound":
				source = strings.Replace(source, "if len(x)", "x=x;if len(x)", 1)
			case "guardAliased":
				source = strings.Replace(source, "if len(x)", "alias:=x;if len(alias)", 1)
			case "guardSkipped":
				source = strings.Replace(source, "if len(x)", "goto bypass;if len(x)", 1)
				source = strings.Replace(source, ";p:=pack(x)", ";bypass:p:=pack(x)", 1)
			}
			pass, owner := ps6141TypedFixture(t, source)
			pass.ReadFile = func(name string) ([]byte, error) {
				if name != "synthetic.go" {
					t.Fatalf("unexpected source read %q", name)
				}
				return []byte(source), nil
			}
			c := ps6141TestContract()
			c.ConsumerForm = "twoInputDot"
			c.WeightArgument = 1
			c.RowsArgument = -1
			admission := ps6141Candidates(pass, owner, &c)
			if len(admission) != 1 {
				t.Fatal("non-vacuous prerequisite candidate missing")
			}
			file := pass.Fset.File(admission[0].quantizer.Pos())
			// The descriptor models an already-observed loader result. It is
			// synthetic test evidence, not host discovery or benchmark execution.
			target := scanscope.Target{GoVersion: "go1.26.0", GOOS: "linux", GOARCH: "arm64", ContextSHA256: strings.Repeat("a", 64)}
			policy := ps6141Policy{Schema: 1, Owner: "fixture.owner", Quantizer: c.Quantizer, Consumer: c.Consumer, ConsumerForm: c.ConsumerForm, File: "synthetic.go", QuantizerOffset: file.Offset(admission[0].quantizer.Pos()), ConsumerOffset: file.Offset(admission[0].consumer.Pos()), Elements: 4, SourceSHA256: map[string]string{"synthetic.go": ps6141PolicyDigest([]byte(source))}, GoVersion: target.GoVersion, GOOS: target.GOOS, GOARCH: target.GOARCH, ContextSHA256: target.ContextSHA256, TargetPolicy: "architecture-wide-operator-reviewed", Boundary: "complete-conversion-and-consumer", Review: "Synthetic reviewed policy covering conversion plus one consumer at four elements; no measured gain."}
			switch name {
			case "differentOS":
				target.GOOS = "windows"
			case "differentArch":
				target.GOARCH = "amd64"
			case "differentSDK":
				target.GoVersion = "go1.25.0"
			case "differentContext":
				target.ContextSHA256 = strings.Repeat("b", 64)
			case "cpuSpecific":
				policy.TargetPolicy = "Apple-M2-only"
			case "wrongShape":
				policy.Elements = 8
			case "otherSite":
				policy.QuantizerOffset++
			case "otherOwner":
				policy.Owner = "fixture.other"
			case "wrongBoundary":
				policy.Boundary = "dot-only"
			case "emptyReview":
				policy.Review = " "
			case "wrongSource":
				policy.SourceSHA256["synthetic.go"] = strings.Repeat("b", 64)
			case "missingSource":
				policy.SourceSHA256 = nil
			case "nonpositiveShape":
				policy.Elements = 0
			}
			pass.ResultOf = map[*analysis.Analyzer]any{}
			if name != "unknownTarget" {
				pass.ResultOf[scanscope.Key] = target
			}
			data, err := json.Marshal(policy)
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "emptyFile":
				data = nil
			case "malformed":
				data = []byte("{")
			case "trailing":
				data = append(data, []byte(" {}")...)
			case "duplicate":
				data = append([]byte(`{"schema":1,`), data[1:]...)
			case "caseAlias":
				data = append([]byte(`{"Schema":1,`), data[1:]...)
			case "unknownField":
				data = append([]byte(`{"unknown":true,`), data[1:]...)
			}
			path := filepath.Join(t.TempDir(), "reviewed-policy.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			ref := config.SingleUseQuantizationExemption{Policy: path, SHA256: ps6141PolicyDigest(data)}
			if name == "wrongPin" {
				ref.SHA256 = strings.Repeat("c", 64)
			}
			c.BenchmarkExemptions = []config.SingleUseQuantizationExemption{ref}
			if name == "staleLoadedAST" {
				// A malicious/current hash match must not certify an older AST.
				source = strings.Replace(source, "panic(\"shape\")", "panic(\"changed\")", 1)
				policy.SourceSHA256["synthetic.go"] = ps6141PolicyDigest([]byte(source))
				data, err = json.Marshal(policy)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				c.BenchmarkExemptions[0].SHA256 = ps6141PolicyDigest(data)
			}
			want := 1
			if name == "exact" {
				want = 0
			}
			if got := len(ps6141Candidates(pass, owner, &c)); got != want {
				t.Fatalf("candidates=%d want=%d", got, want)
			}
		})
	}
}
