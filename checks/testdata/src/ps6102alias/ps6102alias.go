package ps6102alias

import (
	stdtest "testing"
	clock "time"
)

type fakeT struct{}

func (*fakeT) Fatal(...any) {}
func (*fakeT) Skip(...any)  {}
func (*fakeT) Run(string, func(*fakeT)) bool {
	return true
}

type fakeTesting struct{}

func (fakeTesting) Short() bool                      { return true }
func (fakeTesting) AllocsPerRun(int, func()) float64 { return 0 }
func fakeSince(clock.Time) clock.Duration            { return 0 }
func measured()                                      {}

func TestAliasedStandardLibrary(t *stdtest.T) {
	start := clock.Now()
	elapsed := clock.Since(start)
	if elapsed > clock.Second { // want `performance threshold "elapsed > clock.Second" in TestAliasedStandardLibrary remains reachable`
		t.Fatal("slow")
	}
}

func TestFakeShortDoesNotGuard(t *stdtest.T) {
	fake := fakeTesting{}
	if fake.Short() {
		return
	}
	latency := clock.Since(clock.Now())
	if latency > clock.Second { // want `performance threshold "latency > clock.Second" in TestFakeShortDoesNotGuard remains reachable`
		t.Fatal("slow")
	}
}

func TestFakeSkipDoesNotAbort(t *stdtest.T) {
	fake := &fakeT{}
	if stdtest.Short() {
		fake.Skip("not testing.T")
	}
	latency := clock.Since(clock.Now())
	if latency > clock.Second { // want `performance threshold "latency > clock.Second" in TestFakeSkipDoesNotAbort remains reachable`
		t.Fatal("slow")
	}
}

func TestFakeRunDoesNotCreateSubtest(t *stdtest.T) {
	fake := &fakeT{}
	fake.Run("not-real", func(t *fakeT) {
		speedup := 1.0
		if speedup < 1.2 {
			t.Fatal("not a testing subtest")
		}
	})
}

func TestFakeTimeAndAllocsStaySilent(t *stdtest.T) {
	fake := fakeTesting{}
	got := fake.AllocsPerRun(1, measured)
	if got > 0 {
		t.Fatal("fake alloc count")
	}
	got = float64(fakeSince(clock.Now()))
	if got > float64(clock.Second) {
		t.Fatal("fake time")
	}
}
