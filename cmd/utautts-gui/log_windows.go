//go:build windows

package main

import (
	"log"
	"os"
	"path/filepath"
)

func initializeLog() func() {
	directory := "."
	if executable, err := os.Executable(); err == nil {
		directory = filepath.Dir(executable)
	}
	path := filepath.Join(directory, "utautts-gui.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}
	}
	log.SetOutput(file)
	log.Printf("UtauTTS GUI started: executable=%q cwd=%q", filepath.Join(directory, filepath.Base(os.Args[0])), currentDirectory())
	return func() { _ = file.Close() }
}

func currentDirectory() string {
	directory, err := os.Getwd()
	if err != nil {
		return "<unknown>"
	}
	return directory
}
