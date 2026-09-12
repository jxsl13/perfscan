//go:build !darwin

package traceevidence

import "errors"

func inputAccountHome() (string, error) {
	return "", errors.New("Darwin account identity is unavailable on this platform")
}

func observeInputMount(string) (MountObservation, error) {
	return MountObservation{}, errors.New("Darwin mount observation is unavailable on this platform")
}
