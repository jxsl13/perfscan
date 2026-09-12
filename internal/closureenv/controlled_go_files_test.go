package closureenv

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestControlledGoFilesIncludesTestAndDependencyInputs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	want := make(map[string]string)
	for _, name := range []string{"production.go", "factory_test.go", "external_test.go", "helper.go"} {
		data := []byte("package fixture // " + name + "\n")
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		path, err := filepath.EvalSymlinks(path)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		want[path] = hex.EncodeToString(sum[:])
	}
	inputs := []listedBinaryPackage{{ImportPath: "example/fixture", Dir: dir, GoFiles: []string{"production.go"}, TestGoFiles: []string{"factory_test.go"}, XTestGoFiles: []string{"external_test.go"}}, {ImportPath: "example/helper", Dir: dir, GoFiles: []string{"helper.go"}}}
	var stream []byte
	for _, input := range inputs {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		stream = append(stream, data...)
	}
	got, err := binaryControlledGoFiles(stream)
	if err != nil || len(got) != len(want) {
		t.Fatalf("selected Go/test/dependency partition: %v %v", got, err)
	}
	for path, hash := range want {
		if got[path] != hash {
			t.Fatalf("selected input missing or changed: %s", path)
		}
	}
	if _, err := binaryControlledGoFiles([]byte("{")); err == nil {
		t.Fatal("accepted truncated selection")
	}
}
