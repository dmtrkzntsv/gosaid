package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
)

// CaptureSampleRate is fixed at 16kHz mono — matches Whisper's internal rate,
// so no resampling is needed before upload.
const CaptureSampleRate = 16000

// Capturer records from the default input device. Single-use per Start/Stop cycle.
type Capturer struct {
	mu      sync.Mutex
	ctx     *malgo.AllocatedContext
	device  *malgo.Device
	samples []float32
	active  bool
}

// NewCapturer initializes the audio backend. Close() frees it.
func NewCapturer() (*Capturer, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("init audio context: %w", err)
	}
	return &Capturer{ctx: ctx}, nil
}

// Close releases the audio backend.
func (c *Capturer) Close() {
	c.mu.Lock()
	if c.device != nil {
		c.device.Uninit()
		c.device = nil
	}
	c.mu.Unlock()
	if c.ctx != nil {
		_ = c.ctx.Uninit()
		c.ctx.Free()
		c.ctx = nil
	}
}

// Start begins capture. Returns an error if already capturing.
func (c *Capturer) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return errors.New("capture already active")
	}

	devCfg := malgo.DefaultDeviceConfig(malgo.Capture)
	devCfg.Capture.Format = malgo.FormatS16
	devCfg.Capture.Channels = 1
	devCfg.SampleRate = CaptureSampleRate
	devCfg.Alsa.NoMMap = 1

	c.samples = c.samples[:0]

	onRecv := func(_, pSample []byte, framecount uint32) {
		// Int16 -> float32 / 32768.
		frames := int(framecount)
		c.mu.Lock()
		for i := range frames {
			s := int16(binary.LittleEndian.Uint16(pSample[i*2:]))
			c.samples = append(c.samples, float32(s)/32768.0)
		}
		c.mu.Unlock()
	}

	dev, err := malgo.InitDevice(c.ctx.Context, devCfg, malgo.DeviceCallbacks{Data: onRecv})
	if err != nil {
		return fmt.Errorf("init capture device: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return fmt.Errorf("start capture: %w", err)
	}
	c.device = dev
	c.active = true
	return nil
}

// Stop halts capture and returns the accumulated samples.
func (c *Capturer) Stop() ([]float32, error) {
	c.mu.Lock()
	if !c.active {
		c.mu.Unlock()
		return nil, errors.New("capture not active")
	}
	dev := c.device
	c.device = nil
	c.active = false
	out := c.samples
	c.samples = nil
	c.mu.Unlock()

	if dev != nil {
		dev.Uninit()
	}
	return out, nil
}
