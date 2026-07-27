package drivers

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/config"
)

// cacheEntry is a cached loaded model with idle-unload bookkeeping. All
// fields are guarded by modelCache.mu; an entry is only removed from the
// cache (and its model closed) when inflight is zero, so a holder returned
// by acquire can never have the model freed under it.
type cacheEntry[M any] struct {
	m        M
	inflight int
	lastUse  time.Time
	timer    *time.Timer // armed while idle and unloading is enabled
}

// modelCache lazily loads models by name and keeps them resident. A failed
// load is not cached, so the next use retries. If unloadAfter is positive,
// a model idle for that long is freed and reloaded lazily on next use.
// Shared by the whisper_cpp and llama_cpp drivers.
type modelCache[M any] struct {
	mu          sync.Mutex
	paths       map[string]string // model name → file path (from config)
	loaded      map[string]*cacheEntry[M]
	unloadAfter time.Duration
	kind        string // driver type, for errors and logs
	load        func(path string) (M, error)
	close       func(M)
}

func newModelCache[M any](paths map[string]string, unloadAfter time.Duration,
	load func(string) (M, error), close func(M), kind string) *modelCache[M] {
	return &modelCache[M]{
		paths:       paths,
		loaded:      map[string]*cacheEntry[M]{},
		unloadAfter: unloadAfter,
		kind:        kind,
		load:        load,
		close:       close,
	}
}

// acquire returns the cached model for name, loading it if needed, with its
// in-flight count incremented. Callers must pair it with release.
func (c *modelCache[M]) acquire(name string) (*cacheEntry[M], error) {
	c.mu.Lock()
	if e, ok := c.loaded[name]; ok {
		e.inflight++
		c.mu.Unlock()
		return e, nil
	}
	p, ok := c.paths[name]
	c.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%s: unknown model %q", c.kind, name)
	}

	abs, err := config.ExpandPath(p)
	if err != nil {
		return nil, err
	}
	// The load runs a potentially multi-second cgo call; it must not hold
	// c.mu, or a concurrent request for a different, already-cached model
	// would block on it for no reason.
	m, err := c.load(abs)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.loaded[name]; ok {
		// Another goroutine won the race and cached its instance first;
		// close our redundant one and use theirs.
		c.close(m)
		existing.inflight++
		return existing, nil
	}
	e := &cacheEntry[M]{m: m, inflight: 1}
	c.loaded[name] = e
	return e, nil
}

// preload loads name into the cache without running inference. Releasing the
// temporary acquisition also starts the configured idle timer, if any.
func (c *modelCache[M]) preload(name string) error {
	e, err := c.acquire(name)
	if err != nil {
		return err
	}
	c.release(name, e)
	return nil
}

// release ends a use begun by acquire and, once the model is idle, arms the
// unload timer.
func (c *modelCache[M]) release(name string, e *cacheEntry[M]) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e.inflight--
	e.lastUse = time.Now()
	if c.unloadAfter <= 0 || e.inflight > 0 {
		return
	}
	if e.timer == nil {
		e.timer = time.AfterFunc(c.unloadAfter, func() { c.maybeUnload(name) })
	} else {
		e.timer.Reset(c.unloadAfter)
	}
}

// maybeUnload frees the named model if it has been idle for the configured
// duration. Fired by the entry's timer; if a use is in flight the unload is
// skipped and the next release re-arms the timer.
func (c *modelCache[M]) maybeUnload(name string) {
	c.mu.Lock()
	e, ok := c.loaded[name]
	if !ok || e.inflight > 0 {
		c.mu.Unlock()
		return
	}
	if idle := time.Since(e.lastUse); idle < c.unloadAfter {
		// Used again after this timer was armed; try again later.
		e.timer.Reset(c.unloadAfter - idle)
		c.mu.Unlock()
		return
	}
	delete(c.loaded, name)
	c.mu.Unlock()
	c.close(e.m)
	slog.Info("model unloaded after idle timeout", "driver", c.kind,
		"model", name, "unload_after", c.unloadAfter)
}
