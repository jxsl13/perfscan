package ps6091

type DeviceBuffer struct{}
type DeviceAlias = DeviceBuffer

type ValueBuffer struct{}

type Búffer struct{}

func (ValueBuffer) TopKN(int, int) ([]int32, []float32, error) {
	return nil, nil, nil
}

func (*Búffer) TópKN(int, int) ([]int, error) { return nil, nil }

type Integer interface {
	~int | ~int32
}

type IntegerAlias = Integer

type EmbeddedInteger interface {
	IntegerAlias
}

type MixedScalar interface {
	~int | ~string
}

type StringScalar interface {
	~string
}

type EmptyIntegerSet interface {
	int
	int32
}

type ByteSlice = []byte

type ComparableIntegerOrBytes interface {
	comparable
	~int | ~ByteSlice
}

type ComparableStringOrBytes interface {
	comparable
	~string | ~ByteSlice
}

type ComparableContainers interface {
	comparable
	~[]byte | ~map[string]int | ~func()
}

type NarrowInteger interface {
	IntegerAlias
	~int32
}

type GenericBuffer[T Integer] struct{}

func (*GenericBuffer[T]) TopKN(int, int) ([]T, []float32, error) {
	return nil, nil, nil
}

type GenericAnyBuffer[T any] struct{}

func (*GenericAnyBuffer[T]) TopKN(int, int) ([]T, []float32, error) {
	return nil, nil, nil
}

type TokenID int32
type TokenIDs []TokenID
type AliasTokenIDs = TokenIDs

type SelfBox[T any] struct {
	Value T
}

type DirectNode[T any] struct {
	Value T
}

type GuardedNode[T any] struct {
	Value *T
}

type ExactToken interface {
	TokenID
}

type IntegerWithMethod interface {
	~int32
	Int32() int32
}

type ExactTokenWithMethod interface {
	TokenID
	Int32() int32
}

type ImpossibleExactInteger interface {
	int
	Int32() int32
}

type IntegerOrPointerWithMethod interface {
	~int32 | ~*int32
	Int32() int32
}

type PointerWithMethod interface {
	~*int32
	Int32() int32
}

type IntegerOrExistingPointerMethod interface {
	~int32 | ~*TokenID
	Reset()
}

type ExistingPointerMethod interface {
	~*TokenID
	Reset()
}

type ExactPointerWithMethod interface {
	*TokenID
	Reset()
}

type ExistingPointerMissingMethod interface {
	~*TokenID
	Missing()
}

type IntegerOrMethodCapableContainers interface {
	~int32 | ~map[string]int | ~[]byte | ~func() | ~chan int
	Int32() int32
}

type IntegerOrConflictingStructMethod interface {
	~int32 | ~struct{ Int32 int32 }
	Int32() int32
}

type IntegerOrMethodCapableStruct interface {
	~int32 | ~struct{ Value int32 }
	Int32() int32
}

type ExactTokenIntersection interface {
	~int32
	TokenID
}

type ImpossibleExactIntersection interface {
	~int
	TokenID
}

type BackendError struct{}
type AliasBackendError = BackendError

func (BackendError) Error() string { return "backend error" }

func (*TokenID) Reset() {}

func (t TokenID) Int32() int32 { return int32(t) }

func (t *TokenID) Self() *TokenID { return t }

func (t *TokenID) SelfSlice() []*TokenID { return []*TokenID{t} }

func (t *TokenID) SelfBox() SelfBox[*TokenID] { return SelfBox[*TokenID]{Value: t} }

func (*TokenID) AcceptSelf(*TokenID) {}

func (t *TokenID) ValueSelf() TokenID { return *t }

func (t TokenID) ExactSelf() TokenID { return t }

func (*DeviceBuffer) TopKN(int, int) ([]int32, []float32, error) {
	return nil, nil, nil
}

func (*DeviceBuffer) NamedTopKN(int, int) (TokenIDs, error) { return nil, nil }

func (*DeviceBuffer) AliasTopKN(int, int) (AliasTokenIDs, AliasBackendError) {
	return nil, BackendError{}
}

type OtherBuffer struct{}

func (*OtherBuffer) TopKN(int, int) ([]int32, []float32, error) {
	return nil, nil, nil
}

func TopK([]float32, int) ([]int, error) { return nil, nil }

func TópK([]float32, int) ([]int, error) { return nil, nil }

func GenericTopK[T any]([]T, int) ([]T, error) { return nil, nil }

func SingleTopK([]float32, int) []int { return nil }

func consumeInt32(int32)      {}
func consumeIndices([]int32)  {}
func consumeValues([]float32) {}

func methodPositive(device *DeviceBuffer, vocab int) (int32, error) {
	indices, _, err := device.TopKN(vocab, 1) // want "resident TopKN: configured Top-K call uses compile-time k=1.*advisory, no automatic fix"
	if err != nil {
		return 0, err
	}
	return indices[0], nil
}

func pointerMethodExpressionPositive(device *DeviceBuffer, vocab int) int32 {
	indices, _, _ := (*DeviceBuffer).TopKN(device, vocab, 1) // want "resident TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func pointerAliasMethodExpressionPositive(device *DeviceAlias, vocab int) int32 {
	indices, _, _ := (*DeviceAlias).TopKN(device, vocab, 1) // want "resident TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func valueMethodExpressionPositive(device ValueBuffer, vocab int) int32 {
	indices, _, _ := ValueBuffer.TopKN(device, vocab, 1) // want "value TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func methodExpressionDynamicK(device *DeviceBuffer, k int) int32 {
	indices, _, _ := (*DeviceBuffer).TopKN(device, 1, k)
	return indices[0]
}

func functionPositive(values []float32) (int, error) {
	const greedyK = 1
	indices, err := TopK(values, greedyK) // want "host TopK: configured Top-K call uses compile-time k=1.*first-index tie.*backend-error contracts"
	if err != nil {
		return 0, err
	}
	consume := indices[0]
	_ = indices[0]
	return consume, nil
}

func unicodeFunctionPositive(values []float32) int {
	indices, _ := TópK(values, 1) // want "unicode TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func unicodeMethodPositive(buffer *Búffer, vocab int) int {
	var indices, _ = buffer.TópKN(vocab, 1) // want "unicode method TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func scalarCallPositive(values []float32) {
	indices, _ := TopK(values, 1) // want "host TopK: configured Top-K call uses compile-time k=1"
	_ = append([]int(nil), indices[0])
	delete(map[int]bool{}, indices[0])
}

func valueReceiverMethodPositive(device *DeviceBuffer, vocab int) int32 {
	indices, _ := device.NamedTopKN(vocab, 1) // want "named TopKN: configured Top-K call uses compile-time k=1"
	return indices[0].Int32()
}

func mixedFreshShortDeclarationPositive(values []float32) (int, error) {
	var err error
	indices, err := TopK(values, 1) // want "host TopK: configured Top-K call uses compile-time k=1"
	return indices[0], err
}

func genericFunctionPositive(values []int32) int32 {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericConstraintPositive[T EmbeddedInteger](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericNarrowConstraintPositive[T NarrowInteger](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericExactNamedConstraintPositive[T ExactToken](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericIntegerMethodConstraintPositive[T IntegerWithMethod](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericExactMethodConstraintPositive[T ExactTokenWithMethod](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericComparableNarrowedPositive[T ComparableIntegerOrBytes](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericPointerMethodNarrowedPositive[T IntegerOrPointerWithMethod](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericStructMethodNarrowedPositive[T IntegerOrConflictingStructMethod](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericExactIntersectionPositive[T ExactTokenIntersection](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func explicitGenericFunctionPositive(values []int32) int32 {
	indices, _ := GenericTopK[int32](values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericMethodPositive(device *GenericBuffer[int32], vocab int) int32 {
	indices, _, _ := device.TopKN(vocab, 1) // want "generic method TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericConstraintMethodPositive[T EmbeddedInteger](device *GenericBuffer[T], vocab int) T {
	indices, _, _ := device.TopKN(vocab, 1) // want "generic method TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericComparableNarrowedMethodPositive[T ComparableIntegerOrBytes](device *GenericAnyBuffer[T], vocab int) T {
	indices, _, _ := device.TopKN(vocab, 1) // want "generic any method TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericMethodExpressionPositive(device *GenericBuffer[int32], vocab int) int32 {
	indices, _, _ := (*GenericBuffer[int32]).TopKN(device, vocab, 1) // want "generic method TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericIncompatibleInstantiation(values []float32) float32 {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericAnyConstraint[T any](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericComparableConstraint[T comparable](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericMixedConstraint[T MixedScalar](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericStringConstraint[T StringScalar](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericEmptyConstraint[T EmptyIntegerSet](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericImpossibleExactConstraint[T ImpossibleExactInteger](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericComparableStringConstraint[T ComparableStringOrBytes](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericAllComparableTermsInfeasible[T ComparableContainers](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericPointerMethodInfeasible[T PointerWithMethod](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerAlternative[T IntegerOrExistingPointerMethod](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerOnly[T ExistingPointerMethod](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExactPointerWithMethod[T ExactPointerWithMethod](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerMissingMethod[T ExistingPointerMissingMethod](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerInstantiation(values []*TokenID) *TokenID {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerSelf[T interface {
	~int32 | ~*TokenID
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerSelfOnly[T interface {
	~*TokenID
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerSliceSelf[T interface {
	~int32 | ~*TokenID
	SelfSlice() []T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerBoxSelf[T interface {
	~int32 | ~*TokenID
	SelfBox() SelfBox[T]
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericExistingPointerParameterSelf[T interface {
	~int32 | ~*TokenID
	AcceptSelf(T)
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericMismatchedPointerSelfPositive[T interface {
	~int32 | ~*TokenID
	ValueSelf() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericExactIntegerSelfPositive[T interface {
	TokenID
	ExactSelf() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericUnknownExternalMethod[E any, T interface {
	~int32
	AcceptExternal(E)
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericImpossibleDirectSelfPositive[T interface {
	~int32 | ~struct{ Value T }
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericImpossibleArraySelfPositive[T interface {
	~int32 | ~[1]T
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericImpossibleExactSelfPositive[T interface {
	TokenID | struct{ Value T }
	ExactSelf() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericImpossibleNamedSelfPositive[T interface {
	~int32 | ~struct{ Value DirectNode[T] }
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func genericGuardedNamedSelf[T interface {
	~int32 | ~struct{ Value GuardedNode[T] }
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericGuardedPointerSelf[T interface {
	~int32 | ~struct{ Value *T }
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericGuardedSliceSelf[T interface {
	~int32 | ~[]T
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericGuardedArrayPointerSelf[T interface {
	~int32 | ~[1]*T
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericGuardedMapSelf[T interface {
	~int32 | ~map[string]T
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericGuardedChannelSelf[T interface {
	~int32 | ~chan T
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericGuardedFunctionSelf[T interface {
	~int32 | ~func() T
	Self() T
}](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericMethodCapableContainers[T IntegerOrMethodCapableContainers](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericMethodCapableStruct[T IntegerOrMethodCapableStruct](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericImpossibleExactIntersection[T ImpossibleExactIntersection](values []T) T {
	indices, _ := GenericTopK(values, 1)
	return indices[0]
}

func genericAnyConstraintMethod[T any](device *GenericAnyBuffer[T], vocab int) T {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[0]
}

func genericMixedConstraintMethod[T MixedScalar](device *GenericAnyBuffer[T], vocab int) T {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[0]
}

func genericMethodIncompatibleInstantiation(device *GenericAnyBuffer[float32], vocab int) float32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[0]
}

func genericMethodExpressionIncompatibleInstantiation(device *GenericAnyBuffer[float32], vocab int) float32 {
	indices, _, _ := (*GenericAnyBuffer[float32]).TopKN(device, vocab, 1)
	return indices[0]
}

func varDeclarationPositive(values []float32) (int, error) {
	var indices, err = TopK(values, 1) // want "host TopK: configured Top-K call uses compile-time k=1"
	if err != nil {
		return 0, err
	}
	return indices[0], nil
}

func groupedVarDeclarationPositive(values []float32) (int, error) {
	var (
		indices, err = TopK(values, 1) // want "host TopK: configured Top-K call uses compile-time k=1"
	)
	return indices[0], err
}

func varGenericParenthesizedPositive(values []int32) int32 {
	var indices, _ = (GenericTopK[int32](values, 1)) // want "generic TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func varGenericMethodExpressionPositive(device *GenericBuffer[int32], vocab int) int32 {
	var indices, _, _ = (*GenericBuffer[int32]).TopKN(device, vocab, 1) // want "generic method TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func varNamedAliasResultAndErrorPositive(device *DeviceBuffer, vocab int) (TokenID, error) {
	var indices, err = device.AliasTopKN(vocab, 1) // want "alias TopKN: configured Top-K call uses compile-time k=1"
	return indices[0], err
}

func varExplicitTypePositive(values []float32) int {
	var indices []int = SingleTopK(values, 1) // want "single TopK: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func varRetainsValues(device *DeviceBuffer, vocab int) float32 {
	var indices, values, _ = device.TopKN(vocab, 1)
	_ = indices[0]
	return values[0]
}

func varBlankIndices(values []float32) error {
	var _, err = TopK(values, 1)
	return err
}

func varMultiRHS(values []float32) int {
	var indices, other = SingleTopK(values, 1), 0
	return indices[0] + other
}

func shortDeclarationIndicesNotFresh(values []float32) (int, error) {
	var indices []int
	indices, err := TopK(values, 1)
	return indices[0], err
}

func varCapturesSlice(values []float32) func() int {
	var indices, _ = TopK(values, 1)
	return func() int { return indices[0] }
}

func varNestedClosurePositive(values []float32) func() int {
	return func() int {
		var indices, _ = TopK(values, 1) // want "host TopK: configured Top-K call uses compile-time k=1"
		return indices[0]
	}
}

var packageVarIndices, _ = TopK(nil, 1)

func packageVarRead() int { return packageVarIndices[0] }

func equivalentConstants(device *DeviceBuffer, vocab int) (TokenID, error) {
	const greedyK = 2 - 1
	const first = 3 - 3
	indices, err := device.NamedTopKN((vocab), (greedyK)) // want "named TopKN: configured Top-K call uses compile-time k=1"
	if err != nil {
		return 0, err
	}
	return (indices)[first], nil
}

func dynamicK(device *DeviceBuffer, vocab, k int) int32 {
	indices, _, _ := device.TopKN(vocab, k)
	return indices[0]
}

func topTwo(device *DeviceBuffer, vocab int) int32 {
	indices, _, _ := device.TopKN(vocab, 2)
	return indices[0]
}

func retainsValues(device *DeviceBuffer, vocab int) float32 {
	indices, values, _ := device.TopKN(vocab, 1)
	_ = indices[0]
	return values[0]
}

func readsAnotherRank(device *DeviceBuffer, vocab int) int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[1]
}

func rankAliasIsStillAnotherRank(device *DeviceBuffer, vocab int) int32 {
	const second = 1
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[second]
}

func observesLength(device *DeviceBuffer, vocab int) int {
	indices, _, _ := device.TopKN(vocab, 1)
	_ = indices[0]
	return len(indices)
}

func returnsSlice(device *DeviceBuffer, vocab int) []int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	_ = indices[0]
	return indices
}

func passesSlice(device *DeviceBuffer, vocab int) {
	indices, _, _ := device.TopKN(vocab, 1)
	_ = indices[0]
	consumeIndices(indices)
}

func slicesResult(device *DeviceBuffer, vocab int) []int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[:1]
}

func mutatesFirst(device *DeviceBuffer, vocab int) {
	indices, _, _ := device.TopKN(vocab, 1)
	indices[0] = 7
}

func incrementsFirst(device *DeviceBuffer, vocab int) {
	indices, _, _ := device.TopKN(vocab, 1)
	indices[0]++
}

func compoundAssignmentMutatesFirst(values []float32) int {
	indices, _ := TopK(values, 1)
	indices[0] += 2
	return indices[0]
}

func multiAssignmentMutatesFirst(values []float32) int {
	indices, _ := TopK(values, 1)
	indices[0], values[0] = 2, 3
	return indices[0]
}

func rangeKeyMutatesFirst(values, ranged []float32) int {
	indices, _ := TopK(values, 1)
	for indices[0] = range ranged {
	}
	return indices[0]
}

func rangeValueMutatesFirst(values []float32, ranged []int) int {
	indices, _ := TopK(values, 1)
	for _, indices[0] = range ranged {
	}
	return indices[0]
}

func parenthesizedRangeValueMutatesFirst(values []float32, ranged []int) int {
	indices, _ := TopK(values, 1)
	for _, (indices[0]) = range ranged {
	}
	return indices[0]
}

func selectReceiveMutatesFirst(values []float32, incoming <-chan int) int {
	indices, _ := TopK(values, 1)
	select {
	case indices[0] = <-incoming:
	default:
	}
	return indices[0]
}

func appendEscapesSlice(values []float32) int {
	indices, _ := TopK(values, 1)
	indices = append(indices, 2)
	return indices[0]
}

func copyEscapesSlice(values []float32, destination []int) int {
	indices, _ := TopK(values, 1)
	copy(destination, indices)
	return indices[0]
}

func rangesOverSlice(values []float32) int {
	indices, _ := TopK(values, 1)
	for index := range indices {
		return index
	}
	return -1
}

func takesFirstAddress(device *DeviceBuffer, vocab int) *int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return &indices[0]
}

func implicitAddressThroughMethod(device *DeviceBuffer, vocab int) {
	indices, _ := device.NamedTopKN(vocab, 1)
	indices[0].Reset()
}

func capturesSlice(device *DeviceBuffer, vocab int) func() int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return func() int32 { return indices[0] }
}

var packageClosureCandidate = func(device *DeviceBuffer, vocab int) int32 {
	indices, _, _ := device.TopKN(vocab, 1) // want "resident TopKN: configured Top-K call uses compile-time k=1"
	return indices[0]
}

func nestedClosureCandidate(device *DeviceBuffer, vocab int) func() int32 {
	return func() int32 {
		indices, _, _ := device.TopKN(vocab, 1) // want "resident TopKN: configured Top-K call uses compile-time k=1"
		return indices[0]
	}
}

func sameSpellingWrongType(device *OtherBuffer, vocab int) int32 {
	indices, _, _ := device.TopKN(vocab, 1)
	return indices[0]
}

func assignmentNotFresh(device *DeviceBuffer, vocab int) int32 {
	var indices []int32
	indices, _, _ = device.TopKN(vocab, 1)
	return indices[0]
}

func configuredResultIsNotInteger(device *DeviceBuffer, vocab int) float32 {
	_, values, _ := device.TopKN(vocab, 1)
	return values[0]
}

func unusedIndices(device *DeviceBuffer, vocab int) error {
	_, _, err := device.TopKN(vocab, 1)
	return err
}
