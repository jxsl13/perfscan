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

func TestPS5121(t *testing.T) {
	analysistest.RunWithSuggestedFixes(t, analysistest.TestData(), PS5121.Analyzer,
		"ps5121", "ps5121alias", "ps5121comment", "ps5121dot")
}

func TestPS5121CutVersionGate(t *testing.T) {
	for _, test := range []struct {
		version string
		want    bool
	}{
		{version: "go1.17", want: false},
		{version: "go1.18", want: true},
	} {
		t.Run(test.version, func(t *testing.T) {
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
			if got := ps5121CutAvailable(pass, file); got != test.want {
				t.Fatalf("ps5121CutAvailable at %s = %t, want %t (file version %q)", test.version, got, test.want, info.FileVersions[file])
			}
		})
	}
}
