package ps6032

type Tensor struct {
	values []float32
}

type MetalRecorder struct{}

func (*MetalRecorder) ResidualAdd(hidden, residual *Tensor) *Tensor    { return hidden }
func (*MetalRecorder) ElementwiseAdd(hidden, residual *Tensor) *Tensor { return hidden }
func (*MetalRecorder) RMSNorm(input, weight *Tensor) *Tensor           { return input }
func (*MetalRecorder) LayerNorm(input, weight *Tensor) *Tensor         { return input }
func (*MetalRecorder) RecordBarrier()                                  {}
func (*MetalRecorder) Consume(input *Tensor)                           {}

type CPUOps struct{}

func (*CPUOps) ResidualAdd(hidden, residual *Tensor) *Tensor { return hidden }
func (*CPUOps) RMSNorm(input, weight *Tensor) *Tensor        { return input }

func decodeMetalResidualRMSNorm(rec *MetalRecorder, hidden, residual, weight *Tensor) *Tensor {
	tmp := rec.ResidualAdd(hidden, residual) // want `adjacent accelerator ResidualAdd -> RMSNorm calls share single-consumer intermediate tmp; one dispatch/dependency boundary is statically removable`
	out := rec.RMSNorm(tmp, weight)
	return out
}

func decodeMetalElementwiseLayerNorm(rec *MetalRecorder, hidden, residual, weight *Tensor) *Tensor {
	tmp := rec.ElementwiseAdd(hidden, residual) // want `adjacent accelerator ElementwiseAdd -> LayerNorm calls share single-consumer intermediate tmp; one dispatch/dependency boundary is statically removable`
	out := rec.LayerNorm(tmp, weight)
	return out
}

// A later use means the intermediate is not single-consumer.
func decodeMetalResidualRMSNormReused(rec *MetalRecorder, hidden, residual, weight *Tensor) *Tensor {
	tmp := rec.ResidualAdd(hidden, residual)
	out := rec.RMSNorm(tmp, weight)
	rec.Consume(tmp)
	return out
}

// An intervening command invalidates adjacency.
func decodeMetalResidualRMSNormIntervening(rec *MetalRecorder, hidden, residual, weight *Tensor) *Tensor {
	tmp := rec.ResidualAdd(hidden, residual)
	rec.RecordBarrier()
	out := rec.RMSNorm(tmp, weight)
	return out
}

// Different command contexts must not be fused implicitly.
func decodeMetalResidualRMSNormDifferentRecorders(first, second *MetalRecorder, hidden, residual, weight *Tensor) *Tensor {
	tmp := first.ResidualAdd(hidden, residual)
	out := second.RMSNorm(tmp, weight)
	return out
}

// Ordinary CPU helpers are outside the accelerator command boundary.
func decodeCPUResidualRMSNorm(ops *CPUOps, hidden, residual, weight *Tensor) *Tensor {
	tmp := ops.ResidualAdd(hidden, residual)
	out := ops.RMSNorm(tmp, weight)
	return out
}

// A non-normalization consumer stays silent.
func decodeMetalResidualConsumed(rec *MetalRecorder, hidden, residual *Tensor) {
	tmp := rec.ResidualAdd(hidden, residual)
	rec.Consume(tmp)
}
