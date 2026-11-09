package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hiik/core/store"
)

// SaveFeedback writes a feedback draft under the config directory. It never
// sends anything; the person can inspect or copy the file.
func (a *App) SaveFeedback(text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("feedback is empty")
	}
	dir, err := store.Dir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "feedback")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	name := time.Now().UTC().Format("20060102-150405") + ".md"
	path := filepath.Join(dir, name)
	body := "# hiik feedback\n\n" + text + "\n\n---\nversion: " + Version + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return "", err
	}
	return path, nil
}
