package checks

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6068(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6068.Analyzer, "ps6068")
}

func TestPS6068NativeSourceShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   int
		wide   bool
	}{
		{
			name: "owner shaped inline leader and packed header",
			source: `#include <metal_stdlib>
kernel void q3k_candidate(device const uchar* W [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]], ushort sg [[simdgroup_index_in_threadgroup]]) {
  int row = (int)sg * 110;
  int base = row + 96;
  ushort2 header = ushort2(0);
  if (lane == 0) header = *((device const ushort2 *)(W + base));
  header.x = simd_broadcast_first(header.x);
  header.y = simd_broadcast_first(header.y);
}`,
			want: 1,
			wide: true,
		},
		{
			name: "braced indexed metadata load",
			source: `#include <metal_stdlib>
kernel void indexed(device const ushort* scales [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  int block = 7;
  ushort scale = 0;
  if (lane == 0) { scale = scales[block]; }
  scale = simd_broadcast(scale, 0);
}`,
			want: 1,
		},
		{
			name: "simd is first guard",
			source: `#include <metal_stdlib>
kernel void first_guard(device const ushort* scales [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  ushort scale = 0;
  if (simd_is_first()) { scale = scales[7]; }
  scale = simd_broadcast_first(scale);
}`,
			want: 1,
		},
		{
			name: "objective c concatenated shader",
			source: `static NSString* shader =
"#include <metal_stdlib>\n"
"kernel void objc_shader(device const ushort* scales [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]]) {\n"
"  ushort scale = 0; if (lane == 0) scale = scales[7];\n"
"  scale = simd_broadcast_first(scale);\n"
"}\n";`,
			want: 1,
		},
		{
			name: "lane dependent address",
			source: `#include <metal_stdlib>
kernel void lane_address(device const ushort* scales [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  int address = 7 + lane;
  ushort scale = 0;
  if (lane == 0) { scale = scales[address]; }
  scale = simd_broadcast_first(scale);
}`,
		},
		{
			name: "all lanes load",
			source: `#include <metal_stdlib>
kernel void unguarded(device const ushort* scales [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  ushort scale = scales[7];
  scale = simd_broadcast_first(scale);
}`,
		},
		{
			name: "leader computes without a load",
			source: `#include <metal_stdlib>
kernel void computed(ushort lane [[thread_index_in_simdgroup]]) {
  ushort scale = 0;
  if (lane == 0) { scale = 7; }
  scale = simd_broadcast_first(scale);
}`,
		},
		{
			name: "nonzero broadcast source",
			source: `#include <metal_stdlib>
kernel void lane_one(device const ushort* scales [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  ushort scale = 0;
  if (lane == 0) { scale = scales[7]; }
  scale = simd_broadcast(scale, 1);
}`,
		},
		{
			name: "validated annotation",
			source: `#include <metal_stdlib>
//perfscan:simd-uniform-load-broadcast-validated same-binary retention and alignment proof.
kernel void validated(device const ushort* scales [[buffer(0)]],
    ushort lane [[thread_index_in_simdgroup]]) {
  ushort scale = 0;
  if (lane == 0) { scale = scales[7]; }
  scale = simd_broadcast_first(scale);
}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			findings := ps6068UniformLoadBroadcasts(test.source)
			if len(findings) != test.want {
				t.Fatalf("findings = %#v, want %d", findings, test.want)
			}
			if test.want != 0 && findings[0].widened != test.wide {
				t.Fatalf("widened = %t, want %t", findings[0].widened, test.wide)
			}
		})
	}
}
