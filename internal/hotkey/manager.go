package hotkey

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	xh "golang.design/x/hotkey"
)

// Handler pair invoked for each registered hotkey.
//
//	OnTrigger fires on press in hold mode, or on each odd press in toggle mode.
//	OnStop fires on release in hold mode, or on each even press / cap fire in toggle mode.
type Handler struct {
	OnTrigger func()
	OnStop    func()
}

type Mode string

const (
	ModeHold   Mode = "hold"
	ModeToggle Mode = "toggle"
)

type Manager struct {
	mu       sync.Mutex
	hotkeys  []*xh.Hotkey
	done     chan struct{}
	toggleTO time.Duration
}

func NewManager(toggleTimeout time.Duration) *Manager {
	return &Manager{done: make(chan struct{}), toggleTO: toggleTimeout}
}

// Register installs a combo and spawns the dispatcher goroutine. Must be
// called from the main thread on macOS (wrap in mainthread.Init).
func (m *Manager) Register(combo string, mode Mode, h Handler) error {
	mods, key, err := Parse(combo)
	if err != nil {
		return err
	}
	hk := xh.New(mods, key)
	if err := hk.Register(); err != nil {
		return fmt.Errorf("register %q: %w", combo, err)
	}

	m.mu.Lock()
	m.hotkeys = append(m.hotkeys, hk)
	m.mu.Unlock()

	switch mode {
	case ModeToggle:
		go m.runToggle(hk, h)
	default:
		go m.runHold(hk, h)
	}
	return nil
}

func (m *Manager) runHold(hk *xh.Hotkey, h Handler) {
	for {
		select {
		case <-m.done:
			return
		case _, ok := <-hk.Keydown():
			if !ok {
				return
			}
			if h.OnTrigger != nil {
				h.OnTrigger()
			}
			select {
			case <-m.done:
				return
			case <-hk.Keyup():
				if h.OnStop != nil {
					h.OnStop()
				}
			}
		}
	}
}

func (m *Manager) runToggle(hk *xh.Hotkey, h Handler) {
	var recording atomic.Bool
	var stopTimer *time.Timer

	for {
		select {
		case <-m.done:
			return
		case _, ok := <-hk.Keydown():
			if !ok {
				return
			}
			if !recording.Load() {
				recording.Store(true)
				if h.OnTrigger != nil {
					h.OnTrigger()
				}
				if m.toggleTO > 0 {
					stopTimer = time.AfterFunc(m.toggleTO, func() {
						if recording.CompareAndSwap(true, false) {
							if h.OnStop != nil {
								h.OnStop()
							}
						}
					})
				}
			} else {
				if stopTimer != nil {
					stopTimer.Stop()
					stopTimer = nil
				}
				if recording.CompareAndSwap(true, false) {
					if h.OnStop != nil {
						h.OnStop()
					}
				}
			}
			// Drain the matching key-up so it doesn't count as a second press.
			select {
			case <-hk.Keyup():
			case <-time.After(5 * time.Second):
			}
		}
	}
}

// Close unregisters every hotkey and stops the dispatcher goroutines.
func (m *Manager) Close() {
	close(m.done)
	m.mu.Lock()
	for _, hk := range m.hotkeys {
		_ = hk.Unregister()
	}
	m.hotkeys = nil
	m.mu.Unlock()
}
