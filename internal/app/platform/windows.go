//go:build windows

package platform

import (
	"github.com/getlantern/systray"
	"github.com/lxn/win"
	"golang.design/x/hotkey"
)

var options Options

var showWindow = true
var programHotkey = hotkey.New([]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModAlt}, hotkey.KeyB)

func showOrHiddenWindow() {
	consoleWindow := win.GetConsoleWindow()
	if showWindow {
		options.Log("Debug-ShowOrHiddenWindow", options.Text("Debug-ShowOrHiddenWindow_HideWindow"), false)
		showWindow = false
		win.ShowWindow(consoleWindow, win.SW_HIDE)
	} else {
		options.Log("Debug-ShowOrHiddenWindow", options.Text("Debug-ShowOrHiddenWindow_ShowWindow"), false)
		showWindow = true
		win.ShowWindow(consoleWindow, win.SW_SHOW)
	}
}
func Stop() {
	programHotkey.Unregister()
	systray.Quit()
}
func regHotKey() {
	if !options.RegHotKey {
		return
	}

	err := programHotkey.Register()
	if err != nil {
		options.LogError("RegHotKey", options.Text("Error-RegHotkey"), false, err.Error())
		return
	}
	options.Log("RegHotKey", options.Text("Success-RegHotkey"), false)

	for range programHotkey.Keydown() {
		showOrHiddenWindow()
	}
}
func regSysTray() {
	if options.HideSystray {
		return
	}

	systray.Run(func() {
		defer options.Recover("RegSysTray.onReady", true)

		systray.SetIcon(icon_Windows)
		systray.SetTitle(options.ProgramName)
		mShow := systray.AddMenuItem("显示/隐藏", "显示/隐藏程序")
		mQuit := systray.AddMenuItem("退出", "退出程序")

		options.Go("RegSysTray.loop", func() {
			for {
				select {
				case <-mShow.ClickedCh:
					showOrHiddenWindow()
				case <-mQuit.ClickedCh:
					systray.Quit()
				}
			}
		})
	}, func() {
		defer options.Recover("RegSysTray.onExit", true)
		options.RequestStop()
	})
}

func Start(settings Options) {
	options = settings
	if options.HideWindow && showWindow {
		showOrHiddenWindow()
	}
	options.Go("RegHotKey", regHotKey)
	options.Go("RegSysTray", regSysTray)
}
