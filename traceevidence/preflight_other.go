//go:build !darwin

package traceevidence

import "errors"

func inputAccountHome() (string, error) {
	return "", errors.New("darwin account identity is unavailable on this platform")
}

func observeInputMount(string) (MountObservation, error) {
	return MountObservation{}, errors.New("darwin mount observation is unavailable on this platform")
}
