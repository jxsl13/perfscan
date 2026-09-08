package ps6114

type Tensor struct{ shape []int }

func (value *Tensor) Shape() []int { return value.shape }

type Context struct{}
type Operation int

const (
	sliceOp  Operation = 1
	concatOp Operation = 2
)

type SliceAttrs struct{ Axis, Start, End int }
type ConcatAttrs struct{ Axis int }

func gather(*Context, Operation, SliceAttrs, *Tensor) (*Tensor, error) { return nil, nil }
func concat(*Context, Operation, []*Tensor, ConcatAttrs) ([]*Tensor, error) {
	return nil, nil
}

type Transform struct{}

func (Transform) Forward(*Context, *Tensor) (*Tensor, error) { return nil, nil }

type Model struct {
	Norm Transform
	Pos  *Tensor
	Head Transform
}

type rowTransform interface {
	Forward(*Context, *Tensor) (*Tensor, error)
}

func ownerRange(t Transform, ctx *Context, input, positions *Tensor) ([]*Tensor, error) {
	batch := input.Shape()[0]
	stride := positions.Shape()[0]
	h := input
	var err error
	if h, err = t.Forward(ctx, h); err != nil { // want `eliminate batch\*\(stride-1\) row transforms`
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func (m *Model) ownerMethodRange(ctx *Context, input *Tensor) (*Tensor, error) {
	batch := input.Shape()[0]
	stride := m.Pos.Shape()[0]
	h := input
	var err error
	if h, err = m.Norm.Forward(ctx, h); err != nil { // want `eliminate batch\*\(stride-1\) row transforms`
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return m.Head.Forward(ctx, selected[0])
}

func assignmentClassic(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input) // want `eliminate batch\*\(stride-1\) row transforms`
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := 0; index < batch; index++ {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: stride * index, End: 1 + stride*index}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func extraTransformUse(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	_ = h.Shape()
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func wrongAxis(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 1, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func wrongEnd(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 2}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func wrongIndex(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[(index+1)%batch] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func collectionAppend(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	rows = append(rows, input)
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func conditionalLoop(t Transform, ctx *Context, input *Tensor, batch, stride int, enabled bool) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	if enabled {
		for index := range batch {
			row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
			if err != nil {
				return nil, err
			}
			rows[index] = row
		}
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func breakLoop(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		if index == 2 {
			break
		}
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func swallowedGatherError(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, _ := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func interfaceTransform(t rowTransform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func currentFusedFallback(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func unstableStride(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	stride++
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func strideOne(t Transform, ctx *Context, input *Tensor) ([]*Tensor, error) {
	const batch, stride = 4, 1
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func zeroBatch(t Transform, ctx *Context, input *Tensor) ([]*Tensor, error) {
	const batch, stride = 0, 4
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func deadPath(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	if false {
		h, err := t.Forward(ctx, input)
		if err != nil {
			return nil, err
		}
		rows := make([]*Tensor, batch)
		for index := range batch {
			row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
			if err != nil {
				return nil, err
			}
			rows[index] = row
		}
		selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
		if err != nil {
			return nil, err
		}
		return selected, nil
	}
	return nil, nil
}

func postTransformAlias(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	alias := h
	_ = alias
	return selected, nil
}

func postCollectionEscape(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	consumeRows(rows)
	return selected, nil
}

func consumeRows([]*Tensor) {}

func wrongConcatAxis(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 1})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func wrongConcatOperation(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, sliceOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func capturedBatch(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	_ = func() int { return batch }
	return selected, nil
}

func receiverExposure(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	consumeTransform(t)
	return selected, nil
}

func consumeTransform(Transform) {}

func terminalSideEffect(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return sideEffect(), err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func sideEffect() []*Tensor { return nil }

func wrongErrorReturn(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, otherError()
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func otherError() error { return nil }

var globalBatch, globalStride = 4, 5

func globalGeometry(t Transform, ctx *Context, input *Tensor) ([]*Tensor, error) {
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, globalBatch)
	for index := range globalBatch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * globalStride, End: index*globalStride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func constantGeometry(t Transform, ctx *Context, input *Tensor) ([]*Tensor, error) {
	const batch, stride = 4, 5
	h, err := t.Forward(ctx, input) // want `eliminate 16 row transforms`
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func overflowGeometry(t Transform, ctx *Context, input *Tensor) ([]*Tensor, error) {
	const batch, stride = 4, 1 << 62
	h, err := t.Forward(ctx, input)
	if err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func safeOldVersionRead(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h := input
	oldWasNil := h == nil
	_ = oldWasNil
	var err error
	if h, err = t.Forward(ctx, h); err != nil { // want `eliminate batch\*\(stride-1\) row transforms`
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}

func preAddressOutput(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h := input
	alias := &(h)
	var err error
	if h, err = t.Forward(ctx, h); err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	consumeTensor(*alias)
	return selected, nil
}

func consumeTensor(*Tensor) {}

func preCaptureOutput(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	h := input
	read := func() *Tensor { return h }
	var err error
	if h, err = t.Forward(ctx, h); err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, h)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	consumeTensor(read())
	return selected, nil
}

var packageOutput *Tensor

func packageOutputCandidate(t Transform, ctx *Context, input *Tensor, batch, stride int) ([]*Tensor, error) {
	var err error
	if packageOutput, err = t.Forward(ctx, input); err != nil {
		return nil, err
	}
	rows := make([]*Tensor, batch)
	for index := range batch {
		row, err := gather(ctx, sliceOp, SliceAttrs{Axis: 0, Start: index * stride, End: index*stride + 1}, packageOutput)
		if err != nil {
			return nil, err
		}
		rows[index] = row
	}
	selected, err := concat(ctx, concatOp, rows, ConcatAttrs{Axis: 0})
	if err != nil {
		return nil, err
	}
	return selected, nil
}
