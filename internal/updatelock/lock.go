package updatelock

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const Suffix = ".update-lock.json"

type State struct {
	Version    string    `json:"version,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	UpdaterPID int       `json:"updater_pid"`
}

func Path(target string) (string, error) {
	absolute, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute) + Suffix, nil
}

func Write(target, version string, pid int) error {
	path, err := Path(target)
	if err != nil {
		return err
	}
	state := State{Version: version, StartedAt: time.Now().UTC(), UpdaterPID: pid}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write update lock %s: %w", path, err)
	}
	return nil
}

func Read(target string) (State, error) {
	path, err := Path(target)
	if err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode update lock %s: %w", path, err)
	}
	return state, nil
}

func Remove(target string) error {
	path, err := Path(target)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove update lock %s: %w", path, err)
	}
	return nil
}
