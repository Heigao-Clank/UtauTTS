//go:build windows

package main

import (
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"syscall"
)

func runSafely(scope string, operation func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = logRecoveredPanic(scope, recovered)
		}
	}()
	return operation()
}

func logRecoveredPanic(scope string, recovered any) error {
	err := panicError(scope, recovered)
	log.Printf("%v\n%s", err, debug.Stack())
	return err
}

func panicError(scope string, recovered any) error {
	return fmt.Errorf("%sで内部エラーが発生しました: %v", scope, recovered)
}

func windowsString(value string) []uint16 {
	value = strings.ReplaceAll(value, "\x00", "�")
	buffer, err := syscall.UTF16FromString(value)
	if err != nil {
		return []uint16{'�', 0}
	}
	return buffer
}
