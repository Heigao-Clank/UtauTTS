//go:build windows

package main

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"utautts/internal/frontend"
	"utautts/internal/prosody"
)

const (
	manualPitchApplyID = 2301
	manualPitchResetID = 2302
	manualPitchGraphX  = 30
	manualPitchGraphY  = 82
	manualPitchGraphW  = 840
	manualPitchGraphH  = 350
	manualPitchRange   = 300.0

	wmPaint         = 0x000F
	transparentMode = 1
	psSolid         = 0
)

var (
	manualPitchClassName []uint16
	manualPitchWindow    uintptr
	manualPitchReading   string
	manualPitchMorae     []frontend.Mora
	manualPitchEdits     []float64
	manualPitchPositions []int
	manualPitchSelected  = -1
	manualPitchDragging  = -1
	manualPitchValueText uintptr
	activeManualPitch    *prosody.ManualPitchFile

	manualBeginPaint     = user32.NewProc("BeginPaint")
	manualEndPaint       = user32.NewProc("EndPaint")
	manualFillRect       = user32.NewProc("FillRect")
	manualCreatePen      = gdi32.NewProc("CreatePen")
	manualCreateBrush    = gdi32.NewProc("CreateSolidBrush")
	manualSelectObject   = gdi32.NewProc("SelectObject")
	manualMoveToEx       = gdi32.NewProc("MoveToEx")
	manualLineTo         = gdi32.NewProc("LineTo")
	manualEllipse        = gdi32.NewProc("Ellipse")
	manualTextOut        = gdi32.NewProc("TextOutW")
	manualSetTextColor   = gdi32.NewProc("SetTextColor")
	manualSetBkMode      = gdi32.NewProc("SetBkMode")
	manualInvalidateRect = user32.NewProc("InvalidateRect")
	manualSetCapture     = user32.NewProc("SetCapture")
	manualReleaseCapture = user32.NewProc("ReleaseCapture")
)

type manualPaintStruct struct {
	HDC       uintptr
	Erase     int32
	Paint     windowRect
	Restore   int32
	IncUpdate int32
	Reserved  [32]byte
}

func registerManualPitchClass(instance, cursor uintptr) error {
	manualPitchClassName = windowsString("UtauTTSManualPitchWindow")
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WndProc: syscall.NewCallback(manualPitchProc),
		Instance: instance, Cursor: cursor, Background: colorBtnFace + 1, ClassName: &manualPitchClassName[0],
	}
	if result, _, callErr := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("manual pitch window registration failed: %v", callErr)
	}
	return nil
}

func showManualPitchEditor(parent uintptr) {
	if manualPitchWindow != 0 {
		setForegroundWindow.Call(manualPitchWindow)
		return
	}
	text := strings.TrimSpace(windowText(child(parent, idText)))
	reading := strings.TrimSpace(advancedSettings.Reading)
	if reading == "" {
		var err error
		reading, err = frontend.ToKana(text)
		if err != nil {
			showError(parent, fmt.Errorf("読みを作成できません: %w", err))
			return
		}
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
		showError(parent, fmt.Errorf("読みを解析できません: %w", err))
		return
	}
	manualPitchReading = reading
	manualPitchMorae = morae
	manualPitchEdits = nil
	manualPitchPositions = nil
	manualPitchSelected = -1
	manualPitchDragging = -1
	for position, mora := range morae {
		if mora.Pause {
			continue
		}
		manualPitchPositions = append(manualPitchPositions, position)
		manualPitchEdits = append(manualPitchEdits, 0)
	}
	if activeManualPitch != nil && activeManualPitch.Reading == reading {
		for index, position := range manualPitchPositions {
			for _, point := range activeManualPitch.Points {
				if point.Position == position {
					manualPitchEdits[index] = point.Cents
				}
			}
		}
	}
	instance, _, _ := getModuleHandle.Call(0)
	title := windowsString("UtauTTS イントネーション編集")
	manualPitchWindow, _, _ = createWindowEx.Call(
		0, uintptr(unsafe.Pointer(&manualPitchClassName[0])), uintptr(unsafe.Pointer(&title[0])),
		wsOverlappedWindow, 140, 80, 920, 600, parent, 0, instance, 0,
	)
	runtime.KeepAlive(title)
	if manualPitchWindow == 0 {
		showError(parent, fmt.Errorf("イントネーション編集画面を作成できません"))
		return
	}
	createManualPitchControls(manualPitchWindow, instance)
	showWindow.Call(manualPitchWindow, 5)
	updateWindow.Call(manualPitchWindow)
	setForegroundWindow.Call(manualPitchWindow)
}

func createManualPitchControls(parent, instance uintptr) {
	firstControl := len(controls)
	font, _, _ := getStockObject.Call(defaultFont)
	label(parent, instance, "モーラごとのピッチグラフ", 20, 18, 400, 24)
	label(parent, instance, "点を上下にドラッグしてcentを調整します。0は変更なしです。", 20, 44, 780, 24)
	manualPitchValueText = label(parent, instance, "選択: なし", 30, 445, 400, 24)
	control(0, "BUTTON", "リセット", wsChild|wsVisible|wsTabStop, 610, 475, 100, 32, parent, manualPitchResetID, instance)
	control(0, "BUTTON", "適用して閉じる", wsChild|wsVisible|wsTabStop, 720, 475, 150, 32, parent, manualPitchApplyID, instance)
	for _, handle := range controls[firstControl:] {
		sendMessage.Call(handle, wmSetFont, font, 1)
	}
}

func manualPitchProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmPaint:
		manualPaintGraph(hwnd)
		return 0
	case wmLButtonDown:
		manualPitchDragging = manualPitchPointAtX(mouseX(lParam))
		if manualPitchDragging >= 0 {
			manualPitchSelected = manualPitchDragging
			manualUpdateSelectedLabel()
			manualSetCapture.Call(hwnd)
		}
		return 0
	case wmMouseMove:
		if manualPitchDragging >= 0 {
			manualSetPitchFromY(mouseY(lParam))
			manualInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonUp:
		manualPitchDragging = -1
		manualReleaseCapture.Call()
		return 0
	case wmCommand:
		commandID := int(wParam & 0xffff)
		switch commandID {
		case manualPitchResetID:
			for index := range manualPitchEdits {
				manualPitchEdits[index] = 0
			}
			manualUpdateSelectedLabel()
			manualInvalidateRect.Call(hwnd, 0, 0)
		case manualPitchApplyID:
			if err := saveManualPitchFromWindow(); err != nil {
				showError(hwnd, err)
				return 0
			}
			destroyWindow.Call(hwnd)
		}
		return 0
	case wmClose:
		destroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		manualPitchWindow = 0
		manualPitchDragging = -1
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func manualPaintGraph(hwnd uintptr) {
	var paint manualPaintStruct
	hdc, _, _ := manualBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if hdc == 0 {
		return
	}
	defer manualEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	graph := windowRect{Left: manualPitchGraphX, Top: manualPitchGraphY, Right: manualPitchGraphX + manualPitchGraphW, Bottom: manualPitchGraphY + manualPitchGraphH}
	white, _, _ := getStockObject.Call(0)
	manualFillRect.Call(hdc, uintptr(unsafe.Pointer(&graph)), white)
	gridPen, _, _ := manualCreatePen.Call(psSolid, 1, manualRGB(215, 215, 215))
	axisPen, _, _ := manualCreatePen.Call(psSolid, 1, manualRGB(100, 100, 100))
	curvePen, _, _ := manualCreatePen.Call(psSolid, 2, manualRGB(30, 100, 210))
	pointBrush, _, _ := manualCreateBrush.Call(manualRGB(30, 100, 210))
	oldPen, _, _ := manualSelectObject.Call(hdc, gridPen)
	oldBrush, _, _ := manualSelectObject.Call(hdc, pointBrush)
	defer func() {
		manualSelectObject.Call(hdc, oldPen)
		manualSelectObject.Call(hdc, oldBrush)
		deleteObject.Call(gridPen)
		deleteObject.Call(axisPen)
		deleteObject.Call(curvePen)
		deleteObject.Call(pointBrush)
	}()
	for _, cents := range []float64{-300, -150, 0, 150, 300} {
		y := manualPitchY(cents)
		manualSelectObject.Call(hdc, gridPen)
		if cents == 0 {
			manualSelectObject.Call(hdc, axisPen)
		}
		manualMoveToEx.Call(hdc, uintptr(manualPitchGraphX), uintptr(y), 0)
		manualLineTo.Call(hdc, uintptr(manualPitchGraphX+manualPitchGraphW), uintptr(y))
		manualText(hdc, manualPitchGraphX-28, y-8, fmt.Sprintf("%+.0f", cents))
	}
	manualSelectObject.Call(hdc, curvePen)
	for index := range manualPitchPositions {
		x := manualPitchX(index)
		y := manualPitchY(manualPitchEdits[index])
		if index == 0 {
			manualMoveToEx.Call(hdc, uintptr(x), uintptr(y), 0)
		} else {
			manualLineTo.Call(hdc, uintptr(x), uintptr(y))
		}
	}
	manualSelectObject.Call(hdc, pointBrush)
	for index := range manualPitchPositions {
		x, y := manualPitchX(index), manualPitchY(manualPitchEdits[index])
		radius := 6
		manualEllipse.Call(hdc, uintptr(x-radius), uintptr(y-radius), uintptr(x+radius), uintptr(y+radius))
		manualText(hdc, x-12, manualPitchGraphY+manualPitchGraphH+8, fmt.Sprintf("%d:%s", manualPitchPositions[index]+1, manualPitchMorae[manualPitchPositions[index]].Text))
	}
	manualText(hdc, manualPitchGraphX+manualPitchGraphW-90, manualPitchGraphY-22, "cent")
}

func manualText(hdc uintptr, x, y int, value string) {
	text := windowsString(value)
	if len(text) == 0 {
		return
	}
	manualSetBkMode.Call(hdc, transparentMode)
	manualSetTextColor.Call(hdc, manualRGB(50, 50, 50))
	manualTextOut.Call(hdc, uintptr(x), uintptr(y), uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1))
	runtime.KeepAlive(text)
}

func manualRGB(r, g, b byte) uintptr { return uintptr(r) | uintptr(g)<<8 | uintptr(b)<<16 }

func manualPitchX(index int) int {
	if len(manualPitchPositions) <= 1 {
		return manualPitchGraphX + manualPitchGraphW/2
	}
	return manualPitchGraphX + index*manualPitchGraphW/(len(manualPitchPositions)-1)
}

func manualPitchY(cents float64) int {
	cents = math.Max(-manualPitchRange, math.Min(manualPitchRange, cents))
	return manualPitchGraphY + manualPitchGraphH/2 - int(math.Round(cents/manualPitchRange*float64(manualPitchGraphH/2)))
}

func manualSetPitchFromY(y int) {
	if manualPitchDragging < 0 || manualPitchDragging >= len(manualPitchEdits) {
		return
	}
	cents := float64(manualPitchGraphY+manualPitchGraphH/2-y) * manualPitchRange / float64(manualPitchGraphH/2)
	manualPitchEdits[manualPitchDragging] = math.Max(-manualPitchRange, math.Min(manualPitchRange, cents))
	manualUpdateSelectedLabel()
}

func manualPitchPointAtX(x int) int {
	best, distance := -1, 18
	for index := range manualPitchPositions {
		candidate := absInt(x - manualPitchX(index))
		if candidate <= distance {
			best, distance = index, candidate
		}
	}
	return best
}

func manualUpdateSelectedLabel() {
	if manualPitchValueText == 0 {
		return
	}
	if manualPitchSelected < 0 || manualPitchSelected >= len(manualPitchPositions) {
		setText(manualPitchValueText, "選択: なし")
		return
	}
	position := manualPitchPositions[manualPitchSelected]
	setText(manualPitchValueText, fmt.Sprintf("選択: %d %s  %+.0f cent", position+1, manualPitchMorae[position].Text, manualPitchEdits[manualPitchSelected]))
}

func mouseX(value uintptr) int { return int(int16(value & 0xffff)) }
func mouseY(value uintptr) int { return int(int16((value >> 16) & 0xffff)) }
func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func saveManualPitchFromWindow() error {
	points := make([]prosody.ManualPitchPoint, 0, len(manualPitchPositions))
	for index, position := range manualPitchPositions {
		value := manualPitchEdits[index]
		if math.IsNaN(value) || math.IsInf(value, 0) || value < -1200 || value > 1200 {
			return fmt.Errorf("モーラ%dのcent値は-1200から1200の範囲で指定してください", position+1)
		}
		points = append(points, prosody.ManualPitchPoint{Position: position, Mora: manualPitchMorae[position].Text, Cents: value})
	}
	activeManualPitch = &prosody.ManualPitchFile{Version: prosody.ManualPitchVersion, Reading: manualPitchReading, Mode: "offset", Points: points}
	if editor != nil {
		if selected := editor.selected(); selected != nil {
			selected.ManualPitch = activeManualPitch
			selected.Reading = manualPitchReading
		}
	}
	return nil
}

func manualPitchForReading(reading string) *prosody.ManualPitchFile {
	if activeManualPitch == nil || activeManualPitch.Reading != reading {
		return nil
	}
	return activeManualPitch
}
