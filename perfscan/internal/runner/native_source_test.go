package runner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jxsl13/perfscan/perfscan/checks"
	"github.com/jxsl13/perfscan/perfscan/lint"
)

func TestEmbeddedNativeGPUFileIsExposedToAnalyzers(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module nativefixture\n\ngo 1.25\n")
	write("native.go", `package nativefixture

import _ "embed"

//go:embed attention.metal
var attentionSource string
`)
	write("attention.metal", `#include <metal_stdlib>
kernel void file_attention(uint lane [[thread_index_in_simdgroup]], uint3 head [[threadgroup_position_in_grid]]) {
  float acc[128] = {0};
  for (uint d = 0; d < 128; ++d) acc[d] += 1;
  for (uint d = 0; d < 128; ++d) acc[d] += simd_shuffle_down(acc[d], 16);
}
`)

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(workingDirectory) }()

	var stdout, stderr bytes.Buffer
	code := Run(checks.All(), Options{
		Patterns: []string{"./..."},
		Checks:   "PS6053",
		MaxLevel: lint.LevelStructured,
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	output := stdout.String() + stderr.String()
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 with native-source finding\n%s", code, output)
	}
	for _, fragment := range []string{"attention.metal", "PS6053", "file_attention", "acc[128]"} {
		if !strings.Contains(output, fragment) {
			t.Errorf("output does not contain %q:\n%s", fragment, output)
		}
	}
}

func TestSourceOtherFilesDeduplicatesEmbeddedEntries(t *testing.T) {
	got := sourceOtherFiles([]string{"kernel.metal", "helper.c"}, []string{"kernel.metal", "shader.comp"})
	want := []string{"kernel.metal", "helper.c", "shader.comp"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("sourceOtherFiles = %v, want %v", got, want)
	}
}

func TestEmbeddedSwiftProfilerFileIsExposedToPS6058(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module swiftfixture\n\ngo 1.25\n")
	write("native.go", `package swiftfixture

import _ "embed"

//go:embed profiler.swift
var profilerSource string
`)
	write("profiler.swift", `func resolve(_ samples: UnsafePointer<MTLCounterResultTimestamp>, device: MTLDevice) {
  let frequency = device.queryTimestampFrequency()
  consume(frequency)
}
`)

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(workingDirectory) }()

	var stdout, stderr bytes.Buffer
	code := Run(checks.All(), Options{
		Patterns: []string{"./..."},
		Checks:   "PS6058",
		MaxLevel: lint.LevelStructured,
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	output := stdout.String() + stderr.String()
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 with native-source finding\n%s", code, output)
	}
	for _, fragment := range []string{"profiler.swift", "PS6058", "queryTimestampFrequency", "sampleTimestamps"} {
		if !strings.Contains(output, fragment) {
			t.Errorf("output does not contain %q:\n%s", fragment, output)
		}
	}
}
