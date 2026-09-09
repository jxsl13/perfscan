package ps6085neg

import "sync"

var (
	noMultiplyGrid       [16][8]float32
	wrongLaneGrid        [16][8]float32
	conditionalLaneGrid  [16][8]float32
	duplicateSitesGrid   [16][8]float32
	nonconstantStateGrid [16][8]float32
	stateInsideLaneGrid  [16][8]float32
	sideEffectIndexGrid  [16][8]float32
	laneDependentRowGrid [16][8]float32
	breakLoopGrid        [16][8]float32
	stateEscapeGrid      [16][8]float32
	laneScaleGrid        [16][8]float32
	mutableGrid          [16][8]float32
	escapedGrid          [16][8]float32
	rowCopyMutationGrid  [4][8]float32
	rowCopyAddressGrid   [4][8]float32
	methodMutationGrid   [4]mutableRow
	methodValueGrid      [4]mutableRow
	aggregateGrid        [4][8]float32
	interfaceGrid        [4][8]float32
	laneMutationGrid     [4][8]float32
	rowIndexMutationGrid [4][8]float32
	scaleMutationGrid    [4][8]float32
	laneCaptureGrid      [4][8]float32
	initReadGrid         [4][8]float32
	initCallGrid         [4][8]float32
	roundedStateGrid     [4][8]float32
	rowIndexAddressGrid  [4][8]float32
	scaleAddressGrid     [4][8]float32
	initIIFEGrid         [4][8]float32
	initWrapperGrid      [4][8]float32
	initVarGrid          [4][8]float32
	initCallbackGrid     [4][8]float32
	unreachableGrid      [4][8]float32
	namedIndexGrid       [4][8]float32
	namedScaleGrid       [4][8]mutableScale
	escapedGridAlias     = &escapedGrid
	initReadSeed         = initReadGrid[0][0]
	initVarSeed          = initVarWrapper()
	initCallbackOnce     sync.Once
	nextRow              int
)

type mutableRow [8]float32

type mutableIndex int

type mutableScale float32

func (row *mutableRow) mutate() { row[0]++ }

func (index *mutableIndex) bump() { *index = (*index + 1) & 3 }

func (scale *mutableScale) bump() { *scale += 0.25 }

func init() {
	_ = initReadSeed
	_ = initCallCandidate(0, false, 1)
	func() { _ = initIIFECandidate(0, false, 1) }()
	initWrapperCall()
	initCallbackOnce.Do(initCallbackWrapper)
	initReadGrid[0][0] = 1
	initCallGrid[0][0] = 1
	initIIFEGrid[0][0] = 1
	initWrapperGrid[0][0] = 1
	initVarGrid[0][0] = 1
	initCallbackGrid[0][0] = 1
}

func initWrapperCall() { _ = initWrapperCandidate(0, false, 1) }

func initVarWrapper() float32 { return initVarCandidate(0, false, 1) }

func initCallbackWrapper() { _ = initCallbackCandidate(0, false, 1) }

func noMultiply(rowIndex int, negative bool) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &noMultiplyGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += row[lane] + delta
	}
	return sum
}

func wrongLane(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &wrongLaneGrid[rowIndex&15]
	var sum float32
	for lane := range 7 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func conditionalLane(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &conditionalLaneGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		if lane&1 == 0 {
			sum += scale * (row[lane] + delta)
		}
	}
	return sum
}

func duplicateSites(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &duplicateSitesGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func nonconstantState(rowIndex int, negative bool, scale, seed float32) float32 {
	delta := seed
	if negative {
		delta = -seed
	}
	row := &nonconstantStateGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func stateInsideLane(rowIndex int, negative bool, scale float32) float32 {
	row := &stateInsideLaneGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		delta := float32(0.125)
		if negative {
			delta = float32(-0.125)
		}
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func sideEffectIndex(negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &sideEffectIndexGrid[nextIndex()&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func nextIndex() int {
	nextRow++
	return nextRow
}

func laneDependentRow(negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (laneDependentRowGrid[lane][lane] + delta)
	}
	return sum
}

func breakLoop(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &breakLoopGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		if lane == 4 {
			break
		}
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func stateEscape(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	_ = &delta
	row := &stateEscapeGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func laneDependentScale(rowIndex int, negative bool, scales [8]float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &laneScaleGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scales[lane] * (row[lane] + delta)
	}
	return sum
}

func mutableCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &mutableGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func mutateTable() { mutableGrid[0][0] = 1 }

func escapedCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &escapedGrid[rowIndex&15]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func rowCopyMutation(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := rowCopyMutationGrid[rowIndex&3]
	row[0] = 99
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func mutateRowCopy(row *[8]float32) { row[0] = 99 }

func rowCopyAddress(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := rowCopyAddressGrid[rowIndex&3]
	mutateRowCopy(&row)
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func mutateMethodTable() {
	methodMutationGrid[0].mutate()
}

func methodMutationCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &methodMutationGrid[rowIndex&3]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func mutateMethodValueTable() {
	mutate := methodValueGrid[0].mutate
	mutate()
}

func methodValueCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &methodValueGrid[rowIndex&3]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func mutateAggregateTable() {
	row := &aggregateGrid[0]
	holder := struct{ row *[8]float32 }{row: row}
	holder.row[0] = 1
}

func aggregateCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &aggregateGrid[rowIndex&3]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func mutateInterfaceTable() {
	row := &interfaceGrid[0]
	var escaped any = row
	recovered := escaped.(*[8]float32)
	recovered[0] = 1
}

func interfaceCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	row := &interfaceGrid[rowIndex&3]
	var sum float32
	for lane := range 8 {
		sum += scale * (row[lane] + delta)
	}
	return sum
}

func laneMutation(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		lane = 0
		sum += scale * (laneMutationGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func rowIndexMutation(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		rowIndex = (rowIndex + 1) & 3
		sum += scale * (rowIndexMutationGrid[rowIndex][lane] + delta)
	}
	return sum
}

func scaleMutation(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		scale += 0.25
		sum += scale * (scaleMutationGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func laneCapture(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		func() { _ = lane }()
		sum += scale * (laneCaptureGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func initReadCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (initReadGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func initCallCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (initCallGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func roundedState(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(1)
	if negative {
		delta = float32(1.0000000000000000000000000000001)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (roundedStateGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func bumpInt(value *int) { *value = (*value + 1) & 3 }

func rowIndexAddress(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	alias := &rowIndex
	var sum float32
	for lane := range 8 {
		bumpInt(alias)
		sum += scale * (rowIndexAddressGrid[rowIndex][lane] + delta)
	}
	return sum
}

func bumpFloat(value *float32) { *value += 0.25 }

func scaleAddress(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	alias := &scale
	var sum float32
	for lane := range 8 {
		bumpFloat(alias)
		sum += scale * (scaleAddressGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func initIIFECandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (initIIFEGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func initWrapperCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (initWrapperGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func initVarCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (initVarGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func initCallbackCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		sum += scale * (initCallbackGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func unreachableCandidate(rowIndex int, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	return 0
	var sum float32
	for lane := range 8 {
		sum += scale * (unreachableGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func namedIndexMethod(rowIndex mutableIndex, negative bool, scale float32) float32 {
	delta := float32(0.125)
	if negative {
		delta = float32(-0.125)
	}
	var sum float32
	for lane := range 8 {
		rowIndex.bump()
		sum += scale * (namedIndexGrid[rowIndex&3][lane] + delta)
	}
	return sum
}

func namedScaleMethod(rowIndex int, negative bool, scale mutableScale) mutableScale {
	delta := mutableScale(0.125)
	if negative {
		delta = mutableScale(-0.125)
	}
	var sum mutableScale
	for lane := range 8 {
		scale.bump()
		sum += scale * (namedScaleGrid[rowIndex&3][lane] + delta)
	}
	return sum
}
