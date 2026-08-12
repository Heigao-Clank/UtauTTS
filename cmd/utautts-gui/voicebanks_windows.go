//go:build windows

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"utautts/internal/tts"
	"utautts/internal/voicebank"
)

type voicebankScanResult struct {
	root         string
	previousPath string
	banks        []voicebank.Summary
	err          error
}

var (
	voicebankScanInProgress bool
	voicebankScanMutex      sync.Mutex
	completedVoicebankScan  voicebankScanResult
)

func refreshVoicebanks(hwnd uintptr) {
	if voicebankScanInProgress {
		return
	}
	root := voicebank.ResolveDirectory(voiceDirectory)
	tts.ClearCaches()
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
	voicebankScanInProgress = true
	configuredInitial := initialVoicebank
	go func() {
		result := voicebankScanResult{root: root, previousPath: previousPath}
		result.err = runSafely("音源検索", func() error {
			result.banks, result.err = discoverVoicebanks(root, configuredInitial)
			return result.err
		})
		voicebankScanMutex.Lock()
		completedVoicebankScan = result
		voicebankScanMutex.Unlock()
		log.Printf("voicebank scan finished: count=%d elapsed=%s error=%v", len(result.banks), time.Since(started).Round(time.Millisecond), result.err)
		postMessage.Call(hwnd, wmVoicebanksDone, 0, 0)
	}()
}

func discoverVoicebanks(root, configuredInitial string) ([]voicebank.Summary, error) {
	banks, discoverErr := voicebank.Discover(root)
	if configuredInitial == "" {
		return banks, discoverErr
	}
	bank, inspectErr := voicebank.Inspect(configuredInitial)
	if inspectErr != nil {
		return banks, discoverErr
	}
	for _, candidate := range banks {
		if samePath(candidate.Path, bank.Path) {
			return banks, discoverErr
		}
	}
	return append(banks, bank), discoverErr
}

func finishVoicebankRefresh(hwnd uintptr) {
	voicebankScanMutex.Lock()
	result := completedVoicebankScan
	completedVoicebankScan = voicebankScanResult{}
	voicebankScanMutex.Unlock()
	voicebankScanInProgress = false
	availableBanks = result.banks
	combo := child(hwnd, idVoicebank)
	sendMessage.Call(combo, cbResetContent, 0, 0)
	selected := 0
	for index, bank := range result.banks {
		comboAdd(combo, voicebankDisplayName(index))
		if result.previousPath != "" && samePath(bank.Path, result.previousPath) {
			selected = index
		} else if result.previousPath == "" && initialVoicebank != "" && samePath(bank.Path, initialVoicebank) {
			selected = index
		}
	}
	enableWindow.Call(reloadButton, 1)
	enableWindow.Call(combo, 1)
	if len(result.banks) > 0 {
		sendMessage.Call(combo, cbSetCurSel, uintptr(selected), 0)
		updateVoicebankPortrait(selected)
		updateVoicebankSummary(selected)
		setText(statusLabel, fmt.Sprintf("%d音源を読込: %s", len(result.banks), result.root))
	} else if result.err != nil {
		updateVoicebankPortrait(-1)
		updateVoicebankSummary(-1)
		setText(statusLabel, "音源がありません: "+result.root)
	} else {
		updateVoicebankPortrait(-1)
		updateVoicebankSummary(-1)
		setText(statusLabel, "音源がありません")
	}
}

func updateVoicebankSummary(index int) {
	if voiceSummaryLabel == 0 {
		return
	}
	if index < 0 || index >= len(availableBanks) {
		setText(voiceSummaryLabel, "音源情報はありません")
		return
	}
	bank := availableBanks[index]
	image := "画像なし"
	if bank.ImagePath != "" {
		image = "画像: " + filepath.Base(bank.ImagePath)
	}
	readme := "readmeなし"
	if bank.ReadmePath != "" {
		readme = "説明: " + filepath.Base(bank.ReadmePath)
	}
	setText(voiceSummaryLabel, bank.Name+"\r\n"+bank.Path+"\r\n"+image+" / "+readme+"（画像を押すと詳細を表示）")
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
