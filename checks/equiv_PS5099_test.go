package checks

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestEquiv_PS5099ReadAll(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		data [3][]byte
		err  [3]error
	}{
		{data: [3][]byte{[]byte("alpha"), []byte("beta"), []byte("gamma")}, err: [3]error{io.EOF, io.EOF, io.EOF}},
		{data: [3][]byte{nil, []byte{0xff, 0, 0xfe}, nil}, err: [3]error{io.EOF, io.EOF, io.EOF}},
		{data: [3][]byte{[]byte("partial"), nil, []byte("ignored")}, err: [3]error{io.EOF, sentinel, io.EOF}},
	}
	for index, test := range cases {
		beforeReaders := make([]*ps5098Reader, 3)
		afterReaders := make([]*ps5098Reader, 3)
		for readerIndex := range beforeReaders {
			beforeReaders[readerIndex] = &ps5098Reader{data: bytes.Clone(test.data[readerIndex]), err: test.err[readerIndex]}
			afterReaders[readerIndex] = &ps5098Reader{data: bytes.Clone(test.data[readerIndex]), err: test.err[readerIndex]}
		}
		before, beforeErr := io.ReadAll(io.MultiReader(io.MultiReader(beforeReaders[0], beforeReaders[1]), beforeReaders[2]))
		after, afterErr := io.ReadAll(io.MultiReader(afterReaders[0], afterReaders[1], afterReaders[2]))
		if beforeErr != afterErr || (before == nil) != (after == nil) || len(before) != len(after) || cap(before) != cap(after) || !bytes.Equal(before, after) {
			t.Fatalf("case %d differs: before=(%v,nil=%v,len=%d,cap=%d,%v) after=(%v,nil=%v,len=%d,cap=%d,%v)",
				index, beforeErr, before == nil, len(before), cap(before), before, afterErr, after == nil, len(after), cap(after), after)
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

func TestEquiv_PS5099ReadFullAndReadAtLeast(t *testing.T) {
	sentinel := errors.New("sentinel")
	tests := []struct {
		name    string
		data    [3]string
		errors  [3]error
		bufSize int
		minimum int
		full    bool
	}{
		{name: "full across EOF boundaries", data: [3]string{"ab", "cd", "ef"}, errors: [3]error{io.EOF, io.EOF, io.EOF}, bufSize: 5, full: true},
		{name: "short full", data: [3]string{"a", "b", ""}, errors: [3]error{io.EOF, io.EOF, io.EOF}, bufSize: 4, full: true},
		{name: "at least", data: [3]string{"a", "bc", "def"}, errors: [3]error{io.EOF, io.EOF, io.EOF}, bufSize: 6, minimum: 4},
		{name: "custom error", data: [3]string{"a", "", "ignored"}, errors: [3]error{io.EOF, sentinel, io.EOF}, bufSize: 6, minimum: 4},
		{name: "short buffer", data: [3]string{"ignored", "ignored", "ignored"}, bufSize: 2, minimum: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeReaders := make([]*ps5098Reader, 3)
			afterReaders := make([]*ps5098Reader, 3)
			for index := range beforeReaders {
				beforeReaders[index] = &ps5098Reader{data: []byte(test.data[index]), err: test.errors[index]}
				afterReaders[index] = &ps5098Reader{data: []byte(test.data[index]), err: test.errors[index]}
			}
			beforeBuffer, afterBuffer := make([]byte, test.bufSize), make([]byte, test.bufSize)
			beforeTree := io.MultiReader(io.MultiReader(beforeReaders[0], beforeReaders[1]), beforeReaders[2])
			afterTree := io.MultiReader(afterReaders[0], afterReaders[1], afterReaders[2])
			var beforeN, afterN int
			var beforeErr, afterErr error
			if test.full {
				beforeN, beforeErr = io.ReadFull(beforeTree, beforeBuffer)
				afterN, afterErr = io.ReadFull(afterTree, afterBuffer)
			} else {
				beforeN, beforeErr = io.ReadAtLeast(beforeTree, beforeBuffer, test.minimum)
				afterN, afterErr = io.ReadAtLeast(afterTree, afterBuffer, test.minimum)
			}
			if beforeN != afterN || beforeErr != afterErr || !bytes.Equal(beforeBuffer, afterBuffer) {
				t.Fatalf("result differs: before=(%d,%v,%v) after=(%d,%v,%v)", beforeN, beforeErr, beforeBuffer, afterN, afterErr, afterBuffer)
			}
			for index := range beforeReaders {
				if beforeReaders[index].calls != afterReaders[index].calls || !bytes.Equal(beforeReaders[index].data, afterReaders[index].data) {
					t.Fatalf("reader %d state differs: before=(%d,%q) after=(%d,%q)", index,
						beforeReaders[index].calls, beforeReaders[index].data, afterReaders[index].calls, afterReaders[index].data)
				}
			}
		})
	}
}

type ps5099StringWriter struct {
	limit int
	err   error
	log   []string
}

func (writer *ps5099StringWriter) Write(payload []byte) (int, error) {
	writer.log = append(writer.log, "Write:"+string(payload))
	return writer.count(len(payload)), writer.err
}

func (writer *ps5099StringWriter) WriteString(text string) (int, error) {
	writer.log = append(writer.log, "WriteString:"+text)
	return writer.count(len(text)), writer.err
}

func (writer *ps5099StringWriter) count(length int) int {
	if writer.limit >= 0 && writer.limit < length {
		return writer.limit
	}
	return length
}

func TestEquiv_PS5099WriteString(t *testing.T) {
	sentinel := errors.New("sentinel")
	cases := []struct {
		limits [4]int
		errors [4]error
	}{
		{limits: [4]int{-1, -1, -1, -1}},
		{limits: [4]int{-1, 2, -1, -1}},
		{limits: [4]int{-1, -1, -1, -1}, errors: [4]error{nil, nil, sentinel, nil}},
	}
	for index, test := range cases {
		before := make([]*ps5099StringWriter, 4)
		after := make([]*ps5099StringWriter, 4)
		for writerIndex := range before {
			before[writerIndex] = &ps5099StringWriter{limit: test.limits[writerIndex], err: test.errors[writerIndex]}
			after[writerIndex] = &ps5099StringWriter{limit: test.limits[writerIndex], err: test.errors[writerIndex]}
		}
		beforeN, beforeErr := io.WriteString(io.MultiWriter(io.MultiWriter(before[0], before[1]), io.MultiWriter(before[2], before[3])), "payload")
		afterN, afterErr := io.WriteString(io.MultiWriter(after[0], after[1], after[2], after[3]), "payload")
		if beforeN != afterN || beforeErr != afterErr {
			t.Fatalf("case %d result differs: before=(%d,%v) after=(%d,%v)", index, beforeN, beforeErr, afterN, afterErr)
		}
		for writerIndex := range before {
			if !reflect.DeepEqual(before[writerIndex].log, after[writerIndex].log) {
				t.Fatalf("case %d writer %d log differs: before=%v after=%v", index, writerIndex, before[writerIndex].log, after[writerIndex].log)
			}
		}
	}
}
