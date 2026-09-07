package ps6109importmismatch

import "ps6109backend"

func mismatchedConstructorFact() error {
	concrete := &ps6109backend.Device{}
	var provider ps6109backend.CommandFactory = concrete
	for step := 0; step < 4; step++ {
		recorder, err := provider.Acquire()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}
