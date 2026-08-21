package ps6053

// Embedded MSL: full-vector acc is private in every lane and every element is
// shuffled during the reduction.
var metalReplicated = "#include <metal_stdlib>\nkernel void attention_decode(uint lane [[thread_index_in_simdgroup]], uint3 head [[threadgroup_position_in_grid]]) {\n  float acc[128] = {0};\n  for (uint d = 0; d < 128; ++d) { acc[d] += 1; }\n  for (uint d = 0; d < 128; ++d) { acc[d] += simd_shuffle_down(acc[d], 16); }\n}" // want `GPU kernel attention_decode keeps full-vector accumulator acc\[128\] private in every SIMD lane`

// Embedded CUDA: the same narrow conjunction uses CUDA subgroup vocabulary.
var cudaReplicated = "__global__ void cuda_attention() {\n  uint lane = threadIdx.x; uint head = blockIdx.x;\n  float accumulator[DK] = {0};\n  for (int d = 0; d < DK; ++d) accumulator[d] += 1;\n  for (int d = 0; d < DK; ++d) accumulator[d] += __shfl_down_sync(0xffffffff, accumulator[d], 16);\n}" // want `GPU kernel cuda_attention keeps full-vector accumulator accumulator\[DK\] private in every SIMD lane`

// Already striped: lanes own disjoint vector dimensions, so it must not report.
var striped = `#include <metal_stdlib>
kernel void attention_striped(uint lane [[thread_index_in_simdgroup]], uint3 head [[threadgroup_position_in_grid]]) {
  float acc[4] = {0};
  for (uint d = lane; d < 128; d += 32) acc[d & 3] += 1;
}`

// Small array with a shuffle is unrelated to a full logical vector.
var small = `#include <metal_stdlib>
kernel void small_tree(uint lane [[thread_index_in_simdgroup]], uint3 head [[threadgroup_position_in_grid]]) {
  float acc[4] = {0};
  for (uint d = 0; d < 4; ++d) acc[d] += 1;
  for (uint d = 0; d < 4; ++d) acc[d] += simd_shuffle_down(acc[d], 2);
}`

var _ = []string{metalReplicated, cudaReplicated, striped, small}
