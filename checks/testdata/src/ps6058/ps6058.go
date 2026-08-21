package ps6058

import "time"

type MTLCounterResultTimestamp struct {
	Timestamp uint64
}

type MetalDevice struct{}

func (*MetalDevice) QueryTimestampFrequency() uint64 { return 1 }
func (*MetalDevice) SampleTimestamps() (uint64, uint64) {
	return 1, 1
}

type CommandBuffer struct {
	GPUStartTime float64
	GPUEndTime   float64
}

const MTLCounterErrorValue = ^uint64(0)

func badFrequency(device *MetalDevice, samples []MTLCounterResultTimestamp) float64 {
	frequency := device.QueryTimestampFrequency() // want `invalid Metal counter timestamp conversion: queryTimestampFrequency applied in resolved-counter context and GPU timestamp delta treated directly as nanoseconds/time.Duration; missing paired before/after sampleTimestamps CPU/GPU span calibration, MTLCounterErrorValue sentinel rejection, sum-of-intervals versus GPUStartTime/GPUEndTime plausibility gate`
	delta := samples[1].Timestamp - samples[0].Timestamp
	return float64(delta) * 1e9 / float64(frequency)
}

// The exact Metal counter-result type appears only in this signature. That is
// still enough context to reject a raw duration cast in the body.
func badDuration(sample MTLCounterResultTimestamp, delta uint64) time.Duration {
	_ = sample
	return time.Duration(delta) // want `invalid Metal counter timestamp conversion: GPU timestamp delta treated directly as nanoseconds/time.Duration; missing paired before/after sampleTimestamps CPU/GPU span calibration, MTLCounterErrorValue sentinel rejection, sum-of-intervals versus GPUStartTime/GPUEndTime plausibility gate`
}

// A frequency helper outside resolved timestamp-counter context is unrelated.
func unrelatedFrequency(device *MetalDevice) uint64 {
	return device.QueryTimestampFrequency()
}

// Merely resolving a raw timestamp does not imply a wrong conversion.
func resolveOnly(samples []MTLCounterResultTimestamp) uint64 {
	return samples[0].Timestamp
}

func calibrated(device *MetalDevice, samples []MTLCounterResultTimestamp, command CommandBuffer) float64 {
	cpuBefore, gpuBefore := device.SampleTimestamps()
	cpuAfter, gpuAfter := device.SampleTimestamps()
	cpuTimestampSpan := cpuAfter - cpuBefore
	gpuTimestampSpan := gpuAfter - gpuBefore
	if samples[0].Timestamp == MTLCounterErrorValue || samples[1].Timestamp == MTLCounterErrorValue {
		return 0
	}
	delta := samples[1].Timestamp - samples[0].Timestamp
	converted := float64(delta) / float64(gpuTimestampSpan) * float64(cpuTimestampSpan)
	totalEncoderIntervals := converted
	if totalEncoderIntervals > (command.GPUEndTime-command.GPUStartTime)*1e9 {
		return 0
	}
	return converted
}

var embeddedBad = "let set = MTLCommonCounterSetTimestamp; let frequency = device.queryTimestampFrequency()" // want `invalid Metal counter timestamp conversion: queryTimestampFrequency applied in resolved-counter context; missing paired before/after sampleTimestamps CPU/GPU span calibration, MTLCounterErrorValue sentinel rejection, sum-of-intervals versus GPUStartTime/GPUEndTime plausibility gate`

var embeddedCalibrated = `func calibrated(_ samples: UnsafePointer<MTLCounterResultTimestamp>, device: MTLDevice, command: MTLCommandBuffer) {
  let (cpuBefore, gpuBefore) = device.sampleTimestamps()
  let (cpuAfter, gpuAfter) = device.sampleTimestamps()
  let cpuTimestampSpan = cpuAfter - cpuBefore
  let gpuTimestampSpan = gpuAfter - gpuBefore
  guard samples[0].timestamp != MTLCounterErrorValue else { return }
  let converted = Double(samples[1].timestamp - samples[0].timestamp) / Double(gpuTimestampSpan) * Double(cpuTimestampSpan)
  let totalEncoderIntervals = converted
  validate(totalEncoderIntervals, command.GPUStartTime, command.GPUEndTime)
}`

var embeddedComment = `// MTLCommonCounterSetTimestamp; device.queryTimestampFrequency()
/* MTLCounterResultTimestamp delta; double ns = delta * 1e9; */`

var _ = []any{badFrequency, badDuration, unrelatedFrequency, resolveOnly, calibrated, embeddedBad, embeddedCalibrated, embeddedComment}
