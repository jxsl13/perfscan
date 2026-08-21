package checks

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

type ps5095ScriptedReader struct {
	data  []byte
	error error
}

func (reader *ps5095ScriptedReader) Read(buffer []byte) (int, error) {
	count := copy(buffer, reader.data)
	reader.data = reader.data[count:]
	return count, reader.error
}

func TestEquiv_PS5095TerminalRead(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		data  []byte
		error error
		size  int
	}{
		{nil, nil, 0},
		{[]byte("payload"), nil, 3},
		{[]byte("payload"), io.EOF, 32},
		{[]byte{0xff, 0, 0xfe}, sentinel, 2},
	}
	for index, test := range cases {
		beforeReader := &ps5095ScriptedReader{data: bytes.Clone(test.data), error: test.error}
		afterReader := &ps5095ScriptedReader{data: bytes.Clone(test.data), error: test.error}
		beforeBuffer, afterBuffer := make([]byte, test.size), make([]byte, test.size)
		beforeN, beforeErr := io.NopCloser(io.NopCloser(io.NopCloser(beforeReader))).Read(beforeBuffer)
		afterN, afterErr := afterReader.Read(afterBuffer)
		if beforeN != afterN || !errors.Is(beforeErr, afterErr) || !reflect.DeepEqual(beforeBuffer, afterBuffer) || !bytes.Equal(beforeReader.data, afterReader.data) {
			t.Fatalf("case %d differs: before=(%d,%v,%v,%v) after=(%d,%v,%v,%v)", index, beforeN, beforeErr, beforeBuffer, beforeReader.data, afterN, afterErr, afterBuffer, afterReader.data)
		}
	}
}

func TestEquiv_PS5095EvaluationOrderAndClose(t *testing.T) {
	var order []string
	reader := func() io.Reader {
		order = append(order, "reader")
		return &ps5095ScriptedReader{data: []byte("x")}
	}
	buffer := func() []byte {
		order = append(order, "buffer")
		return make([]byte, 1)
	}
	_, _ = io.NopCloser(io.NopCloser(reader())).Read(buffer())
	beforeOrder := append([]string(nil), order...)
	order = nil
	_, _ = (reader()).Read(buffer())
	if !reflect.DeepEqual(beforeOrder, order) || !reflect.DeepEqual(order, []string{"reader", "buffer"}) {
		t.Fatalf("evaluation order differs: before=%v after=%v", beforeOrder, order)
	}

	underlying := &ps5095ReadCloser{}
	beforeErr := io.NopCloser(io.NopCloser(io.NopCloser(underlying))).Close()
	afterErr := io.NopCloser(underlying).Close()
	if beforeErr != nil || afterErr != nil || underlying.closed != 0 {
		t.Fatalf("NopCloser Close reached underlying or changed error: before=%v after=%v closes=%d", beforeErr, afterErr, underlying.closed)
	}
}

type ps5095ReadCloser struct{ closed int }

func (*ps5095ReadCloser) Read([]byte) (int, error) { return 0, io.EOF }
func (closer *ps5095ReadCloser) Close() error {
	closer.closed++
	return errors.New("underlying Close must not run")
}
