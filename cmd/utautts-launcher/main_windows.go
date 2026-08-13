//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

func main() {
	executable, err := os.Executable()
	if err != nil {
		showError(err)
		return
	}
	root := filepath.Dir(executable)
	target := filepath.Join(root, "app", "utautts-gui.exe")
	command := exec.Command(target, os.Args[1:]...)
	command.Dir = root
	if err := command.Start(); err != nil {
		showError(fmt.Errorf("Qt GUIを起動できませんでした。\n%s\n\n%w", target, err))
	}
}

func showError(err error) {
	text, _ := syscall.UTF16PtrFromString(err.Error())
	title, _ := syscall.UTF16PtrFromString("UtauTTS 起動エラー")
	messageBox := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
