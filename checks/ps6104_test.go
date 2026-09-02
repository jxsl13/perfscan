package checks

import (
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6104(t *testing.T) {
	t.Parallel()
	analysistest.Run(t, analysistest.TestData(), PS6104.Analyzer, "ps6104")
}

func TestPS6104PackageOwnedNativeFile(t *testing.T) {
	t.Parallel()
	const filename = "kernel.metal"
	const source = `#include <metal_stdlib>
kernel void native_file(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(row, block);
  }
}`

	var diagnostics []analysis.Diagnostic
	pass := &analysis.Pass{
		Fset:         token.NewFileSet(),
		OtherFiles:   []string{filename, "notes.txt"},
		IgnoredFiles: []string{filename},
		ReadFile: func(requested string) ([]byte, error) {
			if requested != filename {
				t.Fatalf("unexpected native read: %s", requested)
			}
			return []byte(source), nil
		},
		Report: func(diagnostic analysis.Diagnostic) {
			diagnostics = append(diagnostics, diagnostic)
		},
	}
	if _, err := runPS6104(pass); err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one", diagnostics)
	}
	position := pass.Fset.Position(diagnostics[0].Pos)
	if position.Filename != filename {
		t.Fatalf("diagnostic filename = %q, want %q", position.Filename, filename)
	}
}

func TestPS6104NativeSourceShapes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "owner two-row block loop",
			source: `#include <metal_stdlib>
kernel void q4k(device const uchar* weights [[buffer(0)]], uint outputRows [[buffer(1)]],
    uint group [[threadgroup_position_in_grid]]) {
  uint row = group * 2 + 1;
  for (uint block = 0; block < nb; ++block) {
    if (row < outputRows) consume(weights, row, block);
  }
  if (row < outputRows) store(row);
}`,
			want: 1,
		},
		{
			name: "transitive invariant boolean",
			source: `#include <metal_stdlib>
kernel void q6k(uint row [[thread_position_in_grid]], uint nrows [[buffer(0)]]) {
  bool validRow = row < nrows;
  for (uint tile = 0; tile < tiles; tile++) {
    if (validRow) consume(tile);
  }
}`,
			want: 1,
		},
		{
			name: "loop-dependent predicate",
			source: `#include <metal_stdlib>
kernel void changing(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (row + block < outputRows) consume(block);
  }
}`,
		},
		{
			name: "expression loop bound",
			source: `#include <metal_stdlib>
kernel void expression_bound(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint i = 0; i < K / 256; ++i) {
    if (row < outputRows) consume(i);
  }
}`,
			want: 1,
		},
		{
			name: "compound row mutation",
			source: `#include <metal_stdlib>
kernel void row_changes(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    row++;
    if (row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "simple row mutation after guard",
			source: `#include <metal_stdlib>
kernel void row_changes_later(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(block);
    row = row + 1;
  }
}`,
		},
		{
			name: "prefix row mutation",
			source: `#include <metal_stdlib>
kernel void prefix_row_change(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    ++row;
    if (row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "outer loop dependency",
			source: `#include <metal_stdlib>
kernel void outer_dependency(uint baseRow [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint pass = 0; pass < passes; ++pass) {
    uint row = baseRow + pass;
    for (uint block = 0; block < blocks; ++block) {
      if (row < outputRows) consume(block);
    }
  }
}`,
		},
		{
			name: "function constant already specializes",
			source: `#include <metal_stdlib>
constant bool fullRows [[function_constant(0)]];
kernel void specialized(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (fullRows || row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "one trip",
			source: `#include <metal_stdlib>
kernel void once(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < 1; ++block) {
    if (row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "ordinary loop",
			source: `#include <metal_stdlib>
kernel void ordinary(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint attempt = 0; attempt < retries; ++attempt) {
    if (row < outputRows) consume(attempt);
  }
}`,
		},
		{
			name: "nested loop owns predicate",
			source: `#include <metal_stdlib>
kernel void nested(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    for (uint lane = 0; lane < lanes; ++lane) {
      if (row + lane < outputRows) consume(block, lane);
    }
  }
}`,
		},
		{
			name: "validated",
			source: `#include <metal_stdlib>
//perfscan:invariant-tail-guard-validated native compiler and weighted stream evidence
kernel void validated(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "cuda block loop",
			source: `__global__ void q4_cuda(const unsigned char *weights, int outputRows, int blocks) {
  int lane = threadIdx.x;
  int row = blockIdx.x * 2 + (lane == 0);
  for (int block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(weights, row, block);
  }
}`,
			want: 1,
		},
		{
			name: "vulkan block loop",
			source: `#version 450
void main() {
  uint lane = gl_SubgroupInvocationID;
  uint row = gl_WorkGroupID.x * 2 + (lane == 0 ? 1 : 0);
  for (uint block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(row, block);
  }
}`,
			want: 1,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			findings := ps6104InvariantTailGuards(test.source)
			if len(findings) != test.want {
				t.Fatalf("findings = %#v, want %d", findings, test.want)
			}
		})
	}
}
