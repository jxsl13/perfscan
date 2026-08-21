package ps5094dot

import . "bytes"

func extract(text string) []byte {
	return NewBufferString(text).Bytes()
}
