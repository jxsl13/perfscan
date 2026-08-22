package ps7001silent

// A textbook serial-K Metal kernel, but the test runs it with an EMPTY
// gpuReductionKernels vocabulary, so the check must stay silent. No expectation
// comments.

var serial = "#include <metal_stdlib>\nkernel void matvec_q4k(device float* y, uint tid [[thread_position_in_grid]]) { float acc = 0.0; for (uint k = 0; k < 2048; ++k) acc += 1.0; y[tid] = acc; }"
