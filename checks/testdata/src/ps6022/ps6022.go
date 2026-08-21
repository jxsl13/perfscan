package ps6022

type Buffer struct{}
type Recorder struct{}
type CPUConverter struct{}

func (*Recorder) F32ToF16(*Buffer, *Buffer, int, int) error       { return nil }
func (*Recorder) PairedF32ToF16(*Buffer, *Buffer, int, int) error { return nil }
func (*Recorder) MatMul(*Buffer, *Buffer, int, int) error         { return nil }
func (*CPUConverter) F32ToF16(*Buffer, *Buffer, int, int) error   { return nil }

func recordMetalKVCache(r *Recorder, kSource, kCache, vSource, vCache *Buffer, rows, width int) {
	_ = r.F32ToF16(kSource, kCache, rows, width) // want "2 consecutive F32ToF16 conversion dispatches use the same command context and scalar geometry but independent source/destination buffer pairs"
	_ = r.F32ToF16(vSource, vCache, rows, width)
}

func recordMetalQKVCache(r *Recorder, qSource, qCache, kSource, kCache, vSource, vCache *Buffer, rows, width int) {
	_ = r.F32ToF16(qSource, qCache, rows, width) // want "3 consecutive F32ToF16 conversion dispatches use the same command context and scalar geometry but independent source/destination buffer pairs"
	_ = r.F32ToF16(kSource, kCache, rows, width)
	_ = r.F32ToF16(vSource, vCache, rows, width)
}

// Different row geometry cannot share one indexed conversion dispatch.
func recordMetalDifferentGeometry(r *Recorder, kSource, kCache, vSource, vCache *Buffer, rows, width int) {
	_ = r.F32ToF16(kSource, kCache, rows, width)
	_ = r.F32ToF16(vSource, vCache, rows+1, width)
}

// Another command on the same recorder breaks consecutiveness.
func recordMetalInterveningCommand(r *Recorder, kSource, kCache, vSource, vCache, tmp *Buffer, rows, width int) {
	_ = r.F32ToF16(kSource, kCache, rows, width)
	_ = r.MatMul(kCache, tmp, rows, width)
	_ = r.F32ToF16(vSource, vCache, rows, width)
}

// The API already represents a paired conversion.
func recordMetalAlreadyPaired(r *Recorder, a, b, c, d, e, f, g, h *Buffer, rows, width int) {
	_ = r.PairedF32ToF16(a, b, rows, width)
	_ = r.PairedF32ToF16(c, d, rows, width)
	_ = e
	_ = f
	_ = g
	_ = h
}

// A plain CPU helper stays local.
func convertCPU(c *CPUConverter, a, b, d, e *Buffer, rows, width int) {
	_ = c.F32ToF16(a, b, rows, width)
	_ = c.F32ToF16(d, e, rows, width)
}
