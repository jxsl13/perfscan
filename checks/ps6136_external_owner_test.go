package checks

import (
	"bytes"
	"go/types"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
)

// Optional read-only replay against a complete pinned public checkout. The
// ordinary hermetic fixtures remain mandatory; this additional audit neither
// downloads source nor executes a GPU backend during the repository's gates.
func TestPS6136PinnedCompletePackageConstructorInventory(t *testing.T) {
	directory := os.Getenv("PERFSCAN_PS6136_OWNER_DIR")
	if directory == "" {
		t.Skip("set PERFSCAN_PS6136_OWNER_DIR to the exact before-owner checkout")
	}
	for _, name := range []string{"gpt.go", "decoder.go"} {
		actual, err := os.ReadFile(filepath.Join(directory, "llamagpu", name))
		if err != nil {
			t.Fatal(err)
		}
		pinned, err := os.ReadFile("testdata/ps6136-owner/before/" + name + ".txt")
		if err != nil || !bytes.Equal(actual, pinned) {
			t.Fatalf("%s is not the pinned before source", name)
		}
	}
	loaded, err := packages.Load(&packages.Config{Dir: directory, Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps}, "./llamagpu")
	if err != nil || packages.PrintErrors(loaded) != 0 || len(loaded) != 1 {
		t.Fatal("complete owner package does not load cleanly")
	}
	source := loaded[0]
	fixture := &analysisOwnerFixture{source.Fset, source.Syntax, source.TypesInfo, source.Types, nil}
	pkg := ps6136FixtureSSA(fixture)
	for _, name := range []string{"GPTDecoder", "Decoder"} {
		owner := source.Types.Scope().Lookup(name).Type().(*types.Named)
		inventory := ps6136ConstructorInventory(pkg, owner)
		if inventory == nil {
			t.Fatalf("complete real package %s constructor inventory unknown", name)
		}
		t.Logf("complete pinned native package: %s %d fresh constructor functions", name, len(inventory))
	}
}
