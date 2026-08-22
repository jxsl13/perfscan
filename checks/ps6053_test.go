package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6053(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6053.Analyzer, "ps6053")
}

func TestPS6053NativeLanguageDetection(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "metal",
			source: `#include <metal_stdlib>
kernel void metal_decode(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float acc[128] = {0};
  for (uint d = 0; d < 128; ++d) { acc[d] += 1; }
  for (uint d = 0; d < 128; ++d) { acc[d] += simd_shuffle_down(acc[d], 16); }
}`,
			want: "metal_decode",
		},
		{
			name: "cuda",
			source: `__global__ void cuda_decode() {
  uint lane = threadIdx.x; uint head = blockIdx.x;
  float accumulator[DK] = {0};
  for (int d = 0; d < DK; d++) accumulator[d] += 1;
  for (int d = 0; d < DK; d++) accumulator[d] += __shfl_down_sync(0xffffffff, accumulator[d], 16);
}`,
			want: "cuda_decode",
		},
		{
			name: "hip",
			source: `__global__ void hip_decode() {
  uint lane = threadIdx.x; uint head = blockIdx.x;
  float acc[64] = {0};
  for (int d = 0; d < 64; d++) acc[d] += 1;
  for (int d = 0; d < 64; d++) acc[d] += __shfl_down(acc[d], 16);
}`,
			want: "hip_decode",
		},
		{
			name: "vulkan",
			source: `#version 460
void main() {
  uint lane = gl_SubgroupInvocationID; uvec3 head = gl_WorkGroupID;
  float result_sum[64];
  for (int d = 0; d < 64; ++d) result_sum[d] += 1;
  for (int d = 0; d < 64; ++d) result_sum[d] += subgroupShuffleDown(result_sum[d], 16);
}`,
			want: "main",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := ps6053ReplicatedAccumulatorKernels(test.source)
			if len(findings) != 1 || findings[0].kernel != test.want {
				t.Fatalf("findings = %#v, want one for %q", findings, test.want)
			}
		})
	}
}

func TestPS6053Guards(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "already dimension striped",
			source: `#include <metal_stdlib>
kernel void striped(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float acc[128];
  for (uint d = lane; d < 128; d += 32) acc[d] += 1;
}`,
		},
		{
			name: "small unrelated array",
			source: `#include <metal_stdlib>
kernel void small(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float acc[8];
  for (uint d = 0; d < 8; ++d) acc[d] += 1;
  for (uint d = 0; d < 8; ++d) acc[d] += simd_shuffle_down(acc[d], 4);
}`,
		},
		{
			name: "threadgroup shared",
			source: `#include <metal_stdlib>
kernel void shared_array(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  threadgroup float acc[128];
  for (uint d = 0; d < 128; ++d) acc[d] += 1;
  for (uint d = 0; d < 128; ++d) acc[d] += simd_shuffle_down(acc[d], 16);
}`,
		},
		{
			name: "single lane owns array",
			source: `#include <metal_stdlib>
kernel void leader_only(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float acc[128];
  if (lane == 0) {
    for (uint d = 0; d < 128; ++d) acc[d] += 1;
    for (uint d = 0; d < 128; ++d) acc[d] += simd_shuffle_down(acc[d], 16);
  }
}`,
		},
		{
			name: "commented example",
			source: `#include <metal_stdlib>
kernel void clean(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  /* float acc[128];
  for (uint d = 0; d < 128; ++d) acc[d] += 1;
  for (uint d = 0; d < 128; ++d) acc[d] += simd_shuffle_down(acc[d], 16); */
}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if findings := ps6053ReplicatedAccumulatorKernels(test.source); len(findings) != 0 {
				t.Fatalf("unexpected findings: %#v", findings)
			}
		})
	}
}
