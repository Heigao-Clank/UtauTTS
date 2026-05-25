package engine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type Config struct {
	OtoPath   string
	Text      string
	OutPath   string
	ModelPath string
	Python    string
	ToolsDir  string
}

func Synthesize(cfg Config) error {
	engineExe := filepath.Join(cfg.ToolsDir, "engine.exe")
	if _, err := os.Stat(engineExe); err == nil {
		cmd := exec.Command(engineExe,
			"--text", cfg.Text,
			"--oto", cfg.OtoPath,
			"--model", cfg.ModelPath,
			"--out", cfg.OutPath,
		)
		cmd.Stderr = os.Stderr
		cmd.Stdout = os.Stdout
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("engine: %w", err)
		}
		return nil
	}

	script := filepath.Join(cfg.ToolsDir, "engine.py")
	cmd := exec.Command(cfg.Python, script,
		"--text", cfg.Text,
		"--oto", cfg.OtoPath,
		"--model", cfg.ModelPath,
		"--out", cfg.OutPath,
	)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("engine: %w", err)
	}
	return nil
}
