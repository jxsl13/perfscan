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
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/config"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

// Replay the complete selected owner partition through the conventional package
// loader. Build constraints are retained as ordinary provenance comments only
// AFTER the original provider/OS partition has been selected. This lets every
// CI host analyze both providers; it does not execute native APIs or establish
// that the provider can run on the host. External files are type-only metadata.
func TestPS6140AnalysisFixture(t *testing.T) {
	t.Parallel()
	annotated, err := os.ReadFile("testdata/src/ps6140/decoder.go")
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile("testdata/ps6140-owner/before/decoder.go.txt")
	if err != nil {
		t.Fatal(err)
	}
	withoutExpectations := annotated
	for _, member := range []string{"ao", "mo"} {
		expectation := []byte(` // want "candidate unused constructor scratch .*Decoder[.]` + member + `:"`)
		if bytes.Count(withoutExpectations, expectation) != 1 {
			t.Fatal("conventional fixture must mark each original allocation exactly once")
		}
		withoutExpectations = bytes.Replace(withoutExpectations, expectation, nil, 1)
	}
	if !bytes.Equal(withoutExpectations, original) {
		t.Fatal("conventional fixture differs from complete pinned decoder source")
	}
	for _, revision := range []string{"before", "after"} {
		for _, target := range []struct{ provider, os string }{{"metal", "darwin"}, {"cuda", "linux"}} {
			t.Run(revision+"/"+target.provider, func(t *testing.T) {
				t.Parallel()
				var rewrite func(*token.FileSet, []*ast.File) []*ast.File
				if revision == "before" {
					rewrite = func(fs *token.FileSet, files []*ast.File) []*ast.File {
						replaced := 0
						for index, file := range files {
							if fs.Position(file.Pos()).Filename != "decoder.go" {
								continue
							}
							parsed, err := parser.ParseFile(fs, "decoder.go", annotated, parser.ParseComments|parser.SkipObjectResolution)
							if err != nil {
								t.Fatal(err)
							}
							files[index] = parsed
							replaced++
						}
						if replaced != 1 {
							t.Fatal("complete selected partition must contain one original decoder")
						}
						return files
					}
				}
				fixture, err := ps6140CompileLoadedProviderRewrite(t, revision, target.os, target.provider, rewrite)
				if err != nil {
					t.Fatal(err)
				}
				wantFiles := 10
				if target.provider == "cuda" {
					wantFiles = 12
				}
				if len(fixture.files) != wantFiles {
					t.Fatalf("selected original file inventory=%d want=%d", len(fixture.files), wantFiles)
				}
				dir := t.TempDir()
				write := func(relative string, data []byte) {
					t.Helper()
					filename := filepath.Join(dir, "src", relative)
					if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filename, data, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				for path, source := range fixture.sources {
					write(filepath.Join(path, "api.go"), []byte(source))
				}
				for index, file := range fixture.files {
					for _, group := range file.Comments {
						for _, comment := range group.List {
							if strings.HasPrefix(comment.Text, "//go:build ") || strings.HasPrefix(comment.Text, "// +build ") {
								comment.Text = "// selected source partition: " + strings.TrimPrefix(comment.Text, "//")
							}
						}
					}
					var source bytes.Buffer
					if err := format.Node(&source, fixture.fileset, file); err != nil {
						t.Fatal(err)
					}
					write(filepath.Join(fixture.pkg.Path(), fmt.Sprintf("owner%d.go", index)), source.Bytes())
				}
				contracts := []config.UnusedProjectionScratchContract{ps6140AuthenticSourceContract(revision, "ao"), ps6140AuthenticSourceContract(revision, "mo")}
				if target.provider == "cuda" {
					contracts = ps6140CUDAContractsForTest(revision)
				}
				analyzer := *PS6140.Analyzer
				analyzer.Run = func(pass *analysis.Pass) (any, error) {
					return runPS6140WithContracts(pass, contracts)
				}
				for _, result := range analysistest.Run(t, dir, &analyzer, fixture.pkg.Path()) {
					for _, diagnostic := range result.Diagnostics {
						if len(diagnostic.SuggestedFixes) != 0 {
							t.Fatal("source-only advisory supplied an automatic edit")
						}
					}
				}
			})
		}
	}
}
