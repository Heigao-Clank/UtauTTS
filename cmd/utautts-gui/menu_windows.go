//go:build windows

package main

import (
	"fmt"
	"runtime"
	"unsafe"
)

const (
	appVersion = "開発版"
	githubURL  = "https://github.com/yh2237/UtauTTS"
)

func menuAppend(menu uintptr, flags uintptr, id uintptr, text string) {
	value := windowsString(text)
	appendMenu.Call(menu, flags, id, uintptr(unsafe.Pointer(&value[0])))
	runtime.KeepAlive(value)
}

func installMainMenu(hwnd uintptr) {
	menu, _, _ := createMenu.Call()
	file, _, _ := createPopupMenu.Call()
	help, _, _ := createPopupMenu.Call()
	if menu == 0 || file == 0 || help == 0 {
		return
	}
	menuAppend(file, menuString, idMenuProjectOpen, "プロジェクトを開く...")
	menuAppend(file, menuString, idMenuProjectSave, "プロジェクトを保存...")
	menuAppend(file, menuString, idMenuAudioSave, "音声を保存...")
	menuAppend(file, menuString, idMenuSelectedWav, "選択中の発話を書き出す...")
	menuAppend(file, menuString, idMenuAllWav, "全発話を書き出す...")
	menuAppend(file, menuSeparator, 0, "")
	menuAppend(file, menuString, idMenuExit, "終了")
	menuAppend(help, menuString, idMenuLicense, "ライセンス")
	menuAppend(help, menuString, idMenuGitHub, "GitHubリポジトリを開く")
	menuAppend(help, menuString, idMenuAbout, "バージョン情報")
	menuAppend(menu, menuPopup, file, "ファイル")
	menuAppend(menu, menuPopup, help, "ヘルプ")
	setMenu.Call(hwnd, menu)
	drawMenuBar.Call(hwnd)
}

func showLicense(hwnd uintptr) {
	showInfo(hwnd, "ライセンス", "UtauTTSはMIT Licenseで公開されています。\r\n\r\nCopyright (c) 2026 yh\r\n\r\n著作権表示と許諾表示を保持する限り、利用・改変・再配布できます。\r\n\r\nボイスバンクのライセンスは、それぞれに同梱されたreadmeやガイドラインが優先されます。")
}

func showAbout(hwnd uintptr) {
	showInfo(hwnd, "UtauTTSについて", fmt.Sprintf("UtauTTS\r\nバージョン: %s\r\n\r\nUTAUボイスバンクを利用した波形接続型音声合成プロジェクトです。", appVersion))
}

func showInfo(hwnd uintptr, title, text string) {
	titleBuffer := windowsString(title)
	textBuffer := windowsString(text)
	messageBox.Call(hwnd, uintptr(unsafe.Pointer(&textBuffer[0])), uintptr(unsafe.Pointer(&titleBuffer[0])), 0x40)
	runtime.KeepAlive(titleBuffer)
	runtime.KeepAlive(textBuffer)
}

func openGitHub(hwnd uintptr) {
	operation := windowsString("open")
	target := windowsString(githubURL)
	result, _, _ := shellExecute.Call(hwnd, uintptr(unsafe.Pointer(&operation[0])), uintptr(unsafe.Pointer(&target[0])), 0, 0, 1)
	runtime.KeepAlive(operation)
	runtime.KeepAlive(target)
	if result <= 32 {
		showError(hwnd, fmt.Errorf("GitHubリポジトリを開けませんでした: %s", githubURL))
	}
}
