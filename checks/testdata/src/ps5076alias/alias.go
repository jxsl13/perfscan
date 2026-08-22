package ps5076alias

import x "io"

func copy(dst x.Writer, src x.Reader) (int64, error) {
	return x.Copy(dst, x.NopCloser(src)) // want `io\.Copy never calls Close`
}
