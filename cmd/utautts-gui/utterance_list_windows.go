//go:build windows

package main

import (
	"log"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	lbItemFromPoint = 0x01A9
	odsSelected     = 0x0001
	gwlpWndProc     = -4
	colorHighlight  = 13
	colorWindow     = 5
	transparentText = 1
)

var (
	setWindowLongPtr     = user32.NewProc("SetWindowLongPtrW")
	callWindowProc       = user32.NewProc("CallWindowProcW")
	getSysColorBrush     = user32.NewProc("GetSysColorBrush")
	utteranceCreateDC    = gdi32.NewProc("CreateCompatibleDC")
	utteranceDeleteDC    = gdi32.NewProc("DeleteDC")
	utteranceBitBlt      = gdi32.NewProc("BitBlt")
	utteranceListOldProc uintptr
	utteranceDragIndex   = -1
	utteranceBitmaps     = map[string]uintptr{}
)

type drawItemStruct struct {
	CtlType    uint32
	CtlID      uint32
	ItemID     uint32
	ItemAction uint32
	ItemState  uint32
	HwndItem   uintptr
	HDC        uintptr
	Rect       windowRect
	ItemData   uintptr
}

func subclassUtteranceList(list uintptr) {
	if list == 0 || utteranceListOldProc != 0 {
		return
	}
	utteranceListOldProc, _, _ = setWindowLongPtr.Call(list, uintptr(^uint(0)-3), syscall.NewCallback(utteranceListProc))
}

func utteranceListProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmLButtonDown:
		mouseX := int(int16(lParam & 0xffff))
		if mouseX <= 22 {
			utteranceDragIndex = utteranceListItemAt(hwnd, lParam)
		} else {
			utteranceDragIndex = -1
		}
		if utteranceDragIndex >= 0 {
			manualSetCapture.Call(hwnd)
		}
	case wmMouseMove:
		// Capture keeps receiving movement while the user drags. The actual
		// reorder is committed on button release to avoid list churn.
	case wmLButtonUp:
		if utteranceDragIndex >= 0 {
			from := utteranceDragIndex
			to := utteranceListItemAt(hwnd, lParam)
			utteranceDragIndex = -1
			manualReleaseCapture.Call()
			if to >= 0 && to != from {
				reorderUtterance(from, to)
				refreshUtteranceList(mainWindow, hwnd)
				loadSelectedUtteranceText(mainWindow)
				loadSelectedUtteranceControls(mainWindow)
			}
		}
	}
	if utteranceListOldProc != 0 {
		result, _, _ := callWindowProc.Call(utteranceListOldProc, hwnd, uintptr(msg), wParam, lParam)
		return result
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func utteranceListItemAt(hwnd, lParam uintptr) int {
	result, _, _ := sendMessage.Call(hwnd, lbItemFromPoint, 0, lParam)
	if result&0xFFFF0000 != 0 {
		return -1
	}
	return int(result & 0xFFFF)
}

func reorderUtterance(from, to int) {
	if editor == nil || from < 0 || to < 0 || from >= len(editor.Utterances) || to >= len(editor.Utterances) || from == to {
		return
	}
	item := editor.Utterances[from]
	if from < to {
		copy(editor.Utterances[from:to], editor.Utterances[from+1:to+1])
	} else {
		copy(editor.Utterances[to+1:from+1], editor.Utterances[to:from])
	}
	editor.Utterances[to] = item
	if editor.Selected == from {
		editor.Selected = to
	} else if from < editor.Selected && editor.Selected <= to {
		editor.Selected--
	} else if to <= editor.Selected && editor.Selected < from {
		editor.Selected++
	}
}

func drawUtteranceListItem(lParam uintptr) bool {
	if editor == nil || lParam == 0 {
		return false
	}
	var draw drawItemStruct
	rtlMoveMemory.Call(uintptr(unsafe.Pointer(&draw)), lParam, unsafe.Sizeof(draw))
	if int(draw.CtlID) != idUtteranceList || int(draw.ItemID) < 0 || int(draw.ItemID) >= len(editor.Utterances) {
		return false
	}
	background := colorWindow
	textColor := manualRGB(45, 45, 45)
	if draw.ItemState&odsSelected != 0 {
		background = colorHighlight
		textColor = manualRGB(255, 255, 255)
	}
	brush, _, _ := getSysColorBrush.Call(uintptr(background))
	rect := draw.Rect
	manualFillRect.Call(draw.HDC, uintptr(unsafe.Pointer(&rect)), brush)
	gripPen, _, _ := manualCreatePen.Call(psSolid, 1, manualRGB(150, 150, 150))
	previousPen, _, _ := manualSelectObject.Call(draw.HDC, gripPen)
	for offset := int32(0); offset < 3; offset++ {
		y := draw.Rect.Top + 20 + offset*5
		manualMoveToEx.Call(draw.HDC, uintptr(draw.Rect.Left+7), uintptr(y), 0)
		manualLineTo.Call(draw.HDC, uintptr(draw.Rect.Left+17), uintptr(y))
	}
	manualSelectObject.Call(draw.HDC, previousPen)
	deleteObject.Call(gripPen)

	utterance := editor.Utterances[int(draw.ItemID)]
	if bitmap := utteranceBitmap(utterance.VoicebankPath); bitmap != 0 {
		memoryDC, _, _ := utteranceCreateDC.Call(draw.HDC)
		if memoryDC != 0 {
			previous, _, _ := manualSelectObject.Call(memoryDC, bitmap)
			utteranceBitBlt.Call(draw.HDC, uintptr(draw.Rect.Left+25), uintptr(draw.Rect.Top+6), 42, 42, memoryDC, 0, 0, 0x00CC0020)
			manualSelectObject.Call(memoryDC, previous)
			utteranceDeleteDC.Call(memoryDC)
		}
	}
	text := windowsString(utteranceListLabel(int(draw.ItemID), utterance))
	manualSetBkMode.Call(draw.HDC, transparentText)
	manualSetTextColor.Call(draw.HDC, textColor)
	manualTextOut.Call(draw.HDC, uintptr(draw.Rect.Left+73), uintptr(draw.Rect.Top+18), uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1))
	runtime.KeepAlive(text)
	return true
}

func utteranceBitmap(path string) uintptr {
	if path == "" || len(availableBanks) == 0 {
		return 0
	}
	for _, bank := range availableBanks {
		if samePath(bank.Path, path) {
			path = bank.ImagePath
			break
		}
	}
	if path == "" {
		return 0
	}
	if bitmap := utteranceBitmaps[path]; bitmap != 0 {
		return bitmap
	}
	buffer := windowsString(path)
	bitmap, _, callErr := loadImageW.Call(0, uintptr(unsafe.Pointer(&buffer[0])), imageBitmap, 42, 42, lrLoadFromFile|lrCreateDIBSection)
	runtime.KeepAlive(buffer)
	if bitmap == 0 {
		log.Printf("utterance image load failed: path=%q error=%v", path, callErr)
		return 0
	}
	utteranceBitmaps[path] = bitmap
	return bitmap
}

func disposeUtteranceBitmaps() {
	for path, bitmap := range utteranceBitmaps {
		if bitmap != 0 {
			deleteObject.Call(bitmap)
		}
		delete(utteranceBitmaps, path)
	}
}
