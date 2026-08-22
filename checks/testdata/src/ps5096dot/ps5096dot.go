package ps5096dot

import . "io"

func read(reader Reader, buffer []byte, inner, outer int64) (int, error) {
	return LimitReader(LimitReader(reader, inner), outer).Read(buffer)
}
