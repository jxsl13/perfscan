package checks

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

// Conventional analysistest coverage uses the entire pinned GPT owner fixture,
// together with the same complete constructor/observation census as the replay.
// Temporary dependency files are explicitly type-only native/model scaffolds.
func TestPS6136AnalysisFixture(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/src/ps6136/gpt.go")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile("testdata/ps6136-owner/before/gpt.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	withoutExpectation := bytes.Replace(data, []byte(` // want "reviewed common one-row output policy retains"`), nil, 1)
	if !bytes.Equal(withoutExpectation, original) {
		t.Fatal("conventional fixture differs from complete pinned owner source")
	}
	fixture, err := ps6136CompileOwnersRewrite(t, "before", true, true, func(fs *token.FileSet, files []*ast.File) []*ast.File {
		file, err := parser.ParseFile(fs, "testdata/src/ps6136/gpt.go", nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		files[0] = file
		return files
	})
	if err != nil {
		t.Fatal(err)
	}
	c := ps6136CompleteOwnerContract(ps6136FixtureSSA(fixture), "GPTDecoder")
	dir := t.TempDir()
	write := func(path string, data []byte) {
		t.Helper()
		path = filepath.Join(dir, "src", path)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for path, source := range fixture.sources {
		write(filepath.Join(path, "api.go"), []byte(source))
	}
	for index, file := range fixture.files {
		var source bytes.Buffer
		if err := format.Node(&source, fixture.fileset, file); err != nil {
			t.Fatal(err)
		}
		write(filepath.Join(fixture.pkg.Path(), fmt.Sprintf("owner%d.go", index)), source.Bytes())
	}
	analyzer := *PS6136.Analyzer
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runPS6136WithContracts(pass, []config.OutputWorkspaceContract{c})
	}
	analysistest.Run(t, dir, &analyzer, fixture.pkg.Path())
}
