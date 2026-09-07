package ps6104

// want +1 "GPU kernel q4k_tail repeats launch-invariant row-tail predicate `row < outputRows` inside every block<blocks block iteration.*1.0462x.*1.0112x.*1.03x"
var candidate = `#include <metal_stdlib>
kernel void q4k_tail(device const uchar* weights [[buffer(0)]], uint outputRows [[buffer(1)]],
    uint group [[threadgroup_position_in_grid]]) {
  uint row = group * 2 + 1;
  for (uint block = 0; block < blocks; ++block) {
    if (row < outputRows) consume(weights, row, block);
  }
  if (row < outputRows) store(row);
}`

var changing = `#include <metal_stdlib>
kernel void changing(uint row [[thread_position_in_grid]], uint outputRows [[buffer(0)]]) {
  for (uint block = 0; block < blocks; ++block) {
    if (row + block < outputRows) consume(block);
  }
}`
