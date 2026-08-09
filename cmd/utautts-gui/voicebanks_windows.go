//go:build windows

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"utautts/internal/voicebank"
)

func refreshVoicebanks(hwnd uintptr) {
	root := voicebank.ResolveDirectory(voiceDirectory)
	started := time.Now()
	log.Printf("voicebank scan started: configured=%q resolved=%q", voiceDirectory, root)
	combo := child(hwnd, idVoicebank)
	enableWindow.Call(reloadButton, 0)
	enableWindow.Call(combo, 0)
	setText(statusLabel, "音源を検索中...")
	previousPath := ""
	if selected, _, _ := sendMessage.Call(combo, cbGetCurSel, 0, 0); int(selected) >= 0 && int(selected) < len(availableBanks) {
		previousPath = availableBanks[int(selected)].Path
	}
	banks, err := voicebank.Discover(root)
	if initialVoicebank != "" {
		if bank, inspectErr := voicebank.Inspect(initialVoicebank); inspectErr == nil {
			found := false
			for _, candidate := range banks {
				if samePath(candidate.Path, bank.Path) {
					found = true
					break
				}
			}
			if !found {
				banks = append(banks, bank)
			}
		}
	}
	availableBanks = banks
	sendMessage.Call(combo, cbResetContent, 0, 0)
	selected := 0
	for index, bank := range banks {
		comboAdd(combo, voicebankDisplayName(index))
		if previousPath != "" && samePath(bank.Path, previousPath) {
			selected = index
		} else if previousPath == "" && initialVoicebank != "" && samePath(bank.Path, initialVoicebank) {
			selected = index
		}
	}
	enableWindow.Call(reloadButton, 1)
	enableWindow.Call(combo, 1)
	if len(banks) > 0 {
		sendMessage.Call(combo, cbSetCurSel, uintptr(selected), 0)
		setText(statusLabel, fmt.Sprintf("%d音源を読込: %s", len(banks), root))
	} else if err != nil {
		setText(statusLabel, "音源がありません: "+root)
	} else {
		setText(statusLabel, "音源がありません")
	}
	log.Printf("voicebank scan finished: count=%d elapsed=%s error=%v", len(banks), time.Since(started).Round(time.Millisecond), err)
}

func voicebankDisplayName(index int) string {
	bank := availableBanks[index]
	display := sanitizeDisplayName(bank.Name)
	for otherIndex, other := range availableBanks {
		if otherIndex != index && other.Name == bank.Name {
			return display + " (" + filepath.Base(bank.Path) + ")"
		}
	}
	return display
}

func sanitizeDisplayName(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.TrimSpace(value))
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120]) + "…"
	}
	if value == "" {
		return "名称未設定"
	}
	return value
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
}
