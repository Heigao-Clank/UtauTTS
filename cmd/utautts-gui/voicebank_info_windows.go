//go:build windows

package main

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"utautts/internal/voicebank"
)

const (
	idVoicePortrait = 1101
	idInfoTitle     = 1102
	idInfoText      = 1103

	wmClose = 0x0010
	wmSize  = 0x0005

	bmSetImage         = 0x00F7
	imageBitmap        = 0
	lrLoadFromFile     = 0x0010
	lrCreateDIBSection = 0x2000
	bsBitmap           = 0x0080
	bsCenter           = 0x0300
	swpNoMove          = 0x0002
	swpNoZOrder        = 0x0004
)

var (
	loadImageW          = user32.NewProc("LoadImageW")
	destroyWindow       = user32.NewProc("DestroyWindow")
	moveWindow          = user32.NewProc("MoveWindow")
	setWindowPos        = user32.NewProc("SetWindowPos")
	setForegroundWindow = user32.NewProc("SetForegroundWindow")
	getWindowRect       = user32.NewProc("GetWindowRect")
	deleteObject        = gdi32.NewProc("DeleteObject")

	voicePortraitButton uintptr
	voicePortraitBitmap uintptr
	voiceInfoWindow     uintptr
	voiceInfoTitle      uintptr
	voiceInfoText       uintptr
	voiceInfoClassName  []uint16
)

type windowRect struct {
	Left, Top, Right, Bottom int32
}

func registerVoicebankInfoClass(instance, cursor uintptr) error {
	voiceInfoClassName = windowsString("UtauTTSVoicebankInfoWindow")
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WndProc: syscall.NewCallback(voicebankInfoProc),
		Instance: instance, Cursor: cursor, Background: colorBtnFace + 1, ClassName: &voiceInfoClassName[0],
	}
	if result, _, callErr := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("音源情報ウィンドウを登録できません: %v", callErr)
	}
	return nil
}

func createVoicebankPortrait(parent, instance uintptr, x, y, size int) uintptr {
	voicePortraitButton = control(wsExClientEdge, "BUTTON", "", wsChild|wsVisible|wsTabStop|bsBitmap|bsCenter,
		x, y, size, size, parent, idVoicePortrait, instance)
	return voicePortraitButton
}

func matchControlHeight(target, reference uintptr, width int) {
	if target == 0 || reference == 0 {
		return
	}
	var bounds windowRect
	if result, _, _ := getWindowRect.Call(reference, uintptr(unsafe.Pointer(&bounds))); result == 0 {
		return
	}
	height := int(bounds.Bottom - bounds.Top)
	if height > 0 {
		setWindowPos.Call(target, 0, 0, 0, uintptr(width), uintptr(height), swpNoMove|swpNoZOrder)
	}
}

func updateVoicebankPortrait(index int) {
	if voicePortraitButton == 0 {
		return
	}
	if voicePortraitBitmap != 0 {
		sendMessage.Call(voicePortraitButton, bmSetImage, imageBitmap, 0)
		deleteObject.Call(voicePortraitBitmap)
		voicePortraitBitmap = 0
	}
	if index < 0 || index >= len(availableBanks) || availableBanks[index].ImagePath == "" {
		return
	}
	path := windowsString(availableBanks[index].ImagePath)
	bitmap, _, callErr := loadImageW.Call(0, uintptr(unsafe.Pointer(&path[0])), imageBitmap, 100, 100, lrLoadFromFile|lrCreateDIBSection)
	runtime.KeepAlive(path)
	if bitmap == 0 {
		log.Printf("voicebank image load failed: path=%q error=%v", availableBanks[index].ImagePath, callErr)
		return
	}
	voicePortraitBitmap = bitmap
	sendMessage.Call(voicePortraitButton, bmSetImage, imageBitmap, bitmap)
}

func disposeVoicebankPortrait() {
	if voicePortraitBitmap != 0 {
		deleteObject.Call(voicePortraitBitmap)
		voicePortraitBitmap = 0
	}
}

func showSelectedVoicebankInfo(parent uintptr) {
	selected, _, _ := sendMessage.Call(child(parent, idVoicebank), cbGetCurSel, 0, 0)
	if int(selected) < 0 || int(selected) >= len(availableBanks) {
		showError(parent, fmt.Errorf("表示するボイスバンクが選択されていません"))
		return
	}
	summary := availableBanks[int(selected)]
	presentation, readErr := voicebank.LoadPresentation(summary)
	if voiceInfoWindow != 0 {
		destroyWindow.Call(voiceInfoWindow)
	}
	instance, _, _ := getModuleHandle.Call(0)
	title := windowsString("ボイスバンク情報 - " + summary.Name)
	voiceInfoWindow, _, _ = createWindowEx.Call(0, uintptr(unsafe.Pointer(&voiceInfoClassName[0])), uintptr(unsafe.Pointer(&title[0])),
		wsOverlappedWindow, 180, 140, 780, 650, parent, 0, instance, 0)
	runtime.KeepAlive(title)
	if voiceInfoWindow == 0 {
		showError(parent, fmt.Errorf("音源情報ウィンドウを作成できません"))
		return
	}
	font, _, _ := getStockObject.Call(defaultFont)
	voiceInfoTitle = control(0, "STATIC", summary.Name, wsChild|wsVisible, 20, 18, 720, 24, voiceInfoWindow, idInfoTitle, instance)
	voiceInfoText = control(wsExClientEdge, "EDIT", presentationText(presentation, readErr),
		wsChild|wsVisible|wsTabStop|wsVScroll|esMultiline|esAutoVScroll|esReadOnly,
		20, 50, 720, 540, voiceInfoWindow, idInfoText, instance)
	sendMessage.Call(voiceInfoTitle, wmSetFont, font, 1)
	sendMessage.Call(voiceInfoText, wmSetFont, font, 1)
	showWindow.Call(voiceInfoWindow, 5)
	updateWindow.Call(voiceInfoWindow)
	setForegroundWindow.Call(voiceInfoWindow)
}

func presentationText(presentation voicebank.Presentation, readErr error) string {
	var sections []string
	sections = append(sections, "場所: "+presentation.Summary.Path)
	if text := strings.TrimSpace(presentation.CharacterText); text != "" {
		sections = append(sections, "【character.txt】\n"+text)
	}
	if text := strings.TrimSpace(presentation.ReadmeText); text != "" {
		name := filepath.Base(presentation.Summary.ReadmePath)
		sections = append(sections, "【"+name+"】\n"+text)
	}
	if readErr != nil {
		sections = append(sections, "【読込エラー】\n"+readErr.Error())
	}
	if len(sections) == 1 {
		sections = append(sections, "character.txtまたはreadmeは見つかりませんでした。")
	}
	return strings.ReplaceAll(strings.Join(sections, "\n\n"), "\n", "\r\n")
}

func voicebankInfoProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmSize:
		width := int(lParam & 0xffff)
		height := int((lParam >> 16) & 0xffff)
		if voiceInfoTitle != 0 {
			moveWindow.Call(voiceInfoTitle, 20, 18, uintptr(max(100, width-40)), 24, 1)
		}
		if voiceInfoText != 0 {
			moveWindow.Call(voiceInfoText, 20, 50, uintptr(max(100, width-40)), uintptr(max(100, height-70)), 1)
		}
		return 0
	case wmClose:
		destroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		voiceInfoWindow, voiceInfoTitle, voiceInfoText = 0, 0, 0
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}
