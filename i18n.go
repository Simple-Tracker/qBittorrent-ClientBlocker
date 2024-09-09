package main

import (
	"encoding/json"
	"os"

	"github.com/Xuanwo/go-locale"
)

var langContent map[string]string
var defaultLangContent = map[string]string{
	"ProgramVersion":                                   "程序版本",
	"ConfigPath":                                       "配置文件路径",
	"AdditionalConfigPath":                             "附加配置文件路径",
	"DebugMode":                                        "调试模式",
	"StartDelay":                                       "启动延迟",
	"NoChdir":                                          "不切换工作目录",
	"HideWindow":                                       "默认隐藏窗口 (仅 Windows)",
	"HideSystray":                                      "默认隐藏托盘图标 (仅 Windows)",
	"RunConsole_StartDelay":                            "启动延迟: %d 秒",
	"RunConsole_ProgramHasStarted":                     "程序已启动",
	"LoadInitConfig_DetectClientFailed":                "检测客户端失败",
	"LoadInitConfig_AuthFailed":                        "认证失败",
	"Task_BanInfo":                                     "此次封禁客户端: %d 个, 当前封禁客户端: %d 个",
	"Task_BanInfoWithIP":                               "此次封禁客户端: %d 个, 此次封禁 IP: %d 个, 当前封禁 IP 及客户端: %d 个",
	"GC_IPMap":                                         "触发垃圾回收 (ipMap): %d",
	"GC_TorrentMap":                                    "触发垃圾回收 (torrentMap): %s/%d",
	"ReqStop_Stoping":                                  "程序正在停止..",
	"Stop_CaughtPanic":                                 "捕获到程序崩溃: %s",
	"Stop_StacktraceWhenPanic":                         "崩溃时的堆栈信息: %s",
	"GetClientConfig_UseConfig":                        "使用客户端配置文件: %s",
	"LoadLog_HotReload":                                "发现日志目录更改, 正在进行热重载 (%s)",
	"SyncWithServer_Compile-BlockByReason":             "封禁原因: %s",
	"CheckUpdate-ShowVersion":                          "当前版本: %s, 最新版本: %s, 最新版本 (Beta): %s",
	"CheckUpdate-DetectNewVersion":                     "检测到新的版本: %s, 可访问 %s 手动进行更新, 更新内容如下: \n%s",
	"CheckUpdate-DetectNewBetaVersion":                 "检测到新的版本 (Beta): %s, 可访问 %s 手动进行更新, 更新内容如下: \n%s",
	"CheckUpdate-Ignore_UnknownVersion":                "跳过自动检查更新: 未知版本",
	"CheckUpdate-Ignore_NightlyVersion":                "跳过自动检查更新: 夜间构建版本",
	"CheckUpdate-Ignore_BadVersion":                    "跳过自动检查更新: 错误版本 %s",
	"ClientQB_Detect-OldClientURL":                     "检测到 ClientURL (Web UI), 已自动修改至 ClientURL (Web API): %s",
	"Debug-LoadConfig_HotReload":                       "发现配置文件 (%s) 更改, 正在进行热重载",
	"Debug-SetBlockListFromFile_HotReload":             "发现 BlockListFile (%s) 更改, 正在进行热重载",
	"Debug-SetIPBlockListFromFile_HotReload":           "发现 IPBlockListFile (%s) 更改, 正在进行热重载",
	"Debug-ShowOrHiddenWindow_HideWindow":              "窗口隐藏",
	"Debug-ShowOrHiddenWindow_ShowWindow":              "窗口显示",
	"Debug-RestartTorrentByMap_Wait":                   "重新开始 Torrent 前的等待间隔: %d 秒",
	"Abandon-SetURL":                                   "放弃读取客户端配置文件 (WebUIEnabled: %t, Address: %s)",
	"Error":                                            "发生错误",
	"Error-LoadLang":                                   "加载语言文件时发生了错误: %s",
	"Error-ReadLang":                                   "读取语言文件时发生了错误: %s|%s",
	"Error-ParseLang":                                  "解析语言文件时发生了错误: %s|%s",
	"Error-RegHotkey":                                  "注册窗口热键时发生错误: %v",
	"Error-DetectProgramPath":                          "检测程序运行路径时发生了错误: %s",
	"Error-LoadConfigMeta":                             "加载配置文件 (%s) 元数据时发生了错误: %s",
	"Error-LoadConfig":                                 "加载配置文件 (%s) 时发生了错误: %s",
	"Error-ParseConfig":                                "解析配置文件 (%s) 时发生了错误: %s",
	"Error-LoadFile":                                   "加载文件 (%s) 时发生了错误: %s",
	"Error-GetClientConfig_LoadConfig":                 "加载客户端配置文件时发生了错误: %s",
	"Error-GetClientConfig_LoadConfigMeta":             "读取客户端配置文件元数据时发生了错误: %s",
	"Error-SetBlockListFromContent_Compile":            ":%d 表达式 %s 有错误 (来源: %s)",
	"Error-SetIPBlockListFromContent_Compile":          ":%d IP %s 有错误 (来源: %s)",
	"Error-SyncWithServer_Compile":                     ":%d IP %s 有错误",
	"Error-RestartTorrentByMap_Stop":                   "停止 Torrent 时发生了错误: %s",
	"Error-RestartTorrentByMap_Start":                  "开始 Torrent 时发生了错误: %s",
	"Error-LargeFile":                                  "解析时发生了错误: 目标大小大于 8MB",
	"Error-NewRequest":                                 "请求时发生了错误: %s",
	"Error-FetchResponse":                              "获取时发生了错误: %s",
	"Error-FetchResponse2":                             "获取时发生了错误",
	"Error-ReadResponse":                               "读取时发生了错误: %s",
	"Error-NoAuth":                                     "请求时发生了错误: 认证失败",
	"Error-Forbidden":                                  "请求时发生了错误: 禁止访问",
	"Error-NotFound":                                   "请求时发生了错误: 资源不存在",
	"Error-UnknownStatusCode":                          "请求时发生了错误: 未知状态码 %d",
	"Error-Parse":                                      "解析时发生了错误: %s",
	"Error-Login":                                      "登录时发生了错误",
	"Error-FetchUpdate":                                "获取更新时发生了错误",
	"Error-GenJSON":                                    "构造 JSON 时发生了错误: %s",
	"Error-Log_Write":                                  "写入日志时发生了错误: %s",
	"Error-IPFilter_Write":                             "写入 IPFilter 时发生了错误: %s",
	"Error-LoadLog_Mkdir":                              "创建日志目录时发生了错误: %s",
	"Error-LoadLog_Close":                              "关闭日志时发生了错误: %s",
	"Error-MatchRegexpErr":                             "正则匹配过程中发生了错误: %s",
	"Error-Task_EmptyURL":                              "检测到 URL 为空, 可能是未配置且未能自动读取客户端配置文件",
	"Error-Task_NotSupportClient":                      "检测到不支持的客户端, 可能是未配置且未能自动检测客户端: %s",
	"Error-SyncWithServer_ServerError":                 "同步服务器错误: %s",
	"Error-Debug-EmptyLine":                            ":%d 为空",
	"Error-Debug-GetClientConfigPath_GetUserHomeDir":   "获取 User Home 目录时发生了错误: %s",
	"Error-Debug-GetClientConfigPath_GetUserConfigDir": "获取 User Config 目录时发生了错误: %s",
	"Failed-LoadInitConfig":                            "读取配置文件失败或不完整",
	"Failed-ChangeWorkingDir":                          "切换工作目录失败: %s",
	"Failed-Login_BadUsernameOrPassword":               "登录失败: 账号或密码错误",
	"Failed-Login_Other":                               "登录失败: %s",
	"Failed-ExecCommand":                               "执行命令失败, 输出: %s, 错误: %s",
	"Success-RegHotkey":                                "已注册并开始监听窗口热键: CTRL+ALT+B",
	"Success-ChangeWorkingDir":                         "切换工作目录: %s",
	"Success-LoadConfig":                               "加载配置文件 (%s) 成功",
	"Success-SetCSRFToken":                             "设置 CSRF Token 成功: %s",
	"Success-SetURL":                                   "读取客户端配置文件成功 (WebUIEnabled: %t, URL: %s, Username: %s)",
	"Success-GenIPFilter":                              "生成了 %d 条 IP 规则",
	"Success-SetBlockListFromURL":                      "设置了 %d 条 表达式 规则 (来源: BlockListURL)",
	"Success-SetIPBlockListFromURL":                    "设置了 %d 条 IP 规则 (来源: IPBlockListURL)",
	"Success-SetBlockListFromFile":                     "设置了 %d 条 表达式 规则 (来源: BlockListFile)",
	"Success-SetIPBlockListFromFile":                   "设置了 %d 条 IP 规则 (来源: IPBlockListFile)",
	"Success-DetectClient":                             "检测客户端类型成功: %s",
	"Success-Login":                                    "登录成功",
	"Success-ClearBlockPeer":                           "已清理过期客户端: %d 个",
	"Success-ExecCommand":                              "执行命令成功, 输出: %s",
	"Success-SyncWithServer":                           "成功与同步服务器同步",

	// Part ShadowBan.
	"Warning-ShadowBanAPINotExist":                     "未检测到 ShadowBan API, 将使用常规方法",
	"Warning-ShadowBanAPIDisabled":                     "未启用 ShadowBan API, 将使用常规方法",
	"Failed-UnknownShadowBanAPI":                       "检测到未知 Shadow Ban API, 将使用常规方法",
	"Failed-GetQBPreferences":                          "获取 qBittorrent 偏好设置失败",

}

func LoadLang(langCode string) bool {
	langPath := "lang/" + langCode + ".json"

	_, err := os.Stat(langPath)
	if err != nil {
		if !os.IsNotExist(err) {
			Log("LoadLang", GetLangText("Error-LoadLang"), false, langPath)
		}
		return false
	}

	langFile, err := os.ReadFile(langPath)
	if err != nil {
		Log("LoadLang", GetLangText("Error-ReadLang"), false, langPath, err.Error())
		return false
	}

	if err := json.Unmarshal(langFile, &langContent); err != nil {
		Log("LoadLang", GetLangText("Error-ParseLang"), false, langPath, err.Error())
		return false
	}

	return true
}
func GetLangCode() string {
	langTag, err := locale.Detect()
	if err == nil {
		return langTag.String()[0:2]
	}

	return "en"
}
func GetLangText(uniqueID string) string {
	if content, exist := langContent[uniqueID]; exist {
		return content
	}

	if defaultContent, exist := defaultLangContent[uniqueID]; exist {
		return defaultContent
	}

	return uniqueID
}
