package checks

import (
	"bytes"
	"context"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

func TestPS6126ProductionRecollection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	beforeDir, afterDir := filepath.Join(root, "before"), filepath.Join(root, "after")
	for _, directory := range []string{beforeDir, afterDir} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/worker\n\ngo 1.25\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := "package worker\nvar sink func(int,int)\nfunc parallelWork(f func(int,int)){sink=f}\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte){parallelWork(func(lo,hi int){_,_,_,_,_,_,_,_,_,_=a0,a1,a2,a3,a4,a5,a6,a7,a8,lo+hi})}\n"
	after := "package worker\nvar sink func(int,int)\nfunc parallelWork(f func(int,int)){sink=f}\n\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte,width byte){parallelWork(func(lo,hi int){_,_,_,_,_,_,_,_,_,_,_=a0,a1,a2,a3,a4,a5,a6,a7,a8,width,lo+hi})}\n"
	beforePath, afterPath := filepath.Join(beforeDir, "worker.go"), filepath.Join(afterDir, "worker.go")
	if err := os.WriteFile(beforePath, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	request := closureenv.PackageRequest{GoBinary: "go", Pattern: ".", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Function: "Work", File: "worker.go"}
	request.Dir = beforeDir
	left, err := closureenv.CollectPackage(context.Background(), &request)
	if err != nil || len(left) != 1 {
		t.Fatalf("before collection = %d, %v", len(left), err)
	}
	request.Dir = afterDir
	right, err := closureenv.CollectPackage(context.Background(), &request)
	if err != nil || len(right) != 1 {
		t.Fatalf("after collection = %d, %v", len(right), err)
	}
	growth, err := closureenv.Compare(&left[0], &right[0])
	if err != nil {
		t.Fatal(err)
	}
	var artifact bytes.Buffer
	if err := closureenv.WriteArtifact(&artifact, growth); err != nil {
		t.Fatal(err)
	}
	pass := ps6126TestPass(t, afterPath, after)
	reports := 0
	pass.Report = func(analysis.Diagnostic) { reports++ }
	if _, err := runPS6126WithEvidence(pass, []string{artifact.String()}, map[string]bool{"example.com/worker.parallelWork": true}, true); err != nil || reports != 1 {
		t.Fatalf("production analyzer = reports %d, error %v", reports, err)
	}
	if err := os.WriteFile(afterPath, []byte(after+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runPS6126WithEvidence(pass, []string{artifact.String()}, map[string]bool{"example.com/worker.parallelWork": true}, true); err == nil {
		t.Fatal("changed current source retained compiler evidence")
	}
	if err := os.WriteFile(afterPath, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := *growth
	marker := filepath.Join(root, "must-not-execute")
	changed.Before.GOFLAGS, changed.After.GOFLAGS = "-toolexec="+marker, "-toolexec="+marker
	artifact.Reset()
	if err := closureenv.WriteArtifact(&artifact, &changed); err != nil {
		t.Fatal(err)
	}
	if _, err := runPS6126WithEvidence(ps6126TestPass(t, afterPath, after), []string{artifact.String()}, map[string]bool{"example.com/worker.parallelWork": true}, true); err == nil {
		t.Fatal("changed compiler context retained evidence")
	} else if !strings.Contains(err.Error(), "does not match user-selected Go build context") {
		t.Fatalf("injected flags reached beyond the pre-compilation context check: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("artifact-controlled tool execution occurred")
	}
}

func ps6126TestPass(t *testing.T, path, source string) *analysis.Pass {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Types: make(map[ast.Expr]types.TypeAndValue), Defs: make(map[*ast.Ident]types.Object), Uses: make(map[*ast.Ident]types.Object), Selections: make(map[*ast.SelectorExpr]*types.Selection)}
	pkg, err := (&types.Config{Importer: importer.Default()}).Check("example.com/worker", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	return &analysis.Pass{Analyzer: PS6126.Analyzer, Fset: fset, Files: []*ast.File{file}, Pkg: pkg, TypesInfo: info, ReadFile: os.ReadFile}
}
