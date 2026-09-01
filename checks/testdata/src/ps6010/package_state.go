package ps6010

var packageOffset int

func setPackageOffset(output int) { packageOffset = output }

type ps6010NamedArray [8]float64

type ps6010GlobalArrayHolder struct {
	values [8]float64
}

var ps6010GlobalArray [8]float64
var ps6010GlobalNamedArray ps6010NamedArray
var ps6010GlobalArrayField ps6010GlobalArrayHolder
var ps6010GlobalArrayElements [2][8]float64
var ps6010EscapedSlice []float64

func mutatePS6010EscapedSlice(output int) {
	if len(ps6010EscapedSlice) != 0 {
		ps6010EscapedSlice[0] = float64(output)
	}
}

func sendPS6010SliceFromOtherFile(channel chan<- []float64, values []float64) {
	channel <- values
}
