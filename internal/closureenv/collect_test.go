package closureenv

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCollectCompilerConfirmedClassCrossing(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	beforePath := filepath.Join(directory, "before.go")
	afterPath := filepath.Join(directory, "after.go")
	before := "package fixture\nvar Sink func()\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte) { Sink = func() { _,_,_,_,_,_,_,_,_ = a0,a1,a2,a3,a4,a5,a6,a7,a8 } }\n"
	after := "package fixture\nvar Sink func()\nfunc Work(a0,a1,a2,a3,a4,a5,a6,a7,a8 []byte, width byte) { Sink = func() { _,_,_,_,_,_,_,_,_,_ = a0,a1,a2,a3,a4,a5,a6,a7,a8,width } }\n"
	if err := os.WriteFile(beforePath, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(afterPath, []byte(after), 0o600); err != nil {
		t.Fatal(err)
	}
	left, err := CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, beforePath)
	if err != nil {
		t.Fatal(err)
	}
	right, err := CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 1 || len(right) != 1 {
		t.Fatalf("closures = %d/%d, want 1/1", len(left), len(right))
	}
	growth, err := Compare(&left[0], &right[0])
	if err != nil {
		t.Fatal(err)
	}
	if growth.Before.EnvironmentBytes != 224 || growth.After.EnvironmentBytes != 232 || growth.Before.SizeClassBytes != 224 || growth.After.SizeClassBytes != 240 {
		t.Fatalf("growth = %+v", growth)
	}
}

func TestAllocatorRejectsHeaderAndLargeClosures(t *testing.T) {
	t.Parallel()
	goroot := selectedTestGOROOT(t)
	allocator, err := LoadAllocator(goroot, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := allocator.ClassForClosure(allocator.MinSizeForHeader+1, true); err == nil {
		t.Fatal("accepted header-bearing closure")
	}
	if _, err := allocator.ClassForClosure(allocator.MaxSmall+1, false); err == nil {
		t.Fatal("accepted large closure")
	}
}

func TestCollectCompilerCaptureByReference(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "reference.go")
	source := "package fixture\nvar Sink func()\nfunc Work(block [216]byte) { Sink = func() { _ = block[0] } }\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || len(evidence[0].Captures) != 1 || !evidence[0].Captures[0].ByReference || evidence[0].EnvironmentBytes != 16 {
		t.Fatalf("evidence = %+v", evidence)
	}
}

func TestCollectPointerFreeNamedCaptures(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "named.go")
	source := "package fixture\ntype Width uint8\ntype Pair struct{ A, B uint64 }\nvar Sink func()\nfunc Work(width Width, pair Pair) { Sink = func() { _, _ = width, pair } }\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	evidence, err := CollectFile(context.Background(), "go", runtime.GOOS, runtime.GOARCH, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 || evidence[0].Scanned || evidence[0].Captures[0].HasPointers || evidence[0].Captures[1].HasPointers {
		t.Fatalf("pointer-free named evidence = %+v", evidence)
	}
}
