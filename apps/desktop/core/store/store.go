// Package store persists hiik's layout and settings.
//
// state.json and settings.json live in HIIK_HOME, or os.UserConfigDir()/hiik
// when HIIK_HOME is unset. Writes are atomic (temporary file, sync, rename)
// and debounced. An invalid file is renamed aside and defaults are used; a
// file from a newer app version is kept aside and never downgraded.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DefaultDebounce is how long a save waits for another change.
const DefaultDebounce = 200 * time.Millisecond

// Store reads and writes one config directory.
type Store struct {
	dir   string
	delay time.Duration

	mu         sync.Mutex
	timer      *time.Timer
	pending    *State
	pendingSet *Settings
	closed     bool
	warnings   []string
	lastErr    error

	writeMu sync.Mutex
}

// Dir returns the config directory: HIIK_HOME, or the user config directory
// plus "hiik".
func Dir() (string, error) {
	if d := os.Getenv("HIIK_HOME"); d != "" {
		return d, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("store: finding the config directory: %w", err)
	}
	return filepath.Join(d, "hiik"), nil
}

// Open opens the default config directory.
func Open() (*Store, error) {
	return New("")
}

// New opens dir, or the default directory when dir is empty.
func New(dir string) (*Store, error) {
	return newStore(dir, DefaultDebounce)
}

func newStore(dir string, delay time.Duration) (*Store, error) {
	if dir == "" {
		d, err := Dir()
		if err != nil {
			return nil, err
		}
		dir = d
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: creating %s: %w", dir, err)
	}
	if delay <= 0 {
		delay = DefaultDebounce
	}
	return &Store{dir: dir, delay: delay}, nil
}

// Dir returns the store's directory.
func (s *Store) Dir() string { return s.dir }

// StatePath returns the layout file's path.
func (s *Store) StatePath() string { return filepath.Join(s.dir, "state.json") }

// SettingsPath returns the settings file's path.
func (s *Store) SettingsPath() string { return filepath.Join(s.dir, "settings.json") }

// Warnings returns notes about files that were kept aside or values that
// fell back to their defaults.
func (s *Store) Warnings() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.warnings...)
}

// LastError returns the most recent write error, or nil.
func (s *Store) LastError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

func (s *Store) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.mu.Lock()
	s.warnings = append(s.warnings, msg)
	s.mu.Unlock()
}

// SaveState queues a debounced write of the layout.
func (s *Store) SaveState(st State) {
	st.Version = Version
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.pending = &st
	s.scheduleLocked()
	s.mu.Unlock()
}

// SaveSettings queues a debounced write of the settings.
func (s *Store) SaveSettings(se Settings) {
	se.Normalize()
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.pendingSet = &se
	s.scheduleLocked()
	s.mu.Unlock()
}

func (s *Store) scheduleLocked() {
	if s.timer != nil {
		s.timer.Reset(s.delay)
		return
	}
	s.timer = time.AfterFunc(s.delay, s.onTimer)
}

func (s *Store) onTimer() {
	s.mu.Lock()
	s.timer = nil
	st, se := s.pending, s.pendingSet
	s.pending, s.pendingSet = nil, nil
	s.mu.Unlock()
	if st != nil || se != nil {
		_ = s.write(st, se)
	}
}

// Flush writes anything pending now and returns the first error.
func (s *Store) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	st, se := s.pending, s.pendingSet
	s.pending, s.pendingSet = nil, nil
	s.mu.Unlock()
	if st == nil && se == nil {
		return nil
	}
	return s.write(st, se)
}

// Close flushes pending writes and makes later saves no-ops.
func (s *Store) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	return s.Flush()
}

func (s *Store) write(st *State, se *Settings) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	var first error
	if st != nil {
		st.Version = Version
		data, err := json.MarshalIndent(st, "", "  ")
		if err != nil {
			first = fmt.Errorf("store: encoding state: %w", err)
		} else if err := writeFileAtomic(s.StatePath(), append(data, '\n')); err != nil {
			first = fmt.Errorf("store: writing state: %w", err)
		}
	}
	if se != nil {
		data, err := json.MarshalIndent(se, "", "  ")
		if err != nil {
			if first == nil {
				first = fmt.Errorf("store: encoding settings: %w", err)
			}
		} else if err := writeFileAtomic(s.SettingsPath(), append(data, '\n')); err != nil {
			if first == nil {
				first = fmt.Errorf("store: writing settings: %w", err)
			}
		}
	}
	if first != nil {
		s.mu.Lock()
		s.lastErr = first
		s.mu.Unlock()
	}
	return first
}

// LoadState reads state.json. A missing file returns the defaults. Invalid
// JSON is renamed aside; a newer version is kept aside; both use defaults.
func (s *Store) LoadState() (State, error) {
	path := s.StatePath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultState(), nil
	}
	if err != nil {
		return DefaultState(), fmt.Errorf("store: reading state: %w", err)
	}

	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		s.keepAside(path, "corrupt")
		return DefaultState(), nil
	}
	switch {
	case head.Version > Version:
		s.keepAside(path, fmt.Sprintf("newer-%d", head.Version))
		return DefaultState(), nil
	case head.Version < 1:
		s.keepAside(path, "corrupt")
		return DefaultState(), nil
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		s.keepAside(path, "corrupt")
		return DefaultState(), nil
	}
	st.Version = Version
	return st, nil
}

// LoadSettings reads settings.json. A missing or invalid file returns the
// defaults; invalid JSON is renamed aside.
func (s *Store) LoadSettings() (Settings, error) {
	path := s.SettingsPath()
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return defaultSettings(), nil
	}
	if err != nil {
		return defaultSettings(), fmt.Errorf("store: reading settings: %w", err)
	}
	var se Settings
	if err := json.Unmarshal(data, &se); err != nil {
		s.keepAside(path, "corrupt")
		return defaultSettings(), nil
	}
	for _, warning := range se.Normalize() {
		s.warn("%s", warning)
	}
	return se, nil
}

func defaultSettings() Settings {
	se := DefaultSettings()
	se.Normalize()
	return se
}

// keepAside renames a file that must not be overwritten. The original is
// kept as <name>.<reason>-<unix>.
func (s *Store) keepAside(path, reason string) {
	stamp := time.Now().Unix()
	target := fmt.Sprintf("%s.%s-%d", path, reason, stamp)
	for i := 1; ; i++ {
		if _, err := os.Lstat(target); errors.Is(err, fs.ErrNotExist) {
			break
		}
		target = fmt.Sprintf("%s.%s-%d-%d", path, reason, stamp, i)
	}
	if err := os.Rename(path, target); err != nil {
		s.warn("store: keeping %s aside: %v", filepath.Base(path), err)
		return
	}
	s.warn("store: %s was invalid (%s); kept as %s", filepath.Base(path), reason, filepath.Base(target))
}

// writeFileAtomic writes a temporary file in the destination directory,
// syncs it, then renames it into place.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		if tmp != "" {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	tmp = ""
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
