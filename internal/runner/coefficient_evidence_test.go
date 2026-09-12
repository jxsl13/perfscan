package runner

import (
	"encoding/json"
	"github.com/jxsl13/perfscan/internal/closureenv"
	"github.com/jxsl13/perfscan/internal/coefficientcodegen"
	"go/types"
	"golang.org/x/tools/go/packages"
	"strings"
	"testing"
)

func TestCoefficientEvidencePackageScope(t *testing.T) {
	t.Parallel()
	a := coefficientcodegen.Artifact{Function: "Leaf", Build: closureenv.BinaryBuild{Package: "example.test/p", GoVersion: "go1.25.0", GOOS: "darwin", GOARCH: "arm64", ArchitectureLevel: "v8.0", CGOEnabled: "0", MaterialSHA256: strings.Repeat("a", 64), ToolchainSHA256: strings.Repeat("b", 64), BinarySHA256: strings.Repeat("c", 64), SourceSHA256: map[string]string{"f.go": strings.Repeat("d", 64)}}}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateCoefficientEvidencePackages(nil, []string{string(data)}); err == nil {
		t.Fatal("evidence outside loaded packages accepted")
	}
	if err := validateCoefficientEvidencePackages([]*packages.Package{{Types: types.NewPackage("example.test/p", "p")}}, []string{string(data)}); err != nil {
		t.Fatal(err)
	}
	if err := validateCoefficientEvidencePackages(nil, []string{"{}"}); err == nil {
		t.Fatal("invalid evidence accepted")
	}
}
