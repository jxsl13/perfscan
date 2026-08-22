package ps5095comment

import "io"

func read(reader io.Reader, buffer []byte) (int, error) {
	return io.NopCloser( /* retain rationale */ io.NopCloser(reader)).Read(buffer) // want "io.NopCloser chain constructs 2 adapter layer[(]s[)] only to call Read immediately"
}
