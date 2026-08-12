//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"utautts/internal/frontend"
)

var (
	pitchGraphClassName []uint16
	pitchGraphDragging  = -1
)

func registerPitchGraphClass(instance, cursor uintptr) error {
	pitchGraphClassName = windowsString("UtauTTSPitchGraphWindow")
	class := windowClassEx{
		Size: uint32(unsafe.Sizeof(windowClassEx{})), WndProc: syscall.NewCallback(pitchGraphProc),
		Instance: instance, Cursor: cursor, Background: colorBtnFace + 1, ClassName: &pitchGraphClassName[0],
	}
	if result, _, callErr := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); result == 0 {
		return fmt.Errorf("pitch graph window registration failed: %v", callErr)
	}
	return nil
}

func createPitchGraphPanel(parent, instance uintptr) uintptr {
	title := windowsString("")
	hwnd, _, _ := createWindowEx.Call(
		0, uintptr(unsafe.Pointer(&pitchGraphClassName[0])), uintptr(unsafe.Pointer(&title[0])),
		wsChild|wsVisible|wsBorder, 660, 208, 210, 206, parent, 0, instance, 0,
	)
	runtime.KeepAlive(title)
	return hwnd
}

func prepareManualPitchForCurrent(hwnd uintptr) {
	if editor == nil {
		return
	}
	text := stringsTrim(windowText(child(hwnd, idText)))
	reading := stringsTrim(advancedSettings.Reading)
	if reading == "" {
		var err error
		reading, err = frontend.ToKana(text)
		if err != nil {
			return
		}
	}
	if reading == manualPitchReading && len(manualPitchEdits) > 0 {
		return
	}
	morae, err := frontend.ParseKana(reading)
	if err != nil {
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
	if selected := editor.selected(); selected != nil && selected.ManualPitch != nil && selected.ManualPitch.Reading == reading {
		activeManualPitch = selected.ManualPitch
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
}

func invalidatePitchGraph() {
	if pitchGraphPanel != 0 {
		manualInvalidateRect.Call(pitchGraphPanel, 0, 1)
	}
}

func pitchGraphProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmPaint:
		paintPitchGraphPanel(hwnd)
		return 0
	case wmLButtonDown:
		pitchGraphDragging = pitchGraphPointAt(hwnd, mouseX(lParam), mouseY(lParam))
		if pitchGraphDragging >= 0 {
			manualPitchSelected = pitchGraphDragging
			manualSetCapture.Call(hwnd)
			invalidatePitchGraph()
		}
		return 0
	case wmMouseMove:
		if pitchGraphDragging >= 0 {
			_, top, _, bottom := pitchPanelBounds(hwnd)
			cents := float64(top+(bottom-top)/2-mouseY(lParam)) * manualPitchRange / float64((bottom-top)/2)
			manualPitchEdits[pitchGraphDragging] = mathClamp(cents, -manualPitchRange, manualPitchRange)
			invalidatePitchGraph()
		}
		return 0
	case wmLButtonUp:
		if pitchGraphDragging >= 0 {
			pitchGraphDragging = -1
			manualReleaseCapture.Call()
			if err := saveManualPitchFromWindow(); err == nil {
				invalidatePitchGraph()
			}
		}
		return 0
	}
	result, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func pitchPanelBounds(hwnd uintptr) (left, top, right, bottom int) {
	var rect windowRect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	return 38, 24, int(rect.Right) - 16, int(rect.Bottom) - 34
}

func paintPitchGraphPanel(hwnd uintptr) {
	var paint manualPaintStruct
	hdc, _, _ := manualBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	if hdc == 0 {
		return
	}
	defer manualEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
	var client windowRect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	background := windowRect{Left: 0, Top: 0, Right: client.Right, Bottom: client.Bottom}
	white, _, _ := getStockObject.Call(0)
	manualFillRect.Call(hdc, uintptr(unsafe.Pointer(&background)), white)
	left, top, right, bottom := pitchPanelBounds(hwnd)
	gridPen, _, _ := manualCreatePen.Call(psSolid, 1, manualRGB(220, 220, 220))
	axisPen, _, _ := manualCreatePen.Call(psSolid, 1, manualRGB(100, 100, 100))
	curvePen, _, _ := manualCreatePen.Call(psSolid, 2, manualRGB(30, 100, 210))
	pointBrush, _, _ := manualCreateBrush.Call(manualRGB(30, 100, 210))
	defer func() {
		deleteObject.Call(gridPen)
		deleteObject.Call(axisPen)
		deleteObject.Call(curvePen)
		deleteObject.Call(pointBrush)
	}()
	for _, cents := range []float64{-300, 0, 300} {
		y := top + (bottom-top)/2 - int(cents/manualPitchRange*float64((bottom-top)/2))
		pen := gridPen
		if cents == 0 {
			pen = axisPen
		}
		manualSelectObject.Call(hdc, pen)
		manualMoveToEx.Call(hdc, uintptr(left), uintptr(y), 0)
		manualLineTo.Call(hdc, uintptr(right), uintptr(y))
		manualText(hdc, 3, y-7, fmt.Sprintf("%+.0f", cents))
	}
	if len(manualPitchPositions) == 0 {
		manualText(hdc, left, 4, "イントネーション")
		return
	}
	manualSelectObject.Call(hdc, curvePen)
	for index := range manualPitchPositions {
		x := left
		if len(manualPitchPositions) > 1 {
			x += index * (right - left) / (len(manualPitchPositions) - 1)
		}
		y := top + (bottom-top)/2 - int(manualPitchEdits[index]/manualPitchRange*float64((bottom-top)/2))
		if index == 0 {
			manualMoveToEx.Call(hdc, uintptr(x), uintptr(y), 0)
		} else {
			manualLineTo.Call(hdc, uintptr(x), uintptr(y))
		}
	}
	manualSelectObject.Call(hdc, pointBrush)
	for index := range manualPitchPositions {
		x := left
		if len(manualPitchPositions) > 1 {
			x += index * (right - left) / (len(manualPitchPositions) - 1)
		}
		y := top + (bottom-top)/2 - int(manualPitchEdits[index]/manualPitchRange*float64((bottom-top)/2))
		manualEllipse.Call(hdc, uintptr(x-4), uintptr(y-4), uintptr(x+4), uintptr(y+4))
	}
	manualText(hdc, left, bottom+8, "ピッチ（cent）")
}

func pitchGraphPointAt(hwnd uintptr, x, y int) int {
	left, top, right, bottom := pitchPanelBounds(hwnd)
	if x < left-10 || x > right+10 || y < top-10 || y > bottom+10 || len(manualPitchPositions) == 0 {
		return -1
	}
	best, distance := -1, 20
	for index := range manualPitchPositions {
		pointX := left
		if len(manualPitchPositions) > 1 {
			pointX += index * (right - left) / (len(manualPitchPositions) - 1)
		}
		if d := absInt(x - pointX); d < distance {
			best, distance = index, d
		}
	}
	return best
}

func mathClamp(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
