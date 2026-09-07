package ps6109importboth

import "ps6109backend"

func importedInterface() error {
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

func importedDirect() error {
	provider := &ps6109backend.Device{}
	for step := 0; step < 7; step++ {
		recorder, err := provider.Acquire() // want `fixed 7-step source path creates a fresh Go wrapper generation`
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}
