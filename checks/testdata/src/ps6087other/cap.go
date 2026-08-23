package ps6087other

import "ps6087cap"

type UnrelatedInPlace interface {
	Overwrite(gate, up *ps6087cap.Tensor) bool
}
