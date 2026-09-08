package ps6115cgo

/*
static int tiny_accel(int op, long long rows, long long columns) {
	return op + (int)rows + (int)columns;
}
*/
import "C"

const opLoss C.int = 7

func hostForward(C.int, C.longlong, C.longlong) C.int { return 0 }

func realCgo() C.int {
	return C.tiny_accel(opLoss, 8, 10) // want `realCgo: configured tiny synchronous accelerator screen is 8x10=80 elements, 640 bytes, with submission count 1`
}
