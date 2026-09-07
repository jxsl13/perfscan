package ps6106contract

type Buffer struct {
	data []float32
}

type Device struct{}

func (*Device) Alloc(size int) *Buffer                   { return &Buffer{data: make([]float32, size)} }
func (*Device) Produce(buffer *Buffer, active int)       {}
func (*Device) Activate(buffer *Buffer)                  {}
func (*Device) Observe(buffer *Buffer, active int)       {}
func (*Device) BiasGELU(buffer *Buffer, active int) bool { return true }

func configuredLocal(device *Device, capacity, active int) {
	scratch := device.Alloc(capacity)
	device.Produce(scratch, active)
	device.Activate(scratch) // want `device-bounded-gelu: bounded scratch writes and later observers use 16 active elements, but the configured opaque full-capacity extent consumer traverses 1024 elements \(configured workload, 64x ratio\); one static source site; runtime invocation count unknown; 12 configured model repetitions.*BiasGELU.*advisory only.*no automatic fix`
	device.Observe(scratch, active)
}

func runtimeExtent(value int) int { return value }

func configuredImmutableLocals(device *Device) {
	capacity := runtimeExtent(1024)
	active := runtimeExtent(16)
	scratch := device.Alloc(capacity)
	device.Produce(scratch, active)
	device.Activate(scratch) // want `device-bounded-gelu: bounded scratch writes and later observers use 16 active elements.*configured opaque full-capacity extent consumer traverses 1024 elements.*64x ratio`
	device.Observe(scratch, active)
}

func distinctRuntimeExtents(device *Device, capacity, activeProducer, activeObserver int) {
	scratch := device.Alloc(capacity)
	device.Produce(scratch, activeProducer)
	device.Activate(scratch)
	device.Observe(scratch, activeObserver)
}

func distinctProviders(p, q *Device, capacity, active int) {
	scratch := p.Alloc(capacity)
	p.Produce(scratch, active)
	q.Activate(scratch)
	p.Observe(scratch, active)
}

func distinctBuffers(device *Device, capacity, active int) {
	p := device.Alloc(capacity)
	q := device.Alloc(capacity)
	device.Produce(p, active)
	device.Activate(q)
	device.Observe(p, active)
}

type state struct {
	provider *Device
	scratch  *Buffer
}

func newState(provider *Device, capacity int) *state {
	value := &state{provider: provider}
	value.scratch = value.provider.Alloc(capacity)
	return value
}

func (value *state) run(active int) {
	value.provider.Produce(value.scratch, active)
	value.provider.Activate(value.scratch) // want `device-bounded-gelu: bounded scratch writes and later observers use 16 active elements.*configured opaque full-capacity extent consumer traverses 1024 elements.*64x ratio.*12 configured model repetitions`
	value.provider.Observe(value.scratch, active)
}

type splitState struct {
	provider *Device
	scratch  *Buffer
}

func newSplitState(provider *Device, capacity int) *splitState {
	value := &splitState{provider: provider}
	value.scratch = value.provider.Alloc(capacity)
	return value
}

// Field declaration identity cannot equate p and q instances.
func splitScratchInstances(p, q *splitState, active int) {
	p.provider.Produce(p.scratch, active)
	q.provider.Activate(q.scratch)
	p.provider.Observe(p.scratch, active)
}

func splitProviderInstances(p, q *splitState, active int) {
	p.provider.Produce(p.scratch, active)
	q.provider.Activate(p.scratch)
	p.provider.Observe(p.scratch, active)
}

type DeviceAPI interface {
	Produce(*Buffer, int)
	Activate(*Buffer)
	Observe(*Buffer, int)
}

func interfaceDispatch(device DeviceAPI, allocator *Device, capacity, active int) {
	scratch := allocator.Alloc(capacity)
	device.Produce(scratch, active)
	device.Activate(scratch)
	device.Observe(scratch, active)
}

func mutatedExtent(device *Device, capacity, active int) {
	scratch := device.Alloc(capacity)
	device.Produce(scratch, active)
	device.Activate(scratch)
	device.Observe(scratch, active)
	active++
}

var activeAddress *int

func exposedExtent(device *Device, capacity, active int) {
	scratch := device.Alloc(capacity)
	device.Produce(scratch, active)
	device.Activate(scratch)
	device.Observe(scratch, active)
	activeAddress = &active
}

var globalActive = 16

func mutateGlobalExtent() {
	globalActive = 1024
}

func globalExtent(device *Device, capacity int) {
	scratch := device.Alloc(capacity)
	device.Produce(scratch, globalActive)
	device.Activate(scratch)
	device.Observe(scratch, globalActive)
}

func aliasedBuffer(device *Device, capacity, active int) {
	scratch := device.Alloc(capacity)
	alias := scratch
	device.Produce(scratch, active)
	device.Activate(scratch)
	device.Observe(scratch, active)
	_ = alias
}

func (*Device) Tail(buffer *Buffer) float32 { return buffer.data[len(buffer.data)-1] }

func tailObserver(device *Device, capacity, active int) {
	scratch := device.Alloc(capacity)
	device.Produce(scratch, active)
	device.Activate(scratch)
	device.Observe(scratch, active)
	_ = device.Tail(scratch)
}

func wrongOrder(device *Device, capacity, active int) {
	scratch := device.Alloc(capacity)
	device.Activate(scratch)
	device.Produce(scratch, active)
	device.Observe(scratch, active)
}
