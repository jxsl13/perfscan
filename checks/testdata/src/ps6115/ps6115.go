package ps6115

type Operation int

const (
	opLoss  Operation = 7
	opOther Operation = 8
	fakeOp  Operation = 7
)

type Metal struct{}

func (Metal) Forward(Operation, int64, int64, []float32) []float32  { return nil }
func (Metal) Backward(Operation, int64, int64, []float32) []float32 { return nil }
func (Metal) Submit(Operation, int64, int64, []float32)             {}

func hostForward(Operation, int64, int64, []float32) []float32  { return nil }
func hostBackward(Operation, int64, int64, []float32) []float32 { return nil }

func literalGeometry(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits) // want `literalGeometry: configured tiny synchronous accelerator screen is 8x10=80 elements, 640 bytes, with submission count 1 on darwin/arm64 unified memory; screen only the exact same-semantics host alternatives and require paired end-to-end application validation—this is neither a host/application win nor a routing conclusion`
}

func constantGeometry(m Metal, logits []float32) []float32 {
	const rows int64 = 8
	const columns int64 = 10
	return m.Forward(opLoss, rows, columns, logits) // want `constantGeometry: configured tiny synchronous accelerator screen is 8x10=80 elements, 640 bytes, with submission count 1`
}

func stableSnapshots(m Metal, shape []int, logits []float32) []float32 {
	rows := shape[0]
	columns := shape[1]
	return m.Forward(opLoss, int64(rows), int64(columns), logits) // want `stableSnapshots: configured tiny synchronous accelerator screen is 8x10=80 elements, 640 bytes, with submission count 1`
}

func completeForwardBackward(m Metal, logits []float32) []float32 {
	rows := len(logits) / 10
	columns := 10
	forward := m.Forward(opLoss, int64(rows), int64(columns), logits) // want `complete-forward-gradient: configured tiny synchronous accelerator screen is 8x10=80 elements, 640 bytes, with submission count 2`
	return m.Backward(opLoss, int64(rows), int64(columns), forward)
}

type OtherMetal struct{}

func (OtherMetal) Forward(Operation, int64, int64, []float32) []float32 { return nil }

func fakeCallee(m OtherMetal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func wrongOperation(m Metal, logits []float32) []float32 {
	return m.Forward(opOther, 8, 10, logits)
}

func wrongOperationIdentity(m Metal, logits []float32) []float32 {
	return m.Forward(fakeOp, 8, 10, logits)
}

func wrongRoles(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 10, 8, logits)
}

func zeroGeometry(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 0, 10, logits)
}

func negativeGeometry(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, -8, 10, logits)
}

func wideForward(Operation, uint64, uint64, []float32) []float32 { return nil }

func wideConstant(logits []float32) []float32 {
	const rows uint64 = 1 << 63
	return wideForward(opLoss, rows, 10, logits)
}

func rowsValue() int64 { return 8 }

func dynamicGeometry(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, rowsValue(), 10, logits)
}

func mutatedRoot(m Metal, logits []float32) []float32 {
	rows := int64(8)
	rows++
	return m.Forward(opLoss, rows, 10, logits)
}

func addressedRoot(m Metal, logits []float32) []float32 {
	rows := int64(8)
	_ = &rows
	return m.Forward(opLoss, rows, 10, logits)
}

func capturedRoot(m Metal, logits []float32) []float32 {
	rows := int64(8)
	_ = func() int64 { return rows }
	return m.Forward(opLoss, rows, 10, logits)
}

func observe(int64) {}

func opaqueRoot(m Metal, logits []float32) []float32 {
	rows := int64(8)
	observe(rows)
	return m.Forward(opLoss, rows, 10, logits)
}

type Accelerator interface {
	Forward(Operation, int64, int64, []float32) []float32
}

func interfaceCall(m Accelerator, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func functionValue(m Metal, logits []float32) []float32 {
	call := m.Forward
	return call(opLoss, 8, 10, logits)
}

func each(func()) {}

func callbackCall(m Metal, logits []float32) {
	each(func() { _ = m.Forward(opLoss, 8, 10, logits) })
}

func goCall(m Metal, logits []float32) {
	go m.Submit(opLoss, 8, 10, logits)
}

func deferCall(m Metal, logits []float32) {
	defer m.Submit(opLoss, 8, 10, logits)
}

func genericForward[T any](Operation, int64, int64, []float32) []float32 { return nil }

func genericCall(logits []float32) []float32 {
	return genericForward[int](opLoss, 8, 10, logits)
}

func variadicForward(Operation, int64, int64, ...[]float32) []float32 { return nil }

func variadicCall(logits []float32) []float32 {
	return variadicForward(opLoss, 8, 10, logits)
}

func methodExpression(m Metal, logits []float32) []float32 {
	return Metal.Forward(m, opLoss, 8, 10, logits)
}

func wrapperForward(m Metal, op Operation, rows, columns int64, logits []float32) []float32 {
	return m.Forward(op, rows, columns, logits)
}

func wrapperCall(m Metal, logits []float32) []float32 {
	return wrapperForward(m, opLoss, 8, 10, logits)
}

func identity(values []float32) []float32 { return values }

func nestedWrapper(m Metal, logits []float32) []float32 {
	return identity(m.Forward(opLoss, 8, 10, logits))
}

func metalConsumeValue(values []float32) []float32 { return values }

func nestedDeviceWrapper(m Metal, logits []float32) []float32 {
	return metalConsumeValue(m.Forward(opLoss, 8, 10, logits))
}

func duplicateCall(m Metal, logits []float32) []float32 {
	_ = m.Forward(opLoss, 8, 10, logits)
	return m.Forward(opLoss, 8, 10, logits)
}

func toDevice([]float32) {}

func visibleTransfer(m Metal, logits []float32) []float32 {
	out := m.Forward(opLoss, 8, 10, logits)
	toDevice(out)
	return out
}

type DeviceTensor struct{}
type DeviceMetal struct{}

func (DeviceMetal) Forward(Operation, int64, int64, DeviceTensor) DeviceTensor { return DeviceTensor{} }

func deviceResident(m DeviceMetal, logits DeviceTensor) DeviceTensor {
	return m.Forward(opLoss, 8, 10, logits)
}

type DeviceBuffer struct{ values []float32 }

func deviceBufferContext(m Metal, logits DeviceBuffer) []float32 {
	return m.Forward(opLoss, 8, 10, logits.values)
}

type Graph struct{}

func graphContext(graph *Graph, m Metal, logits []float32) []float32 {
	_ = graph
	return m.Forward(opLoss, 8, 10, logits)
}

type Recorder struct{}

func recorderContext(recorder *Recorder, m Metal, logits []float32) []float32 {
	_ = recorder
	return m.Forward(opLoss, 8, 10, logits)
}

type Stream struct{}

func streamContext(stream *Stream, m Metal, logits []float32) []float32 {
	_ = stream
	return m.Forward(opLoss, 8, 10, logits)
}

type CommandBuffer struct{}

func commandBufferContext(command *CommandBuffer, m Metal, logits []float32) []float32 {
	_ = command
	return m.Forward(opLoss, 8, 10, logits)
}

type NarrowMetal struct{}

func (NarrowMetal) Forward(Operation, int32, int32, []float32) []float32 { return nil }

func narrowingConversion(m NarrowMetal, rows, columns int64, logits []float32) []float32 {
	return m.Forward(opLoss, int32(rows), int32(columns), logits)
}

func uninitializedRoot(m Metal, logits []float32) []float32 {
	var rows int64
	rows = 8
	return m.Forward(opLoss, rows, 10, logits)
}

type EmbeddedMetal struct{ Metal }

func promotedMethod(m EmbeddedMetal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func genericSite[T any](m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func unresolvedHost(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func metalConsume([]float32) {}

func laterDeviceConsumer(m Metal, logits []float32) []float32 {
	out := m.Forward(opLoss, 8, 10, logits)
	metalConsume(out)
	return out
}

type outputState struct{ output []float32 }

func selectorLaterDeviceConsumer(m Metal, logits []float32, state *outputState) []float32 {
	state.output = m.Forward(opLoss, 8, 10, logits)
	metalConsume(state.output)
	return state.output
}

func indexLaterDeviceConsumer(m Metal, logits []float32, values [][]float32) []float32 {
	values[0] = m.Forward(opLoss, 8, 10, logits)
	metalConsume(values[0])
	return values[0]
}

func aliasLaterDeviceConsumer(m Metal, logits []float32) []float32 {
	output := m.Forward(opLoss, 8, 10, logits)
	alias := output
	metalConsume(alias)
	return output
}

var escapedOutput []float32

func globalAliasEscape(m Metal, logits []float32) []float32 {
	output := m.Forward(opLoss, 8, 10, logits)
	escapedOutput = output
	return output
}

func consumeEscapedOutput() { metalConsume(escapedOutput) }

func unreachableCall(m Metal, logits []float32) []float32 {
	if false {
		return m.Forward(opLoss, 8, 10, logits)
	}
	return logits
}

type fakeC struct{}

func (fakeC) Forward(Operation, int64, int64, []float32) []float32 { return nil }

var C fakeC

func fakeCShape(logits []float32) []float32 {
	return C.Forward(opLoss, 8, 10, logits)
}

type ShadowMetal struct{}

func (ShadowMetal) Forward(Operation, int64, int64, []float32) []float32 { return nil }

func shadowedMethod(m ShadowMetal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func missingCall(logits []float32) []float32 { return logits }

func existingSelector(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func profiledRetain(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func missingGroupedForward(m Metal, logits []float32) []float32 {
	return m.Forward(opLoss, 8, 10, logits)
}

func missingGroupedBackward(logits []float32) []float32 { return logits }
