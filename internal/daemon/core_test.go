package daemon

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCoreTransitionAndSubscribe(t *testing.T) {
	c := NewCore()
	var got []StateEvent
	var mu sync.Mutex
	c.Subscribe(func(e StateEvent) {
		mu.Lock()
		got = append(got, e)
		mu.Unlock()
	})

	c.Transition(StateRecording, nil)
	c.Transition(StateTranscribing, nil)
	c.Transition(StateIdle, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 3 {
		t.Fatalf("got %d events, want 3", len(got))
	}
	if got[0].State != StateRecording || got[0].Previous != StateIdle {
		t.Errorf("first event = %+v", got[0])
	}
	if got[2].State != StateIdle || got[2].Previous != StateTranscribing {
		t.Errorf("third event = %+v", got[2])
	}
}

func TestCoreNoDuplicateTransition(t *testing.T) {
	c := NewCore()
	var n int32
	c.Subscribe(func(StateEvent) { atomic.AddInt32(&n, 1) })
	c.Transition(StateIdle, nil) // same as initial
	if atomic.LoadInt32(&n) != 0 {
		t.Fatal("transition to current state should be a no-op")
	}
}

func TestCoreTryStartRecording(t *testing.T) {
	c := NewCore()
	if !c.TryStartRecording() {
		t.Fatal("first TryStartRecording should succeed from Idle")
	}
	if c.TryStartRecording() {
		t.Fatal("second TryStartRecording should fail while Recording")
	}
	if c.State() != StateRecording {
		t.Fatalf("state = %s, want Recording", c.State())
	}
}

func TestCoreErrorAutoRecovery(t *testing.T) {
	c := NewCore()
	// Shrink the timer hook for a fast test by replacing afterFunc with an
	// inline call — safe because we invoke Transition from the test goroutine.
	c.afterFunc = func(_ time.Duration, f func()) *time.Timer {
		go f()
		return nil
	}
	done := make(chan struct{})
	c.Subscribe(func(e StateEvent) {
		if e.State == StateIdle && e.Previous == StateError {
			close(done)
		}
	})
	c.Transition(StateError, nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("did not recover from Error to Idle")
	}
}

func TestCoreConcurrentSubscribersNoDataRace(t *testing.T) {
	c := NewCore()
	var n int32
	for range 5 {
		c.Subscribe(func(StateEvent) { atomic.AddInt32(&n, 1) })
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			c.Transition(StateRecording, nil)
			c.Transition(StateIdle, nil)
		})
	}
	wg.Wait()
	// We don't assert a specific count; transitions to the current state no-op.
	// The goal is `go test -race` cleanliness, enforced via CI / make test.
}
