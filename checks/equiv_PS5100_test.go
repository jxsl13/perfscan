package checks

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

type ps5100Writer struct {
	limit int
	err   error
	calls int
	data  []byte
}

func (writer *ps5100Writer) Write(payload []byte) (int, error) {
	writer.calls++
	count := len(payload)
	if writer.limit >= 0 && writer.limit < count {
		count = writer.limit
	}
	writer.data = append(writer.data, payload[:count]...)
	return count, writer.err
}

func TestEquiv_PS5100Copy(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		data        [3]string
		readerError [3]error
		writerLimit int
		writerError error
	}{
		{data: [3]string{"alpha", "beta", "gamma"}, readerError: [3]error{io.EOF, io.EOF, io.EOF}, writerLimit: -1},
		{data: [3]string{"partial", "ignored", "ignored"}, readerError: [3]error{sentinel, io.EOF, io.EOF}, writerLimit: -1},
		{data: [3]string{"payload", "ignored", "ignored"}, readerError: [3]error{io.EOF, io.EOF, io.EOF}, writerLimit: 2},
		{data: [3]string{"payload", "ignored", "ignored"}, readerError: [3]error{io.EOF, io.EOF, io.EOF}, writerLimit: -1, writerError: sentinel},
	}
	for index, test := range cases {
		beforeReaders := make([]*ps5098Reader, 3)
		afterReaders := make([]*ps5098Reader, 3)
		for readerIndex := range beforeReaders {
			beforeReaders[readerIndex] = &ps5098Reader{data: []byte(test.data[readerIndex]), err: test.readerError[readerIndex]}
			afterReaders[readerIndex] = &ps5098Reader{data: []byte(test.data[readerIndex]), err: test.readerError[readerIndex]}
		}
		beforeWriter := &ps5100Writer{limit: test.writerLimit, err: test.writerError}
		afterWriter := &ps5100Writer{limit: test.writerLimit, err: test.writerError}
		beforeN, beforeErr := io.Copy(beforeWriter, io.MultiReader(io.MultiReader(beforeReaders[0], beforeReaders[1]), beforeReaders[2]))
		afterN, afterErr := io.Copy(afterWriter, io.MultiReader(afterReaders[0], afterReaders[1], afterReaders[2]))
		if beforeN != afterN || beforeErr != afterErr || beforeWriter.calls != afterWriter.calls || !bytes.Equal(beforeWriter.data, afterWriter.data) {
			t.Fatalf("case %d differs: before=(%d,%v,%d,%q) after=(%d,%v,%d,%q)", index,
				beforeN, beforeErr, beforeWriter.calls, beforeWriter.data, afterN, afterErr, afterWriter.calls, afterWriter.data)
		}
		for readerIndex := range beforeReaders {
			if beforeReaders[readerIndex].calls != afterReaders[readerIndex].calls || !bytes.Equal(beforeReaders[readerIndex].data, afterReaders[readerIndex].data) {
				t.Fatalf("case %d reader %d state differs: before=(%d,%q) after=(%d,%q)", index, readerIndex,
					beforeReaders[readerIndex].calls, beforeReaders[readerIndex].data,
					afterReaders[readerIndex].calls, afterReaders[readerIndex].data)
			}
		}
	}
}

type ps5100WriterToReader struct {
	text  string
	calls int
}

func (*ps5100WriterToReader) Read([]byte) (int, error) {
	panic("WriterTo fast path must bypass Read")
}

func (reader *ps5100WriterToReader) WriteTo(writer io.Writer) (int64, error) {
	reader.calls++
	count, err := io.WriteString(writer, reader.text)
	return int64(count), err
}

func TestEquiv_PS5100CopyBufferAndWriterTo(t *testing.T) {
	beforeReaders := []*ps5100WriterToReader{{text: "alpha"}, {text: "beta"}, {text: "gamma"}}
	afterReaders := []*ps5100WriterToReader{{text: "alpha"}, {text: "beta"}, {text: "gamma"}}
	beforeWriter, afterWriter := &bytes.Buffer{}, &bytes.Buffer{}
	beforeScratch := bytes.Repeat([]byte{0x7f}, 17)
	afterScratch := bytes.Clone(beforeScratch)
	beforeN, beforeErr := io.CopyBuffer(beforeWriter,
		io.MultiReader(io.MultiReader(beforeReaders[0], beforeReaders[1]), beforeReaders[2]),
		beforeScratch,
	)
	afterN, afterErr := io.CopyBuffer(afterWriter,
		io.MultiReader(afterReaders[0], afterReaders[1], afterReaders[2]),
		afterScratch,
	)
	if beforeN != afterN || beforeErr != afterErr || beforeWriter.String() != afterWriter.String() || !bytes.Equal(beforeScratch, afterScratch) {
		t.Fatalf("CopyBuffer differs: before=(%d,%v,%q,%v) after=(%d,%v,%q,%v)",
			beforeN, beforeErr, beforeWriter.String(), beforeScratch, afterN, afterErr, afterWriter.String(), afterScratch)
	}
	for index := range beforeReaders {
		if beforeReaders[index].calls != afterReaders[index].calls || beforeReaders[index].calls != 1 {
			t.Fatalf("WriterTo reader %d calls differ: before=%d after=%d", index, beforeReaders[index].calls, afterReaders[index].calls)
		}
	}
}

func TestEquiv_PS5100EvaluationOrder(t *testing.T) {
	var order []string
	destination := func() io.Writer {
		order = append(order, "destination")
		return io.Discard
	}
	reader := func(name string) io.Reader {
		order = append(order, name)
		return &ps5098Reader{err: io.EOF}
	}
	buffer := func() []byte {
		order = append(order, "buffer")
		return make([]byte, 8)
	}
	_, _ = io.CopyBuffer(destination(), io.MultiReader(io.MultiReader(reader("first"), reader("second")), reader("third")), buffer())
	beforeOrder := append([]string(nil), order...)
	order = nil
	_, _ = io.CopyBuffer(destination(), io.MultiReader(reader("first"), reader("second"), reader("third")), buffer())
	want := []string{"destination", "first", "second", "third", "buffer"}
	if !reflect.DeepEqual(beforeOrder, order) || !reflect.DeepEqual(order, want) {
		t.Fatalf("evaluation order differs: before=%v after=%v want=%v", beforeOrder, order, want)
	}
}

func TestEquiv_PS5100CopyBufferEmptyPanic(t *testing.T) {
	invoke := func(flat bool) (panicValue any) {
		defer func() { panicValue = recover() }()
		first, second, third := &ps5098Reader{}, &ps5098Reader{}, &ps5098Reader{}
		if flat {
			_, _ = io.CopyBuffer(io.Discard, io.MultiReader(first, second, third), []byte{})
		} else {
			_, _ = io.CopyBuffer(io.Discard, io.MultiReader(io.MultiReader(first, second), third), []byte{})
		}
		return nil
	}
	before, after := invoke(false), invoke(true)
	if !reflect.DeepEqual(before, after) || before == nil {
		t.Fatalf("empty-buffer panic differs: before=%v after=%v", before, after)
	}
}
