package audio

import (
	_ "embed"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

//go:embed sounds/start.wav
var startWAV []byte

//go:embed sounds/stop.wav
var stopWAV []byte

//go:embed sounds/error.wav
var errorWAV []byte

// Feedback plays the three status cues. Safe to call play methods concurrently;
// each call spawns a short-lived playback goroutine.
type Feedback struct {
	enabled bool
	ctx     *malgo.AllocatedContext
	start   PCM
	stop    PCM
	errSnd  PCM

	mu sync.Mutex
}

func NewFeedback(enabled bool) (*Feedback, error) {
	f := &Feedback{enabled: enabled}
	if !enabled {
		return f, nil
	}
	var err error
	if f.start, err = ParseWAV(startWAV); err != nil {
		return nil, err
	}
	if f.stop, err = ParseWAV(stopWAV); err != nil {
		return nil, err
	}
	if f.errSnd, err = ParseWAV(errorWAV); err != nil {
		return nil, err
	}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, err
	}
	f.ctx = ctx
	return f, nil
}

func (f *Feedback) Close() {
	if f.ctx != nil {
		_ = f.ctx.Uninit()
		f.ctx.Free()
		f.ctx = nil
	}
}

func (f *Feedback) PlayStart() { f.play(f.start) }
func (f *Feedback) PlayStop()  { f.play(f.stop) }
func (f *Feedback) PlayError() { f.play(f.errSnd) }

// play blocks while the cue is running. Typical cues are 150-300ms.
// Callers should invoke via `go f.PlayX()` if they want fire-and-forget.
func (f *Feedback) play(p PCM) {
	if !f.enabled || f.ctx == nil || len(p.Data) == 0 {
		return
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Playback)
	cfg.Playback.Format = malgo.FormatS16
	cfg.Playback.Channels = uint32(p.Channels)
	cfg.SampleRate = uint32(p.SampleRate)

	f.mu.Lock()
	data := p.Data
	pos := 0
	done := make(chan struct{})
	f.mu.Unlock()

	onSend := func(out, _ []byte, framecount uint32) {
		bytesNeeded := int(framecount) * int(cfg.Playback.Channels) * 2
		remaining := len(data) - pos
		n := min(bytesNeeded, remaining)
		if n > 0 {
			copy(out[:n], data[pos:pos+n])
			pos += n
		}
		if n < bytesNeeded {
			for i := n; i < bytesNeeded; i++ {
				out[i] = 0
			}
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}

	dev, err := malgo.InitDevice(f.ctx.Context, cfg, malgo.DeviceCallbacks{Data: onSend})
	if err != nil {
		return
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return
	}
	// Wait for callback to signal EOF, capped so a stuck device doesn't hang us.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	dev.Uninit()
}
