package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6059(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6059.Analyzer, "ps6059")
}

func TestPS6059FusedNativeKernel(t *testing.T) {
	source := `#include <metal_stdlib>
using namespace metal;
kernel void fused_q4k_swiglu(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float gateAcc = 0;
  float upAcc = 0;
  for (uint k = lane; k < 2048; k += 32) gateAcc += load_gate(k);
  for (uint k = lane; k < 2048; k += 32) upAcc += load_up(k);
  out[group.x] = precise::silu(gateAcc) * upAcc;
}`
	findings := ps6059NativeFindings(source)
	if len(findings) != 1 {
		t.Fatalf("findings = %#v, want one fused-kernel hypothesis", findings)
	}
	finding := findings[0]
	if finding.kernel != "fused_q4k_swiglu" || finding.gate != "gateAcc" || finding.up != "upAcc" {
		t.Fatalf("finding = %#v", finding)
	}
	message := ps6059NativeMessage(finding)
	for _, fragment := range []string{"only a hypothesis", "register pressure", "occupancy", "complete production seams", "repeated alternating", "unavailable counters as unknown"} {
		if !strings.Contains(message, fragment) {
			t.Errorf("message does not contain %q: %s", fragment, message)
		}
	}
}

func TestPS6059BarrierAndSharedStateAreNamed(t *testing.T) {
	source := `#include <metal_stdlib>
using namespace metal;
kernel void split_simd_fused(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  threadgroup float reduced[4];
  float gateReduced = 0;
  float upReduced = 0;
  gateReduced += load_gate(lane);
  upReduced += load_up(lane);
  threadgroup_barrier(mem_flags::mem_threadgroup);
  out[group.x] = silu(gateReduced) * upReduced;
}`
	findings := ps6059NativeFindings(source)
	if len(findings) != 1 || !findings[0].barrier || !findings[0].shared {
		t.Fatalf("findings = %#v, want barrier/shared risk", findings)
	}
	if message := ps6059NativeMessage(findings[0]); !strings.Contains(message, "exchange or barriers") {
		t.Fatalf("barrier risk omitted: %s", message)
	}
}

func TestPS6059NativeGuards(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "separate kernels",
			source: `#include <metal_stdlib>
kernel void gate_projection(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float gateAcc = 0; gateAcc += load_gate(lane); out[group.x] = gateAcc;
}
kernel void up_projection(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float upAcc = 0; upAcc += load_up(lane); out[group.x] = upAcc;
}
kernel void swiglu_activation(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  out[group.x] = silu(gate[group.x]) * up[group.x];
}`,
		},
		{
			name: "single accumulator",
			source: `#include <metal_stdlib>
kernel void gate_only(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float gateAcc = 0; gateAcc += load_gate(lane); out[group.x] = gateAcc;
}`,
		},
		{
			name: "initializer only",
			source: `#include <metal_stdlib>
kernel void no_reductions(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  float gateAcc = 0; float upAcc = 0; out[group.x] = silu(gateAcc) * upAcc;
}`,
		},
		{
			name:   "cpu helper",
			source: `float helper(float gateAcc, float upAcc) { return silu(gateAcc) * upAcc; }`,
		},
		{
			name: "commented fused example",
			source: `#include <metal_stdlib>
kernel void clean(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) {
  /* float gateAcc = 0; float upAcc = 0;
     gateAcc += 1; upAcc += 1; out[0] = silu(gateAcc) * upAcc; */
}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if findings := ps6059NativeFindings(test.source); len(findings) != 0 {
				t.Fatalf("unexpected findings: %#v", findings)
			}
		})
	}
}
