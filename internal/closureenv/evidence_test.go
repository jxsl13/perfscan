package closureenv

import (
	"go/types"
	"math"
	"testing"
)

func TestCompareClassCrossing(t *testing.T) {
	t.Parallel()
	base := Evidence{GoVersion: "go1.27.1", GOOS: "darwin", GOARCH: "arm64", Package: "p", Function: "work", Escapes: true, EnvironmentBytes: 224, SizeClassBytes: 224, Captures: []Capture{{Name: "buffers", Type: "[]float32", Size: 24, Align: 8}}}
	after := base
	after.EnvironmentBytes, after.SizeClassBytes = 232, 240
	after.Captures = append(slicesClone(base.Captures), Capture{Name: "loadWidth", Type: "uint8", Size: 1, Align: 1})
	growth, err := Compare(&base, &after)
	if err != nil || len(growth.Added) != 1 || growth.Added[0].Name != "loadWidth" {
		t.Fatalf("Compare() = %+v, %v", growth, err)
	}
}

func TestLayoutPadsTrailingZeroSizedCapture(t *testing.T) {
	t.Parallel()
	got, err := Layout(types.SizesFor("gc", "amd64"), []Capture{{Name: "x", Size: 8, Align: 8}, {Name: "empty", Size: 0, Align: 1}})
	if err != nil || got != 24 {
		t.Fatalf("Layout() = %d, %v, want 24", got, err)
	}
	if _, err := Layout(types.SizesFor("gc", "amd64"), []Capture{{Name: "huge", Size: math.MaxInt64, Align: 8}}); err == nil {
		t.Fatal("accepted overflowing closure layout")
	}
}

func TestCompareRejectsRemovedOrDuplicateCaptureIdentity(t *testing.T) {
	t.Parallel()
	before := Evidence{GoVersion: "go1.27", GOOS: "linux", GOARCH: "amd64", Package: "p", Function: "work", Escapes: true, EnvironmentBytes: 16, SizeClassBytes: 16, Captures: []Capture{{Name: "old", Size: 8, Align: 8}}}
	after := before
	after.EnvironmentBytes, after.SizeClassBytes = 24, 24
	after.Captures = []Capture{{Name: "new", Size: 16, Align: 8}}
	if _, err := Compare(&before, &after); err == nil {
		t.Fatal("accepted replacement capture as additive growth")
	}
	after.Captures = []Capture{{Name: "old", Size: 8, Align: 8}, {Name: "old", Size: 8, Align: 8}}
	if _, err := Compare(&before, &after); err == nil {
		t.Fatal("accepted duplicate capture identity")
	}
}

func TestCompareRejectsColdOrStaleEvidence(t *testing.T) {
	t.Parallel()
	base := Evidence{GoVersion: "go1.27.1", GOOS: "darwin", GOARCH: "arm64", Package: "p", Function: "work", Escapes: true, EnvironmentBytes: 224, SizeClassBytes: 224, Captures: []Capture{{Name: "x", Size: 8, Align: 8}}}
	for name, mutate := range map[string]func(*Evidence){
		"compiler":    func(e *Evidence) { e.GoVersion = "go1.28" },
		"target":      func(e *Evidence) { e.GOARCH = "amd64" },
		"nonescaping": func(e *Evidence) { e.Escapes = false },
		"sameclass":   func(e *Evidence) { e.EnvironmentBytes = 225; e.SizeClassBytes = 224 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			after := base
			mutate(&after)
			if _, err := Compare(&base, &after); err == nil {
				t.Fatal("accepted unsupported evidence")
			}
		})
	}
}

func TestLayoutUsesTargetAlignment(t *testing.T) {
	t.Parallel()
	got, err := Layout(types.SizesFor("gc", "arm64"), []Capture{{Name: "a", Size: 1, Align: 1}, {Name: "b", Size: 24, Align: 8}})
	if err != nil || got != 40 {
		t.Fatalf("Layout() = %d, %v, want 40", got, err)
	}
}

func slicesClone[T any](in []T) []T { return append([]T(nil), in...) }
