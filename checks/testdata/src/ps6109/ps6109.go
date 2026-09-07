package ps6109

type nativeHandle int

type commandRecorder interface {
	Encode()
	Free()
	Reset() error
	ResetFast()
}

type commandFactory interface {
	NewRecorder() (commandRecorder, error)
}

type recorder struct {
	handle    nativeHandle
	encoder   int
	committed bool
}

type device struct {
	next int
}

type owner struct {
	provider commandFactory
}

type distinctOwner struct {
	provider commandFactory
}

type aggregateOwner struct {
	provider commandFactory
}

type copiedOwner struct {
	provider commandFactory
}

type asyncOwner struct {
	provider commandFactory
}

var escaped any

func (d *device) NewCommandBuffer() nativeHandle {
	d.next++
	return nativeHandle(d.next)
}

func newRecorder(d *device) *recorder {
	return &recorder{handle: d.NewCommandBuffer()}
}

func newRecorderWithState(d *device) *recorder {
	return &recorder{handle: d.NewCommandBuffer(), encoder: 1}
}

func newRecorderDistinctNative(d *device) *recorder {
	return &recorder{handle: (&device{}).NewCommandBuffer()}
}

func newRecorderBoxed(value any) *recorder {
	d := value.(*device)
	return &recorder{handle: d.NewCommandBuffer()}
}

func (d *device) NewRecorder() (commandRecorder, error) { // want NewRecorder:`proved reusable one-shot wrapper factory`
	return newRecorder(d), nil
}

func (d *device) NewRecorderFast() commandRecorder { // want NewRecorderFast:`proved reusable one-shot wrapper factory`
	return newRecorder(d)
}

func (d *device) NewRecorderAltered() *recorder   { return newRecorder(&device{}) }
func (d *device) NewRecorderWithState() *recorder { return newRecorderWithState(d) }
func (d *device) NewRecorderDistinctNative() *recorder {
	return newRecorderDistinctNative(d)
}
func (d *device) NewRecorderBoxed() *recorder { return newRecorderBoxed(d) }

func (r *recorder) Encode() { r.encoder++ }

func (r *recorder) Free() {
	r.handle = 0
	r.encoder = 0
	r.committed = false
}

func (r *recorder) Reset() error {
	r.handle++
	r.encoder = 0
	r.committed = false
	return nil
}

func (r *recorder) ResetFast() { _ = r.Reset() }

func lifecycle(provider commandFactory) error {
	recorder, err := provider.NewRecorder()
	if err != nil {
		return err
	}
	recorder.Encode()
	recorder.Free()
	return nil
}

func lifecycleViaForwarder(provider commandFactory) error {
	recorder, err := provider.NewRecorder()
	if err != nil {
		return err
	}
	recorder.Encode()
	recorder.Free()
	return nil
}

func forwardLifecycle(provider commandFactory) error { return lifecycleViaForwarder(provider) }

func lifecycleWithProviderEscape(provider commandFactory) error {
	recorder, err := provider.NewRecorder()
	if err != nil {
		return err
	}
	escaped = provider
	recorder.Encode()
	recorder.Free()
	return nil
}

func forwardedPositive() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 4; step++ {
		if err := lifecycle(provider); err != nil { // want `fixed 4-step source path creates a fresh Go wrapper generation`
			return err
		}
	}
	return nil
}

func twoEdgeForwardedPositive() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 7; step++ {
		if err := forwardLifecycle(provider); err != nil { // want `fixed 7-step source path creates a fresh Go wrapper generation`
			return err
		}
	}
	return nil
}

func forwardedProviderEscape() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 4; step++ {
		if err := lifecycleWithProviderEscape(provider); err != nil {
			return err
		}
	}
	return nil
}

func discardedForwardedError() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 4; step++ {
		lifecycle(provider)
	}
	return nil
}

func directPositive() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder() // want `fixed 3-step source path creates a fresh Go wrapper generation`
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func invalidFactoryProvenance() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.NewRecorderAltered()
		recorder.Encode()
		recorder.Free()
	}
}

func invalidConstructorState() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.NewRecorderWithState()
		recorder.Encode()
		recorder.Free()
	}
}

func invalidDistinctNativeRoot() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.NewRecorderDistinctNative()
		recorder.Encode()
		recorder.Free()
	}
}

func invalidBoxedFactoryArgument() {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.NewRecorderBoxed()
		recorder.Encode()
		recorder.Free()
	}
}

func skippedInfallibleTerminal(stop bool) {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder := provider.NewRecorderFast()
		if stop {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
}

func earlyBreakAfterTerminal() error {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
		break
	}
	return nil
}

func mutatedIndexAfterTerminal() error {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
		step = 100
	}
	return nil
}

func returnAfterTerminal() error {
	provider := &device{}
	for step := 0; step < 4; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
		return nil
	}
	return nil
}

func sharedLifecycle(provider commandFactory) error {
	recorder, err := provider.NewRecorder()
	if err != nil {
		return err
	}
	recorder.Encode()
	recorder.Free()
	return nil
}

func sharedFour() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 4; step++ {
		if err := sharedLifecycle(provider); err != nil { // want `fixed 4-step source path creates a fresh Go wrapper generation`
			return err
		}
	}
	return nil
}

func sharedSeven() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 7; step++ {
		if err := sharedLifecycle(provider); err != nil { // want `fixed 7-step source path creates a fresh Go wrapper generation`
			return err
		}
	}
	return nil
}

func deadSharedCall() error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 4; step++ {
		break
		sharedLifecycle(provider)
	}
	return nil
}

func conditionalSharedCall(skip bool) error {
	concrete := &device{}
	var provider commandFactory = concrete
	for step := 0; step < 4; step++ {
		if skip {
			continue
		}
		sharedLifecycle(provider)
	}
	return nil
}

func concreteAliasEscape() error {
	concrete := &device{}
	var provider commandFactory = concrete
	escaped = concrete
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func unreachableAcquisition() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		return nil
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func unreachableLoop() error {
	provider := &device{}
	return nil
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func newOwner() *owner {
	value := &owner{}
	value.provider = &device{}
	return value
}

func (value *owner) fieldPositive() error {
	for step := 0; step < 6; step++ {
		recorder, err := value.provider.NewRecorder() // want `fixed 6-step source path creates a fresh Go wrapper generation`
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func callFieldPositive() error { return newOwner().fieldPositive() }

func newDistinctOwner() *distinctOwner {
	value := &distinctOwner{}
	value.provider = &device{}
	return value
}

func (value *distinctOwner) distinctRoots(other *distinctOwner) error {
	for step := 0; step < 6; step++ {
		recorder, err := other.provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func callDistinctRoots() error { return newDistinctOwner().distinctRoots(newDistinctOwner()) }

func newAggregateOwner() *aggregateOwner {
	value := &aggregateOwner{}
	value.provider = &device{}
	escaped = []any{value}
	return value
}

func (value *aggregateOwner) aggregateEscapeOwner() error {
	for step := 0; step < 6; step++ {
		recorder, err := value.provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func callAggregateOwner() error { return newAggregateOwner().aggregateEscapeOwner() }

func newCopiedOwner() *copiedOwner {
	value := &copiedOwner{}
	value.provider = &device{}
	copyOfValue := *value
	_ = copyOfValue
	return value
}

func (value *copiedOwner) copiedOwnerValue() error {
	for step := 0; step < 6; step++ {
		recorder, err := value.provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func callCopiedOwner() error { return newCopiedOwner().copiedOwnerValue() }

func newAsyncOwner() *asyncOwner {
	value := &asyncOwner{}
	value.provider = &device{}
	return value
}

func (value *asyncOwner) asyncOwnerValue() error {
	for step := 0; step < 6; step++ {
		recorder, err := value.provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func callAsyncOwner() { go newAsyncOwner().asyncOwnerValue() }

func twoLivePositive() {
	provider := &device{}
	for step := 0; step < 5; step++ {
		first := provider.NewRecorderFast() // want `fixed 5-step source path creates a fresh Go wrapper generation`
		first.Encode()
		second := provider.NewRecorderFast() // want `fixed 5-step source path creates a fresh Go wrapper generation`
		second.Encode()
		second.Free()
		first.Free()
	}
}

func twoDifferentProviders() {
	provider := &device{}
	other := &device{}
	for step := 0; step < 5; step++ {
		first := provider.NewRecorderFast()
		first.Encode()
		second := other.NewRecorderFast()
		second.Encode()
		second.Free()
		first.Free()
	}
}

func missingTerminal() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
	}
	return nil
}

func postTerminalUse() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Free()
		recorder.Encode()
	}
	return nil
}

func asyncUse() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		go recorder.Encode()
		recorder.Free()
	}
	return nil
}

func unstableProvider() error {
	provider := &device{}
	other := &device{}
	for step := 0; step < 3; step++ {
		provider = other
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func aggregateEscape() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		escaped = []any{recorder}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func methodValueEscape() error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		encode := recorder.Encode
		encode()
		recorder.Free()
	}
	return nil
}

func branchBeforeTerminal(stop bool) error {
	provider := &device{}
	for step := 0; step < 3; step++ {
		recorder, err := provider.NewRecorder()
		if err != nil {
			continue
		}
		recorder.Encode()
		if stop {
			return nil
		}
		recorder.Free()
	}
	return nil
}

func threeLiveGenerations() {
	provider := &device{}
	for step := 0; step < 5; step++ {
		first := provider.NewRecorderFast()
		first.Encode()
		second := provider.NewRecorderFast()
		second.Encode()
		third := provider.NewRecorderFast()
		third.Encode()
		third.Free()
		second.Free()
		first.Free()
	}
}

func nonOverlappingPair() {
	provider := &device{}
	for step := 0; step < 5; step++ {
		first := provider.NewRecorderFast()
		first.Encode()
		first.Free()
		second := provider.NewRecorderFast()
		second.Encode()
		second.Free()
	}
}
