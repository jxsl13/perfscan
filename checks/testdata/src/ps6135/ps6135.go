package ps6135

type buffer interface{}
type recorder interface {
	Binary(a, b, o buffer, op int) error
}
type binaryNRecorder interface {
	BinaryN(a, b, o buffer, op, n int) error
}
type Decoder struct{}

func (d *Decoder) binElem(r recorder, a, b, o buffer, op, rows, width int) error {
	if bn, ok := r.(binaryNRecorder); ok && rows > 0 && width > 0 {
		return bn.BinaryN(a, b, o, op, rows*width)
	}
	return r.Binary(a, b, o, op) // want `for positive active rows/width`
}

func (d *Decoder) unconfigured(r recorder, a, b, o buffer, op, rows, width int) error {
	if bn, ok := r.(binaryNRecorder); ok && rows > 0 && width > 0 {
		return bn.BinaryN(a, b, o, op, rows*width)
	}
	return r.Binary(a, b, o, op)
}
