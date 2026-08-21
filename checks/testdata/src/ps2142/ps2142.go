package ps2142

import (
	"bufio"
	"bytes"
	"io"
	"os"
)

type model struct{ n int }

func decode([]byte) (model, error) { return model{}, nil }

func direct(path string) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(f) // want `os\.Open file is fully heap-staged by io\.ReadAll before decode`
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

func buffered(path string) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	defer f.Close()
	_, _ = f.Stat()
	data, err := io.ReadAll(bufio.NewReader(f)) // want `os\.Open file is fully heap-staged by io\.ReadAll before decode`
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

// --- negatives ---

// Returning the staging bytes transfers their ownership; mmap would change the
// API lifetime and aliasing contract.
func returnsBytes(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	return data, err
}

// A seek means ReadAll no longer denotes the whole untouched file.
func partial(path string) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	_, _ = f.Seek(64, io.SeekStart)
	data, err := io.ReadAll(f)
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

// An immediate close is not the normal deferred cleanup and means ReadAll
// cannot stage the opened file successfully.
func closed(path string) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	_ = f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

// A visible mutation fails the immutable-consumer screen.
func mutates(path string) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return model{}, err
	}
	if len(data) > 0 {
		data[0] = 0
	}
	return decode(data)
}

// A local slice alias is not followed interprocedurally and fails closed.
func aliases(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	payload := data[1:]
	return payload, nil
}

// A non-file reader is outside this rule.
func memory(b []byte) (model, error) {
	data, err := io.ReadAll(bytes.NewReader(b))
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

// The presence of some os.Open in the function must not make an unrelated
// reader look file-backed.
func mixed(path string, r io.Reader) (model, error) {
	f, err := os.Open(path)
	if err != nil {
		return model{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

// A composite return visibly retains the staging bytes.
func compositeAlias(path string) (struct{ Raw []byte }, error) {
	f, err := os.Open(path)
	if err != nil {
		return struct{ Raw []byte }{}, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return struct{ Raw []byte }{}, err
	}
	return struct{ Raw []byte }{Raw: data}, nil
}

// os.ReadFile owns its returned bytes by contract and has no open file whose
// immutable lifetime can be audited here.
func readFile(path string) (model, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model{}, err
	}
	return decode(data)
}

// Merely taking the length is not evidence of a decode/materialization path.
func length(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	data, err := io.ReadAll(f)
	return len(data), err
}

// Asynchronous consumption makes the staging lifetime escape the local proof.
func async(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	go func() { _, _ = decode(data) }()
	return nil
}
