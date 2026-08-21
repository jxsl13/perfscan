package checks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS5073(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5073.Analyzer, "ps5073", "ps5073alias")
}

func TestPS5073GroupAttrsVersionGate(t *testing.T) {
	t.Parallel()
	group := ps5073Variant{fixVersion: "go1.25"}
	for _, test := range []struct {
		version string
		want    bool
	}{
		{version: "go1.24", want: false},
		{version: "go1.25", want: true},
	} {
		t.Run(test.version, func(t *testing.T) {
			t.Parallel()
			fileSet := token.NewFileSet()
			file, err := parser.ParseFile(fileSet, "gate.go", "package gate", 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{FileVersions: make(map[*ast.File]string)}
			pkg, err := (&types.Config{GoVersion: test.version}).Check("gate", fileSet, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			pass := &analysis.Pass{Fset: fileSet, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info}
			if got := ps5073FixAvailable(pass, file, group); got != test.want {
				t.Fatalf("ps5073FixAvailable at %s = %t, want %t (file version %q)", test.version, got, test.want, info.FileVersions[file])
			}
		})
	}
}
