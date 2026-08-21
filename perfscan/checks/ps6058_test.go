package checks

import (
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestPS6058(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), PS6058.Analyzer, "ps6058")
}

func TestPS6058NativeTimestampConversion(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   []string
	}{
		{
			name: "swift frequency",
			source: `func resolve(_ samples: UnsafePointer<MTLCounterResultTimestamp>, device: MTLDevice) {
  let frequency = device.queryTimestampFrequency()
  consume(frequency)
}`,
			want: []string{"queryTimestampFrequency", "paired before/after sampleTimestamps", "MTLCounterErrorValue", "GPUStartTime/GPUEndTime"},
		},
		{
			name: "objective c direct nanoseconds",
			source: `void resolve(const MTLCounterResultTimestamp *samples) {
  uint64_t delta = samples[1].timestamp - samples[0].timestamp;
  double nanoseconds = delta * 1000000000;
}`,
			want: []string{"treated directly as nanoseconds/time.Duration", "cpuTimestampSpan/gpuTimestampSpan"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			problem, found := ps6058NativeProblem(test.source)
			if !found {
				t.Fatal("expected timestamp-conversion finding")
			}
			message := ps6058Message(problem)
			for _, fragment := range test.want {
				if !strings.Contains(message, fragment) {
					t.Errorf("message does not contain %q: %s", fragment, message)
				}
			}
		})
	}
}

func TestPS6058NativeGuards(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "complete paired calibration",
			source: `func calibrated(_ samples: UnsafePointer<MTLCounterResultTimestamp>, device: MTLDevice, command: MTLCommandBuffer) {
  let (cpuBefore, gpuBefore) = device.sampleTimestamps()
  let (cpuAfter, gpuAfter) = device.sampleTimestamps()
  let cpuTimestampSpan = cpuAfter - cpuBefore
  let gpuTimestampSpan = gpuAfter - gpuBefore
  guard samples[0].timestamp != MTLCounterErrorValue else { return }
  let converted = Double(samples[1].timestamp - samples[0].timestamp) / Double(gpuTimestampSpan) * Double(cpuTimestampSpan)
  let totalEncoderIntervals = converted
  validate(totalEncoderIntervals, command.GPUStartTime, command.GPUEndTime)
}`,
		},
		{
			name: "frequency outside counter context",
			source: `func unrelated(device: MTLDevice) {
  consume(device.queryTimestampFrequency())
}`,
		},
		{
			name: "raw counter resolution only",
			source: `func resolve(_ samples: UnsafePointer<MTLCounterResultTimestamp>) {
  consume(samples[0].timestamp)
}`,
		},
		{
			name: "commented bad example",
			source: `func clean() {
  // MTLCommonCounterSetTimestamp; device.queryTimestampFrequency()
  /* MTLCounterResultTimestamp delta; double ns = delta * 1e9; */
}`,
		},
		{
			name: "documentation prose",
			source: `MTLCounterResultTimestamp values use a GPU clock. Do not apply
queryTimestampFrequency to resolved samples; use paired calibration.`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if problem, found := ps6058NativeProblem(test.source); found {
				t.Fatalf("unexpected finding: %#v", problem)
			}
		})
	}
}
