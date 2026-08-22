package ps6059

type Tensor struct{}

type MetalRecorder struct{}

func (*MetalRecorder) Q4KMatmul(input, weight *Tensor) *Tensor      { return input }
func (*MetalRecorder) Q6KMatmul(input, weight *Tensor) *Tensor      { return input }
func (*MetalRecorder) FusedQ4KMatmul(input, weight *Tensor) *Tensor { return input }
func (*MetalRecorder) SwiGLU(gate, up *Tensor) *Tensor              { return gate }
func (*MetalRecorder) Consume(inputs ...*Tensor)                    {}

type CPUOps struct{}

func (*CPUOps) Q4KMatmul(input, weight *Tensor) *Tensor { return input }
func (*CPUOps) SwiGLU(gate, up *Tensor) *Tensor         { return gate }

func adjacentCompatible(rec *MetalRecorder, input, gateWeight, upWeight *Tensor) *Tensor {
	gate := rec.Q4KMatmul(input, gateWeight) // want `adjacent compatible Q4KMatmul/Q4KMatmul -> SwiGLU GPU calls expose a possible saved intermediate and dispatch, but fusion is only a hypothesis`
	up := rec.Q4KMatmul(input, upWeight)
	out := rec.SwiGLU(gate, up)
	return out
}

func incompatibleMethods(rec *MetalRecorder, input, gateWeight, upWeight *Tensor) *Tensor {
	gate := rec.Q4KMatmul(input, gateWeight)
	up := rec.Q6KMatmul(input, upWeight)
	return rec.SwiGLU(gate, up)
}

func differentRecorders(first, second *MetalRecorder, input, gateWeight, upWeight *Tensor) *Tensor {
	gate := first.Q4KMatmul(input, gateWeight)
	up := second.Q4KMatmul(input, upWeight)
	return first.SwiGLU(gate, up)
}

func reusedIntermediate(rec *MetalRecorder, input, gateWeight, upWeight *Tensor) *Tensor {
	gate := rec.Q4KMatmul(input, gateWeight)
	rec.Consume(gate)
	up := rec.Q4KMatmul(input, upWeight)
	return rec.SwiGLU(gate, up)
}

func alreadyFusedMethod(rec *MetalRecorder, input, gateWeight, upWeight *Tensor) *Tensor {
	gate := rec.FusedQ4KMatmul(input, gateWeight)
	up := rec.FusedQ4KMatmul(input, upWeight)
	return rec.SwiGLU(gate, up)
}

func ordinaryCPU(cpu *CPUOps, input, gateWeight, upWeight *Tensor) *Tensor {
	gate := cpu.Q4KMatmul(input, gateWeight)
	up := cpu.Q4KMatmul(input, upWeight)
	return cpu.SwiGLU(gate, up)
}

var embeddedFused = "#include <metal_stdlib>\nkernel void embedded_fused(uint lane [[thread_index_in_simdgroup]], uint3 group [[threadgroup_position_in_grid]]) { float gateAcc = 0; float upAcc = 0; gateAcc += load_gate(lane); upAcc += load_up(lane); out[group.x] = silu(gateAcc) * upAcc; }" // want `GPU kernel embedded_fused combines gate/up reductions and activation through gateAcc/upAcc; removing an intermediate and dispatch is only a hypothesis`

var _ = []any{adjacentCompatible, incompatibleMethods, differentRecorders, reusedIntermediate, alreadyFusedMethod, ordinaryCPU, embeddedFused}
