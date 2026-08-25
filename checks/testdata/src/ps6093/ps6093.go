package ps6093

type Tensor struct{ n int }
type UnsignedTensor struct{ n uint }
type NarrowTensor struct{ n int8 }
type ByteTensor struct{ n uint8 }
type CountTensor struct{ n MutableCount }

func (t Tensor) Numel() int               { return t.n }
func (t Tensor) Len() int                 { return t.n }
func (t Tensor) Size() int                { return t.n }
func (t UnsignedTensor) Numel() uint      { return t.n }
func (t NarrowTensor) Numel() int8        { return t.n }
func (t ByteTensor) Numel() uint8         { return t.n }
func (t CountTensor) Numel() MutableCount { return t.n }
func (t Tensor) SizeAt(int) int {
	return t.n
}

type Sizer interface {
	Size() int
}

type Named []float64
type Alias = []float64
type MutableBytes []byte
type MutableCount int
type Predicate func() bool

var packageStorage []byte
var packageStoragePointer = &packageStorage
var packageStorageTrigger = make(chan struct{})
var packageStorageDone = make(chan struct{})
var packageStorageStream = make(chan struct{})
var packageStorageCloseTrigger = make(chan struct{})
var packageSink byte

func (n Named) Len() int        { return len(n) }
func (m *MutableBytes) Reset()  { *m = nil }
func (m MutableBytes) Observe() {}
func (m *MutableCount) Reset()  { *m = 0 }
func (m *MutableCount) Advance() bool {
	(*m)++
	return true
}

func advanceIndex(index *int) bool {
	*index += 2
	return true
}

func observeIndex(index int) bool {
	return index >= 0
}

func invokePredicate(predicate Predicate) bool {
	return predicate()
}

func rangeCount(t Tensor, in, out []float64) {
	n := t.Numel()
	for i := range n {
		out[i] = in[i] // want `slice out is indexed by i` `slice in is indexed by i`
	}
}

func counted(t Tensor, values Named) {
	n := t.Len()
	for i := 0; i < n; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func countedLessEqual(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i <= n-1; i++ {
		_ = values[i]
	}
}

func unsignedCounted(t UnsignedTensor, values []byte) {
	n := t.Numel()
	for i := uint(0); i < n; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func unsignedLessEqual(t UnsignedTensor, values []byte) {
	n := t.Numel()
	for i := uint(0); i <= n-1; i++ {
		_ = values[i]
	}
}

func countedReversed(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; n > i; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func multipleUpperBounds(t Tensor, values []byte, limit int) {
	n := t.Numel()
	for i := 0; i < limit && i < n; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func multipleUpperBoundsReversed(t Tensor, values []byte, limit int) {
	n := t.Numel()
	for i := 0; i < n && i < limit; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func multipleUpperBoundsWithLengthProof(t Tensor, values []byte, limit int) {
	n := t.Numel()
	for i := 0; i < limit && i < n && i < len(values); i++ {
		_ = values[i]
	}
}

func conditionAddressEscapesIndex(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && advanceIndex(&i); i++ {
		_ = values[i]
	}
}

func conditionDirectlyMutatesIndex(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && func() bool {
		i++
		return true
	}(); i++ {
		_ = values[i]
	}
}

func conditionUninvokedLiteralDoesNotMutateIndex(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && (func() bool {
		i++
		return true
	}) != nil; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func conditionConvertedUninvokedLiteralDoesNotMutateIndex(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && Predicate(func() bool {
		i++
		return true
	}) != nil; i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func conditionPassedLiteralMayMutateIndex(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && invokePredicate(func() bool {
		i++
		return true
	}); i++ {
		_ = values[i]
	}
}

func conditionImplicitPointerMutatesIndex(t CountTensor, values []byte) {
	n := t.Numel()
	for i := MutableCount(0); i < n && i.Advance(); i++ {
		_ = values[i]
	}
}

func conditionPureIndexRead(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && observeIndex(i); i++ {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func multipleMethodBoundsProofSecond(a, b Tensor, values []byte) {
	n := a.Numel()
	m := b.Numel()
	if len(values) < m {
		return
	}
	for i := 0; i < n && i < m; i++ {
		_ = values[i]
	}
}

func multipleMethodBoundsProofFirstAfterReorder(a, b Tensor, values []byte) {
	n := a.Numel()
	m := b.Numel()
	if len(values) < m {
		return
	}
	for i := 0; i < m && i < n; i++ {
		_ = values[i]
	}
}

func manyMethodBoundsRemainBudgeted(t Tensor, values []byte) {
	n0 := t.Numel()
	n1 := t.Numel()
	n2 := t.Numel()
	n3 := t.Numel()
	n4 := t.Numel()
	n5 := t.Numel()
	n6 := t.Numel()
	n7 := t.Numel()
	n8 := t.Numel()
	n9 := t.Numel()
	n10 := t.Numel()
	n11 := t.Numel()
	for i := 0; i < n0 && i < n1 && i < n2 && i < n3 && i < n4 && i < n5 &&
		i < n6 && i < n7 && i < n8 && i < n9 && i < n10 && i < n11; i++ {
		_ = values[i] // want `slice values is indexed by i`
		_ = values[i]
		_ = values[i]
		_ = values[i]
	}
}

func methodExpression(t Tensor, values Alias) {
	n := Tensor.Size(t)
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func interfaceReceiver(t Sizer, values []byte) {
	n := t.Size()
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func affine(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = values[2*i+1] // want `slice values is indexed by 2\*i\+1`
	}
}

func affineResliced(t Tensor, values []byte) {
	n := t.Numel()
	values = values[:2*n]
	for i := range n {
		_ = values[2*i+1]
	}
}

func genericSlice[S ~[]byte](t Tensor, values S) {
	n := t.Numel()
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func closureBody(t Tensor, values []byte) {
	func() {
		n := t.Numel()
		for i := range n {
			_ = values[i] // want `slice values is indexed by i`
		}
	}()
}

func sliceReceiver(values Named) {
	n := values.Len()
	for i := range n {
		_ = values[i]
	}
}

func pointerSliceReceiver(values *Named) {
	n := values.Len()
	for i := range n {
		_ = (*values)[i]
	}
}

func freeSize(Tensor) int { return 1 }

func freeFunction(t Tensor, values []byte) {
	n := freeSize(t)
	for i := range n {
		_ = values[i]
	}
}

func methodWithArgument(t Tensor, values []byte) {
	n := t.SizeAt(0)
	for i := range n {
		_ = values[i]
	}
}

func nonCanonicalStart(t Tensor, values []byte) {
	n := t.Numel()
	for i := 1; i < n; i++ {
		_ = values[i]
	}
}

func existingRangeIndex(t Tensor, values []byte) {
	n := t.Numel()
	var i int
	pointer := &i
	_ = pointer
	for i = range n {
		_ = values[i]
	}
}

func existingCountedIndex(t Tensor, values []byte) {
	n := t.Numel()
	var i int
	pointer := &i
	_ = pointer
	for i = 0; i < n; i++ {
		_ = values[i]
	}
}

func constantIndex(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = i
		_ = values[0]
	}
}

func negativeAffine(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = values[i-1]
	}
}

func selfReslice(t Tensor, values []byte) {
	n := t.Numel()
	values = values[:n]
	for i := range n {
		_ = values[i]
	}
}

func zeroLowReslice(t Tensor, values []byte) {
	n := t.Numel()
	values = values[0:n]
	for i := range n {
		_ = values[i]
	}
}

func nonzeroLowReslice(t Tensor, values []byte) {
	n := t.Numel()
	values = values[1:n]
	for i := range n {
		_ = values[i]
	}
}

func freshNonzeroLowReslice(t Tensor, values []byte) {
	n := t.Numel()
	view := values[1:n]
	for i := range n {
		_ = view[i] // want `slice view is indexed by i`
	}
}

func freshMake(t Tensor) {
	n := t.Numel()
	view := make([]byte, n)
	for i := range n {
		_ = view[i]
	}
}

func freshAffineMake(t Tensor) {
	n := t.Numel()
	view := make([]byte, 2*n)
	for i := range n {
		_ = view[2*i+1] // want `slice view is indexed by 2\*i\+1`
	}
}

func freshAffineReslice(t Tensor, values []byte) {
	n := t.Numel()
	view := values[:2*n]
	for i := range n {
		_ = view[2*i+1] // want `slice view is indexed by 2\*i\+1`
	}
}

func narrowAffineReslice(t NarrowTensor, values []byte) {
	n := t.Numel()
	view := values[:4*n]
	for i := range n {
		_ = view[4*i] // want `slice view is indexed by 4\*i`
	}
}

func narrowAffineMake(t NarrowTensor) {
	n := t.Numel()
	view := make([]byte, 4*n)
	for i := range n {
		_ = view[4*i] // want `slice view is indexed by 4\*i`
	}
}

func offsetReslice(t Tensor, values []byte) {
	n := t.Numel()
	view := values[:n+2]
	for i := range n {
		_ = view[i] // want `slice view is indexed by i`
	}
}

func makeCapacityIsNotLength(t Tensor) {
	n := t.Numel()
	view := make([]byte, n, 2*n)
	for i := range n {
		_ = view[2*i] // want `slice view is indexed by 2\*i`
	}
}

func fakeMake(t Tensor, values []byte) {
	make := func([]byte, int) []byte { return values }
	n := t.Numel()
	view := make(values, n)
	for i := range n {
		_ = view[i] // want `slice view is indexed by i`
	}
}

func freshReslice(t Tensor, values []byte) {
	n := t.Numel()
	view := values[:n]
	for i := range n {
		_ = view[i]
	}
}

func oneSliceProven(t Tensor, in, out []byte) {
	n := t.Numel()
	in = in[:n]
	for i := range n {
		out[i] = in[i] // want `slice out is indexed by i`
	}
}

func lengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	if len(values) < n {
		return
	}
	for i := range n {
		_ = values[i]
	}
}

func reverseLengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	if n > len(values) {
		panic("shape")
	}
	for i := range n {
		_ = values[i]
	}
}

func enclosingLengthProof(t Tensor, values []byte) {
	n := t.Numel()
	if n <= len(values) {
		for i := range n {
			_ = values[i]
		}
	}
}

func equalLengthProof(t Tensor, values []byte) {
	n := t.Numel()
	if len(values) == n {
		for i := range n {
			_ = values[i]
		}
	}
}

func unequalLengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	if len(values) != n {
		return
	}
	for i := range n {
		_ = values[i]
	}
}

func affineLengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	if len(values) < 2*n {
		return
	}
	for i := range n {
		_ = values[2*i+1] // want `slice values is indexed by 2\*i\+1`
	}
}

func offsetLengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	if len(values) < n+2 {
		return
	}
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func convertedLengthGuard(t UnsignedTensor, values []byte) {
	n := t.Numel()
	if uint(len(values)) < n {
		return
	}
	for i := range n {
		_ = values[i]
	}
}

func narrowingLengthGuard(t ByteTensor, values []byte) {
	n := t.Numel()
	if uint8(len(values)) < n {
		return
	}
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func headerIndexProof(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && i < len(values); i++ {
		_ = values[i]
	}
}

func headerIndexProofLessEqual(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n && i <= len(values)-1; i++ {
		_ = values[i]
	}
}

func bodyIndexProof(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		if i >= len(values) {
			break
		}
		_ = values[i]
	}
}

func breakGuardDoesNotProveLaterLoop(t Tensor, values []byte) {
	n := t.Numel()
	for {
		if len(values) < n {
			break
		}
	}
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func affineBodyIndexGuard(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		if 2*i+1 >= len(values) {
			break
		}
		_ = values[2*i+1] // want `slice values is indexed by 2\*i\+1`
	}
}

func jumpedLengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	goto loop
	if len(values) < n {
		return
	}
loop:
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func gotoOutOfGuard(t Tensor, values []byte) {
	n := t.Numel()
	if len(values) < n {
		goto loop
		return
	}
loop:
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func nestedGotoOutOfGuard(t Tensor, values []byte, jump bool) {
	n := t.Numel()
	if len(values) < n {
		if jump {
			goto loop
		}
		panic("shape")
	}
loop:
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func unstableBound(t Tensor, values []byte) {
	n := t.Numel()
	for i := 0; i < n; i++ {
		n = len(values)
		_ = values[i]
	}
}

func unstableSlice(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		values = values[:0]
		_ = values[i]
	}
}

func addressTaken(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = &values
		_ = values[i]
	}
}

func addressTakenBeforeLoop(t Tensor, values []byte) {
	n := t.Numel()
	pointer := &values
	_ = pointer
	for i := range n {
		_ = values[i]
	}
}

func parenthesizedUnstableBound(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		(n) = len(values)
		_ = values[i]
	}
}

func parenthesizedUnstableSlice(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		(values) = values[:0]
		_ = values[i]
	}
}

func parenthesizedAddressTaken(t Tensor, values []byte) {
	n := t.Numel()
	pointer := &(values)
	_ = pointer
	for i := range n {
		_ = values[i]
	}
}

func resetPackageStorage() {
	packageStorage = nil
}

func consumeByte(byte) {}

func resetPackageStorageValue() int {
	packageStorage = nil
	return 0
}

func resetPackageStorageTrue() bool {
	packageStorage = nil
	return true
}

func packageStorageWorker() {
	for range packageStorageTrigger {
		packageStorage = nil
		packageStorageDone <- struct{}{}
	}
}

func packageStorageStreamWorker() {
	packageStorage = nil
	packageStorageStream <- struct{}{}
	close(packageStorageStream)
}

func packageStorageIterator(yield func(int) bool) {
	packageStorage = nil
	yield(0)
}

func packageSliceLocalProofIsInsufficient(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	resetPackageStorage()
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceProofStalesAfterFirstAccess(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
		resetPackageStorage()
	}
}

func packageSliceRecurringGuardSurvivesLaterMutation(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		_ = packageStorage[i]
		resetPackageStorage()
	}
}

func packageSliceGuardIgnoresEffectOnExitingPath(t Tensor, stop bool) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		if stop {
			resetPackageStorage()
			return
		}
		_ = packageStorage[i]
	}
}

func packageSliceGuardIgnoresEffectBeforeNextProof(t Tensor, stop bool) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		if stop {
			resetPackageStorage()
			continue
		}
		_ = packageStorage[i]
	}
}

func packageSliceGuardRejectsEffectReachingAccess(t Tensor, stop bool) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		if stop {
			resetPackageStorage()
		}
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceAssignmentRHSAccessOnly(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		packageStorage = append(packageStorage, packageStorage[i])
	}
}

func packageSliceMultiAssignmentRHSAccessOnly(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		packageStorage, packageSink = packageStorage[:0], packageStorage[i]
	}
}

func packageSliceAssignmentBeforeAccess(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		packageStorage = packageStorage[:0]
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceNestedLoopBackedgeStalesGuard(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i < len(packageStorage) {
			for j := 0; j < 2; j++ {
				_ = packageStorage[i] // want `slice packageStorage is indexed by i`
				resetPackageStorage()
			}
		}
	}
}

func packageSliceGotoBackedgeStalesGuard(t Tensor, jump bool) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		if jump {
			goto mutate
		}
	access:
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
		continue
	mutate:
		resetPackageStorage()
		goto access
	}
}

func packageSliceGotoBackedgeWithoutEffect(t Tensor, jump bool) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		if jump {
			goto harmless
		}
	access:
		_ = packageStorage[i]
		continue
	harmless:
		_ = jump
		goto access
	}
}

func packageSliceBackwardGotoEffectAfterProof(t Tensor, jump bool) {
	n := t.Numel()
	for i := range n {
		goto proof
	mutate:
		resetPackageStorage()
		goto access
	proof:
		if i >= len(packageStorage) {
			break
		}
		if jump {
			goto mutate
		}
	access:
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceBackwardGotoWithoutEffectAfterProof(t Tensor, jump bool) {
	n := t.Numel()
	for i := range n {
		goto proof
	harmless:
		_ = jump
		goto access
	proof:
		if i >= len(packageStorage) {
			break
		}
		if jump {
			goto harmless
		}
	access:
		_ = packageStorage[i]
	}
}

func packageSliceReversedGuardStalesBeforeAccess(t Tensor) {
	n := t.Numel()
	for i := range n {
		goto guard
	access:
		resetPackageStorage()
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
		continue
	guard:
		if i >= len(packageStorage) {
			break
		}
		goto access
	}
}

func packageSliceImmediateResliceProof(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSlicePreLoopGuardIsInsufficient(t Tensor) {
	n := t.Numel()
	if len(packageStorage) < n {
		return
	}
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceHeaderProof(t Tensor) {
	n := t.Numel()
	for i := 0; i < n && i < len(packageStorage); i++ {
		_ = packageStorage[i]
	}
}

func packageSliceCallBeforeHeaderProof(t Tensor) {
	n := t.Numel()
	for i := 0; i < n && resetPackageStorageTrue() && i < len(packageStorage); i++ {
		_ = packageStorage[i]
	}
}

func packageSliceCallAfterHeaderProof(t Tensor) {
	n := t.Numel()
	for i := 0; i < n && i < len(packageStorage) && resetPackageStorageTrue(); i++ {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceBuiltinDoesNotRewriteHeader(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	clear(packageStorage)
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceEnclosingCallRunsAfterAccess(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	for i := range n {
		consumeByte(packageStorage[i]) // want `slice packageStorage is indexed by i`
	}
}

func packageSliceDeferredMutationRunsAfterAccess(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	defer resetPackageStorage()
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceDeferredArgumentRunsImmediately(t Tensor) {
	n := t.Numel()
	packageStorage = packageStorage[:n]
	defer func(int) {}(resetPackageStorageValue())
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceIndirectWriteInvalidatesHeaderProof(t Tensor) {
	n := t.Numel()
	for i := 0; i < n && i < len(packageStorage); i++ {
		*packageStoragePointer = nil
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceSynchronizationInvalidatesHeaderProof(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		packageStorageTrigger <- struct{}{}
		<-packageStorageDone
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceChannelRangeInvalidatesOuterGuard(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		for range packageStorageStream {
			_ = packageStorage[i] // want `slice packageStorage is indexed by i`
		}
	}
}

func packageSliceIteratorRangeInvalidatesOuterGuard(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		for range packageStorageIterator {
		}
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceGenericChannelRangeInvalidatesOuterGuard[C ~chan struct{}](t Tensor, stream C) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		for range stream {
		}
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceGenericSliceRangePreservesOuterGuard[S ~[]int](t Tensor, values S) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		for range values {
		}
		_ = packageStorage[i]
	}
}

func packageSliceCloseInvalidatesHeaderProof(t Tensor) {
	n := t.Numel()
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		close(packageStorageCloseTrigger)
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func packageSliceDeferredClosureRunsAfterGuardedAccess(t Tensor) byte {
	n := t.Numel()
	defer func() { packageStorage = nil }()
	var result byte
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		result ^= packageStorage[i]
	}
	return result
}

func packageSliceUncalledClosureDoesNotStaleGuard(t Tensor) byte {
	n := t.Numel()
	_ = func() { packageStorage = nil }
	var result byte
	for i := range n {
		if i >= len(packageStorage) {
			break
		}
		result ^= packageStorage[i]
	}
	return result
}

func packageSliceEscapedHeaderIsUnproven(t Tensor) {
	n := t.Numel()
	pointer := &(packageStorage)
	_ = pointer
	packageStorage = packageStorage[:n]
	for i := range n {
		_ = packageStorage[i] // want `slice packageStorage is indexed by i`
	}
}

func capturedHeaderWrite(t Tensor, values []byte) {
	n := t.Numel()
	reset := func() { values = nil }
	for i := range n {
		if i == 10 {
			reset()
		}
		_ = values[i]
	}
}

func changedInduction(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		i = n
		_ = values[i]
	}
}

func pointerReceiverSlice(t Tensor, values MutableBytes) {
	n := t.Numel()
	for i := range n {
		values.Reset()
		_ = values[i]
	}
}

func pointerMethodValueSlice(t Tensor, values MutableBytes) {
	reset := values.Reset
	_ = reset
	n := t.Numel()
	for i := range n {
		_ = values[i]
	}
}

func pointerMethodValueBound(t CountTensor, values []byte) {
	n := t.Numel()
	reset := n.Reset
	_ = reset
	for i := range n {
		_ = values[i]
	}
}

func valueReceiverSlice(t Tensor, values MutableBytes) {
	n := t.Numel()
	values.Observe()
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func pointerReceiverBound(t CountTensor, values []byte) {
	n := t.Numel()
	for i := range n {
		n.Reset()
		_ = values[i]
	}
}

func pointerReceiverIndex(t CountTensor, values []byte) {
	n := t.Numel()
	for i := range n {
		i.Reset()
		_ = values[i]
	}
}

func dead(t Tensor, values []byte) {
	if false {
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadShortCircuit(t Tensor, values []byte, dynamic bool) {
	if false && dynamic {
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadOuterLoop(t Tensor, values []byte) {
	for false {
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadOuterRange(t Tensor, values []byte) {
	for range 0 {
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadEmptyStringRange(t Tensor, values []byte) {
	for range "" {
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadEmptyArrayRange(t Tensor, values []byte) {
	for range [0]int{} {
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadSwitchCase(t Tensor, values []byte) {
	switch 1 {
	case 2:
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func liveSwitchCase(t Tensor, values []byte) {
	switch 1 {
	case 1:
		n := t.Numel()
		for i := range n {
			_ = values[i] // want `slice values is indexed by i`
		}
	}
}

func liveSwitchFallthrough(t Tensor, values []byte) {
	switch 1 {
	case 1:
		fallthrough
	case 2:
		n := t.Numel()
		for i := range n {
			_ = values[i] // want `slice values is indexed by i`
		}
	}
}

func liveSwitchDefault(t Tensor, values []byte) {
	switch 1 {
	default:
		n := t.Numel()
		for i := range n {
			_ = values[i] // want `slice values is indexed by i`
		}
	}
}

func deadSwitchDefault(t Tensor, values []byte) {
	switch 1 {
	case 1:
	default:
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func deadTaglessSwitch(t Tensor, values []byte) {
	switch {
	case false:
		n := t.Numel()
		for i := range n {
			_ = values[i]
		}
	}
}

func switchCaseLengthGuard(t Tensor, values []byte, tag int) {
	switch tag {
	case 1:
		n := t.Numel()
		if len(values) < n {
			return
		}
		for i := range n {
			_ = values[i]
		}
	}
}

func switchFallthroughLengthGuard(t Tensor, values []byte) {
	n := t.Numel()
	switch 1 {
	case 1:
		if len(values) < n {
			return
		}
		fallthrough
	case 2:
		for i := range n {
			_ = values[i]
		}
	}
}

func conditionalSwitchFallthroughGuard(t Tensor, values []byte, check bool) {
	n := t.Numel()
	switch 1 {
	case 1:
		if check {
			if len(values) < n {
				return
			}
		}
		fallthrough
	case 2:
		for i := range n {
			_ = values[i] // want `slice values is indexed by i`
		}
	}
}

func impossibleReceiver[T interface {
	[]byte
	Numel() int
}](t T, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = values[i]
	}
}

func impossibleUnionReceiver[T interface {
	[]byte | []rune
	Numel() int
}](t T, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = values[i]
	}
}

func impossibleIntersectionReceiver[T interface {
	~[]byte
	int
	Numel() int
}](t T, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = values[i]
	}
}

func multipleAffineAccesses(t Tensor, values []byte) {
	n := t.Numel()
	for i := range n {
		_ = values[i]     // want `slice values is indexed by i`
		_ = values[2*i+1] // want `slice values is indexed by 2\*i\+1`
	}
}

func reassignedAfterBound(t Tensor, values, other []byte) {
	n := t.Numel()
	values = other
	for i := range n {
		_ = values[i]
	}
}

func unusedSliceExpression(t Tensor, values []byte) {
	n := t.Numel()
	_ = values[:n]
	for i := range n {
		_ = values[i] // want `slice values is indexed by i`
	}
}

func fakeLen(t Tensor, values []byte) {
	len := func([]byte) int { return 1 }
	n := t.Numel()
	if len(values) >= n {
		for i := range n {
			_ = values[i] // want `slice values is indexed by i`
		}
	}
}
