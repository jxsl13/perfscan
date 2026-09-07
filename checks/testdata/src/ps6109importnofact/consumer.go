package ps6109importnofact

import "ps6109opaque"

func importedWithoutFact() error {
	concrete := &ps6109opaque.Device{}
	var provider ps6109opaque.CommandFactory = concrete
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
