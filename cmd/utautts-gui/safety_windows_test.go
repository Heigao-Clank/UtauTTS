//go:build windows

package main

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestRunSafelyConvertsPanicToError(t *testing.T) {
	err := runSafely("テスト", func() error { panic("broken bank") })
	if err == nil || !strings.Contains(err.Error(), "テスト") || !strings.Contains(err.Error(), "broken bank") {
		t.Fatalf("error = %v", err)
	}
	want := errors.New("ordinary error")
	if got := runSafely("テスト", func() error { return want }); !errors.Is(got, want) {
		t.Fatalf("ordinary error = %v", got)
	}
}

func TestWindowsStringReplacesEmbeddedNUL(t *testing.T) {
	got := syscall.UTF16ToString(windowsString("音\x00源"))
	if got != "音�源" {
		t.Fatalf("windows string = %q", got)
	}
}
