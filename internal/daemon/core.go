package daemon

import (
	"sync"
	"time"
)

// ErrorRecoveryDelay is the time the core spends in StateError before
// auto-recovering to StateIdle.
const ErrorRecoveryDelay = 2 * time.Second

type Core struct {
	mu          sync.Mutex
	state       State
	subscribers []Subscriber

	// now is a hook for tests. Unused here but reserved for future scheduling.
	afterFunc func(d time.Duration, f func()) *time.Timer
}

func NewCore() *Core {
	return &Core{
		state:     StateIdle,
		afterFunc: time.AfterFunc,
	}
}

func (c *Core) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// Subscribe registers a state-change callback. Callbacks are invoked
// synchronously on the goroutine that called Transition; keep them cheap.
func (c *Core) Subscribe(fn Subscriber) {
	c.mu.Lock()
	c.subscribers = append(c.subscribers, fn)
	c.mu.Unlock()
}

// Transition moves to the new state and broadcasts the change.
// If moving into StateError, schedules auto-recovery back to StateIdle.
func (c *Core) Transition(next State, err error) {
	c.mu.Lock()
	prev := c.state
	if prev == next {
		c.mu.Unlock()
		return
	}
	c.state = next
	subs := make([]Subscriber, len(c.subscribers))
	copy(subs, c.subscribers)
	c.mu.Unlock()

	evt := StateEvent{State: next, Previous: prev, Err: err}
	for _, s := range subs {
		s(evt)
	}

	if next == StateError {
		c.afterFunc(ErrorRecoveryDelay, func() {
			c.recoverFromError()
		})
	}
}

func (c *Core) recoverFromError() {
	c.mu.Lock()
	if c.state != StateError {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()
	c.Transition(StateIdle, nil)
}

// TryStartRecording transitions Idle → Recording and returns true.
// If not currently Idle, returns false without transitioning.
func (c *Core) TryStartRecording() bool {
	c.mu.Lock()
	if c.state != StateIdle {
		c.mu.Unlock()
		return false
	}
	c.mu.Unlock()
	c.Transition(StateRecording, nil)
	return true
}
