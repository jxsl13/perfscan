package closureenv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/internal/observedpath"
)

// Use the selected Go executable's actual SDK source spelling, not runtime.GOROOT
// or a synthetic same-volume alias. setup-go can list C: paths backed by D:.
func TestControlledInventoriesObservedSDKSources(t *testing.T) {
	t.Parallel()
	cmd := exec.CommandContext(t.Context(), "go", "list", "-json", "fmt")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=")
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	assertControlledPaths(t, data)
}

func assertControlledPaths(t *testing.T, data []byte) {
	t.Helper()
	var pkg listedBinaryPackage
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if len(pkg.GoFiles) == 0 {
		t.Fatal("selected SDK inventory is empty")
	}
	hashes, err := binaryControlledGoFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	files, err := binaryControlledPackageFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range pkg.GoFiles {
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(pkg.Dir, path)
		}
		physical, err := observedpath.Canonical(path)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(physical)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		if hashes[physical] != hex.EncodeToString(sum[:]) || !slices.Contains(files[pkg.ImportPath], physical) {
			t.Fatalf("controlled hash/package inventories disagree at %q", path)
		}
	}
}

func TestControlledInventoryMissingPathContext(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "missing.go")
	data, err := json.Marshal(listedBinaryPackage{ImportPath: "example/missing", GoFiles: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []func([]byte) error{
		func(data []byte) error { _, err := binaryControlledGoFiles(data); return err },
		func(data []byte) error { _, err := binaryControlledPackageFiles(data); return err },
	} {
		if err := check(data); err == nil || !strings.Contains(err.Error(), "missing.go") {
			t.Fatalf("missing inventory path not retained in rejection: %v", err)
		}
	}
}
