package checks

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

type ps5098Reader struct {
	data  []byte
	err   error
	calls int
}

func (reader *ps5098Reader) Read(buffer []byte) (int, error) {
	reader.calls++
	count := copy(buffer, reader.data)
	reader.data = reader.data[count:]
	return count, reader.err
}

func TestEquiv_PS5098MultiReaderTerminalRead(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		data [3]string
		err  [3]error
	}{
		{data: [3]string{"a", "b", "c"}},
		{data: [3]string{"", "b", "c"}, err: [3]error{io.EOF, nil, nil}},
		{data: [3]string{"a", "b", "c"}, err: [3]error{io.EOF, nil, nil}},
		{data: [3]string{"", "b", "c"}, err: [3]error{sentinel, nil, nil}},
		{data: [3]string{"", "b", "c"}, err: [3]error{nil, nil, nil}},
		{data: [3]string{"", "", "c"}, err: [3]error{io.EOF, io.EOF, sentinel}},
	}
	for index, test := range cases {
		beforeReaders := make([]*ps5098Reader, 3)
		afterReaders := make([]*ps5098Reader, 3)
		for readerIndex := range beforeReaders {
			beforeReaders[readerIndex] = &ps5098Reader{data: []byte(test.data[readerIndex]), err: test.err[readerIndex]}
			afterReaders[readerIndex] = &ps5098Reader{data: []byte(test.data[readerIndex]), err: test.err[readerIndex]}
		}
		beforeBuffer, afterBuffer := make([]byte, 8), make([]byte, 8)
		beforeN, beforeErr := io.MultiReader(
			io.MultiReader(beforeReaders[0], beforeReaders[1]),
			beforeReaders[2],
		).Read(beforeBuffer)
		afterN, afterErr := io.MultiReader(afterReaders[0], afterReaders[1], afterReaders[2]).Read(afterBuffer)
		if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(beforeBuffer, afterBuffer) {
			t.Fatalf("case %d result differs: before=(%d,%v,%v) after=(%d,%v,%v)", index, beforeN, beforeErr, beforeBuffer, afterN, afterErr, afterBuffer)
		}
		for readerIndex := range beforeReaders {
			if beforeReaders[readerIndex].calls != afterReaders[readerIndex].calls ||
				!bytes.Equal(beforeReaders[readerIndex].data, afterReaders[readerIndex].data) {
				t.Fatalf("case %d reader %d state differs: before=(%d,%q) after=(%d,%q)", index, readerIndex,
					beforeReaders[readerIndex].calls, beforeReaders[readerIndex].data,
					afterReaders[readerIndex].calls, afterReaders[readerIndex].data)
			}
		}
	}
}

type ps5098Writer struct {
	limit  int
	err    error
	calls  int
	writes [][]byte
}

func (writer *ps5098Writer) Write(payload []byte) (int, error) {
	writer.calls++
	writer.writes = append(writer.writes, bytes.Clone(payload))
	count := len(payload)
	if writer.limit >= 0 && writer.limit < count {
		count = writer.limit
	}
	return count, writer.err
}

func TestEquiv_PS5098MultiWriterTerminalWrite(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		limits [4]int
		errors [4]error
	}{
		{limits: [4]int{-1, -1, -1, -1}},
		{limits: [4]int{-1, -1, -1, -1}, errors: [4]error{sentinel, nil, nil, nil}},
		{limits: [4]int{-1, 2, -1, -1}},
		{limits: [4]int{-1, -1, -1, -1}, errors: [4]error{nil, nil, sentinel, nil}},
		{limits: [4]int{-1, -1, -1, 0}},
	}
	payload := []byte("payload")
	for index, test := range cases {
		beforeWriters := make([]*ps5098Writer, 4)
		afterWriters := make([]*ps5098Writer, 4)
		beforeArgs := make([]io.Writer, 4)
		afterArgs := make([]io.Writer, 4)
		for writerIndex := range beforeWriters {
			beforeWriters[writerIndex] = &ps5098Writer{limit: test.limits[writerIndex], err: test.errors[writerIndex]}
			afterWriters[writerIndex] = &ps5098Writer{limit: test.limits[writerIndex], err: test.errors[writerIndex]}
			beforeArgs[writerIndex], afterArgs[writerIndex] = beforeWriters[writerIndex], afterWriters[writerIndex]
		}
		beforeN, beforeErr := io.MultiWriter(
			io.MultiWriter(beforeArgs[0], beforeArgs[1]),
			io.MultiWriter(beforeArgs[2], beforeArgs[3]),
		).Write(payload)
		afterN, afterErr := io.MultiWriter(afterArgs...).Write(payload)
		if beforeN != afterN || beforeErr != afterErr {
			t.Fatalf("case %d result differs: before=(%d,%v) after=(%d,%v)", index, beforeN, beforeErr, afterN, afterErr)
		}
		for writerIndex := range beforeWriters {
			if beforeWriters[writerIndex].calls != afterWriters[writerIndex].calls ||
				!reflect.DeepEqual(beforeWriters[writerIndex].writes, afterWriters[writerIndex].writes) {
				t.Fatalf("case %d writer %d state differs: before=(%d,%v) after=(%d,%v)", index, writerIndex,
					beforeWriters[writerIndex].calls, beforeWriters[writerIndex].writes,
					afterWriters[writerIndex].calls, afterWriters[writerIndex].writes)
			}
		}
	}
}

func TestEquiv_PS5098EvaluationOrder(t *testing.T) {
	var order []string
	writer := func(name string) io.Writer {
		order = append(order, name)
		return io.Discard
	}
	payload := func() []byte {
		order = append(order, "payload")
		return []byte("x")
	}
	_, _ = io.MultiWriter(
		io.MultiWriter(writer("first"), writer("second")),
		writer("third"),
	).Write(payload())
	beforeOrder := append([]string(nil), order...)
	order = nil
	_, _ = io.MultiWriter(writer("first"), writer("second"), writer("third")).Write(payload())
	want := []string{"first", "second", "third", "payload"}
	if !reflect.DeepEqual(beforeOrder, order) || !reflect.DeepEqual(order, want) {
		t.Fatalf("evaluation order differs: before=%v after=%v want=%v", beforeOrder, order, want)
	}
}
