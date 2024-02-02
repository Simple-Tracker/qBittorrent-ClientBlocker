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
		Log("Debug-ShowOrHiddenWindow", "窗口隐藏", false)
		showWindow = false
		win.ShowWindow(consoleWindow, win.SW_HIDE)
	} else {
		Log("Debug-ShowOrHiddenWindow", "窗口显示", false)
		showWindow = true
		win.ShowWindow(consoleWindow, win.SW_SHOW)
	}
}
func RegHotKey() {
	qBCBHotkey := hotkey.New([]hotkey.Modifier { hotkey.ModCtrl, hotkey.ModAlt }, hotkey.KeyB)
	err := qBCBHotkey.Register()
	if err != nil {
		Log("RegHotKey", "无法注册窗口热键: %v", false, err.Error())
		return
	}
	Log("RegHotKey", "已注册窗口热键: CTRL+ALT+B", false)

	for range qBCBHotkey.Keydown() {
		Log("Debug-RegHotKey", "开始监听窗口热键", false)
		ShowOrHiddenWindow()
	}
}
func main() {
	go RegHotKey()
	RunConsole()
}
