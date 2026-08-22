package ps5095dot

import . "io"

func read(reader Reader, buffer []byte) (int, error) {
	return NopCloser(NopCloser(reader)).Read(buffer)
}
