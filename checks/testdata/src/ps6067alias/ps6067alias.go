package ps6067alias

import (
	stdtesting "testing"
	clock "time"
)

func TestAliasedTiming(t *stdtesting.T) {
	start := clock.Now()
	if clock.Since(start) > 25*clock.Microsecond { // want `time.Since-derived elapsed time is compared with an absolute performance threshold`
		t.Errorf("slow")
	}
}
