package daemon

import "github.com/dmtrkzntsv/gosaid/internal/audio"

// WireFeedback subscribes audio cues to state transitions:
//   - Idle → Recording         : start cue
//   - Recording → Transcribing : stop cue (fires immediately on hotkey release,
//     before the transcription/injection delay — gives the user prompt feedback
//     that their release was registered)
//   - * → Error                : error cue
func WireFeedback(c *Core, fb *audio.Feedback) {
	c.Subscribe(func(e StateEvent) {
		switch {
		case e.State == StateRecording && e.Previous == StateIdle:
			go fb.PlayStart()
		case e.State == StateTranscribing && e.Previous == StateRecording:
			go fb.PlayStop()
		case e.State == StateError:
			go fb.PlayError()
		}
	})
}
