package closureenv

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func TestControlledInventoriesWindowsJunction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	physical := filepath.Join(root, "physical-source")
	if err := os.Mkdir(physical, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(physical, "fixture.go"), []byte("package fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "source-junction")
	if out, err := exec.CommandContext(t.Context(), "cmd.exe", "/d", "/c", "mklink", "/J", alias, physical).CombinedOutput(); err != nil {
		t.Fatalf("ordinary task-owned junction: %v: %s", err, out)
	}
	data, err := json.Marshal(listedBinaryPackage{ImportPath: "example/fixture", Dir: alias, GoFiles: []string{"fixture.go"}})
	if err != nil {
		t.Fatal(err)
	}
	assertControlledPaths(t, data)
	realData, err := json.Marshal(listedBinaryPackage{ImportPath: "example/fixture", Dir: physical, GoFiles: []string{"fixture.go"}})
	if err != nil {
		t.Fatal(err)
	}
	aliasHashes, err := binaryControlledGoFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	realHashes, err := binaryControlledGoFiles(realData)
	if err != nil {
		t.Fatal(err)
	}
	aliasFiles, err := binaryControlledPackageFiles(data)
	if err != nil {
		t.Fatal(err)
	}
	realFiles, err := binaryControlledPackageFiles(realData)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(aliasHashes, realHashes) || !reflect.DeepEqual(aliasFiles, realFiles) {
		t.Fatal("alias and physical source spellings differ in controlled hash/package inventories")
	}
}
