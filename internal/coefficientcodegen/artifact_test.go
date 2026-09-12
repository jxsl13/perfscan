package coefficientcodegen

import (
	"encoding/json"
	"github.com/jxsl13/perfscan/internal/closureenv"
	"strings"
	"testing"
)

func TestArtifactStrictInputs(t *testing.T) {
	t.Parallel()
	valid := Artifact{Function: "Leaf", Build: closureenv.BinaryBuild{Package: "example.test/p", GoVersion: "go1.25.0", GOOS: "darwin", GOARCH: "arm64", ArchitectureLevel: "v8.0", CGOEnabled: "0", MaterialSHA256: strings.Repeat("a", 64), ToolchainSHA256: strings.Repeat("b", 64), BinarySHA256: strings.Repeat("c", 64), SourceSHA256: map[string]string{"f.go": strings.Repeat("d", 64)}}}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Read(strings.NewReader(string(data))); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"{}", string(data) + " {}", strings.Replace(string(data), "\"Leaf\"", "\"../Leaf\"", 1), strings.Replace(string(data), "\"darwin\"", "\"linux\"", 1), strings.Replace(string(data), strings.Repeat("a", 64), "bad-hash", 1), strings.Replace(string(data), "\"f.go\"", "\"../f.go\"", 1), strings.Replace(string(data), "\"function\":", "\"unexpected\":1,\"function\":", 1)} {
		if _, err := Read(strings.NewReader(text)); err == nil {
			t.Fatalf("accepted invalid evidence %s", text)
		}
	}
}
