package ps6113

type buffer []float32
type binaryOp int

const (
	binaryAdd binaryOp = 1
	binaryMul binaryOp = 2
)

type recorder interface {
	Binary(left, right, output buffer, operation binaryOp) error
	AddBias(value, bias, output buffer, rows, width int) error
}

type projection interface {
	record(recorder, buffer, buffer, int) error
	recordAdd(recorder, buffer, buffer, buffer, int, int) error
}

type concreteProjection struct{}

func (concreteProjection) record(recorder, buffer, buffer, int) error { return nil }
func (concreteProjection) recordAdd(recorder, buffer, buffer, buffer, int, int) error {
	return nil
}

type secondProjection struct{}

func (secondProjection) record(recorder, buffer, buffer, int) error { return nil }
func (secondProjection) recordAdd(recorder, buffer, buffer, buffer, int, int) error {
	return nil
}

func firstErr(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func wrap(value error) error { return value }

type ownerBlock struct{ output projection }
type ownerBuffers struct {
	source      buffer
	temporary   buffer
	destination buffer
}

func ownerEager(block ownerBlock, state *ownerBuffers, r recorder, rows int) error {
	return firstErr(
		block.output.record(r, state.source, state.temporary, rows), // want `configured ps6113.projection.record immediately feeds an in-place ps6113.recorder.Binary residual add`
		r.Binary(state.destination, state.temporary, state.destination, binaryAdd),
	)
}

type directRecorder struct{}

func (directRecorder) Binary(buffer, buffer, buffer, binaryOp)  {}
func (directRecorder) AddBias(buffer, buffer, buffer, int, int) {}

type directProjection struct{}

func (directProjection) record(directRecorder, buffer, buffer, int)                 {}
func (directProjection) recordAdd(directRecorder, buffer, buffer, buffer, int, int) {}

func directPair(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1) // want `configured ps6113.directProjection.record immediately feeds an in-place ps6113.directRecorder.Binary residual add`
	r.Binary(destination, temporary, destination, binaryAdd)
}

func biasChain(p directProjection, r directRecorder, source, temporary, destination, bias buffer) {
	p.record(r, source, temporary, 1)
	r.AddBias(temporary, bias, temporary, 1, len(temporary))
	r.Binary(destination, temporary, destination, binaryAdd)
}

func temporaryReused(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
	_ = temporary
}

func temporaryAliased(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
	alias := temporary
	_ = alias
}

func temporaryAddressed(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
	_ = &temporary
}

func temporaryCaptured(p directProjection, r directRecorder, source, temporary, destination buffer) func() int {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
	return func() int { return len(temporary) }
}

func temporaryReturned(p directProjection, r directRecorder, source, temporary, destination buffer) buffer {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
	return temporary
}

func temporarySent(p directProjection, r directRecorder, source, temporary, destination buffer, output chan buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
	output <- temporary
}

func differentRecorder(p directProjection, first, second directRecorder, source, temporary, destination buffer) {
	p.record(first, source, temporary, 1)
	second.Binary(destination, temporary, destination, binaryAdd)
}

func sameSourceAndTemporary(p directProjection, r directRecorder, source, destination buffer) {
	p.record(r, source, source, 1)
	r.Binary(destination, source, destination, binaryAdd)
}

func sameTemporaryAndDestination(p directProjection, r directRecorder, source, destination buffer) {
	p.record(r, source, destination, 1)
	r.Binary(destination, destination, destination, binaryAdd)
}

func wrongOutput(p directProjection, r directRecorder, source, temporary, destination, output buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, output, binaryAdd)
}

func commutedAdd(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(temporary, destination, destination, binaryAdd)
}

func wrongOperation(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryMul)
}

func shadowedOperation(p directProjection, r directRecorder, source, temporary, destination buffer) {
	binaryAdd := binaryOp(1)
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
}

func interveningEffect(p directProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	consume(source)
	r.Binary(destination, temporary, destination, binaryAdd)
}

func consume(buffer) {}

func nestedEager(p projection, r recorder, source, temporary, destination buffer, rows int) error {
	return wrap(firstErr(p.record(r, source, temporary, rows), r.Binary(destination, temporary, destination, binaryAdd)))
}

func ellipsisEager(p projection, r recorder, source, temporary, destination buffer, rows int) error {
	errors := []error{p.record(r, source, temporary, rows), r.Binary(destination, temporary, destination, binaryAdd)}
	return firstErr(errors...)
}

func asyncEager(p projection, r recorder, source, temporary, destination buffer, rows int) {
	go firstErr(p.record(r, source, temporary, rows), r.Binary(destination, temporary, destination, binaryAdd))
}

func deferredEager(p projection, r recorder, source, temporary, destination buffer, rows int) {
	defer firstErr(p.record(r, source, temporary, rows), r.Binary(destination, temporary, destination, binaryAdd))
}

func unreachablePair(p directProjection, r directRecorder, source, temporary, destination buffer) {
	if false {
		p.record(r, source, temporary, 1)
		r.Binary(destination, temporary, destination, binaryAdd)
	}
}

type promotedProjection struct{ directProjection }

func promotedPair(p promotedProjection, r directRecorder, source, temporary, destination buffer) {
	p.record(r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
}

func methodExpression(p directProjection, r directRecorder, source, temporary, destination buffer) {
	directProjection.record(p, r, source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
}

func genericPair[T ~int](p directProjection, r directRecorder, source, temporary, destination buffer, rows T) {
	p.record(r, source, temporary, int(rows))
	r.Binary(destination, temporary, destination, binaryAdd)
}

type buffers struct{ value buffer }

func (value buffer) Len() int { return len(value) }

func distinctFieldRoots(p directProjection, r directRecorder, source, destination buffer, left, right buffers) {
	p.record(r, source, left.value, 1)
	r.Binary(destination, right.value, destination, binaryAdd)
}

func fieldTemporaryMethodUse(p directProjection, r directRecorder, source, destination buffer, value buffers) {
	p.record(r, source, value.value, 1)
	r.Binary(destination, value.value, destination, binaryAdd)
	_ = value.value.Len()
}

func consumeOwnerBuffers(*ownerBuffers) {}

func wholeOwnerCall(p directProjection, r directRecorder, state *ownerBuffers) {
	p.record(r, state.source, state.temporary, 1)
	r.Binary(state.destination, state.temporary, state.destination, binaryAdd)
	consumeOwnerBuffers(state)
}

func wholeOwnerReturn(p directProjection, r directRecorder, state *ownerBuffers) *ownerBuffers {
	p.record(r, state.source, state.temporary, 1)
	r.Binary(state.destination, state.temporary, state.destination, binaryAdd)
	return state
}

func wholeOwnerCapture(p directProjection, r directRecorder, state *ownerBuffers) func() {
	p.record(r, state.source, state.temporary, 1)
	r.Binary(state.destination, state.temporary, state.destination, binaryAdd)
	return func() { consumeOwnerBuffers(state) }
}

type receiverProjection struct{ source buffer }

func (*receiverProjection) record(directRecorder, buffer, buffer, int)                 {}
func (*receiverProjection) recordAdd(directRecorder, buffer, buffer, buffer, int, int) {}

func receiverPrefix(p *receiverProjection, r directRecorder, temporary, destination buffer) {
	p.record(r, p.source, temporary, 1)
	r.Binary(destination, temporary, destination, binaryAdd)
}

type recursiveBuffer struct{ child *recursiveBuffer }
type recursiveRecorder struct{}

func (recursiveRecorder) Binary(*recursiveBuffer, *recursiveBuffer, *recursiveBuffer, binaryOp) {}

type recursiveProjection struct{}

func (recursiveProjection) record(recursiveRecorder, *recursiveBuffer, *recursiveBuffer, int) {}
func (recursiveProjection) recordAdd(recursiveRecorder, *recursiveBuffer, *recursiveBuffer, *recursiveBuffer, int) {
}

func recursiveSourceDestinationOverlap(p recursiveProjection, r recursiveRecorder, source, temporary *recursiveBuffer) {
	p.record(r, source, temporary, 1)
	r.Binary(source.child, temporary, source.child, binaryAdd)
}
