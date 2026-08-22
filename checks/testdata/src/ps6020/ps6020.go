package ps6020

type Buffer struct{}
type Recorder struct{}
type mixedProjection struct{}

func (*Recorder) FusedQKV(*Buffer, *Buffer) error                        { return nil }
func (*Recorder) MatMul(*Buffer, *Buffer) error                          { return nil }
func (*Recorder) RoPEPair(*Buffer) error                                 { return nil }
func (*Recorder) RoPEPairSplit(*Buffer, *Buffer, *Buffer, *Buffer) error { return nil }
func (*Recorder) Copy2D(*Buffer, *Buffer) error                          { return nil }
func (*mixedProjection) record(*Recorder, *Buffer, *Buffer) error        { return nil }

const (
	OpSlice     = "slice"
	OpTranspose = "transpose"
)

func groupProjection(*Buffer) *Buffer { return &Buffer{} }
func exec(string, ...*Buffer) *Buffer { return &Buffer{} }

func recordMixedFusedQKV(r *Recorder, input, packed, q, k, v *Buffer) error {
	if err := r.FusedQKV(input, packed); err != nil { // want "fused producer FusedQKV is followed by 3 layout-only operations.*Copy2D x3"
		return err
	}
	if err := r.RoPEPair(packed); err != nil {
		return err
	}
	if err := r.Copy2D(packed, q); err != nil {
		return err
	}
	if err := r.Copy2D(packed, k); err != nil {
		return err
	}
	return r.Copy2D(packed, v)
}

func recordGroupedProjection(m *mixedProjection, r *Recorder, input, packed, q, k, v *Buffer) error {
	if err := m.record(r, input, packed); err != nil { // want "fused producer record is followed by 3 layout-only operations.*Copy2D x3"
		return err
	}
	_ = r.Copy2D(packed, q)
	_ = r.Copy2D(packed, k)
	return r.Copy2D(packed, v)
}

func combinedGraphLayout(input *Buffer) *Buffer {
	packed := groupProjection(input) // want "fused producer groupProjection is followed by 2 layout-only operations.*exec x2"
	q := exec(OpSlice, packed)
	k := exec(OpTranspose, packed)
	return exec("compute", q, k)
}

// A fused epilogue removes the layout debt.
func recordFusedQKVWithEpilogue(r *Recorder, input, packed, q, k, v *Buffer) error {
	if err := r.FusedQKV(input, packed); err != nil {
		return err
	}
	return r.RoPEPairSplit(packed, q, k, v)
}

// Copies over an unrelated buffer are not charged to the producer.
func recordFusedQKVUnrelated(r *Recorder, input, packed, other, q, k *Buffer) error {
	if err := r.FusedQKV(input, packed); err != nil {
		return err
	}
	_ = r.Copy2D(other, q)
	return r.Copy2D(other, k)
}

// An ordinary standalone producer is outside this rule.
func ordinaryProjection(r *Recorder, input, output, q, k, v *Buffer) error {
	if err := r.MatMul(input, output); err != nil {
		return err
	}
	_ = r.Copy2D(output, q)
	_ = r.Copy2D(output, k)
	return r.Copy2D(output, v)
}

// One contract-boundary conversion is not multiple post-op layout debt.
func recordPackedFusedOnce(r *Recorder, input, packed, output *Buffer) error {
	if err := r.FusedQKV(input, packed); err != nil {
		return err
	}
	return r.Copy2D(packed, output)
}

// required public layout contract: explicitly retained and documented.
func recordRetainedFusedLayout(r *Recorder, input, packed, q, k, v *Buffer) error {
	if err := r.FusedQKV(input, packed); err != nil {
		return err
	}
	_ = r.Copy2D(packed, q)
	_ = r.Copy2D(packed, k)
	return r.Copy2D(packed, v)
}
