package ps6109importconsumer

import "ps6109backend"

type consumerFactory interface {
	Acquire() (ps6109backend.CommandRecorder, error)
}

func importedConsumerInterface() error {
	concrete := &ps6109backend.Device{}
	var provider consumerFactory = concrete
	for step := 0; step < 6; step++ {
		recorder, err := provider.Acquire() // want `fixed 6-step source path creates a fresh Go wrapper generation`
		if err != nil {
			continue
		}
		recorder.Encode()
		recorder.Free()
	}
	return nil
}
