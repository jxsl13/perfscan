package ps6089

import (
	"testing"
	"unsafe"

	"metal"
	shapeone "ps6089shapeone"
	shapetwo "ps6089shapetwo"
)

type Buffer struct{}

type ShapeKey int

type MetalCommandBuffer struct{}

func (*MetalCommandBuffer) EncodeRoPE(buffer *Buffer, shape ShapeKey)  {}
func (*MetalCommandBuffer) AppendKV(buffer *Buffer, shape ShapeKey)    {}
func (*MetalCommandBuffer) FusedRoPEKV(buffer *Buffer, shape ShapeKey) {}
func (*MetalCommandBuffer) Commit()                                    {}
func (*MetalCommandBuffer) WaitUntilCompleted()                        {}
func (*MetalCommandBuffer) ReplaceCommand()                            {}

type MetalDevice struct{}

func (*MetalDevice) NewCommandBuffer() *MetalCommandBuffer { return &MetalCommandBuffer{} }

type CallbackMetalCommandBuffer struct{}

func (*CallbackMetalCommandBuffer) EncodeRoPE(buffer *Buffer, shape ShapeKey) {}
func (*CallbackMetalCommandBuffer) AppendKV(buffer *Buffer, shape ShapeKey)   {}
func (*CallbackMetalCommandBuffer) Commit(callback func())                    { callback() }
func (*CallbackMetalCommandBuffer) WaitUntilCompleted()                       {}

type CallbackMetalDevice struct{}

func (*CallbackMetalDevice) NewCommandBuffer() *CallbackMetalCommandBuffer {
	return &CallbackMetalCommandBuffer{}
}

type CheckedMetalDevice struct{}

type GPUCommandError string

func (err GPUCommandError) Error() string { return string(err) }

func (*CheckedMetalDevice) NewCommandBuffer() (*MetalCommandBuffer, GPUCommandError) {
	return &MetalCommandBuffer{}, ""
}

type ErrorFirstMetalDevice struct{}

func (*ErrorFirstMetalDevice) NewCommandBuffer() (error, *MetalCommandBuffer) {
	return nil, &MetalCommandBuffer{}
}

type FlaggedMetalDevice struct{}

func (*FlaggedMetalDevice) NewCommandBuffer() (*MetalCommandBuffer, bool) {
	return &MetalCommandBuffer{}, true
}

type AmbiguousMetalDevice struct{}

func (*AmbiguousMetalDevice) NewCommandBuffer() (*MetalCommandBuffer, *MetalCommandBuffer) {
	return &MetalCommandBuffer{}, &MetalCommandBuffer{}
}

type MetalCommandState struct {
	command *MetalCommandBuffer
}

type MetalCommandStatePointer *MetalCommandState

type MetalCommandBox struct {
	state *MetalCommandState
}

type MetalCommandMutator func()

type MetalCommandSlot **MetalCommandBuffer

type MetalCommandSlotMutator struct {
	slot **MetalCommandBuffer
}

func (mutator *MetalCommandSlotMutator) replace() {
	replaceCommand(mutator.slot)
}

type MetalCommandValueSlotMutator struct {
	slot **MetalCommandBuffer
}

func (mutator MetalCommandValueSlotMutator) replace() {
	replaceCommand(mutator.slot)
}

type MetalCommandScalarObserver struct {
	value int
}

func (observer MetalCommandScalarObserver) observe() { _ = observer.value }

type MetalCommandValueCallback struct {
	mutate func()
}

func (callback MetalCommandValueCallback) run() { callback.mutate() }

type MetalCommandValueStateMutator struct {
	state *MetalCommandState
}

func (mutator MetalCommandValueStateMutator) replace() {
	mutator.state.command = &MetalCommandBuffer{}
}

type MetalCommandSlotHolder struct {
	slot **MetalCommandBuffer
}

type MetalCommandScalarHolder struct {
	value int
}

type MetalCommandCallbackHolder struct {
	mutate func()
	other  **MetalCommandBuffer
}

type MetalCommandGetterHolder struct {
	get func() **MetalCommandBuffer
}

func retainMetalCommandSlotHolder(holder MetalCommandSlotHolder) { _ = holder }
func observeMetalCommandScalarHolder(holder MetalCommandScalarHolder) {
	_ = holder
}
func retainMetalCommandCallbackHolder(holder MetalCommandCallbackHolder) { _ = holder }
func retainMetalCommandGetterHolder(holder MetalCommandGetterHolder)     { _ = holder }
func retainMetalCommandCallbackHolderPointer(holder *MetalCommandCallbackHolder) {
	_ = holder
}
func retainMetalCommandSlotMap(slots map[**MetalCommandBuffer]**MetalCommandBuffer) {
	_ = slots
}
func observeMetalCommandScalarMap(values map[int]int) { _ = values }

type MetalCommandAction func()

type MetalCommandSlotIdentity func(**MetalCommandBuffer) **MetalCommandBuffer

type NamedMetalCommandSlot **MetalCommandBuffer

var storedMetalCommandSlot **MetalCommandBuffer
var storedMetalCommandCallbackHolder MetalCommandCallbackHolder
var namedCallbackCommand *MetalCommandBuffer
var namedGetterCommand *MetalCommandBuffer
var namedGetterCommandAlias = &namedGetterCommand

func invokeMetalCommandMutator(mutator MetalCommandMutator) {
	mutator()
}

func retainMetalCommandSlot(slot **MetalCommandBuffer) {
	_ = slot
}

func retainAnyMetalCommandSlot(slot any)               { _ = slot }
func retainUnsafeMetalCommandSlot(slot unsafe.Pointer) { _ = slot }
func blockingPS6089Int() int                           { select {} }
func returningPS6089Int() int                          { return 1 }
func mutateNamedCallbackCommand()                      { namedCallbackCommand = &MetalCommandBuffer{} }
func readNamedCallbackCommand()                        { _ = namedCallbackCommand }
func namedGetterCommandSlot() **MetalCommandBuffer     { return &namedGetterCommand }
func recursePS6089Forever()                            { recursePS6089Forever() }
func maybeRecursePS6089Forever() {
	if namedGetterCommand != nil {
		recursePS6089Forever()
	}
}

func opaqueMetalCommandSlotMutator(slot **MetalCommandBuffer) *MetalCommandSlotMutator {
	return &MetalCommandSlotMutator{slot: slot}
}

type MetalCommandHolder struct {
	command *MetalCommandBuffer
}

type EmbeddedMetalCommandState struct {
	*MetalCommandHolder
}

type EmbeddedMetalCommandStateAlias = EmbeddedMetalCommandState

type GenericCommandHolder[T any] struct {
	command T
}

type GenericEmbeddedMetalCommandState struct {
	*GenericCommandHolder[*MetalCommandBuffer]
}

type DeepEmbeddedMetalCommandState struct {
	*EmbeddedMetalCommandState
}

var persistentCommandSlot **MetalCommandBuffer

type PromotedMetalCommand struct {
	*MetalCommandBuffer
}

type PromotedMetalCommandAlias = PromotedMetalCommand

type GenericPromotedMetalCommand[T any] struct {
	*MetalCommandBuffer
	marker T
}

type DeepPromotedMetalCommand struct {
	*PromotedMetalCommand
}

type OtherMetalCommandBuffer struct{}

func (*OtherMetalCommandBuffer) EncodeRoPE(buffer *Buffer, shape ShapeKey) {}
func (*OtherMetalCommandBuffer) AppendKV(buffer *Buffer, shape ShapeKey)   {}
func (*OtherMetalCommandBuffer) Commit()                                   {}
func (*OtherMetalCommandBuffer) WaitUntilCompleted()                       {}

type AmbiguousPromotedMetalCommand struct {
	*MetalCommandBuffer
	*OtherMetalCommandBuffer
}

type PromotedMetalDevice struct{}

func (*PromotedMetalDevice) NewCommandBuffer() *PromotedMetalCommandAlias {
	return &PromotedMetalCommand{MetalCommandBuffer: &MetalCommandBuffer{}}
}

type GenericPromotedMetalDevice[T any] struct{}

func (*GenericPromotedMetalDevice[T]) NewCommandBuffer() *GenericPromotedMetalCommand[T] {
	return &GenericPromotedMetalCommand[T]{MetalCommandBuffer: &MetalCommandBuffer{}}
}

type AmbiguousPromotedMetalDevice struct{}

func (*AmbiguousPromotedMetalDevice) NewCommandBuffer() *AmbiguousPromotedMetalCommand {
	return &AmbiguousPromotedMetalCommand{
		MetalCommandBuffer:      &MetalCommandBuffer{},
		OtherMetalCommandBuffer: &OtherMetalCommandBuffer{},
	}
}

type GenericMetalCommand[T any] struct{}

func (*GenericMetalCommand[T]) EncodeRoPE(buffer *Buffer, shape ShapeKey) {}
func (*GenericMetalCommand[T]) AppendKV(buffer *Buffer, shape ShapeKey)   {}
func (*GenericMetalCommand[T]) Commit()                                   {}
func (*GenericMetalCommand[T]) WaitUntilCompleted()                       {}

func (state *MetalCommandState) ReplaceCommand() {
	state.command = &MetalCommandBuffer{}
}

func (state MetalCommandState) Observe() {}

type GenericMetalDevice[T any] struct {
	command T
}

func (device *GenericMetalDevice[T]) NewCommandBuffer() (T, error) {
	return device.command, nil
}

func NewCommandBuffer[T any](command T) (T, error) {
	return command, nil
}

type GPUCommand interface {
	EncodeRoPE(*Buffer, ShapeKey)
	AppendKV(*Buffer, ShapeKey)
	Commit()
	WaitUntilCompleted()
	ReplaceCommand()
}

type InterfaceMetalCommand struct{}

func (*InterfaceMetalCommand) EncodeRoPE(buffer *Buffer, shape ShapeKey) {}
func (*InterfaceMetalCommand) AppendKV(buffer *Buffer, shape ShapeKey)   {}
func (*InterfaceMetalCommand) Commit()                                   {}
func (*InterfaceMetalCommand) WaitUntilCompleted()                       {}
func (*InterfaceMetalCommand) ReplaceCommand()                           {}

type InterfaceMetalDevice struct{}

func (*InterfaceMetalDevice) NewCommandBuffer() (GPUCommand, error) {
	return &InterfaceMetalCommand{}, nil
}

type FusionInputs[T ~int] struct {
	buffer     *Buffer
	other      *Buffer
	shape      T
	otherShape T
}

type GPUCommandAlias = MetalCommandBuffer
type GPUBufferAlias = Buffer
type SharedPackageShapeAlias = shapeone.ShapeKey
type DistinctLocalShapeKey int

type CrossPackageGPUCommand struct{}

func (*CrossPackageGPUCommand) EncodeRoPE(buffer *Buffer, shape shapeone.ShapeKey) {}
func (*CrossPackageGPUCommand) AppendKV(buffer *Buffer, shape shapetwo.ShapeKey)   {}

type AliasedShapeGPUCommand struct{}

func (*AliasedShapeGPUCommand) EncodeRoPE(buffer *Buffer, shape shapeone.ShapeKey)     {}
func (*AliasedShapeGPUCommand) AppendKV(buffer *Buffer, shape SharedPackageShapeAlias) {}

type DistinctShapeGPUCommand struct{}

func (*DistinctShapeGPUCommand) EncodeRoPE(buffer *Buffer, shape ShapeKey)            {}
func (*DistinctShapeGPUCommand) AppendKV(buffer *Buffer, shape DistinctLocalShapeKey) {}

type CPURecorder struct{}

func (*CPURecorder) EncodeRoPE(buffer *Buffer, shape ShapeKey) {}
func (*CPURecorder) AppendKV(buffer *Buffer, shape ShapeKey)   {}

func exactTypedFusionCandidate(command *MetalCommandBuffer, key *Buffer, shape ShapeKey) {
	command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
	command.AppendKV(key, shape)
}

func stableSelectorFusionCandidate(state *MetalCommandState, key *Buffer, shape ShapeKey) {
	state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
	state.command.AppendKV(key, shape)
}

func parenthesizedReceiverFusionCandidate(command *MetalCommandBuffer, key *Buffer, shape ShapeKey) {
	(command).EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
	(command).AppendKV(key, shape)
}

func selectorArgumentFusionCandidate(command *MetalCommandBuffer, inputs *FusionInputs[ShapeKey]) {
	command.EncodeRoPE(inputs.buffer, inputs.shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
	command.AppendKV(inputs.buffer, inputs.shape)

	command.EncodeRoPE(inputs.buffer, inputs.shape)
	command.AppendKV(inputs.other, inputs.shape)

	command.EncodeRoPE(inputs.buffer, inputs.shape)
	command.AppendKV(inputs.buffer, inputs.otherShape)
}

func aliasedTypesFusionCandidate(command *GPUCommandAlias, key *GPUBufferAlias, shape ShapeKey) {
	command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
	command.AppendKV(key, shape)
}

func constantShapeTypeIdentityCandidates(cross *CrossPackageGPUCommand, aliased *AliasedShapeGPUCommand, distinct *DistinctShapeGPUCommand, key *Buffer) {
	cross.EncodeRoPE(key, shapeone.ShapeKey(64))
	cross.AppendKV(key, shapetwo.ShapeKey(64))

	aliased.EncodeRoPE(key, shapeone.ShapeKey(64)) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
	aliased.AppendKV(key, SharedPackageShapeAlias(64))

	distinct.EncodeRoPE(key, ShapeKey(64))
	distinct.AppendKV(key, DistinctLocalShapeKey(64))
}

func incompatibleCandidates(first, second *MetalCommandBuffer, key, other *Buffer, shape, otherShape ShapeKey, cpu *CPURecorder) {
	first.EncodeRoPE(key, shape)
	second.AppendKV(key, shape)

	first.EncodeRoPE(key, shape)
	first.AppendKV(other, shape)

	first.EncodeRoPE(key, shape)
	first.AppendKV(key, otherShape)

	first.FusedRoPEKV(key, shape)
	first.AppendKV(key, shape)

	cpu.EncodeRoPE(key, shape)
	cpu.AppendKV(key, shape)

	first.EncodeRoPE(key, shape)
	consume(key)
	first.AppendKV(key, shape)
}

func consume(*Buffer) {}

func replaceCommand(command **MetalCommandBuffer) {
	*command = &MetalCommandBuffer{}
}

func replaceState(state *MetalCommandState) {
	*state = MetalCommandState{command: &MetalCommandBuffer{}}
}

func replaceStatePointer(state **MetalCommandState) {
	*state = &MetalCommandState{command: &MetalCommandBuffer{}}
}

func replaceStateAndZero(state **MetalCommandState) int {
	*state = &MetalCommandState{command: &MetalCommandBuffer{}}
	return 0
}

func replaceEmbeddedHolder(state *EmbeddedMetalCommandState) {
	state.MetalCommandHolder = &MetalCommandHolder{}
}

func replaceEmbeddedHolderAfterAlias(_ **MetalCommandBuffer, state *EmbeddedMetalCommandState) {
	state.MetalCommandHolder = &MetalCommandHolder{}
}

func exposeStates(states ...*MetalCommandState) {}

func exposeAny(value any) {}

func exposeGeneric[T any](value T) {}

func stateIdentity(state *MetalCommandState) *MetalCommandState { return state }

func readState(state MetalCommandState) {}

func makeError() error { return nil }

func BenchmarkGPUFusionLeafLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for index := 0; index < b.N; index++ { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionProductionLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	command := device.NewCommandBuffer()
	for index := 0; index < b.N; index++ {
		command.FusedRoPEKV(key, shape)
	}
	command.Commit()
	command.WaitUntilCompleted()
}

// Lifecycle calls on different command objects must not be joined into one warning.
func BenchmarkGPUFusionSplitLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for index := 0; index < b.N; index++ {
		first := device.NewCommandBuffer()
		second := device.NewCommandBuffer()
		first.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		first.AppendKV(key, shape)
		second.Commit()
		second.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionRangeLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for range b.N { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// The exact lifecycle ordering is required; mere calls in one loop stay silent.
func BenchmarkGPUFusionOutOfOrderLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for index := 0; index < b.N; index++ {
		command := device.NewCommandBuffer()
		command.Commit()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionSubLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	b.Run("leaf", func(b *testing.B) {
		for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}
	})
}

func BenchmarkGPUFusionSelectorLifecycle(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Rebinding the identifier means the factory-created command does not enclose
// the candidate lifecycle, even though the later recorder calls are adjacent.
func BenchmarkGPUFusionReboundIdentifier(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command = &MetalCommandBuffer{}
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Rebinding a selector's root invalidates the complete receiver path.
func BenchmarkGPUFusionReboundSelectorRoot(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state = &MetalCommandState{command: &MetalCommandBuffer{}}
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A conditional write can replace the command between creation and submission.
func BenchmarkGPUFusionConditionalRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	reset := b.N > 0
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		if reset {
			command = &MetalCommandBuffer{}
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A switch-nested selector write can replace the command before wait.
func BenchmarkGPUFusionSwitchRebindBeforeWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		switch b.N {
		case 1:
			state.command = &MetalCommandBuffer{}
		}
		state.command.WaitUntilCompleted()
	}
}

// Writes in an inner block are part of the enclosing lifecycle statement.
func BenchmarkGPUFusionInnerBlockRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		{
			command = &MetalCommandBuffer{}
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A select clause is another nested control-flow location for a rebind.
func BenchmarkGPUFusionSelectRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		select {
		default:
			command = &MetalCommandBuffer{}
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A direct function literal runs synchronously and can rebind the receiver.
func BenchmarkGPUFusionImmediateLiteralRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		(func() {
			command = &MetalCommandBuffer{}
		})()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A deferred literal cannot rebind the receiver before commit and wait.
func BenchmarkGPUFusionDeferredLiteralRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		defer func() {
			command = &MetalCommandBuffer{}
		}()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A goroutine can rebind the receiver before commit and wait execute.
func BenchmarkGPUFusionGoroutineLiteralRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		go func() {
			command = &MetalCommandBuffer{}
		}()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A defer owned by a synchronous IIFE runs when that IIFE returns, before wait.
func BenchmarkGPUFusionImmediateLiteralDeferredRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		func() {
			defer func() {
				command = &MetalCommandBuffer{}
			}()
		}()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A defer owned by a goroutine can also run before the outer wait.
func BenchmarkGPUFusionGoroutineDeferredRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		go func() {
			defer func() {
				command = &MetalCommandBuffer{}
			}()
		}()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A deferred helper owned by an IIFE executes before the outer lifecycle.
func BenchmarkGPUFusionImmediateLiteralDeferredHelper(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		func() {
			defer replaceState(state)
		}()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A deferred helper receives the root only after the current lifecycle ends.
func BenchmarkGPUFusionDeferredHelperExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		defer replaceState(state)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Taking an address solely for a deferred helper also happens after wait.
func BenchmarkGPUFusionDeferredAddressExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		defer replaceCommand(&state.command)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Nested calls in defer arguments are evaluated immediately and can expose state.
func BenchmarkGPUFusionDeferredEagerArgumentExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		defer exposeAny(stateIdentity(state))
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A goroutine helper can mutate the root before commit and wait.
func BenchmarkGPUFusionGoroutineHelperExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		go replaceState(state)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A deferred pointer-receiver method also runs after the lifecycle.
func BenchmarkGPUFusionDeferredMethodExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		defer state.ReplaceCommand()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A deferred method expression has the same post-wait ordering as a method value.
func BenchmarkGPUFusionDeferredMethodExpressionExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		defer (*MetalCommandState).ReplaceCommand(state)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A range assignment is a write even without an AssignStmt node.
func BenchmarkGPUFusionRangeAssignmentRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		for _, command = range []*MetalCommandBuffer{{}} {
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A write to another receiver must not invalidate the complete lifecycle.
func BenchmarkGPUFusionConditionalOtherReceiver(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		other := &MetalCommandBuffer{}
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		if b.N > 0 {
			other = command
		}
		_ = other
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A nested write after wait does not erase the completed lifecycle.
func BenchmarkGPUFusionConditionalRebindAfterWait(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
		if b.N > 0 {
			command = &MetalCommandBuffer{}
		}
	}
}

// An explicit dereference write rebinds the selector's root object.
func BenchmarkGPUFusionDereferencedRootRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		*state = MetalCommandState{command: &MetalCommandBuffer{}}
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A dereference write after wait does not erase the completed sequence.
func BenchmarkGPUFusionDereferencedRootAfterWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
		*state = MetalCommandState{}
	}
}

func BenchmarkGPUFusionDereferencedRootAfterCandidate(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*state = MetalCommandState{command: &MetalCommandBuffer{}}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionDereferencedRootBeforeWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		*state = MetalCommandState{command: &MetalCommandBuffer{}}
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionNestedDereferencedRoot(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		(*slot).command = device.NewCommandBuffer()
		**slot = MetalCommandState{command: &MetalCommandBuffer{}}
		(*slot).command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*slot).command.AppendKV(key, shape)
		(*slot).command.Commit()
		(*slot).command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionReadOnlyDereference(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		(*state).command = device.NewCommandBuffer()
		_ = *state
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*state).command.AppendKV(key, shape)
		state.command.Commit()
		(*state).command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPointerRootHelperExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		replaceState(state)
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionVariadicPointerRootExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		exposeStates(state)
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionInterfacePointerRootExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		exposeAny(any(state))
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericPointerRootExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		exposeGeneric(state)
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionNestedPointerRootExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		exposeGeneric(stateIdentity(state))
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPointerRootExposureAfterWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
		replaceState(state)
	}
}

func BenchmarkGPUFusionValueRootReadOnlyCall(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		readState(*state)
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPurePointerConversion(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		_ = any(state)
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPromotedHolderRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.MetalCommandHolder = &MetalCommandHolder{command: &MetalCommandBuffer{}}
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPromotedDirectIdentity(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandStateAlias{MetalCommandHolder: &MetalCommandHolder{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.MetalCommandHolder.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.MetalCommandHolder.command.AppendKV(key, shape)
		state.command.Commit()
		state.MetalCommandHolder.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericPromotedIdentity(b *testing.B) {
	device := &MetalDevice{}
	state := &GenericEmbeddedMetalCommandState{GenericCommandHolder: &GenericCommandHolder[*MetalCommandBuffer]{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.GenericCommandHolder.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericPromotedRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &GenericEmbeddedMetalCommandState{GenericCommandHolder: &GenericCommandHolder[*MetalCommandBuffer]{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.GenericCommandHolder = &GenericCommandHolder[*MetalCommandBuffer]{command: &MetalCommandBuffer{}}
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionDeepPromotedHolderRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &DeepEmbeddedMetalCommandState{EmbeddedMetalCommandState: &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.EmbeddedMetalCommandState = &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{command: &MetalCommandBuffer{}}}
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A factory-created wrapper with one complete promoted command path retains
// that embedded receiver identity throughout the lifecycle.
func BenchmarkGPUFusionPromotedFactoryLifecycle(b *testing.B) {
	device := &PromotedMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericPromotedFactoryLifecycle(b *testing.B) {
	device := &GenericPromotedMetalDevice[int]{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Replacing the uniquely promoted command after wrapper creation breaks the
// lifecycle even though every later method is promoted from that field.
func BenchmarkGPUFusionPromotedFactoryRebind(b *testing.B) {
	device := &PromotedMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.MetalCommandBuffer = &MetalCommandBuffer{}
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A factory result with two command embeddings does not identify which child
// the factory created. Explicit lifecycle calls on one child stay conservative.
func BenchmarkGPUFusionAmbiguousPromotedFactory(b *testing.B) {
	device := &AmbiguousPromotedMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.MetalCommandBuffer.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.MetalCommandBuffer.AppendKV(key, shape)
		command.MetalCommandBuffer.Commit()
		command.MetalCommandBuffer.WaitUntilCompleted()
	}
}

// Promoted method selections and direct embedded-field selections identify
// the same command object throughout a complete lifecycle.
func BenchmarkGPUFusionPromotedMethodIdentity(b *testing.B) {
	device := &MetalDevice{}
	command := &PromotedMetalCommand{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command.MetalCommandBuffer = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.MetalCommandBuffer.AppendKV(key, shape)
		command.Commit()
		command.MetalCommandBuffer.WaitUntilCompleted()
	}
}

// Replacing the embedded command breaks the factory-to-method lifecycle even
// though all later operations use the promoted method spelling.
func BenchmarkGPUFusionPromotedMethodRebind(b *testing.B) {
	device := &MetalDevice{}
	command := &PromotedMetalCommand{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command.MetalCommandBuffer = device.NewCommandBuffer()
		command.MetalCommandBuffer = &MetalCommandBuffer{}
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// An unrecognized promoted pointer-receiver method may mutate the embedded
// command, so it conservatively breaks the lifecycle identity.
func BenchmarkGPUFusionPromotedMethodExposure(b *testing.B) {
	device := &MetalDevice{}
	command := &PromotedMetalCommand{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command.MetalCommandBuffer = device.NewCommandBuffer()
		command.ReplaceCommand()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionMethodExpressionLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		((*GPUCommandAlias).EncodeRoPE)(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*MetalCommandBuffer).AppendKV(command, key, shape)
		(*MetalCommandBuffer).Commit(command)
		(*GPUCommandAlias).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionMethodExpressionWrongReceiver(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		other := &MetalCommandBuffer{}
		(*MetalCommandBuffer).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*MetalCommandBuffer).AppendKV(command, key, shape)
		(*MetalCommandBuffer).Commit(other)
		(*MetalCommandBuffer).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionMethodExpressionRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command = &MetalCommandBuffer{}
		(*MetalCommandBuffer).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*MetalCommandBuffer).AppendKV(command, key, shape)
		(*MetalCommandBuffer).Commit(command)
		(*MetalCommandBuffer).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionMethodExpressionExposure(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		(*MetalCommandBuffer).ReplaceCommand(command)
		(*MetalCommandBuffer).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*MetalCommandBuffer).AppendKV(command, key, shape)
		(*MetalCommandBuffer).Commit(command)
		(*MetalCommandBuffer).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionGenericMethodExpressionLifecycle(b *testing.B) {
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := NewCommandBuffer(&GenericMetalCommand[int]{})
		(*GenericMetalCommand[int]).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*GenericMetalCommand[int]).AppendKV(command, key, shape)
		(*GenericMetalCommand[int]).Commit(command)
		(*GenericMetalCommand[int]).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionPromotedMethodExpressionLifecycle(b *testing.B) {
	device := &MetalDevice{}
	command := &PromotedMetalCommandAlias{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command.MetalCommandBuffer = device.NewCommandBuffer()
		(*PromotedMetalCommandAlias).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.MetalCommandBuffer.AppendKV(key, shape)
		(*PromotedMetalCommand).Commit(command)
		command.MetalCommandBuffer.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPromotedMethodExpressionRebind(b *testing.B) {
	device := &MetalDevice{}
	command := &PromotedMetalCommand{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command.MetalCommandBuffer = device.NewCommandBuffer()
		command.MetalCommandBuffer = &MetalCommandBuffer{}
		(*PromotedMetalCommand).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*PromotedMetalCommand).AppendKV(command, key, shape)
		(*PromotedMetalCommand).Commit(command)
		(*PromotedMetalCommand).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionPromotedMethodExpressionExposure(b *testing.B) {
	device := &MetalDevice{}
	command := &PromotedMetalCommand{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command.MetalCommandBuffer = device.NewCommandBuffer()
		(*PromotedMetalCommand).ReplaceCommand(command)
		(*PromotedMetalCommand).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*PromotedMetalCommand).AppendKV(command, key, shape)
		(*PromotedMetalCommand).Commit(command)
		(*PromotedMetalCommand).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionGenericPromotedMethodExpressionLifecycle(b *testing.B) {
	device := &MetalDevice{}
	command := &GenericPromotedMetalCommand[int]{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command.MetalCommandBuffer = device.NewCommandBuffer()
		(*GenericPromotedMetalCommand[int]).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*GenericPromotedMetalCommand[int]).AppendKV(command, key, shape)
		(*GenericPromotedMetalCommand[int]).Commit(command)
		(*GenericPromotedMetalCommand[int]).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionDeepPromotedMethodExpressionLifecycle(b *testing.B) {
	device := &MetalDevice{}
	command := &DeepPromotedMetalCommand{PromotedMetalCommand: &PromotedMetalCommand{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command.MetalCommandBuffer = device.NewCommandBuffer()
		(*DeepPromotedMetalCommand).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*DeepPromotedMetalCommand).AppendKV(command, key, shape)
		(*DeepPromotedMetalCommand).Commit(command)
		(*DeepPromotedMetalCommand).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionDeepPromotedMethodExpressionRebind(b *testing.B) {
	device := &MetalDevice{}
	command := &DeepPromotedMetalCommand{PromotedMetalCommand: &PromotedMetalCommand{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command.MetalCommandBuffer = device.NewCommandBuffer()
		command.PromotedMetalCommand = &PromotedMetalCommand{MetalCommandBuffer: &MetalCommandBuffer{}}
		(*DeepPromotedMetalCommand).EncodeRoPE(command, key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*DeepPromotedMetalCommand).AppendKV(command, key, shape)
		(*DeepPromotedMetalCommand).Commit(command)
		(*DeepPromotedMetalCommand).WaitUntilCompleted(command)
	}
}

func BenchmarkGPUFusionPointerMethodExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.ReplaceCommand()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPointerMethodValueExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		replace := state.ReplaceCommand
		replace()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionValueMethodRead(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.Observe()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericMethodFactory(b *testing.B) {
	device := &GenericMetalDevice[*MetalCommandBuffer]{command: &MetalCommandBuffer{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericFunctionFactory(b *testing.B) {
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := NewCommandBuffer(&MetalCommandBuffer{})
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGenericMethodExpressionFactory(b *testing.B) {
	device := &GenericMetalDevice[*MetalCommandBuffer]{command: &MetalCommandBuffer{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := (*GenericMetalDevice[*MetalCommandBuffer]).NewCommandBuffer(device)
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionInterfaceLifecycle(b *testing.B) {
	device := &InterfaceMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionInterfaceMethodExposure(b *testing.B) {
	device := &InterfaceMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command, _ := device.NewCommandBuffer()
		command.ReplaceCommand()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func genericWrongFactoryInstantiation() {
	value, _ := NewCommandBuffer(1)
	_ = value
}

// Rebinding after the candidate also breaks the create/commit identity chain.
func BenchmarkGPUFusionReboundAfterCandidate(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command = &MetalCommandBuffer{}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAddressRebound(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		replaceCommand(&command)
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Rebinding the selector between commit and wait also invalidates the chain.
func BenchmarkGPUFusionReboundBeforeWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command = &MetalCommandBuffer{}
		state.command.WaitUntilCompleted()
	}
}

// Merely mentioning b.N in a larger condition is not an exact benchmark loop.
func BenchmarkGPUFusionFalseCondition(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for index := 0; index < b.N && false; index++ {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionParenthesizedSubLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	b.Run("leaf", (func(b *testing.B) {
		for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}
	}))
}

func BenchmarkGPUFusionCheckedFactory(b *testing.B) {
	device := &CheckedMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, err := device.NewCommandBuffer()
		_ = err
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// The command result is correlated by signature position, not assumed first.
func BenchmarkGPUFusionErrorFirstFactory(b *testing.B) {
	device := &ErrorFirstMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		err, command := device.NewCommandBuffer()
		_ = err
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionBlankErrorFactory(b *testing.B) {
	device := &CheckedMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A non-error companion result is not the supported checked-factory contract.
func BenchmarkGPUFusionFlaggedFactory(b *testing.B) {
	device := &FlaggedMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command, ok := device.NewCommandBuffer()
		_ = ok
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Two command-typed results do not identify a unique lifecycle receiver.
func BenchmarkGPUFusionAmbiguousFactory(b *testing.B) {
	device := &AmbiguousMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command, other := device.NewCommandBuffer()
		_ = other
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Multiple RHS expressions are not one typed factory result tuple.
func BenchmarkGPUFusionMultipleRHS(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command, err := device.NewCommandBuffer(), makeError()
		_ = err
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A later incomplete reuse must not erase an earlier complete lifecycle.
func BenchmarkGPUFusionValidThenIncomplete(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
		command = device.NewCommandBuffer()
	}
}

// An incomplete first reuse must not erase the later complete lifecycle.
func BenchmarkGPUFusionIncompleteThenValid(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionTwoValidSequences(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionMultipleEvents(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.Commit()
		command.WaitUntilCompleted()
		command.WaitUntilCompleted()
	}
}

// An unconditional continue prevents the lexically later submission from
// completing the candidate lifecycle in the same iteration.
func BenchmarkGPUFusionContinueSkipsSubmission(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		continue
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A conditional loop exit is enough to make the lexical lifecycle sequence
// path-dependent instead of a per-iteration command lifecycle.
func BenchmarkGPUFusionConditionalContinueSkipsSubmission(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		if b.N > 0 {
			continue
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionReturnSkipsSubmission(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		return
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGotoSkipsCommit(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		goto waited
		command.Commit()
	waited:
		command.WaitUntilCompleted()
	}
}

// A goto may bypass an unreachable terminator while still forcing the full
// submission path; CFG ordering must retain this positive lifecycle.
func BenchmarkGPUFusionGotoReachesSubmission(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		goto submit
		command.Commit()
		continue
	submit:
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A syntactically intervening write that is bypassed by an unconditional goto
// cannot replace the factory-created command.
func BenchmarkGPUFusionGotoSkipsRebind(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		goto candidate
		command = &MetalCommandBuffer{}
	candidate:
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A potentially nonterminating inner cycle means commit and wait do not occur
// on every candidate path even though they are lexically later.
func BenchmarkGPUFusionInnerCycleSkipsSubmission(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		for b.N > 0 {
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// A labeled break that merely completes a nested loop still reaches commit
// and wait on every candidate path.
func BenchmarkGPUFusionLabeledBreakReachesSubmission(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
	inner:
		for {
			break inner
		}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Passing a stable pointer alias to an ordinary helper can replace the
// original selector root before submission.
func BenchmarkGPUFusionPointerAliasExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		alias := state
		replaceState(alias)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Stable aliases created outside the measured loop retain the same pointer
// identity, including through a second alias.
func BenchmarkGPUFusionOuterPointerAliasExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	alias := state
	second := alias
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceState(second)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// An alias of a receiver field address remains capable of replacing that
// field after the alias-creation statement has completed.
func BenchmarkGPUFusionAddressDerivedFieldAliasExposure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A pointer escape after wait may remain live across the next benchmark-loop
// backedge, so it conservatively dilutes the lifecycle.
func BenchmarkGPUFusionAddressDerivedFieldAliasAfterWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	second := slot
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
		replaceCommand(second)
	}
}

// Rebinding the pointer root after taking a field address leaves the alias
// attached to the old object, so it cannot replace the current lifecycle.
func BenchmarkGPUFusionAddressDerivedReboundRoot(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	state = &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Defined pointer roots have the same snapshot semantics as unnamed pointers:
// rebinding the root leaves an earlier field address attached to the old state.
func BenchmarkGPUFusionAddressDerivedReboundDefinedPointerRoot(b *testing.B) {
	device := &MetalDevice{}
	state := MetalCommandStatePointer(&MetalCommandState{})
	slot := &(*state).command
	state = MetalCommandStatePointer(&MetalCommandState{})
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		(*state).command = device.NewCommandBuffer()
		(*state).command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*state).command.AppendKV(key, shape)
		replaceCommand(slot)
		(*state).command.Commit()
		(*state).command.WaitUntilCompleted()
	}
}

// Rebinding a promoted pointer prefix likewise detaches an earlier field
// address from the receiver path used by the measured lifecycle.
func BenchmarkGPUFusionAddressDerivedReboundPromotedPrefix(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	slot := &state.command
	state.MetalCommandHolder = &MetalCommandHolder{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Package storage remains an opaque persistent escape even when the locally
// visible alias becomes stale after a loop backedge.
func BenchmarkGPUFusionAddressAliasStaleAcrossBackedge(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state = &MetalCommandState{}
		if persistentCommandSlot == nil {
			persistentCommandSlot = &state.command
		}
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(persistentCommandSlot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// The same package-storage escape remains conservative through nested loops.
func BenchmarkGPUFusionAddressAliasStaleAcrossOuterBackedge(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state = &MetalCommandState{}
		for {
			if persistentCommandSlot == nil {
				persistentCommandSlot = &state.command
			}
			state.command = device.NewCommandBuffer()
			state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
			state.command.AppendKV(key, shape)
			replaceCommand(persistentCommandSlot)
			state.command.Commit()
			state.command.WaitUntilCompleted()
			break
		}
	}
}

// A local address alias of the factory result can replace the root command
// variable before commit and wait.
func BenchmarkGPUFusionAddressDerivedRootAliasExposure(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		slot := &command
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		replaceCommand(slot)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Range targets are assigned before each body entry, so a body-local address
// alias snapshots the current receiver and can replace it before submission.
func BenchmarkGPUFusionAddressAliasAfterRangeEntry(b *testing.B) {
	device := &MetalDevice{}
	states := []*MetalCommandState{{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		for _, state := range states {
			slot := &state.command
			state.command = device.NewCommandBuffer()
			state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
			state.command.AppendKV(key, shape)
			replaceCommand(slot)
			state.command.Commit()
			state.command.WaitUntilCompleted()
		}
	}
}

// Rebinding a receiver root through an earlier address alias detaches a field
// address captured before that write. Submission through the stale field must
// not be conflated with the newly created command.
func BenchmarkGPUFusionDependentAddressAliasRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	stateSlot := &state
	oldSlot := &state.command
	*stateSlot = &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// An ordinary helper receives the same address alias and may rebind the root
// storage before the lifecycle. The dependent field address is stale too.
func BenchmarkGPUFusionDependentAddressAliasHelperRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	stateSlot := &state
	oldSlot := &state.command
	replaceStatePointer(stateSlot)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// Multi-assignment snapshots the alias RHS before rebinding the root LHS, so
// the saved field address still belongs to the old state.
func BenchmarkGPUFusionAddressAliasMultiAssignmentSnapshot(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	var oldSlot **MetalCommandBuffer
	state, oldSlot = &MetalCommandState{}, &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(oldSlot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// RHS evaluation is left-to-right: the helper rebinds state before the second
// RHS takes its field address, so slot names the current storage.
func BenchmarkGPUFusionAddressAliasAfterEarlierRHSRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	var slot **MetalCommandBuffer
	_, slot = replaceStateAndZero(&state), &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Range assignment rebinds the root before the body; a field address captured
// from the old root must not match the range-selected state.
func BenchmarkGPUFusionAddressAliasBeforeRangeRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	oldSlot := &state.command
	states := []*MetalCommandState{{}}
	for _, state = range states {
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// Replacing an aggregate root can replace an indirect prefix even though the
// root itself is a value type.
func BenchmarkGPUFusionAddressAliasAggregatePrefixRebind(b *testing.B) {
	device := &MetalDevice{}
	box := MetalCommandBox{state: &MetalCommandState{}}
	oldSlot := &box.state.command
	box = MetalCommandBox{state: &MetalCommandState{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		box.state.command = device.NewCommandBuffer()
		box.state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		box.state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// Overwriting a pointed-to aggregate can replace a later pointer-valued
// prefix even though the aggregate pointer variable itself is unchanged.
func BenchmarkGPUFusionAddressAliasAggregatePointeeRebind(b *testing.B) {
	device := &MetalDevice{}
	box := &MetalCommandBox{state: &MetalCommandState{}}
	oldSlot := &box.state.command
	*box = MetalCommandBox{state: &MetalCommandState{}}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		box.state.command = device.NewCommandBuffer()
		box.state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		box.state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// A direct selector write through a stable pointer copy replaces the same
// promoted pointer prefix and detaches the earlier field address.
func BenchmarkGPUFusionAddressAliasPointerCopyPrefixRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	oldSlot := &state.command
	alias := state
	alias.MetalCommandHolder = &MetalCommandHolder{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// A selector write through an address alias contains an implicit dereference;
// it replaces the same promoted pointer prefix.
func BenchmarkGPUFusionAddressAliasDescendantPrefixRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	oldSlot := &state.command
	stateSlot := &state
	(*stateSlot).MetalCommandHolder = &MetalCommandHolder{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// An opaque helper can perform the same promoted-prefix replacement through
// the state pointee.
func BenchmarkGPUFusionAddressAliasOpaquePrefixRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	oldSlot := &state.command
	replaceEmbeddedHolder(state)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// Call arguments run before the opaque helper body. The IIFE captures the old
// field address, then the helper replaces its promoted pointer prefix.
func BenchmarkGPUFusionAddressAliasCreatedInEarlierCallArgument(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	var oldSlot **MetalCommandBuffer
	replaceEmbeddedHolderAfterAlias(func() **MetalCommandBuffer {
		oldSlot = &state.command
		return oldSlot
	}(), state)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// Sending the field-address alias exposes it to a synchronized mutator before
// commit and wait.
func BenchmarkGPUFusionAddressAliasChannelEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := make(chan **MetalCommandBuffer)
	done := make(chan struct{})
	go func() {
		for pointer := range mutate {
			replaceCommand(pointer)
			done <- struct{}{}
		}
	}()
	defer close(mutate)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate <- slot
		<-done
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Interface conversion around the sent alias does not hide the escaped
// command-field address.
func BenchmarkGPUFusionAddressAliasConvertedChannelEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := make(chan any)
	done := make(chan struct{})
	go func() {
		for value := range mutate {
			replaceCommand(value.(**MetalCommandBuffer))
			done <- struct{}{}
		}
	}()
	defer close(mutate)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate <- any(slot)
		<-done
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Storing the address alias in aggregate storage conservatively exposes the
// command field to later indirect mutation.
func BenchmarkGPUFusionAddressAliasAggregateEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	var holder struct {
		slot **MetalCommandBuffer
	}
	holder.slot = &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*holder.slot = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Copying a known address alias into aggregate storage is the same escape as
// storing the address expression directly.
func BenchmarkGPUFusionAddressAliasTransitiveAggregateEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	var holder struct {
		slot **MetalCommandBuffer
	}
	holder.slot = slot
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*holder.slot = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A stored local closure only becomes an escape when it is actually invoked.
func BenchmarkGPUFusionAddressAliasInvokedClosure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := func() { *slot = &MetalCommandBuffer{} }
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// The closure effect walker must retain opaque helper calls in the invoked
// body, not only direct assignment statements.
func BenchmarkGPUFusionAddressAliasInvokedHelperClosure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := func() { replaceCommand(slot) }
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Pure named function conversions preserve the invoked literal's effects.
func BenchmarkGPUFusionAddressAliasConvertedClosure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := MetalCommandMutator(func() { replaceCommand(slot) })
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A captured method value carries the address alias in its receiver.
func BenchmarkGPUFusionAddressAliasInvokedMethodValue(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := (&MetalCommandSlotMutator{slot: slot}).replace
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Merely creating the closure cannot mutate the command field.
func BenchmarkGPUFusionAddressAliasUncalledClosure(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := func() { *slot = &MetalCommandBuffer{} }
	_ = mutate
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A captured method value that is never called cannot mutate the command.
func BenchmarkGPUFusionAddressAliasUncalledMethodValue(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := (&MetalCommandSlotMutator{slot: slot}).replace
	_ = mutate
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Closure-local pointer copies, typed conversions, and aggregate storage all
// preserve the captured command-field address when the closure is invoked.
func BenchmarkGPUFusionAddressAliasTransitiveClosureStorage(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := func() {
		local := MetalCommandSlot(slot)
		holder := struct {
			slot **MetalCommandBuffer
		}{slot: (**MetalCommandBuffer)(local)}
		replaceCommand(holder.slot)
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// ValueSpec and later aggregate-field assignment use the same transitive
// closure-local provenance as short declarations.
func BenchmarkGPUFusionAddressAliasClosureValueSpecStorage(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := func() {
		var local MetalCommandSlot = MetalCommandSlot(slot)
		var holder struct{ slot **MetalCommandBuffer }
		holder.slot = (**MetalCommandBuffer)(local)
		replaceCommand(holder.slot)
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Read-only closure-local copies do not expose or mutate the captured slot.
func BenchmarkGPUFusionAddressAliasReadOnlyClosureStorage(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	observe := func() {
		local := MetalCommandSlot(slot)
		holder := struct {
			slot **MetalCommandBuffer
		}{slot: (**MetalCommandBuffer)(local)}
		_ = holder.slot
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		observe()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A direct invocation through a named function conversion retains the
// literal's captured mutation effect.
func BenchmarkGPUFusionAddressAliasDirectConvertedLiteral(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		MetalCommandMutator(func() { replaceCommand(slot) })()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Passing a known callable to an opaque invoker is a possible invocation, and
// callable aliases preserve the same summary.
func BenchmarkGPUFusionAddressAliasOpaqueCallableAlias(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := MetalCommandMutator(func() { replaceCommand(slot) })
	alias := mutate
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		invokeMetalCommandMutator(alias)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Multiple callable definitions are deliberately fail-closed when any
// possible definition mutates the captured slot.
func BenchmarkGPUFusionAddressAliasMultipleCallableDefinitions(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	var mutate MetalCommandMutator
	if b.N > 0 {
		mutate = func() { replaceCommand(slot) }
	} else {
		mutate = func() {}
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A read-only callable alias has a complete empty effect summary.
func BenchmarkGPUFusionAddressAliasReadOnlyCallableAlias(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	observe := MetalCommandMutator(func() { _ = slot })
	alias := observe
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		alias()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A method value stored from a separately constructed aggregate still carries
// the interior command-field address in that aggregate.
func BenchmarkGPUFusionAddressAliasAggregateMethodValue(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	holder := &MetalCommandSlotMutator{slot: slot}
	mutate := holder.replace
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Interior aggregate selection before method-value creation has the same
// captured provenance.
func BenchmarkGPUFusionAddressAliasInteriorMethodValue(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	box := struct{ mutator MetalCommandSlotMutator }{
		mutator: MetalCommandSlotMutator{slot: slot},
	}
	mutate := (&box.mutator).replace
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// An opaque aggregate-producing helper hides the method value's interior
// pointer provenance, so the lifecycle proof must fail closed.
func BenchmarkGPUFusionAddressAliasOpaqueAggregateMethodValue(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	holder := opaqueMetalCommandSlotMutator(slot)
	mutate := holder.replace
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A copied value receiver still carries pointer fields that may expose mutable
// pointee state, so a stored method value remains conservative.
func BenchmarkGPUFusionAddressAliasReadOnlyMethodValue(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	observe := state.Observe
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		observe()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// An unresolved channel escape before the lifecycle remains live until wait.
func BenchmarkGPUFusionAddressAliasPersistentChannelEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	escaped := make(chan **MetalCommandBuffer, 1)
	escaped <- slot
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// An unresolved opaque retention before creation is likewise persistent.
func BenchmarkGPUFusionAddressAliasPersistentOpaqueEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	retainMetalCommandSlot(slot)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A channel escape after wait remains live across the next benchmark-loop
// backedge and may mutate the following iteration.
func BenchmarkGPUFusionAddressAliasPersistentEscapeAfterWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	escaped := make(chan **MetalCommandBuffer, 1)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
		escaped <- slot
	}
}

// CFG reachability, rather than token order, connects a pointer-copy write
// reached through a backward goto after the alias definition.
func BenchmarkGPUFusionAddressAliasGotoReversedWrite(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	var alias *EmbeddedMetalCommandState
	var oldSlot **MetalCommandBuffer
	key := &Buffer{}
	shape := ShapeKey(64)
	goto define
mutate:
	alias.MetalCommandHolder = &MetalCommandHolder{}
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
	return
define:
	alias = state
	oldSlot = &state.command
	goto mutate
}

// The lexically earlier pointer-copy write is unreachable after definition,
// so the alias continues to name the current field storage.
func BenchmarkGPUFusionAddressAliasGotoSkipsEarlierWrite(b *testing.B) {
	device := &MetalDevice{}
	state := &EmbeddedMetalCommandState{MetalCommandHolder: &MetalCommandHolder{}}
	var alias *EmbeddedMetalCommandState
	var slot **MetalCommandBuffer
	key := &Buffer{}
	shape := ShapeKey(64)
	if false {
		goto mutate
	}
	goto define
mutate:
	alias.MetalCommandHolder = &MetalCommandHolder{}
	return
define:
	alias = state
	slot = &state.command
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Callable definitions are indexed independently of source order, so a goto
// from a later definition to an earlier call retains the closure effect.
func BenchmarkGPUFusionAddressAliasGotoLaterClosureDefinition(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	var mutate func()
	key := &Buffer{}
	shape := ShapeKey(64)
	goto define
invoke:
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
	return
define:
	mutate = func() { replaceCommand(slot) }
	goto invoke
}

// Each copied pointer is a snapshot. Rebinding the original pointer after a
// field address is taken must not collapse the old generation into the new.
func BenchmarkGPUFusionAddressAliasCopiedPointerGeneration(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	pointer := state
	oldSlot := &pointer.command
	state = &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		(*oldSlot).Commit()
		(*oldSlot).WaitUntilCompleted()
	}
}

// Overwriting the pointed-to state preserves the storage address of its
// command field. The saved slot can therefore replace the current command and
// must dilute the lifecycle.
func BenchmarkGPUFusionAddressAliasSurvivesPointeeOverwrite(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	*state = MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Defined pointer types preserve the same pointee-overwrite storage identity.
func BenchmarkGPUFusionAddressAliasSurvivesNamedPointeeOverwrite(b *testing.B) {
	device := &MetalDevice{}
	state := MetalCommandStatePointer(&MetalCommandState{})
	slot := &(*state).command
	*state = MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		(*state).command = device.NewCommandBuffer()
		(*state).command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		(*state).command.AppendKV(key, shape)
		replaceCommand(slot)
		(*state).command.Commit()
		(*state).command.WaitUntilCompleted()
	}
}

// A source write after the measured loop cannot retroactively detach an alias
// at any earlier lifecycle use.
func BenchmarkGPUFusionAddressAliasPostLoopRebind(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
	state = &MetalCommandState{}
}

// The alias is refreshed after the root rebind on every iteration, so every
// use observes the current command field rather than a prior iteration.
func BenchmarkGPUFusionAddressAliasUnconditionalRefresh(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	var slot **MetalCommandBuffer
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state = &MetalCommandState{}
		slot = &state.command
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand(slot)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Stable aliases used for every event still refer to the factory-created
// command and form one complete per-iteration lifecycle.
func BenchmarkGPUFusionAliasedLifecycleEvents(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		alias := command
		alias.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		alias.AppendKV(key, shape)
		alias.Commit()
		alias.WaitUntilCompleted()
	}
}

// The alias binding in a multi-assignment is not a write through the copied
// command pointer and therefore preserves the factory-created lifecycle.
func BenchmarkGPUFusionMultiAssignmentAliasedLifecycleEvents(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		alias, _ := command, 0
		alias.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		alias.AppendKV(key, shape)
		alias.Commit()
		alias.WaitUntilCompleted()
	}
}

// A stable copy of a command interface preserves the same dynamic command and
// therefore participates in the factory-created lifecycle.
func BenchmarkGPUFusionAliasedInterfaceLifecycle(b *testing.B) {
	device := &InterfaceMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command, _ := device.NewCommandBuffer()
		alias := command
		alias.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		alias.AppendKV(key, shape)
		alias.Commit()
		alias.WaitUntilCompleted()
	}
}

// A rebound source is not a stable identity chain, even though the copied
// interface value remains usable for the static recorder pair.
func BenchmarkGPUFusionInterfaceSnapshotSourceRebound(b *testing.B) {
	device := &InterfaceMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command, _ := device.NewCommandBuffer()
		alias := command
		command = &InterfaceMetalCommand{}
		alias.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		alias.AppendKV(key, shape)
		alias.Commit()
		alias.WaitUntilCompleted()
	}
}

// A rebound command-interface copy is no longer the factory result.
func BenchmarkGPUFusionReboundInterfaceLifecycleAlias(b *testing.B) {
	device := &InterfaceMetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command, _ := device.NewCommandBuffer()
		alias := command
		alias = &InterfaceMetalCommand{}
		alias.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		alias.AppendKV(key, shape)
		alias.Commit()
		alias.WaitUntilCompleted()
	}
}

// Rebinding the alias prevents it from being joined to the factory result.
func BenchmarkGPUFusionReboundLifecycleAlias(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		alias := command
		alias = &MetalCommandBuffer{}
		alias.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		alias.AppendKV(key, shape)
		alias.Commit()
		alias.WaitUntilCompleted()
	}
}

// GPU context may be supplied by a defining package even when the command
// type itself uses only the conventional CommandBuffer name.
func BenchmarkExternalMetalLifecycle(b *testing.B) {
	device := &metal.Device{}
	key := &metal.Buffer{}
	shape := metal.ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// An opaque helper escape after wait may persist into the next iteration.
func BenchmarkGPUFusionPointerAliasAfterWait(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		alias := state
		state.command.Commit()
		state.command.WaitUntilCompleted()
		replaceState(alias)
	}
}

// Rebinding an alias before exposure means that helper cannot mutate state.
func BenchmarkGPUFusionReboundPointerAlias(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	other := &MetalCommandState{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		alias := state
		alias = other
		replaceState(alias)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// An unconditional lexical block preserves the same per-iteration lifecycle.
func BenchmarkGPUFusionNestedBlockLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		{
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}
	}
}

// A write keeps its actual nested source order and splits a lifecycle whose
// create/candidate and commit/wait operate on different commands.
func BenchmarkGPUFusionNestedBlockRebindAfterCandidate(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		{
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
			command.AppendKV(key, shape)
			command = &MetalCommandBuffer{}
			command.Commit()
			command.WaitUntilCompleted()
		}
	}
}

// A synchronously invoked literal is part of the measured loop iteration.
func BenchmarkGPUFusionImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		func() {
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}()
	}
}

// A conditional nested lifecycle is not guaranteed on every measured
// iteration and must remain a static fusion finding only.
func BenchmarkGPUFusionConditionalNestedLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		if b.N > 0 {
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}
	}
}

// The same conditionality rule applies when the lifecycle is wrapped in an
// immediately invoked literal.
func BenchmarkGPUFusionConditionalImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		if b.N > 0 {
			func() {
				command := device.NewCommandBuffer()
				command.EncodeRoPE(key, shape)
				command.AppendKV(key, shape)
				command.Commit()
				command.WaitUntilCompleted()
			}()
		}
	}
}

// An arbitrary closure inside a benchmark is not a synchronously owned sub-benchmark.
func BenchmarkGPUFusionDormantClosure(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	dormant := func(b *testing.B) {
		for b.Loop() {
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}
	}
	_ = dormant
}

type GPUFusionCoverageEvidence struct {
	Hardware                      string
	WorkloadIdentity              string
	ProductionVariantLabels       []string
	CoveredVariantLabels          []string
	UnfusedVariantLabels          []string
	CoveredProductionVariantCount int
	TotalProductionVariantCount   int
	LeafCommandsPerBuffer         int
	ProductionCommandsPerBuffer   int
	LeafControlNS                 float64
	LeafCandidateNS               float64
	ProductionControlNS           float64
	ProductionCandidateNS         float64
	ControlEventCount             int
	CandidateEventCount           int
	EventCountOraclePassed        bool
	ProfileOraclePassed           bool
	ExactnessPassed               bool
	SameWorkloadPassed            bool
	AlternatingOrderPassed        bool
	PromotionThreshold            float64
	CandidatePromoted             bool
	FinalDecision                 string
}

type PackageGPUFusionCoverageEvidence = GPUFusionCoverageEvidence

var packageGPUFusionCoverageEvidence = PackageGPUFusionCoverageEvidence{ // want `GPU fusion coverage evidence is incomplete; missing workload identity, production variant labels`
	Hardware: "Apple M2 Pro",
}

// Dynamically assembled package values are outside the strict literal schema.
var packageDynamicGPUFusionCoverageEvidence = func() GPUFusionCoverageEvidence {
	return GPUFusionCoverageEvidence{}
}()

// A value receiver can still mutate lifecycle storage carried through an
// indirect field in its copy.
func BenchmarkGPUFusionValueReceiverIndirectMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutator := MetalCommandValueSlotMutator{slot: slot}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutator.replace()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionStoredValueReceiverIndirectMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := MetalCommandValueSlotMutator{slot: slot}.replace
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionValueReceiverCallbackMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	callback := MetalCommandValueCallback{mutate: func() { *slot = &MetalCommandBuffer{} }}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		callback.run()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionValueReceiverPointerFieldMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	mutator := MetalCommandValueStateMutator{state: state}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutator.replace()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionStoredValueReceiverPointerFieldMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	mutate := MetalCommandValueStateMutator{state: state}.replace
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		mutate()
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionScalarValueReceiverRead(b *testing.B) {
	device := &MetalDevice{}
	command := &MetalCommandBuffer{}
	observer := MetalCommandScalarObserver{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		observer.observe()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

// Aggregate arguments can retain an interior address even when the aggregate
// itself is passed by value.
func BenchmarkGPUFusionOpaqueAggregateRetention(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	holder := MetalCommandSlotHolder{slot: &state.command}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		retainMetalCommandSlotHolder(holder)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionScalarAggregateRead(b *testing.B) {
	device := &MetalDevice{}
	command := &MetalCommandBuffer{}
	holder := MetalCommandScalarHolder{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		observeMetalCommandScalarHolder(holder)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionOpaqueMapKeyRetention(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	other := &MetalCommandBuffer{}
	slots := map[**MetalCommandBuffer]**MetalCommandBuffer{&state.command: &other}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		retainMetalCommandSlotMap(slots)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionOpaqueMapValueRetention(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	other := &MetalCommandBuffer{}
	slots := map[**MetalCommandBuffer]**MetalCommandBuffer{&other: &state.command}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		retainMetalCommandSlotMap(slots)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionScalarMapRead(b *testing.B) {
	device := &MetalDevice{}
	command := &MetalCommandBuffer{}
	values := map[int]int{1: 2}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		observeMetalCommandScalarMap(values)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionOpaqueAggregateCallbackRetention(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	other := &MetalCommandBuffer{}
	holder := MetalCommandCallbackHolder{
		mutate: func() { state.command = &MetalCommandBuffer{} },
		other:  &other,
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		retainMetalCommandCallbackHolder(holder)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionReadOnlyAggregateCallback(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	other := &MetalCommandBuffer{}
	holder := MetalCommandCallbackHolder{
		mutate: func() { _ = state.command },
		other:  &other,
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		retainMetalCommandCallbackHolder(holder)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAggregatePointerWithUnrelatedCallback(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	holder := MetalCommandCallbackHolder{
		mutate: func() {},
		other:  &state.command,
	}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		retainMetalCommandCallbackHolder(holder)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// An escape after the measured loop cannot reach an earlier lifecycle.
func BenchmarkGPUFusionEscapeAfterLoop(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
	retainMetalCommandSlot(slot)
}

func BenchmarkGPUFusionConvertedImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		MetalCommandAction(func() {
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		})()
	}
}

func BenchmarkGPUFusionConditionalConvertedImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		if shape > 0 {
			MetalCommandAction(func() {
				command := device.NewCommandBuffer()
				command.EncodeRoPE(key, shape)
				command.AppendKV(key, shape)
				command.Commit()
				command.WaitUntilCompleted()
			})()
		}
	}
}

func BenchmarkGPUFusionNestedImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		func() {
			func() {
				command := device.NewCommandBuffer()
				command.EncodeRoPE(key, shape)
				command.AppendKV(key, shape)
				command.Commit()
				command.WaitUntilCompleted()
			}()
		}()
	}
}

func BenchmarkGPUFusionConditionalOuterNestedImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		if shape > 0 {
			func() {
				func() {
					command := device.NewCommandBuffer()
					command.EncodeRoPE(key, shape)
					command.AppendKV(key, shape)
					command.Commit()
					command.WaitUntilCompleted()
				}()
			}()
		}
	}
}

func BenchmarkGPUFusionConvertedNestedImmediateLiteralLifecycle(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		MetalCommandAction(func() {
			MetalCommandAction(func() {
				command := device.NewCommandBuffer()
				command.EncodeRoPE(key, shape)
				command.AppendKV(key, shape)
				command.Commit()
				command.WaitUntilCompleted()
			})()
		})()
	}
}

func BenchmarkGPUFusionImmediateLiteralResultMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	alias := func() **MetalCommandBuffer { return slot }()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionImmediateLiteralResultRead(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	alias := func() **MetalCommandBuffer { return slot }()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		_ = alias
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionParameterizedImmediateLiteralResultMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	alias := func(slot **MetalCommandBuffer) **MetalCommandBuffer { return slot }(&state.command)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionConvertedParameterizedImmediateLiteralResultMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	alias := MetalCommandSlotIdentity(func(slot **MetalCommandBuffer) **MetalCommandBuffer { return slot })(&state.command)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionImmediateLiteralReturnedArgumentRead(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	other := &MetalCommandBuffer{}
	alias := func(first, second **MetalCommandBuffer) **MetalCommandBuffer { return first }(&other, &state.command)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionMultipleImmediateLiteralResultsMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	alias, _ := func() (**MetalCommandBuffer, int) { return slot, 0 }()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// Storing an interior address in package storage is itself persistent: opaque
// code may retain and mutate it during a later lifecycle.
func BenchmarkGPUFusionGlobalSlotStorage(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	storedMetalCommandSlot = &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionInvokedLiteralGlobalSlotStorage(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	func() { storedMetalCommandSlot = &state.command }()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionLocalSlotStorage(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	localSlot := &state.command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		_ = localSlot
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

// A package slot plus an asynchronous writer is also a persistent escape.
func BenchmarkGPUFusionGlobalSlotEscape(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	storedMetalCommandSlot = &state.command
	go func() {
		for {
			*storedMetalCommandSlot = &MetalCommandBuffer{}
		}
	}()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionDefinedPointerSlotMutation(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	alias := NamedMetalCommandSlot(&state.command)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		replaceCommand((**MetalCommandBuffer)(alias))
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAsyncCapturedSlot(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	go func() {
		for {
			*slot = &MetalCommandBuffer{}
		}
	}()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionConvertedAsyncCapturedSlot(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	mutate := MetalCommandAction(func() {
		for {
			*slot = &MetalCommandBuffer{}
		}
	})
	go mutate()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAsyncMethodValueReceiver(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	mutator := &MetalCommandSlotMutator{slot: &state.command}
	go mutator.replace()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAsyncMethodExpressionReceiver(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	mutator := &MetalCommandSlotMutator{slot: &state.command}
	go (*MetalCommandSlotMutator).replace(mutator)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAsyncStoredMethodValueReceiver(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	mutate := (&MetalCommandSlotMutator{slot: &state.command}).replace
	go mutate()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionSynchronousMethodReceiverBeforeLoop(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	mutator := &MetalCommandSlotMutator{slot: &state.command}
	mutator.replace()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionSynchronousCapturedSlotBeforeLoop(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	func() { *slot = &MetalCommandBuffer{} }()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionDeferredCapturedSlotAfterLoop(b *testing.B) {
	device := &MetalDevice{}
	state := &MetalCommandState{}
	slot := &state.command
	defer func() { *slot = &MetalCommandBuffer{} }()
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		state.command = device.NewCommandBuffer()
		state.command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		state.command.AppendKV(key, shape)
		state.command.Commit()
		state.command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionLifecycleCallbackMutation(b *testing.B) {
	device := &CallbackMetalDevice{}
	command := &CallbackMetalCommandBuffer{}
	slot := &command
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit(func() { *slot = &CallbackMetalCommandBuffer{} })
		command.WaitUntilCompleted()
	}
}

func TestGPUFusionCoverageIncomplete(t *testing.T) {
	t.Parallel()
	evidence := GPUFusionCoverageEvidence{ // want `GPU fusion coverage evidence is incomplete; missing workload identity, production variant labels`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
}

func TestGPUFusionCoveragePartialAndDiluted(t *testing.T) {
	t.Parallel()
	evidence := GPUFusionCoverageEvidence{ // want `GPU fusion coverage evidence is invalid: covered topology count 10 of 22 leaves 12 unfused production variants; leaf benchmark records 1 command per buffer while production records 22, so leaf timing is command-lifecycle diluted; candidate is promoted without full production coverage`
		Hardware:                      "Apple M2 Pro",
		WorkloadIdentity:              "22-layer grouped-QKV decode",
		ProductionVariantLabels:       []string{"exact-00", "exact-01", "exact-02", "exact-03", "exact-04", "exact-05", "exact-06", "exact-07", "exact-08", "exact-09", "grouped-00", "grouped-01", "grouped-02", "grouped-03", "grouped-04", "grouped-05", "grouped-06", "grouped-07", "grouped-08", "grouped-09", "grouped-10", "grouped-11"},
		CoveredVariantLabels:          []string{"exact-00", "exact-01", "exact-02", "exact-03", "exact-04", "exact-05", "exact-06", "exact-07", "exact-08", "exact-09"},
		UnfusedVariantLabels:          []string{"grouped-00", "grouped-01", "grouped-02", "grouped-03", "grouped-04", "grouped-05", "grouped-06", "grouped-07", "grouped-08", "grouped-09", "grouped-10", "grouped-11"},
		CoveredProductionVariantCount: 10,
		TotalProductionVariantCount:   22,
		LeafCommandsPerBuffer:         1,
		ProductionCommandsPerBuffer:   22,
		LeafControlNS:                 109,
		LeafCandidateNS:               100,
		ProductionControlNS:           182,
		ProductionCandidateNS:         100,
		ControlEventCount:             54,
		CandidateEventCount:           22,
		EventCountOraclePassed:        true,
		ProfileOraclePassed:           true,
		ExactnessPassed:               true,
		SameWorkloadPassed:            true,
		AlternatingOrderPassed:        true,
		PromotionThreshold:            1.01,
		CandidatePromoted:             true,
		FinalDecision:                 "promote",
	}
	_ = evidence
}

func TestGPUFusionCoverageInvalidVectorsAndOracles(t *testing.T) {
	t.Parallel()
	evidence := GPUFusionCoverageEvidence{ // want `GPU fusion coverage evidence is invalid: topology label sets contain duplicates; covered topology labels are not a subset of production labels; candidate event count 54 does not reduce positive control count 54; event-count oracle is explicitly false`
		Hardware:                      "GPU",
		WorkloadIdentity:              "decode",
		ProductionVariantLabels:       []string{"exact", "exact"},
		CoveredVariantLabels:          []string{"exact", "exact"},
		UnfusedVariantLabels:          []string{"stale"},
		CoveredProductionVariantCount: 2,
		TotalProductionVariantCount:   2,
		LeafCommandsPerBuffer:         22,
		ProductionCommandsPerBuffer:   22,
		LeafControlNS:                 109,
		LeafCandidateNS:               100,
		ProductionControlNS:           105,
		ProductionCandidateNS:         100,
		ControlEventCount:             54,
		CandidateEventCount:           54,
		EventCountOraclePassed:        false,
		ProfileOraclePassed:           true,
		ExactnessPassed:               true,
		SameWorkloadPassed:            true,
		AlternatingOrderPassed:        true,
		PromotionThreshold:            1.01,
		CandidatePromoted:             false,
		FinalDecision:                 "candidate-only",
	}
	_ = evidence
}

func TestGPUFusionCoverageStrictSchemaMismatch(t *testing.T) {
	t.Parallel()
	evidence := GPUFusionCoverageEvidence{ // want `GPU fusion coverage evidence is invalid: hardware and workload identities must be non-empty; production topology count 3 disagrees with 2 labels; covered topology count 2 disagrees with 1 labels; unfused topology labels do not equal production minus covered labels; covered topology count 2 of 3 leaves 1 unfused production variants; commands-per-buffer depths must be positive; leaf and production control/candidate times must be positive; candidate event count 54 does not reduce positive control count 54; profile oracle is explicitly false; exactness oracle is explicitly false; same-workload gate is explicitly false; alternating-order gate is explicitly false; promotion threshold 1x is not above parity; candidate is promoted without full production coverage and a production ratio above the frozen 1x threshold; candidate promotion status disagrees with final decision`
		Hardware:                      "",
		WorkloadIdentity:              "",
		ProductionVariantLabels:       []string{"exact-QKV", "grouped-QKV"},
		CoveredVariantLabels:          []string{"exact-QKV"},
		UnfusedVariantLabels:          []string{},
		CoveredProductionVariantCount: 2,
		TotalProductionVariantCount:   3,
		LeafCommandsPerBuffer:         1,
		ProductionCommandsPerBuffer:   0,
		LeafControlNS:                 0,
		LeafCandidateNS:               100,
		ProductionControlNS:           105,
		ProductionCandidateNS:         100,
		ControlEventCount:             54,
		CandidateEventCount:           54,
		EventCountOraclePassed:        true,
		ProfileOraclePassed:           false,
		ExactnessPassed:               false,
		SameWorkloadPassed:            false,
		AlternatingOrderPassed:        false,
		PromotionThreshold:            1,
		CandidatePromoted:             true,
		FinalDecision:                 "candidate-only",
	}
	_ = evidence
}

func BenchmarkGPUFusionBlockedImmediateInvocationArgument(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		func(_ int) {
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}(blockingPS6089Int())
	}
}

func BenchmarkGPUFusionReturningImmediateInvocationArgument(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		func(_ int) {
			command := device.NewCommandBuffer()
			command.EncodeRoPE(key, shape)
			command.AppendKV(key, shape)
			command.Commit()
			command.WaitUntilCompleted()
		}(returningPS6089Int())
	}
}

func BenchmarkGPUFusionBlockedBeforeCommit(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		_ = blockingPS6089Int()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionBlockedAfterWait(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
		select {}
	}
}

func BenchmarkGPUFusionNamedCallbackMutation(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	holder := MetalCommandCallbackHolder{mutate: func() { mutateNamedCallbackCommand() }}
	for b.Loop() {
		namedCallbackCommand = device.NewCommandBuffer()
		namedCallbackCommand.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		namedCallbackCommand.AppendKV(key, shape)
		retainMetalCommandCallbackHolder(holder)
		namedCallbackCommand.Commit()
		namedCallbackCommand.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionNamedReadOnlyCallback(b *testing.B) {
	device := &MetalDevice{}
	key := &Buffer{}
	shape := ShapeKey(64)
	holder := MetalCommandCallbackHolder{mutate: func() { readNamedCallbackCommand() }}
	for b.Loop() { // want `leaf GPU benchmark creates, commits, and waits for one command buffer per iteration`
		namedCallbackCommand = device.NewCommandBuffer()
		namedCallbackCommand.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		namedCallbackCommand.AppendKV(key, shape)
		retainMetalCommandCallbackHolder(holder)
		namedCallbackCommand.Commit()
		namedCallbackCommand.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionCallableReturnEscape(b *testing.B) {
	device := &MetalDevice{}
	var command *MetalCommandBuffer
	slot := func() **MetalCommandBuffer { return &command }
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		retainMetalCommandSlot(slot())
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPersistentAggregateCallbackEscape(b *testing.B) {
	device := &MetalDevice{}
	var command *MetalCommandBuffer
	storedMetalCommandCallbackHolder = MetalCommandCallbackHolder{mutate: func() { command = &MetalCommandBuffer{} }}
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionInterfaceWrappedSlotEscape(b *testing.B) {
	device := &MetalDevice{}
	var command *MetalCommandBuffer
	wrapped := any(&command)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		retainAnyMetalCommandSlot(wrapped)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionUnsafeWrappedSlotEscape(b *testing.B) {
	device := &MetalDevice{}
	var command *MetalCommandBuffer
	wrapped := unsafe.Pointer(&command)
	key := &Buffer{}
	shape := ShapeKey(64)
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		retainUnsafeMetalCommandSlot(wrapped)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGetterEscape(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	holder := MetalCommandGetterHolder{get: func() **MetalCommandBuffer { return &command }}
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		retainMetalCommandGetterHolder(holder)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPointerCallbackEscape(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	holder := &MetalCommandCallbackHolder{mutate: func() { command = &MetalCommandBuffer{} }}
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		retainMetalCommandCallbackHolderPointer(holder)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionNamedGetterAliasMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	alias := namedGetterCommandSlot()
	for b.Loop() {
		namedGetterCommand = device.NewCommandBuffer()
		namedGetterCommand.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		namedGetterCommand.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		namedGetterCommand.Commit()
		namedGetterCommand.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionNamedGetterDirectMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	for b.Loop() {
		namedGetterCommand = device.NewCommandBuffer()
		namedGetterCommand.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		namedGetterCommand.AppendKV(key, shape)
		*namedGetterCommandSlot() = &MetalCommandBuffer{}
		namedGetterCommand.Commit()
		namedGetterCommand.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionReceiverAddressAliasRebind(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	alias := &command
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionPackageAliasMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	for b.Loop() {
		namedGetterCommand = device.NewCommandBuffer()
		namedGetterCommand.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		namedGetterCommand.AppendKV(key, shape)
		*namedGetterCommandAlias = &MetalCommandBuffer{}
		namedGetterCommand.Commit()
		namedGetterCommand.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionNonreturnAfterWait(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		command.Commit()
		command.WaitUntilCompleted()
		recursePS6089Forever()
	}
}

func BenchmarkGPUFusionConditionalNonreturnBeforeCommit(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	for b.Loop() {
		command := device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		maybeRecursePS6089Forever()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionAssertedSlotMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	wrapped := any(&command)
	slot := wrapped.(**MetalCommandBuffer)
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		replaceCommand(slot)
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionGotoRebindBetweenCandidateAndCommit(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		goto mutate
	commit:
		command.Commit()
		command.WaitUntilCompleted()
		continue
	mutate:
		command = &MetalCommandBuffer{}
		goto commit
	}
}

func BenchmarkGPUFusionFunctionFieldMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	holder := MetalCommandCallbackHolder{mutate: func() { command = &MetalCommandBuffer{} }}
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		holder.mutate()
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionDirectNamedHelperMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	for b.Loop() {
		namedCallbackCommand = device.NewCommandBuffer()
		namedCallbackCommand.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		namedCallbackCommand.AppendKV(key, shape)
		mutateNamedCallbackCommand()
		namedCallbackCommand.Commit()
		namedCallbackCommand.WaitUntilCompleted()
	}
}

func BenchmarkGPUFusionCompositeInteriorAliasMutation(b *testing.B) {
	device := &MetalDevice{}
	key, shape := &Buffer{}, ShapeKey(64)
	var command *MetalCommandBuffer
	holder := &MetalCommandValueSlotMutator{slot: &command}
	alias := holder.slot
	for b.Loop() {
		command = device.NewCommandBuffer()
		command.EncodeRoPE(key, shape) // want `adjacent typed GPU recorder operations EncodeRoPE -> AppendKV share one command receiver, data buffer, and shape key`
		command.AppendKV(key, shape)
		*alias = &MetalCommandBuffer{}
		command.Commit()
		command.WaitUntilCompleted()
	}
}

func TestGPUFusionCoverageComplete(t *testing.T) {
	t.Parallel()
	evidence := GPUFusionCoverageEvidence{
		Hardware:                      "Apple M2 Pro",
		WorkloadIdentity:              "exact plus grouped-QKV decode",
		ProductionVariantLabels:       []string{"exact-QKV", "grouped-QKV"},
		CoveredVariantLabels:          []string{"exact-QKV", "grouped-QKV"},
		UnfusedVariantLabels:          []string{},
		CoveredProductionVariantCount: 2,
		TotalProductionVariantCount:   2,
		LeafCommandsPerBuffer:         22,
		ProductionCommandsPerBuffer:   22,
		LeafControlNS:                 108,
		LeafCandidateNS:               100,
		ProductionControlNS:           105.74,
		ProductionCandidateNS:         100,
		ControlEventCount:             54,
		CandidateEventCount:           22,
		EventCountOraclePassed:        true,
		ProfileOraclePassed:           true,
		ExactnessPassed:               true,
		SameWorkloadPassed:            true,
		AlternatingOrderPassed:        true,
		PromotionThreshold:            1.01,
		CandidatePromoted:             true,
		FinalDecision:                 "promote",
	}
	_ = evidence
}

func TestGPUFusionCoverageNegativeDecision(t *testing.T) {
	t.Parallel()
	evidence := GPUFusionCoverageEvidence{
		Hardware:                      "Apple M2 Pro",
		WorkloadIdentity:              "exact plus grouped-QKV decode",
		ProductionVariantLabels:       []string{"exact-QKV", "grouped-QKV"},
		CoveredVariantLabels:          []string{"exact-QKV", "grouped-QKV"},
		UnfusedVariantLabels:          []string{},
		CoveredProductionVariantCount: 2,
		TotalProductionVariantCount:   2,
		LeafCommandsPerBuffer:         22,
		ProductionCommandsPerBuffer:   22,
		LeafControlNS:                 100.5,
		LeafCandidateNS:               100,
		ProductionControlNS:           100.5,
		ProductionCandidateNS:         100,
		ControlEventCount:             54,
		CandidateEventCount:           22,
		EventCountOraclePassed:        true,
		ProfileOraclePassed:           true,
		ExactnessPassed:               true,
		SameWorkloadPassed:            true,
		AlternatingOrderPassed:        true,
		PromotionThreshold:            1.01,
		CandidatePromoted:             false,
		FinalDecision:                 "not-promoted",
	}
	_ = evidence
}

func TestGPUFusionCoverageLocalAlias(t *testing.T) {
	t.Parallel()
	type LocalEvidence = GPUFusionCoverageEvidence
	evidence := LocalEvidence{ // want `GPU fusion coverage evidence is incomplete; missing workload identity, production variant labels`
		Hardware: "Apple M2 Pro",
	}
	_ = evidence
}

// Invalid-only vocabulary without one of the exact manifest identities stays silent.
type gpuFusionCoverageEvidenceDraft struct {
	Hardware string
	Note     string
}

func TestGPUFusionCoverageInvalidOnlyVocabulary(t *testing.T) {
	t.Parallel()
	draft := gpuFusionCoverageEvidenceDraft{Hardware: "GPU", Note: "partial fusion coverage and command lifecycle"}
	_ = draft
}
