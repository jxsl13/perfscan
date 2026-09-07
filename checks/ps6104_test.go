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
			name: "resolved boolean compound mutation",
			source: `#include <metal_stdlib>
kernel void resolved_boolean_change(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  bool validRow = row < outputRows;
  for (uint block = 0; block < blocks; ++block) {
    validRow ^= true;
    if (validRow) consume(block);
  }
}`,
		},
		{
			name: "resolved boolean address exposure",
			source: `#include <metal_stdlib>
kernel void resolved_boolean_address(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  bool validRow = row < outputRows;
  for (uint block = 0; block < blocks; ++block) {
    mutate(&validRow);
    if (validRow) consume(block);
  }
}`,
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
			name: "direct row address exposure",
			source: `#include <metal_stdlib>
kernel void direct_address(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(&row);
    if (row < outputRows) consume(row, block);
  }
}`,
		},
		{
			name: "parenthesized row address exposure",
			source: `#include <metal_stdlib>
kernel void parenthesized_address(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(&(row));
    if (row < outputRows) consume(row, block);
  }
}`,
		},
		{
			name: "aggregate field address exposure",
			source: `#include <metal_stdlib>
kernel void aggregate_address(RowState state, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(&state.row);
    if (state.row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "indexed address exposure",
			source: `#include <metal_stdlib>
kernel void indexed_address(uint outputRows [[buffer(0)]]) {
  uint rows[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    advance(&rows[0]);
    if (rows[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "pointer field address exposure",
			source: `#include <metal_stdlib>
kernel void pointer_field_address(device RowState* state, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(&state->row);
    if (state->row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "dereferenced address exposure",
			source: `#include <metal_stdlib>
kernel void dereferenced_address(device uint* row, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(&(*row));
    if (*row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "row address exposure after guard",
			source: `#include <metal_stdlib>
kernel void address_after_guard(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(row, block);
    advance(&row);
  }
}`,
		},
		{
			name: "boundary address exposure",
			source: `#include <metal_stdlib>
kernel void boundary_address(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(&outputRows);
    if (row < outputRows) consume(row, block);
  }
}`,
		},
		{
			name: "direct mutable pointer root call",
			source: `#include <metal_stdlib>
kernel void pointer_argument(device uint* row, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(row);
    if (*row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "scalar multiplication is not pointer indirection",
			source: `#include <metal_stdlib>
kernel void scalar_product(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]], uint factor) {
  for (uint block = 0; block < blocks; ++block) {
    if (row * factor < outputRows) consume(factor, block);
  }
}`,
			want: 1,
		},
		{
			name: "direct pointer-valued field call",
			source: `#include <metal_stdlib>
kernel void pointer_field_argument(RowState state, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    advance(state.rows);
    if (state.rows[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "unrelated address exposure remains invariant",
			source: `#include <metal_stdlib>
kernel void unrelated_address(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  uint scratch = 0;
  for (uint block = 0; block < blocks; ++block) {
    advance(&scratch);
    if (row < outputRows) consume(row, block);
  }
}`,
			want: 1,
		},
		{
			name: "indexed row mutation",
			source: `#include <metal_stdlib>
kernel void indexed_row_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 0};
  for (uint block = 0; block < blocks; ++block) {
    row[block & 1] = block;
    if (row[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "indexed row read remains invariant",
			source: `#include <metal_stdlib>
kernel void indexed_row_read(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    if (row[0] < outputRows) consume(block);
  }
}`,
			want: 1,
		},
		{
			name: "indexed postfix row mutation",
			source: `#include <metal_stdlib>
kernel void indexed_postfix_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    row[0]++;
    if (row[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "nested indexed row mutation",
			source: `#include <metal_stdlib>
kernel void nested_index_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  uint index[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    row[index[block & 1]] = block;
    if (row[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "nested indexed row mutation with internal delimiters",
			source: `#include <metal_stdlib>
kernel void delimited_nested_index_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    row[select(block & 1, 0, block > 0)] = block;
    if (row[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "multiline indexed row mutation",
			source: `#include <metal_stdlib>
kernel void multiline_index_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    row[
      block & 1
    ] = block;
    if (row[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "parenthesized indexed row mutation",
			source: `#include <metal_stdlib>
kernel void parenthesized_index_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  for (uint block = 0; block < blocks; ++block) {
    (row[block & 1]) = block;
    if (row[0] < outputRows) consume(block);
  }
}`,
		},
		{
			name: "unrelated aggregate mutation preserves finding",
			source: `#include <metal_stdlib>
kernel void unrelated_index_change(uint outputRows [[buffer(0)]]) {
  uint row[2] = {0, 1};
  uint scratch[2] = {0, 0};
  for (uint block = 0; block < blocks; ++block) {
    scratch[block & 1] = block;
    if (row[0] < outputRows) consume(block);
  }
}`,
			want: 1,
		},
		{
			name: "comparisons do not masquerade as mutations",
			source: `#include <metal_stdlib>
kernel void comparisons_are_reads(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    bool exact = row == outputRows;
    if (row < outputRows) consume(exact, block);
  }
}`,
			want: 1,
		},
		{
			name: "aggregate field mutation",
			source: `#include <metal_stdlib>
kernel void field_row_change(uint outputRows [[buffer(0)]]) {
  RowState state;
  for (uint block = 0; block < blocks; ++block) {
    state.row = block;
    if (state.row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "pointer field mutation",
			source: `#include <metal_stdlib>
kernel void pointer_field_change(device RowState* state, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    state->row = block;
    if (state->row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "dereferenced row mutation",
			source: `#include <metal_stdlib>
kernel void dereferenced_row_change(device uint* row, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    *row = block;
    if (*row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "prefix dereferenced row mutation",
			source: `#include <metal_stdlib>
kernel void prefix_dereferenced_row_change(device uint* row, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    ++*row;
    if (*row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "prefix parenthesized dereferenced row mutation",
			source: `#include <metal_stdlib>
kernel void prefix_parenthesized_row_change(device uint* row, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    ++(*row);
    if (*row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "dereference of parenthesized pointer mutation",
			source: `#include <metal_stdlib>
kernel void parenthesized_pointer_row_change(device uint* row, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    *(row) = block;
    if (*row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "parenthesized dereferenced field mutation",
			source: `#include <metal_stdlib>
kernel void parenthesized_pointer_change(device RowState* state, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    (*state).row = block;
    if ((*state).row < outputRows) consume(block);
  }
}`,
		},
		{
			name: "multiply parenthesized dereferenced field mutation",
			source: `#include <metal_stdlib>
kernel void multiply_parenthesized_pointer_change(device RowState* state, uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    (((*state).row)) = block;
    if ((*state).row < outputRows) consume(block);
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
