package ps6109import

import "ps6109backend"

func importedPositive() error {
	concrete := &ps6109backend.Device{}
	var provider ps6109backend.CommandFactory = concrete
	for step := 0; step < 4; step++ {
		recorder, err := provider.Acquire() // want `fixed 4-step source path creates a fresh Go wrapper generation`
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}

func importedWithoutFact() error {
	concrete := &ps6109backend.Device{}
	var provider ps6109backend.OpaqueFactory = concrete
	for step := 0; step < 4; step++ {
		recorder, err := provider.AcquireOpaque()
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}
