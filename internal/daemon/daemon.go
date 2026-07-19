package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/dmtrkzntsv/gosaid/internal/audio"
	"github.com/dmtrkzntsv/gosaid/internal/config"
	"github.com/dmtrkzntsv/gosaid/internal/drivers"
	"github.com/dmtrkzntsv/gosaid/internal/hotkey"
	"github.com/dmtrkzntsv/gosaid/internal/inject"
	"github.com/dmtrkzntsv/gosaid/internal/platform"
)

// Version is stamped by main; exposed for logging.
var Version = "dev"

// Run is the daemon entrypoint. It wires together every subsystem, blocks
// until SIGINT/SIGTERM, and returns after graceful shutdown.
func Run(injector inject.Injector) error {
	cfgPath, err := config.Path()
	if err != nil {
		return fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		return fmt.Errorf("config invalid: %w", err)
	}

	log := InitLogger(cfg.LogLevel)
	log.Info("starting",
		"version", Version,
		"os", runtime.GOOS, "arch", runtime.GOARCH,
		"config", cfgPath,
	)
	if cfg.Version != config.CurrentVersion {
		log.Warn("config version mismatch — unknown fields are ignored; update the `version` field to silence this warning",
			"got", cfg.Version, "expected", config.CurrentVersion)
	}

	pidPath, err := platform.PIDFile()
	if err != nil {
		return err
	}
	if err := Acquire(pidPath); err != nil {
		return err
	}
	defer func() { _ = Release(pidPath) }()

	// Expose live daemon status (state, pid, binary path, last injection)
	// to external tools via state.json. Non-fatal — the daemon runs even
	// if the file can't be written.
	exe, err := os.Executable()
	if err != nil {
		log.Warn("resolve executable path", "err", err)
		exe = ""
	}
	if err := InitStateFile(os.Getpid(), exe); err != nil {
		log.Warn("init state file", "err", err)
	}
	defer func() { _ = ClearStateFile() }()

	reg, err := drivers.BuildRegistry(cfg)
	if err != nil {
		return err
	}

	capturer, err := audio.NewCapturer()
	if err != nil {
		return fmt.Errorf("init audio capture: %w", err)
	}
	defer capturer.Close()

	feedback, err := audio.NewFeedback(cfg.SoundFeedback)
	if err != nil {
		return fmt.Errorf("init feedback: %w", err)
	}
	defer feedback.Close()

	core := NewCore()
	WireFeedback(core, feedback)
	core.Subscribe(func(e StateEvent) {
		if e.Err != nil {
			log.Warn("state", "from", e.Previous.String(), "to", e.State.String(), "err", e.Err)
			return
		}
		log.Debug("state", "from", e.Previous.String(), "to", e.State.String())
	})
	// Mirror every transition into state.json so the UI can react without
	// IPC. Errors here are non-fatal and only logged at Warn.
	core.Subscribe(func(e StateEvent) {
		if err := UpdateState(e.State); err != nil {
			log.Warn("state file", "err", err)
		}
	})

	pipe := &Pipeline{
		Core:       core,
		Capture:    capturer,
		Registry:   reg,
		Injector:   injector,
		Config:     cfg,
		SampleRate: audio.CaptureSampleRate,
		Log:        log,
	}

	mgr := hotkey.NewManager(time.Duration(cfg.ToggleMaxSeconds) * time.Second)
	defer mgr.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Selection capture is available only for injectors that support it
	// (PasteInjector does; the test Stub doesn't). Without it, compose
	// hotkeys behave as before: fresh composition, no clipboard touch.
	selReader, _ := injector.(inject.SelectionReader)

	for combo, hk := range cfg.Hotkeys {
		combo, hk := combo, hk
		mode := hotkey.Mode(hk.Mode)
		if mode == "" {
			mode = hotkey.ModeHold
		}
		// captureLive tracks whether the most recent OnTrigger actually
		// started capture. OnStop reads it to decide whether to run the
		// pipeline — guards against a Start() failure followed by error
		// auto-recovery (Error→Idle) racing the user's hotkey release.
		// The selection-capture goroutine also swaps it to abort the run
		// when copy synthesis fails while recording is still live.
		var captureLive atomic.Bool
		// selCh carries the selection-capture result from press to release.
		// OnTrigger and OnStop run on the hotkey manager's event goroutine,
		// so plain assignment is safe.
		var selCh chan inject.SelectionResult
		handler := hotkey.Handler{
			OnTrigger: func() {
				captureLive.Store(false)
				selCh = nil
				if !core.TryStartRecording() {
					log.Debug("hotkey press ignored — core busy", "combo", combo)
					return
				}
				opened, err := capturer.Start(hk.Microphone)
				if err != nil {
					core.Transition(StateError, err)
					return
				}
				if hk.Microphone != "" && !audio.MatchesDevice(opened, hk.Microphone) {
					log.Warn("configured microphone not found — using fallback",
						"combo", combo, "want", hk.Microphone, "using", opened)
				}
				captureLive.Store(true)
				if hk.Compose.IsEnabled() && selReader != nil {
					ch := make(chan inject.SelectionResult, 1)
					selCh = ch
					go func() {
						res := selReader.GetSelection(ctx)
						// Copy synthesis failure means the later paste would
						// fail too — abort now with the error cue rather than
						// composing text the user meant as a rewrite command.
						// Swap decides the winner if release races us: whoever
						// flips captureLive first owns stopping the capturer.
						if res.Err != nil && captureLive.Swap(false) {
							_, _ = capturer.Stop()
							core.Transition(StateError, res.Err)
						}
						ch <- res
					}()
				}
			},
			OnStop: func() {
				if !captureLive.Swap(false) {
					log.Debug("hotkey release ignored — capture never started", "combo", combo)
					return
				}
				sel := selCh
				go func() {
					pctx, pcancel := context.WithTimeout(ctx, 90*time.Second)
					defer pcancel()
					if err := pipe.Run(pctx, hk, sel); err != nil {
						log.Error("pipeline", "combo", combo, "err", err)
					}
				}()
			},
		}
		if err := mgr.Register(combo, mode, handler); err != nil {
			return fmt.Errorf("register hotkey %q: %w", combo, err)
		}
		log.Info("hotkey registered", "combo", combo, "mode", mode)
	}

	fmt.Fprintln(os.Stderr, "gosaid running — press configured hotkey to dictate, Ctrl+C to quit")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Info("shutting down")

	cancel()
	// Best-effort drain — audio close + hotkey unregister run via defers.
	time.Sleep(200 * time.Millisecond)
	return nil
}
