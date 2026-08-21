package ps5095orphan

import "io"

type reader struct{}

func (reader) Read(buffer []byte) (int, error) { return len(buffer), nil }

func read(buffer []byte) (int, error) {
	return io.NopCloser(reader{}).Read(buffer) // want "io.NopCloser chain constructs 1 adapter layer[(]s[)] only to call Read immediately"
}
