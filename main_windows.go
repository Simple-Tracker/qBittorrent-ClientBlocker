//go:build windows
package main

import (
	"github.com/lxn/win"
	"golang.design/x/hotkey"
	"os"
	"flag"
	"github.com/chai2010/winsvc"
)

var showWindow = true

var (
	appPath string

	flagServiceName = flag.String("service-name", "qbittorrent-clientblocker", "Set service name")
	flagServiceDesc = flag.String("service-desc", "Qbittorrent Client Blocker Service", "Set service description")

	flagServiceInstall   = flag.Bool("service-install", false, "Install service")
	flagServiceUninstall = flag.Bool("service-remove", false, "Remove service")
	flagServiceStart     = flag.Bool("service-start", false, "Start service")
	flagServiceStop      = flag.Bool("service-stop", false, "Stop service")
)

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
	flag.Parse()

	appPath = os.Args[0]

	if *flagServiceInstall {
		if err := winsvc.InstallService(appPath, *flagServiceName, *flagServiceDesc); err != nil {
			Log("main", "安装服务时发生了错误: %s", true, err.Error())
		}
		Log("main", "完成服务安装", true)
		return
	}

	if *flagServiceUninstall {
		if err := winsvc.RemoveService(*flagServiceName); err != nil {
			Log("main", "移除服务时发生了错误: %s", true, err.Error())
		}
		Log("main", "完成服务移除", true)
		return
	}

	if *flagServiceStart {
		if err := winsvc.StartService(*flagServiceName); err != nil {
			Log("main", "启动服务时发生了错误: %s", true, err.Error())
		}
		Log("main", "完成服务启动", true)
		return
	}

	if *flagServiceStop {
		if err := winsvc.StopService(*flagServiceName); err != nil {
			Log("main", "停止服务时发生了错误: %s", true, err.Error())
		}
		Log("main", "完成服务停止", true)
		return
	}

	if !winsvc.InServiceMode() {
		if err := winsvc.RunAsService(*flagServiceName, RunConsole, StopService, false); err != nil {
			Log("main", "运行服务时发生了错误: %s", true, err.Error())
		}
		return
	}
	go RegHotKey()
	RunConsole()
}
