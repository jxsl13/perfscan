package ps6106

func write32(buf []float32, value float32) {
	for i := range buf {
		buf[i] = value
	}
}

func activate32(buf []float32, scale float32) {
	for i := range buf {
		buf[i] = buf[i] * scale
	}
}

func observe32(buf []float32, scale float32) {
	for i := range buf {
		_ = buf[i] * scale
	}
}

func localLengthPositive() {
	scratch := make([]float32, 64, 128)
	write32(scratch[:16:16], 1)
	activate32(scratch, 2) // want `local-float-scratch: bounded scratch writes and later observers use 16 active elements, but the Go slice logical length consumer traverses 64 elements \(source-proven, 4x ratio\); one static source site; runtime invocation count unknown.*advisory, no automatic fix`
	observe32(scratch[:16:16], 3)
}

func localFixedRepetitions() {
	scratch := make([]float32, 64)
	for iteration := 0; iteration < 3; iteration++ {
		write32(scratch[:16:16], 1)
		activate32(scratch, 2) // want `4x ratio.*3 source-proven static enclosing repetitions; runtime invocation count unknown`
		observe32(scratch[:16:16], 3)
	}
}

// The backing capacity is larger, but range traverses len == active.
func largeCapacityOnly() {
	scratch := make([]float32, 16, 128)
	write32(scratch[:16:16], 1)
	activate32(scratch, 2)
	observe32(scratch[:16:16], 3)
}

func twoIndexProducer() {
	scratch := make([]float32, 64)
	write32(scratch[:16], 1)
	activate32(scratch, 2)
	observe32(scratch[:16:16], 3)
}

func tailObserved() float32 {
	scratch := make([]float32, 64)
	write32(scratch[:16:16], 1)
	activate32(scratch, 2)
	observe32(scratch[:16:16], 3)
	return scratch[63]
}

func separateScratchObjects() {
	p := make([]float32, 64)
	q := make([]float32, 64)
	write32(p[:16:16], 1)
	activate32(q, 2)
	observe32(p[:16:16], 3)
}

const activeA = 16
const activeB = 16

func equalButDistinctExtents() {
	scratch := make([]float32, 64)
	write32(scratch[:activeA:activeA], 1)
	activate32(scratch, 2)
	observe32(scratch[:activeB:activeB], 3)
}

func effectful(buf []float32, scale float32) {
	for i := range buf {
		buf[i] = helper(buf[i], scale)
	}
}

func helper(value, scale float32) float32 { return value * scale }

func callInConsumer() {
	scratch := make([]float32, 64)
	write32(scratch[:16:16], 1)
	effectful(scratch, 2)
	observe32(scratch[:16:16], 3)
}

var mutableScale float32 = 2

func mutableScalarConsumer(buf []float32) {
	for i := range buf {
		buf[i] = buf[i] * mutableScale
	}
}

func mutableScalarBody() {
	scratch := make([]float32, 64)
	write32(scratch[:16:16], 1)
	mutableScalarConsumer(scratch)
	observe32(scratch[:16:16], 3)
}

type fieldState struct {
	scratch []float32
}

func newFieldState() *fieldState {
	state := &fieldState{}
	state.scratch = make([]float32, 64, 128)
	return state
}

func (state *fieldState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2) // want `local-float-scratch: bounded scratch writes and later observers use 16 active elements.*Go slice logical length consumer traverses 64 elements.*4x ratio`
	observe32(state.scratch[:16:16], 3)
}

func callValidFieldState() {
	newFieldState().run()
}

type splitState struct {
	scratch []float32
}

func newSplitState() *splitState {
	state := &splitState{}
	state.scratch = make([]float32, 64)
	return state
}

// p.scratch and q.scratch share one field declaration object but not storage.
func splitInstances(p, q *splitState) {
	write32(p.scratch[:16:16], 1)
	activate32(q.scratch, 2)
	observe32(p.scratch[:16:16], 3)
}

type escapedOwnerState struct {
	scratch []float32
}

var escapedOwner *escapedOwnerState

func newEscapedOwnerState() *escapedOwnerState {
	state := &escapedOwnerState{}
	state.scratch = make([]float32, 64)
	escapedOwner = state
	return state
}

func (state *escapedOwnerState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type wholeOwnerState struct {
	scratch []float32
}

func newWholeOwnerState() *wholeOwnerState {
	state := &wholeOwnerState{}
	state.scratch = make([]float32, 64)
	return state
}

func (state *wholeOwnerState) resetWhole() {
	*state = wholeOwnerState{make([]float32, 16)}
}

func (state *wholeOwnerState) copyWhole() wholeOwnerState {
	return *state
}

func (state *wholeOwnerState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type wholeWriteOnlyState struct {
	scratch []float32
}

func newWholeWriteOnlyState() *wholeWriteOnlyState {
	state := &wholeWriteOnlyState{}
	state.scratch = make([]float32, 64)
	return state
}

func (state *wholeWriteOnlyState) resetWhole() {
	*state = wholeWriteOnlyState{make([]float32, 16)}
}

func (state *wholeWriteOnlyState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type wholeCopyOnlyState struct {
	scratch []float32
}

func newWholeCopyOnlyState() *wholeCopyOnlyState {
	state := &wholeCopyOnlyState{}
	state.scratch = make([]float32, 64)
	return state
}

func (state *wholeCopyOnlyState) copyWhole() wholeCopyOnlyState {
	return *state
}

func (state *wholeCopyOnlyState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type aggregateEscapeState struct {
	scratch []float32
}

var aggregateEscapeSink []any

func newAggregateEscapeState() *aggregateEscapeState {
	state := &aggregateEscapeState{}
	state.scratch = make([]float32, 64)
	return state
}

func exposeAggregate(state *aggregateEscapeState) {
	aggregateEscapeSink = []any{state}
}

func (state *aggregateEscapeState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type boxedEscapeState struct {
	scratch []float32
}

var boxedEscapeSink any

func newBoxedEscapeState() *boxedEscapeState {
	state := &boxedEscapeState{}
	state.scratch = make([]float32, 64)
	return state
}

func exposeBoxed(state *boxedEscapeState) {
	boxedEscapeSink = state
}

func (state *boxedEscapeState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type mapEscapeState struct {
	scratch []float32
}

var mapEscapeSink map[string]any

func newMapEscapeState() *mapEscapeState {
	state := &mapEscapeState{}
	state.scratch = make([]float32, 64)
	return state
}

func exposeMap(state *mapEscapeState) {
	mapEscapeSink = map[string]any{"state": state}
}

func (state *mapEscapeState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

type structEscapeState struct {
	scratch []float32
}

type structEscapeBox struct {
	value any
}

var structEscapeSink structEscapeBox

func newStructEscapeState() *structEscapeState {
	state := &structEscapeState{}
	state.scratch = make([]float32, 64)
	return state
}

func exposeStruct(state *structEscapeState) {
	structEscapeSink = structEscapeBox{value: state}
}

func (state *structEscapeState) run() {
	write32(state.scratch[:16:16], 1)
	activate32(state.scratch, 2)
	observe32(state.scratch[:16:16], 3)
}

func unreachable() {
	return
	scratch := make([]float32, 64)
	write32(scratch[:16:16], 1)
	activate32(scratch, 2)
	observe32(scratch[:16:16], 3)
}
