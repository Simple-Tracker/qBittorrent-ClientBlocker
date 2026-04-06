//go:build windows

package main

import (
	"github.com/getlantern/systray"
	"github.com/lxn/win"
	"golang.design/x/hotkey"
)

var showWindow = true
var programHotkey = hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeyB)

func Platform_ShowOrHiddenWindow() {
	consoleWindow := win.GetConsoleWindow()
	if showWindow {
		Log("Debug-ShowOrHiddenWindow", GetLangText("Debug-ShowOrHiddenWindow_HideWindow"), false)
		showWindow = false
		win.ShowWindow(consoleWindow, win.SW_HIDE)
	} else {
		Log("Debug-ShowOrHiddenWindow", GetLangText("Debug-ShowOrHiddenWindow_ShowWindow"), false)
		showWindow = true
		win.ShowWindow(consoleWindow, win.SW_SHOW)
	}
}
func Platform_Stop() {
	programHotkey.Unregister()
	systray.Quit()
}
func RegHotKey() {
	if !needRegHotKey {
		return
	}

	err := programHotkey.Register()
	if err != nil {
		Log("RegHotKey", GetLangText("Error-RegHotkey"), false, err.Error())
		return
	}
	Log("RegHotKey", GetLangText("Success-RegHotkey"), false)

	for range programHotkey.Keydown() {
		Platform_ShowOrHiddenWindow()
	}
}
func RegSysTray() {
	if needHideSystray {
		return
	}

	systray.Run(func() {
		defer RecoverAndStop("RegSysTray.onReady", true)

		systray.SetIcon(icon_Windows)
		systray.SetTitle(programName)
		mShow := systray.AddMenuItem("显示/隐藏", "显示/隐藏程序")
		mQuit := systray.AddMenuItem("退出", "退出程序")

		GoWithCrashLog("RegSysTray.loop", func() {
			for {
				select {
				case <-mShow.ClickedCh:
					Platform_ShowOrHiddenWindow()
				case <-mQuit.ClickedCh:
					systray.Quit()
				}
			}
		})
	}, func() {
		defer RecoverAndStop("RegSysTray.onExit", true)
		ReqStop()
	})
}
func main() {
	defer RecoverAndStop("main_windows", true)

	if PrepareEnv() {
		if needHideWindow && showWindow {
			Platform_ShowOrHiddenWindow()
		}
		GoWithCrashLog("RegHotKey", RegHotKey)
		GoWithCrashLog("RegSysTray", RegSysTray)
		RunConsole()
	}
}
