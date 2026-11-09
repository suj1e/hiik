package ui

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/audio"
	_ "github.com/egoist/mygo/audio/mp3"
	_ "github.com/egoist/mygo/audio/vorbis"
	mui "github.com/egoist/mygo/ui"

	"hiik/core/store"
	"hiik/mods/api"
)

// The notification tone: two short notes, quiet and faded at both ends so it
// does not click. It is generated once and played over itself, and stands in
// for a custom sound that is not set or cannot be read.
const (
	toneRate   = 44100
	toneVolume = 0.16
	noteLength = 90 * time.Millisecond
)

var (
	soundOnce    sync.Once
	soundDone    *audio.Sound
	soundBlocked *audio.Sound
)

// soundSet is the user's sounds, loaded when their paths change. A path
// that is empty or cannot be decoded keeps the generated tone.
type soundSet struct {
	donePath    string
	blockedPath string
	done        *audio.Sound
	blocked     *audio.Sound
}

var customSounds struct {
	sync.Mutex
	set soundSet
}

// init configures the user's sounds from the settings once per frame. The
// hook is cheap: it loads a file only when its path changed.
func init() {
	registerShellHook(func(s *Shell, _ *mui.Context) {
		configureSounds(s.app.Settings())
	})
}

// PlayNotification plays the notification sound for level: the user's file
// when one is configured, else the generated tone. The settings panel uses
// it to preview the blocked and done sounds.
func PlayNotification(level string) {
	playNotificationSound(level)
}

// playNotificationSound plays the sound for a level. It never waits for the
// device: audio starts the voice in the background, and where no output can
// open the player still runs silently.
func playNotificationSound(level string) {
	if s := configuredSound(level); s != nil {
		p := s.NewPlayer()
		p.Play()
		return
	}
	soundOnce.Do(func() {
		soundDone = audio.NewSound(toneRate, 1, notes([]float64{523.25, 659.25}))
		soundBlocked = audio.NewSound(toneRate, 1, notes([]float64{392.00, 329.63}))
	})
	s := soundDone
	if level == api.LevelBlocked {
		s = soundBlocked
	}
	p := s.NewPlayer()
	p.SetVolume(toneVolume)
	p.Play()
}

// configuredSound returns the user's sound for a level, or nil.
func configuredSound(level string) *audio.Sound {
	customSounds.Lock()
	defer customSounds.Unlock()
	if level == api.LevelBlocked {
		return customSounds.set.blocked
	}
	return customSounds.set.done
}

// configureSounds loads the files the settings name when a path changed.
// Paths are absolute, or relative to HIIK_HOME.
func configureSounds(settings map[string]any) {
	donePath := resolveSoundPath(settingString(settings, "sound_done"))
	blockedPath := resolveSoundPath(settingString(settings, "sound_blocked"))
	customSounds.Lock()
	defer customSounds.Unlock()
	if donePath == customSounds.set.donePath && blockedPath == customSounds.set.blockedPath {
		return
	}
	customSounds.set = soundSet{
		donePath:    donePath,
		blockedPath: blockedPath,
		done:        loadSound(donePath),
		blocked:     loadSound(blockedPath),
	}
}

// resolveSoundPath makes a configured path absolute: a configured path may
// be relative to HIIK_HOME.
func resolveSoundPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	dir, err := store.Dir()
	if err != nil {
		return path
	}
	return filepath.Join(dir, path)
}

// loadSound decodes a sound file once; nil when it cannot be read, so that
// playback falls back to the generated tone.
func loadSound(path string) *audio.Sound {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	s, err := audio.DecodeSound(data)
	if err != nil {
		return nil
	}
	return s
}

// notes joins short sine notes, equal in length, into one mono sound.
func notes(hz []float64) []float32 {
	per := int(toneRate * noteLength.Seconds())
	out := make([]float32, 0, per*len(hz))
	for _, f := range hz {
		out = append(out, tone(f, per)...)
	}
	return out
}

// tone returns n frames of a sine at hz, faded in and out.
func tone(hz float64, n int) []float32 {
	out := make([]float32, n)
	fade := max(n/6, 1)
	for i := range out {
		env := 1.0
		if i < fade {
			env = float64(i) / float64(fade)
		}
		if rest := n - 1 - i; rest < fade {
			env = math.Min(env, float64(rest)/float64(fade))
		}
		out[i] = float32(math.Sin(2*math.Pi*hz*float64(i)/toneRate) * env)
	}
	return out
}
