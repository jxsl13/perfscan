// Package closureenv records and compares compiler-confirmed closure layouts.
// It deliberately does not estimate closure layout from source captures.
package closureenv

import (
	"errors"
	"fmt"
	"go/types"
	"maps"
	"math"
	"slices"
)

// Capture is one field emitted by the compiler after the closure code pointer.
// ByReference records the compiler's decision, not a source-level guess.
type Capture struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Size        int64  `json:"size"`
	Align       int64  `json:"align"`
	ByReference bool   `json:"byReference"`
	HasPointers bool   `json:"hasPointers"`
}

// Evidence is tied to one compiler, target, source identity, and closure site.
type Evidence struct {
	GoVersion         string    `json:"goVersion"`
	GOOS              string    `json:"goos"`
	GOARCH            string    `json:"goarch"`
	SourceSHA256      string    `json:"sourceSHA256"`
	Package           string    `json:"package"`
	Function          string    `json:"function"`
	File              string    `json:"file"`
	Line              int       `json:"line"`
	Column            int       `json:"column"`
	Escapes           bool      `json:"escapes"`
	Captures          []Capture `json:"captures"`
	EnvironmentBytes  int64     `json:"environmentBytes"`
	SizeClassBytes    int64     `json:"sizeClassBytes"`
	Scanned           bool      `json:"scanned"`
	ModulePath        string    `json:"modulePath,omitempty"`
	ModuleVersion     string    `json:"moduleVersion,omitempty"`
	GoLanguageVersion string    `json:"goLanguageVersion,omitempty"`
	BuildTags         []string  `json:"buildTags,omitempty"`
	GOFLAGS           string    `json:"goflags,omitempty"`
	GOEXPERIMENT      string    `json:"goexperiment,omitempty"`
	ArchitectureLevel string    `json:"architectureLevel,omitempty"`
	CGOEnabled        string    `json:"cgoEnabled,omitempty"`
}

// Growth is emitted only for like-for-like compiler and target evidence where
// an already escaping closure grew across an allocator class boundary.
type Growth struct {
	Before, After Evidence
	Added         []Capture
}

func Compare(before, after *Evidence) (*Growth, error) {
	if before.GoVersion == "" || before.GoVersion != after.GoVersion || before.GOOS != after.GOOS || before.GOARCH != after.GOARCH {
		return nil, errors.New("closure evidence toolchain or target mismatch")
	}
	if before.Package == "" || before.Package != after.Package || before.Function != after.Function {
		return nil, errors.New("closure source identity mismatch")
	}
	if before.ModulePath != after.ModulePath || before.ModuleVersion != after.ModuleVersion || before.GoLanguageVersion != after.GoLanguageVersion || !slices.Equal(before.BuildTags, after.BuildTags) || before.GOFLAGS != after.GOFLAGS || before.GOEXPERIMENT != after.GOEXPERIMENT || before.ArchitectureLevel != after.ArchitectureLevel || before.CGOEnabled != after.CGOEnabled {
		return nil, errors.New("closure package build provenance mismatch")
	}
	if !before.Escapes || !after.Escapes {
		return nil, errors.New("closure is not compiler-confirmed escaping in both revisions")
	}
	if before.EnvironmentBytes <= 0 || after.EnvironmentBytes <= before.EnvironmentBytes || before.SizeClassBytes <= 0 || after.SizeClassBytes <= before.SizeClassBytes {
		return nil, errors.New("closure did not grow across an allocator class")
	}
	old := make(map[string]Capture, len(before.Captures))
	for _, capture := range before.Captures {
		if _, duplicate := old[capture.Name]; duplicate || capture.Name == "" {
			return nil, errors.New("duplicate or empty before capture identity")
		}
		old[capture.Name] = capture
	}
	remaining := maps.Clone(old)
	added := make([]Capture, 0, len(after.Captures))
	seenAfter := make(map[string]bool, len(after.Captures))
	for _, capture := range after.Captures {
		if capture.Name == "" || seenAfter[capture.Name] {
			return nil, errors.New("duplicate or empty after capture identity")
		}
		seenAfter[capture.Name] = true
		if previous, exists := old[capture.Name]; !exists {
			added = append(added, capture)
		} else if previous != capture {
			return nil, fmt.Errorf("capture %q changed representation", capture.Name)
		} else {
			delete(remaining, capture.Name)
		}
	}
	if len(remaining) != 0 {
		return nil, errors.New("before capture disappeared")
	}
	if len(added) == 0 {
		return nil, errors.New("environment grew without an added capture")
	}
	slices.SortFunc(added, func(a, b Capture) int { return stringCompare(a.Name, b.Name) })
	return &Growth{Before: *before, After: *after, Added: added}, nil
}

func stringCompare(a, b string) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Layout reproduces the compiler's struct layout once the compiler-confirmed
// capture field sizes/alignments are known. A trailing zero-sized capture is
// kept inside the enclosing allocation, matching Go struct-size rules.
func Layout(sizes types.Sizes, captures []Capture) (int64, error) {
	if sizes == nil {
		return 0, errors.New("missing target sizes")
	}
	pointer := sizes.Sizeof(types.Typ[types.UnsafePointer])
	if pointer <= 0 {
		return 0, errors.New("invalid target pointer size")
	}
	offset, maxAlign := pointer, pointer
	for _, capture := range captures {
		if capture.Size < 0 || capture.Align <= 0 || capture.Align&(capture.Align-1) != 0 {
			return 0, fmt.Errorf("invalid compiler capture layout for %q", capture.Name)
		}
		if offset > math.MaxInt64-(capture.Align-1) {
			return 0, errors.New("closure layout overflow")
		}
		offset = align(offset, capture.Align)
		if capture.Size > math.MaxInt64-offset {
			return 0, errors.New("closure layout overflow")
		}
		offset += capture.Size
		if capture.Align > maxAlign {
			maxAlign = capture.Align
		}
	}
	if len(captures) != 0 && captures[len(captures)-1].Size == 0 && offset > 0 {
		if offset == math.MaxInt64 {
			return 0, errors.New("closure layout overflow")
		}
		offset++
	}
	if offset > math.MaxInt64-(maxAlign-1) {
		return 0, errors.New("closure layout overflow")
	}
	return align(offset, maxAlign), nil
}

func align(value, alignment int64) int64 { return (value + alignment - 1) &^ (alignment - 1) }
