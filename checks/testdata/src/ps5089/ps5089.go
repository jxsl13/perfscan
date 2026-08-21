package ps5089

import (
	"bytes"
	"io"
	"os"
	"slices"
	"strings"
)

func fileWrite(file *os.File, data []byte) (int, error) {
	return file.Write(bytes.Clone(slices.Clone(bytes.Clone(data)))) // want `os.File.Write consumes its input synchronously but receives 3 throwaway standard-library Clone layer`
}

func fileWriteAt(file *os.File, data []byte, offset int64) (int, error) {
	return file.WriteAt(slices.Clone(data), offset) // want `os.File.WriteAt consumes its input synchronously but receives 1 throwaway standard-library Clone layer`
}

func fileWriteString(file *os.File, text string) (int, error) {
	return file.WriteString(strings.Clone(strings.Clone(text))) // want `os.File.WriteString consumes its input synchronously but receives 2 throwaway standard-library Clone layer`
}

func writeFile(name string, data []byte, mode os.FileMode) error {
	return os.WriteFile(name, bytes.Clone(data), mode) // want `os.WriteFile consumes its input synchronously but receives 1 throwaway standard-library Clone layer`
}

func commentPreserved(file *os.File, data []byte) (int, error) {
	return file.Write(bytes.Clone( /* snapshot rationale */ data)) // want `os.File.Write consumes its input synchronously but receives 1 throwaway standard-library Clone layer`
}

func mutateOffset(data []byte) int64 {
	if len(data) != 0 {
		data[0]++
	}
	return 0
}

// The offset expression mutates data after Clone takes its snapshot.
func laterOffsetMutation(file *os.File, data []byte) (int, error) {
	return file.WriteAt(bytes.Clone(data), mutateOffset(data))
}

func mutateMode(data []byte) os.FileMode {
	if len(data) != 0 {
		data[0]++
	}
	return 0o600
}

func laterModeMutation(name string, data []byte) error {
	return os.WriteFile(name, bytes.Clone(data), mutateMode(data))
}

// Interface dispatch is outside this exact concrete-stdlib rule.
func interfaceWriter(writer io.Writer, data []byte) (int, error) {
	return writer.Write(bytes.Clone(data))
}

type writer struct{}

func (writer) Write(data []byte) (int, error) { return len(data), nil }

func userMethod(value writer, data []byte) (int, error) {
	return value.Write(bytes.Clone(data))
}

func methodValue(file *os.File, data []byte) (int, error) {
	write := file.Write
	return write(bytes.Clone(data))
}

var _ = []any{fileWrite, fileWriteAt, fileWriteString, writeFile, commentPreserved, laterOffsetMutation, laterModeMutation, interfaceWriter, userMethod, methodValue}
