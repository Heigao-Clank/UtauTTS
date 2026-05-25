package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("UtauTTS - UTAU voicebank TTS")
		fmt.Println()
		fmt.Println("  utautts.exe --oto <oto.ini> --text <text> --model <model> --out <wav>")
		fmt.Println()
		fmt.Println("For HTTP server: utautts-server.exe")
		return
	}

	exe, _ := os.Executable()
	baseDir := filepath.Dir(exe)
	coreExe := filepath.Join(baseDir, "core", "utautts-core.exe")

	args := append([]string{coreExe}, os.Args[1:]...)
	cmd := exec.Command(coreExe, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = args
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
