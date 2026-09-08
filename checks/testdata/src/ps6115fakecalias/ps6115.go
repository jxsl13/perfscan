package ps6115fakecalias

import C "ps6115fakec"

const opLoss C.Operation = 7

func hostForward(C.Operation, int64, int64) int { return 0 }

func fakeAlias() int {
	return C.Forward(opLoss, 8, 10)
}
