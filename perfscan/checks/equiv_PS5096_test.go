package checks

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

type ps5096TraceReader struct {
	data    []byte
	err     error
	calls   int
	lengths []int
}

func (reader *ps5096TraceReader) Read(buffer []byte) (int, error) {
	reader.calls++
	reader.lengths = append(reader.lengths, len(buffer))
	count := copy(buffer, reader.data)
	reader.data = reader.data[count:]
	return count, reader.err
}

func TestEquiv_PS5096TerminalRead(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		limits [3]int64
		size   int
		err    error
	}{
		{[3]int64{-3, 5, 8}, 16, nil},
		{[3]int64{5, -3, 8}, 16, io.EOF},
		{[3]int64{5, 8, -3}, 16, sentinel},
		{[3]int64{0, 0, 0}, 16, nil},
		{[3]int64{2, 5, 9}, 16, nil},
		{[3]int64{9, 2, 5}, 16, io.EOF},
		{[3]int64{9, 5, 2}, 16, sentinel},
		{[3]int64{100, 200, 300}, 0, sentinel},
	}
	for index, test := range cases {
		beforeReader := &ps5096TraceReader{data: []byte("payload"), err: test.err}
		afterReader := &ps5096TraceReader{data: []byte("payload"), err: test.err}
		beforeBuffer, afterBuffer := make([]byte, test.size), make([]byte, test.size)
		a, b, c := test.limits[0], test.limits[1], test.limits[2]
		beforeN, beforeErr := io.LimitReader(io.LimitReader(io.LimitReader(beforeReader, a), b), c).Read(beforeBuffer)
		afterN, afterErr := io.LimitReader(afterReader, min(a, b, c)).Read(afterBuffer)
		if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(beforeBuffer, afterBuffer) ||
			!bytes.Equal(beforeReader.data, afterReader.data) || beforeReader.calls != afterReader.calls ||
			!reflect.DeepEqual(beforeReader.lengths, afterReader.lengths) {
			t.Fatalf("case %d differs: before=(%d,%v,%v,%v,%d,%v) after=(%d,%v,%v,%v,%d,%v)",
				index, beforeN, beforeErr, beforeBuffer, beforeReader.data, beforeReader.calls, beforeReader.lengths,
				afterN, afterErr, afterBuffer, afterReader.data, afterReader.calls, afterReader.lengths)
		}
	}
}

func TestEquiv_PS5096EvaluationOrder(t *testing.T) {
	var order []string
	reader := func() io.Reader {
		order = append(order, "reader")
		return &ps5096TraceReader{data: []byte("x")}
	}
	limit := func(name string, value int64) int64 {
		order = append(order, name)
		return value
	}
	buffer := func() []byte {
		order = append(order, "buffer")
		return make([]byte, 1)
	}
	_, _ = io.LimitReader(
		io.LimitReader(io.LimitReader(reader(), limit("inner", 4)), limit("middle", 3)),
		limit("outer", 2),
	).Read(buffer())
	beforeOrder := append([]string(nil), order...)
	order = nil
	_, _ = io.LimitReader(
		reader(),
		min(limit("inner", 4), limit("middle", 3), limit("outer", 2)),
	).Read(buffer())
	want := []string{"reader", "inner", "middle", "outer", "buffer"}
	if !reflect.DeepEqual(beforeOrder, order) || !reflect.DeepEqual(order, want) {
		t.Fatalf("evaluation order differs: before=%v after=%v want=%v", beforeOrder, order, want)
	}
}
