package daemon

import "github.com/dmtrkzntsv/gosaid/internal/audio"

// WireFeedback subscribes audio cues to state transitions:
//   - Idle → Recording : start cue
//   - Injecting → Idle : stop cue (one successful round-trip)
//   - * → Error        : error cue
func WireFeedback(c *Core, fb *audio.Feedback) {
	c.Subscribe(func(e StateEvent) {
		switch {
		case e.State == StateRecording && e.Previous == StateIdle:
			go fb.PlayStart()
		case e.State == StateIdle && e.Previous == StateInjecting:
			go fb.PlayStop()
		case e.State == StateError:
			go fb.PlayError()
		}
	})
}
