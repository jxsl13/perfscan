package ps7001

// Vocabulary for this fixture: gpuReductionKernels =
// {matvec_q4k, matvec_coop, matvec_noloop, serial_a, coop_b}.
//
// Metal kernels are embedded as single-line interpreted strings so the // want
// expectation sits on the string-literal's line, outside the source.

// POSITIVE: one thread per row, serial K loop, no simd reduction.
var serial = "#include <metal_stdlib>\nkernel void matvec_q4k(device float* y, device float* w, device float* x, uint tid [[thread_position_in_grid]]) { uint row = tid; float acc = 0.0; for (uint k = 0; k < 2048; ++k) { acc += w[row*2048+k] * x[k]; } y[row] = acc; }" // want `GPU kernel matvec_q4k accumulates the reduction dimension serially`

// GUARD: cooperative — the K loop strides by the SIMD width and reduces with
// simd_sum, so it is NOT serial.
var coop = "#include <metal_stdlib>\nkernel void matvec_coop(device float* y, uint tid [[thread_position_in_grid]], uint lane [[thread_index_in_simdgroup]]) { float part = 0.0; for (uint k = lane; k < 2048; k += 32) part += 1.0; float acc = simd_sum(part); if (lane == 0) y[tid] = acc; }"

// GUARD: configured but has no reduction loop at all.
var noLoop = "#include <metal_stdlib>\nkernel void matvec_noloop(device float* y, uint tid [[thread_position_in_grid]]) { y[tid] = 0.0; }"

// GUARD: a serial Metal kernel, but its name is NOT in the vocabulary.
var notListed = "#include <metal_stdlib>\nkernel void some_other(device float* y, uint tid [[thread_position_in_grid]]) { float acc = 0.0; for (uint k = 0; k < 10; ++k) acc += 1.0; y[tid] = acc; }"

// GUARD: not Metal source (no Metal marker) even though it contains "kernel"
// and a for-loop.
var notMetal = "func kernel() { for (i := 0; i < 10; i++) { acc += 1 } }"

// COMBINED: one string holds a serial kernel and a cooperative one; only the
// serial one is reported, and the coop sibling's simd_sum must NOT suppress it
// (per-kernel region split).
var combined = "#include <metal_stdlib>\nkernel void serial_a(device float* y, uint t [[thread_position_in_grid]]) { float a = 0.0; for (uint k = 0; k < 99; ++k) a += 1.0; y[t] = a; }\nkernel void coop_b(uint t [[thread_position_in_grid]], uint lane [[thread_index_in_simdgroup]]) { float p = 0.0; for (uint k = lane; k < 99; k += 32) p += 1.0; float s = simd_sum(p); }" // want `GPU kernel serial_a accumulates the reduction dimension serially`
