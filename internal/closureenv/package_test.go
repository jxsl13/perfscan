package closureenv

import (
	"context"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCollectPackageUsesModuleBuildContext(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	write := func(name, source string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(directory, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/closurefixture\n\ngo 1.25\n")
	if err := os.Mkdir(filepath.Join(directory, "worker"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "shape"), 0o700); err != nil {
		t.Fatal(err)
	}
	write("shape/shape.go", "package shape\ntype Width uint8\n")
	write("worker/worker.go", `package worker
import (
 "strings"
 "example.com/closurefixture/shape"
)
type Descriptor struct { Data []byte; Width shape.Width }
var Sink func()
func Work(d Descriptor) { Sink = func() { _, _, _ = d.Data, d.Width, strings.Builder{} } }
`)
	write("worker/tagged.go", "//go:build closuretest\n\npackage worker\nconst Tagged = true\n")
	evidence, err := CollectPackage(context.Background(), &PackageRequest{GoBinary: "go", Dir: directory, Pattern: "./worker", GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Tags: []string{"closuretest"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 {
		t.Fatalf("evidence count = %d, want 1", len(evidence))
	}
	got := evidence[0]
	if got.Package != "example.com/closurefixture/worker" || got.ModulePath != "example.com/closurefixture" || got.GoLanguageVersion != "1.25" || got.File != "worker.go" || got.Function != "Work" || len(got.BuildTags) != 1 || got.BuildTags[0] != "closuretest" {
		t.Fatalf("package provenance = %+v", got)
	}
	if !got.Scanned || got.EnvironmentBytes <= 0 || got.SizeClassBytes < got.EnvironmentBytes {
		t.Fatalf("compiler layout = %+v", got)
	}
}

func TestTypeHasPointersIncludesStringAndUnsafePointer(t *testing.T) {
	t.Parallel()
	for _, typ := range []types.Type{types.Typ[types.String], types.Typ[types.UnsafePointer]} {
		if !typeHasPointers(typ, make(map[types.Type]bool)) {
			t.Fatalf("%v classified pointer-free", typ)
		}
	}
}

func TestClosureTypeSiteRequiresExactBasename(t *testing.T) {
	t.Parallel()
	output := "\t0x0000 (/tmp/barfoo.go:12) MOVD $type:noalg.struct { F uintptr; X0 int }(SB), R1\n"
	if got := closureTypeAtFile(output, "foo.go", 12); got != "" {
		t.Fatalf("prefix filename matched descriptor %q", got)
	}
	if got := closureTypeAtFile(output, "barfoo.go", 12); got == "" {
		t.Fatal("exact basename lost descriptor")
	}
}

func TestCollectPackageHonorsCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CollectPackage(ctx, &PackageRequest{GoBinary: "go", Dir: t.TempDir(), Pattern: "."}); err == nil {
		t.Fatal("canceled package collection succeeded")
	}
}

func TestCollectPackageDoesNotMutateRequestDefaults(t *testing.T) {
	t.Parallel()
	request := PackageRequest{}
	if _, err := CollectPackage(context.Background(), &request); err == nil {
		t.Fatal("package collection without a pattern succeeded")
	}
	if request.GoBinary != "" {
		t.Fatalf("GoBinary mutated to %q", request.GoBinary)
	}
}
