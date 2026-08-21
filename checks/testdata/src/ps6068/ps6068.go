package ps6068

var ownerShapedCandidate = "#include <metal_stdlib>\nusing namespace metal;\nkernel void qmatmul_q3k_broadcast(device const uchar* W [[buffer(0)]], ushort lane [[thread_index_in_simdgroup]], ushort simdgroup [[simdgroup_index_in_threadgroup]]) { int rowOff = (int)simdgroup * 110; int scaleOff = rowOff + 96; ushort4 scales = ushort4(0); if (lane == 0) scales = *((device const ushort4 *)(W + scaleOff)); scales.x = simd_broadcast_first(scales.x); scales.y = simd_broadcast_first(scales.y); scales.z = simd_broadcast_first(scales.z); scales.w = simd_broadcast_first(scales.w); }" // want `Metal kernel qmatmul_q3k_broadcast loads lane-uniform device value\(s\) scales only in the SIMD leader and explicitly broadcasts them`

var laneDependentAddress = `#include <metal_stdlib>
kernel void lane_dependent(device const ushort* scales [[buffer(0)]],
                           ushort lane [[thread_index_in_simdgroup]]) {
  int address = 32 + lane;
  ushort value = 0;
  if (lane == 0) value = scales[address];
  value = simd_broadcast_first(value);
}`

var noLeaderSerialization = `#include <metal_stdlib>
kernel void all_lanes(device const ushort* scales [[buffer(0)]],
                      ushort lane [[thread_index_in_simdgroup]]) {
  ushort value = scales[7];
  value = simd_broadcast_first(value);
}`

var commentedCandidate = `#include <metal_stdlib>
kernel void comments(device const ushort* scales [[buffer(0)]],
                     ushort lane [[thread_index_in_simdgroup]]) {
  ushort value = 0;
  /* if (lane == 0) value = scales[7];
     value = simd_broadcast_first(value); */
}`
