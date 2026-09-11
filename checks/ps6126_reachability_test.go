package checks

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"golang.org/x/tools/go/analysis"

	"github.com/jxsl13/perfscan/internal/closureenv"
)

func TestPS6126SkipsBlockedWorker(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, prefix, suffix string
		want                 int
	}{
		{"nil receive", "<-(chan int)(nil);", "", 0},
		{"nil select", "select { case <-(chan int)(nil): };", "", 0},
		{"blocked IIFE", "<-(chan int)(nil);func(){defer anchor();anchor();anchor();", "}()", 0},
		{"live IIFE", "func(){defer anchor();anchor();anchor();", "}()", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			before := "package worker\nvar sink func(int,int)\nfunc parallelWork(f func(int,int)){sink=f}\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte){" + test.prefix + "parallelWork(func(lo,hi int){_,_,_,_,_,_,_,_,_,_=a0,a1,a2,a3,a4,a5,a6,a7,a8,lo+hi})" + test.suffix + "}\n"
			after := "package worker\nvar sink func(int,int)\nfunc parallelWork(f func(int,int)){sink=f}\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte,width byte){" + test.prefix + "parallelWork(func(lo,hi int){_,_,_,_,_,_,_,_,_,_,_=a0,a1,a2,a3,a4,a5,a6,a7,a8,width,lo+hi})" + test.suffix + "}\n"
			// Preserve the actual inner closure site instead of inlined positions.
			before += "//go:noinline\nfunc anchor() {}\n"
			after += "//go:noinline\nfunc anchor() {}\n"
			collect := func(name, source string) *closureenv.Evidence {
				t.Helper()
				path := filepath.Join(dir, name)
				if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
					t.Fatal(err)
				}
				evidence, err := closureenv.CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, path)
				if err != nil || len(evidence) != 1 {
					t.Fatalf("compiler still emits the blocked callback: %+v, %v", evidence, err)
				}
				return &evidence[0]
			}
			growth, err := closureenv.Compare(collect("before.go", before), collect("after.go", after))
			if err != nil {
				t.Fatal(err)
			}
			var artifact bytes.Buffer
			if err := closureenv.WriteArtifact(&artifact, growth); err != nil {
				t.Fatal(err)
			}
			pass := ps6126TestPass(t, filepath.Join(dir, "after.go"), after)
			// Bare-file evidence uses the source package name, not module identity.
			pass.Pkg.SetName("worker")
			growth.Before.Package, growth.After.Package = pass.Pkg.Path(), pass.Pkg.Path()
			artifact.Reset()
			if err := closureenv.WriteArtifact(&artifact, growth); err != nil {
				t.Fatal(err)
			}
			reports := 0
			pass.Report = func(analysis.Diagnostic) { reports++ }
			if _, err := runPS6126WithEvidence(pass, []string{artifact.String()}, map[string]bool{"example.com/worker.parallelWork": true}, false); err != nil || reports != test.want {
				t.Fatalf("unreachable worker: reports=%d error=%v", reports, err)
			}
		})
	}
}
