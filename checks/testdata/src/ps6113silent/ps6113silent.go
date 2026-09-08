package ps6113silent

type buffer []float32
type binaryOp int

const binaryAdd binaryOp = 1

type recorder interface {
	Binary(buffer, buffer, buffer, binaryOp) error
}

type projection interface {
	record(recorder, buffer, buffer, int) error
	recordAdd(recorder, buffer, buffer, buffer, int, int) error
}

func firstErr(values ...error) error { return nil }

func candidate(p projection, r recorder, source, temporary, destination buffer) error {
	return firstErr(p.record(r, source, temporary, 1), r.Binary(destination, temporary, destination, binaryAdd))
}
