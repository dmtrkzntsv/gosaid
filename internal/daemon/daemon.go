package daemon

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
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
		"translate_template", TranslateTemplateVersion,
		"enhance_template", EnhanceTemplateVersion,
	)

	pidPath, err := platform.PIDFile()
	if err != nil {
		return err
	}
	if err := Acquire(pidPath); err != nil {
		return err
	}
	defer func() { _ = Release(pidPath) }()

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

	for combo, hk := range cfg.Hotkeys {
		combo, hk := combo, hk
		mode := hotkey.Mode(hk.Mode)
		if mode == "" {
			mode = hotkey.ModeHold
		}
		handler := hotkey.Handler{
			OnTrigger: func() {
				if !core.TryStartRecording() {
					log.Debug("hotkey press ignored — core busy", "combo", combo)
					return
				}
				if err := capturer.Start(); err != nil {
					core.Transition(StateError, err)
				}
			},
			OnStop: func() {
				go func() {
					pctx, pcancel := context.WithTimeout(ctx, 90*time.Second)
					defer pcancel()
					if err := pipe.Run(pctx, hk); err != nil {
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
