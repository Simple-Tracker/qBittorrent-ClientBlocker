package platform

// Options 由启动入口提供, 平台代码只处理窗口、热键和托盘.
type Options struct {
	ProgramName                        string
	RegHotKey, HideWindow, HideSystray bool
	Log                                func(string, string, bool, ...any)
	LogError                           func(string, string, bool, ...any)
	Text                               func(string) string
	Recover                            func(string, bool)
	Go                                 func(string, func())
	RequestStop                        func()
}
