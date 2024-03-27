//go:build windows
package main

import (
	"github.com/lxn/win"
	"golang.design/x/hotkey"
)

var showWindow = true

func ShowOrHiddenWindow() {
	consoleWindow := win.GetConsoleWindow()
	if showWindow == true {
		Log("Debug-ShowOrHiddenWindow", GetLangText("Debug-ShowOrHiddenWindow_HideWindow"), false)
		showWindow = false
		win.ShowWindow(consoleWindow, win.SW_HIDE)
	} else {
		Log("Debug-ShowOrHiddenWindow", GetLangText("Debug-ShowOrHiddenWindow_ShowWindow"), false)
		showWindow = true
		win.ShowWindow(consoleWindow, win.SW_SHOW)
	}
}
func RegHotKey() {
	qBCBHotkey := hotkey.New([]hotkey.Modifier { hotkey.ModCtrl, hotkey.ModAlt }, hotkey.KeyB)
	err := qBCBHotkey.Register()
	if err != nil {
		Log("RegHotKey", GetLangText("Error-RegHotkey"), false, err.Error())
		return
	}
	Log("RegHotKey", GetLangText("Success-RegHotkey"), false)

	for range qBCBHotkey.Keydown() {
		ShowOrHiddenWindow()
	}
}
func main() {
	if PrepareEnv() {
		go RegHotKey()
		RunConsole()
	}
}
