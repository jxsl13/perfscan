package ps6105

import "fmt"

type noReadState struct {
	scratch []float32
}

func newNoReadState() *noReadState {
	return &noReadState{
		scratch: make([]float32, 16), // want `constructor stores a fixed 64-byte noReadState.scratch scratch-allocation candidate that has no source-visible field read.*reflection.*whole-receiver escape.*advisory, no automatic fix`
	}
}

// A second constructor that leaves scratch at its zero value is not an
// alternate field initializer and must not hide the allocating candidate.
func newZeroNoReadState() *noReadState {
	return &noReadState{}
}

type functionState struct {
	scratch []float32
}

func ignoreScratch(_ []float32) {}

func newFunctionState() *functionState {
	state := &functionState{}
	state.scratch = make([]float32, 16) // want `constructor stores a fixed 64-byte functionState.scratch scratch-allocation candidate that is read only as a whole argument to zero-use formals of closed package-local concrete callees.*interface/backend gates`
	return state
}

func (state *functionState) accumulate() {
	ignoreScratch(state.scratch)
}

type kernel struct{}

func (*kernel) accumulate(_ int, _ []float32) {}

type methodState struct {
	scratch []float32
}

func newMethodState() *methodState {
	return &methodState{scratch: make([]float32, 16)} // want `constructor stores a fixed 64-byte methodState.scratch scratch-allocation candidate.*zero-use formals.*closed package-local concrete callees`
}

func (state *methodState) accumulate(kernel *kernel) {
	kernel.accumulate(1, state.scratch)
}

type methodExpressionState struct {
	scratch []float32
}

func newMethodExpressionState() *methodExpressionState {
	return &methodExpressionState{scratch: make([]float32, 16)} // want `constructor stores a fixed 64-byte methodExpressionState.scratch scratch-allocation candidate.*zero-use formals`
}

func (state *methodExpressionState) accumulate(instance *kernel) {
	(*kernel).accumulate(instance, 1, state.scratch)
}

type assertedState struct {
	scratch []float32
}

func newAssertedState() *assertedState {
	return &assertedState{scratch: make([]float32, 16)} // want `constructor stores a fixed 64-byte assertedState.scratch scratch-allocation candidate.*zero-use formals`
}

func (state *assertedState) accumulate(candidate any) {
	if concrete, ok := candidate.(*kernel); ok {
		concrete.accumulate(1, (state.scratch))
	}
}

type pairedState struct {
	projection   []float32
	accumulation []float32
}

func newPairedState() *pairedState {
	return &pairedState{
		projection:   make([]float32, 1024*768), // want `constructor stores a fixed 3145728-byte pairedState.projection scratch-allocation candidate`
		accumulation: make([]float32, 1024*768), // want `constructor stores a fixed 3145728-byte pairedState.accumulation scratch-allocation candidate`
	}
}

type capacityState struct {
	scratch []float32
}

func newCapacityState() *capacityState {
	return &capacityState{scratch: make([]float32, 8, 16)} // want `constructor stores a fixed 64-byte capacityState.scratch scratch-allocation candidate`
}

type usedState struct {
	scratch []float32
}

func useScratch(scratch []float32) {
	_ = scratch[0]
}

func newUsedState() *usedState {
	return &usedState{scratch: make([]float32, 16)}
}

func (state *usedState) accumulate() {
	useScratch(state.scratch)
}

type closureUsedState struct {
	scratch []float32
}

func closureUsesScratch(scratch []float32) {
	use := func() int { return len(scratch) }
	_ = use()
}

func newClosureUsedState() *closureUsedState {
	return &closureUsedState{scratch: make([]float32, 16)}
}

func (state *closureUsedState) accumulate() {
	closureUsesScratch(state.scratch)
}

type mixedState struct {
	scratch []float32
}

func newMixedState() *mixedState {
	return &mixedState{scratch: make([]float32, 16)}
}

func (state *mixedState) accumulate(use bool) {
	ignoreScratch(state.scratch)
	if use {
		useScratch(state.scratch)
	}
}

type runner interface {
	accumulate(int, []float32)
}

type interfaceState struct {
	scratch []float32
}

func newInterfaceState() *interfaceState {
	return &interfaceState{scratch: make([]float32, 16)}
}

func (state *interfaceState) run(kernel runner) {
	kernel.accumulate(1, state.scratch)
}

type importedState struct {
	scratch []float32
}

func newImportedState() *importedState {
	return &importedState{scratch: make([]float32, 16)}
}

func (state *importedState) observe() {
	_ = fmt.Sprint(state.scratch)
}

type indirectState struct {
	scratch []float32
}

func newIndirectState() *indirectState {
	return &indirectState{scratch: make([]float32, 16)}
}

func (state *indirectState) run() {
	call := ignoreScratch
	call(state.scratch)
}

type genericCalleeState struct {
	scratch []float32
}

func ignoreGeneric[T any](_ T) {}

func newGenericCalleeState() *genericCalleeState {
	return &genericCalleeState{scratch: make([]float32, 16)}
}

func (state *genericCalleeState) run() {
	ignoreGeneric(state.scratch)
}

type variadicState struct {
	scratch []float32
}

func ignoreVariadic(_ ...[]float32) {}

func newVariadicState() *variadicState {
	return &variadicState{scratch: make([]float32, 16)}
}

func (state *variadicState) run() {
	ignoreVariadic(state.scratch)
}

type indexedState struct {
	scratch []float32
}

func newIndexedState() *indexedState {
	return &indexedState{scratch: make([]float32, 16)}
}

func (state *indexedState) read() float32 {
	return state.scratch[0]
}

type lengthState struct {
	scratch []float32
}

func newLengthState() *lengthState {
	return &lengthState{scratch: make([]float32, 16)}
}

func (state *lengthState) read() int {
	return len(state.scratch) + cap(state.scratch)
}

type nilState struct {
	scratch []float32
}

func newNilState() *nilState {
	return &nilState{scratch: make([]float32, 16)}
}

func (state *nilState) empty() bool {
	return state.scratch == nil
}

type addressState struct {
	scratch []float32
}

func newAddressState() *addressState {
	return &addressState{scratch: make([]float32, 16)}
}

func (state *addressState) expose() *[]float32 {
	return &state.scratch
}

type storedState struct {
	scratch []float32
}

var stored []float32

func newStoredState() *storedState {
	return &storedState{scratch: make([]float32, 16)}
}

func (state *storedState) store() {
	stored = state.scratch
}

type aliasState struct {
	scratch []float32
}

func newAliasState() *aliasState {
	return &aliasState{scratch: make([]float32, 16)}
}

func (state *aliasState) run() {
	alias := state.scratch
	ignoreScratch(alias)
}

type sliceState struct {
	scratch []float32
}

func newSliceState() *sliceState {
	return &sliceState{scratch: make([]float32, 16)}
}

func (state *sliceState) run() {
	ignoreScratch(state.scratch[:])
}

type appendState struct {
	scratch []float32
}

func newAppendState() *appendState {
	return &appendState{scratch: make([]float32, 16)}
}

func (state *appendState) run() {
	stored = append(stored, state.scratch...)
}

type capturedState struct {
	scratch []float32
}

func newCapturedState() *capturedState {
	return &capturedState{scratch: make([]float32, 16)}
}

func (state *capturedState) run() func() {
	return func() { ignoreScratch(state.scratch) }
}

type reassignedState struct {
	scratch []float32
}

func newReassignedState() *reassignedState {
	return &reassignedState{scratch: make([]float32, 16)}
}

func (state *reassignedState) reset() {
	state.scratch = nil
}

type multipleConstructorState struct {
	scratch []float32
}

func newMultipleConstructorState() *multipleConstructorState {
	return &multipleConstructorState{scratch: make([]float32, 16)}
}

func anotherMultipleConstructorState() *multipleConstructorState {
	return &multipleConstructorState{scratch: make([]float32, 32)}
}

type unkeyedState struct {
	scratch []float32
}

func newUnkeyedState() *unkeyedState {
	return &unkeyedState{scratch: make([]float32, 16)}
}

var _ = unkeyedState{nil}

type ExportedFieldState struct {
	Scratch []float32
}

func newExportedFieldState() *ExportedFieldState {
	return &ExportedFieldState{Scratch: make([]float32, 16)}
}

type dynamicState struct {
	scratch []float32
}

func newDynamicState(size int) *dynamicState {
	return &dynamicState{scratch: make([]float32, size)}
}

type zeroElement struct{}

type zeroSizeState struct {
	scratch []zeroElement
}

func newZeroSizeState() *zeroSizeState {
	return &zeroSizeState{scratch: make([]zeroElement, 16)}
}

type genericState[T any] struct {
	scratch []T
}

func newGenericState[T any]() *genericState[T] {
	return &genericState[T]{scratch: make([]T, 16)}
}

type unreachableState struct {
	scratch []float32
}

func newUnreachableState() *unreachableState {
	panic("constructor stops")
	return &unreachableState{scratch: make([]float32, 16)}
}
