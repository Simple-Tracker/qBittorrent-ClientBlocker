package main

import (
	"encoding/json"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var lastCheckUpdateVer = ""
var lastCheckUpdateBetaVer = "None"
var lastCheckUpdateTimestamp int64 = 0
var githubAPIHeader = map[string]string{"Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28"}
var reqStopChan = make(chan struct{})
var reqStopOnce sync.Once
var reqStopLogged atomic.Bool

type ReleaseStruct struct {
	URL        string `json:"html_url"`
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	PreRelease bool   `json:"prerelease"`
}

func ProcessVersion(version string) (int, int, int, int, string) {
	realVersion := strings.SplitN(version, " ", 2)[0]
	versionSplit := strings.SplitN(realVersion, ".", 2)

	if strings.Contains(version, "Unknown") {
		return -1, 0, 0, 0, ""
	}

	if strings.Contains(version, "(Nightly)") {
		return -2, 0, 0, 0, ""
	}

	if strings.Contains(version, "-") || strings.Contains(version, "_") {
		return -3, 0, 0, 0, ""
	}

	if len(versionSplit) != 2 {
		return -1, 0, 0, 0, ""
	}

	mainVersion, err1 := strconv.Atoi(versionSplit[0])

	versionType := 0 // 0: 公共版本, 1: Beta 版本.
	versionSplit2 := strings.SplitN(versionSplit[1], "p", 2)
	versionSplit3 := strings.SplitN(versionSplit[1], "b", 2)

	subVersionStr := versionSplit[1]
	sub2VersionStr := "0"

	if len(versionSplit2) >= 2 {
		subVersionStr = versionSplit2[0]
		sub2VersionStr = versionSplit2[1]
	} else if len(versionSplit3) >= 2 {
		versionType = 1
		subVersionStr = versionSplit3[0]
		sub2VersionStr = versionSplit3[1]
	}

	subVersion, err2 := strconv.Atoi(subVersionStr)
	sub2Version, err3 := strconv.Atoi(sub2VersionStr)

	if err1 != nil || err2 != nil || err3 != nil {
		return -3, 0, 0, 0, ""
	}

	return versionType, mainVersion, subVersion, sub2Version, realVersion
}

// CheckUpdate 检查软件更新.
func CheckUpdate() {
	if !config.CheckUpdate || (lastCheckUpdateTimestamp+86400) > currentTimestamp {
		return
	}

	lastCheckUpdateTimestamp = currentTimestamp

	currentVersionType, currentMainVersion, currentSubVersion, currentSub2Version, currentVersion := ProcessVersion(programVersion)

	if currentVersionType == -1 {
		Log("CheckUpdate", GetLangText("CheckUpdate-Ignore_UnknownVersion"), true)
		return
	}

	if currentVersionType == -2 {
		Log("CheckUpdate", GetLangText("CheckUpdate-Ignore_NightlyVersion"), true)
		return
	}

	if currentVersionType == -3 {
		Log("CheckUpdate", GetLangText("CheckUpdate-Ignore_BadVersion"), true, programVersion)
		return
	}

	listResponseCode, _, listReleaseContent := Fetch("https://api.github.com/repos/Simple-Tracker/qBittorrent-ClientBlocker/releases?per_page=5", false, false, true, &githubAPIHeader)
	if listResponseCode == 304 {
		return
	}

	if listReleaseContent == nil {
		Log("CheckUpdate", GetLangText("Error-FetchUpdate"), true)
		return
	}

	var releasesStruct []ReleaseStruct
	if err := json.Unmarshal(listReleaseContent, &releasesStruct); err != nil {
		Log("CheckUpdate", GetLangText("Error-Parse"), true, err.Error())
		return
	}

	matchLatestReleaseVersion := false
	matchLatestPreReleaseVersion := false
	var latestReleaseStruct ReleaseStruct
	var latestPreReleaseStruct ReleaseStruct

	for _, releaseStruct := range releasesStruct {
		if releaseStruct.TagName == "" {
			continue
		}

		if matchLatestPreReleaseVersion && matchLatestReleaseVersion {
			break
		}

		if !matchLatestPreReleaseVersion && releaseStruct.PreRelease {
			matchLatestPreReleaseVersion = true
			latestPreReleaseStruct = releaseStruct
		}
		if !matchLatestReleaseVersion && !releaseStruct.PreRelease {
			matchLatestReleaseVersion = true
			latestReleaseStruct = releaseStruct
		}
	}

	hasNewReleaseVersion := false
	hasNewPreReleaseVersion := false

	if matchLatestPreReleaseVersion && currentVersionType == 1 {
		versionType, mainVersion, subVersion, sub2Version, _ := ProcessVersion(latestPreReleaseStruct.TagName)

		if versionType == currentVersionType {
			if mainVersion > currentMainVersion {
				hasNewPreReleaseVersion = true
			} else if mainVersion == currentMainVersion {
				if subVersion > currentSubVersion {
					hasNewPreReleaseVersion = true
				} else if subVersion == currentSubVersion && sub2Version > currentSub2Version {
					hasNewPreReleaseVersion = true
				}
			}
		}
	}

	if matchLatestReleaseVersion {
		versionType, mainVersion, subVersion, sub2Version, _ := ProcessVersion(latestReleaseStruct.TagName)

		if versionType == 0 {
			if mainVersion > currentMainVersion {
				hasNewReleaseVersion = true
			} else if mainVersion == currentMainVersion {
				if subVersion > currentSubVersion {
					hasNewReleaseVersion = true
				} else if subVersion == currentSubVersion && sub2Version > currentSub2Version {
					hasNewReleaseVersion = true
				}
			}
		}
	}

	if latestPreReleaseStruct.TagName == "" {
		latestPreReleaseStruct.TagName = "None"
	}

	Log("CheckUpdate", GetLangText("CheckUpdate-ShowVersion"), true, currentVersion, latestReleaseStruct.TagName, latestPreReleaseStruct.TagName)

	if hasNewReleaseVersion && lastCheckUpdateVer != latestReleaseStruct.TagName {
		lastCheckUpdateVer = latestReleaseStruct.TagName
		Log("CheckUpdate", GetLangText("CheckUpdate-DetectNewVersion"), true, latestReleaseStruct.TagName, ("https://github.com/Simple-Tracker/" + programName + "/releases/tag/" + latestReleaseStruct.TagName), strings.Replace(latestReleaseStruct.Body, "\r", "", -1))
	}

	if hasNewPreReleaseVersion && lastCheckUpdateBetaVer != latestPreReleaseStruct.TagName {
		lastCheckUpdateBetaVer = latestPreReleaseStruct.TagName
		Log("CheckUpdate", GetLangText("CheckUpdate-DetectNewBetaVersion"), true, latestPreReleaseStruct.TagName, ("https://github.com/Simple-Tracker/" + programName + "/releases/tag/" + latestPreReleaseStruct.TagName), strings.Replace(latestPreReleaseStruct.Body, "\r", "", -1))
	}
}

// Task 执行主要的循环任务.
func Task() {
	if config.ClientURL == "" {
		Log("Task", GetLangText("Error-Task_EmptyURL"), true)
		return
	}
	if !IsSupportClient() {
		Log("Task", GetLangText("Error-Task_NotSupportClient"), true, currentClientType)
		return
	}

	torrents, err := FetchTorrents()
	if err != nil || torrents == nil {
		return
	}

	cleanCount := ClearBlockPeer()

	emptyHashCount := 0
	noLeechersCount := 0
	badTorrentInfoCount := 0
	ptTorrentCount := 0

	blockCount := 0
	ipBlockCount := 0
	badPeersCount := 0
	emptyPeersCount := 0

	for _, torrentInfo := range torrents {
		ProcessTorrent(torrentInfo, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount, &emptyPeersCount)
	}

	ipBlockCount += CheckAllIP(ipMap, lastIPMap)
	torrentBlockCount, torrentIPBlockCount := CheckAllTorrent(torrentMap, lastTorrentMap)
	blockCount += torrentBlockCount
	ipBlockCount += torrentIPBlockCount

	Log("Debug-Task_IgnoreEmptyHashCount", "%d", false, emptyHashCount)
	Log("Debug-Task_IgnoreNoLeechersCount", "%d", false, noLeechersCount)
	Log("Debug-Task_IgnorePTTorrentCount", "%d", false, ptTorrentCount)
	Log("Debug-Task_IgnoreBadTorrentInfoCount", "%d", false, badTorrentInfoCount)
	Log("Debug-Task_IgnoreBadPeersCount", "%d", false, badPeersCount)
	Log("Debug-Task_IgnoreEmptyPeersCount", "%d", false, emptyPeersCount)

	if cleanCount != 0 || blockCount != 0 || ipBlockCount != 0 {
		if config.GenIPDat == 1 || config.GenIPDat == 2 {
			ipfilterCount, ipfilterStr := GenIPFilter(config.GenIPDat, blockPeerMap)
			err := SaveIPFilter(ipfilterStr)
			if err != "" {
				Log("Task", GetLangText("Error-IPFilter_Write"), true, err)
			} else {
				Log("Task", GetLangText("Success-GenIPFilter"), true, ipfilterCount)
			}
		}

		SubmitBlockPeer(blockPeerMap)

		iblcLen := 0
		ipBlockListCompiled.Range(func(key, value any) bool {
			iblcLen++
			return true
		})

		if !config.IPUploadedCheck && iblcLen <= 0 && len(ipBlockCIDRMapFromSyncServerCompiled) <= 0 {
			Log("Task", GetLangText("Task_BanInfo"), true, blockCount, len(blockPeerMap))
		} else {
			Log("Task", GetLangText("Task_BanInfoWithIP"), true, blockCount, ipBlockCount, len(blockPeerMap))
		}
	}

	SyncWithServer()
}

// GC 执行垃圾回收任务以清理过期数据并释放内存.
func GC() {
	// 保持旧阈值, 避免频繁清理.
	const ipMapThreshold = 23333333
	const peerMapThreshold = 2333333

	ipMapMutex.Lock()
	ipMapGCCount := (len(ipMap) - ipMapThreshold)
	if ipMapGCCount > 0 {
		Log("GC", GetLangText("GC_IPMap"), true, ipMapGCCount)
		for ip := range ipMap {
			ipMapGCCount--
			delete(ipMap, ip)
			if ipMapGCCount <= 0 {
				break
			}
		}
	}
	ipMapMutex.Unlock()

	torrentMapMutex.Lock()
	for torrentInfoHash, torrentInfo := range torrentMap {
		torrentInfoGCCount := (len(torrentInfo.Peers) - peerMapThreshold)
		if torrentInfoGCCount > 0 {
			Log("GC", GetLangText("GC_TorrentMap"), true, torrentInfoHash, torrentInfoGCCount)
			for peerIP := range torrentInfo.Peers {
				torrentInfoGCCount--
				delete(torrentMap[torrentInfoHash].Peers, peerIP)
				if torrentInfoGCCount <= 0 {
					break
				}
			}
		}
	}
	torrentMapMutex.Unlock()
	runtime.GC()
}

// WaitStop 监听退出信号.
func WaitStop() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM)
	select {
	case <-signalChan:
		ReqStop()
	case <-reqStopChan:
	}
}

// ReqStop 请求停止程序, 用于系统托盘等非信号退出路径.
func ReqStop() {
	reqStopOnce.Do(func() {
		reqStopLogged.Store(true)
		Log("ReqStop", GetLangText("ReqStop_Stoping"), true)
		close(reqStopChan)
	})
}

func Stop(recoverErr any, recoverStack []byte) {
	if recoverErr != nil {
		Log("Stop", GetLangText("Stop_CaughtPanic"), true, recoverErr)
		if len(recoverStack) == 0 {
			recoverStack = debug.Stack()
		}
		Log("Stop", GetLangText("Stop_StacktraceWhenPanic"), true, string(recoverStack))
		if !reqStopLogged.Load() {
			reqStopLogged.Store(true)
			Log("ReqStop", GetLangText("ReqStop_Stoping"), true)
		}
	}

	DeleteIPFilter()
	SubmitBlockPeer(nil)
	httpClient.CloseIdleConnections()
	httpClientExternal.CloseIdleConnections()
	StopServer()
	Platform_Stop()

	if recoverErr != nil {
		os.Exit(2)
	}
}

// RunConsole 启动控制台主循环.
func RunConsole() {
	if startDelay > 0 {
		Log("RunConsole", GetLangText("RunConsole_StartDelay"), false, startDelay)
		time.Sleep(time.Duration(startDelay) * time.Second)
	}

	for !LoadInitConfig(true) {
		time.Sleep(2 * time.Second)
		if !config.IgnoreFailureExit {
			os.Exit(1)
		}
	}

	Log("RunConsole", GetLangText("RunConsole_ProgramHasStarted"), true)
	StartServer()

	if config.ExecCommand_Run != "" {
		status, out, err := ExecCommand(config.ExecCommand_Run)
		if status {
			Log("RunConsole", GetLangText("Success-ExecCommand"), true, out)
		} else {
			Log("RunConsole", GetLangText("Failed-ExecCommand"), true, out, err)
		}
	}

	stopChan := make(chan struct{})
	doneChan := make(chan struct{})
	var loopPanic any
	var loopPanicStack []byte

	go func() {
		defer close(doneChan)
		defer func() {
			if recoverErr := recover(); recoverErr != nil {
				loopPanic = recoverErr
				loopPanicStack = debug.Stack()
				WriteCrashLog("RunConsole.loop", loopPanic, loopPanicStack)
				ReqStop()
			}
		}()

		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				now := time.Now().Unix()
				if (atomic.LoadInt64(&currentTimestamp) + int64(config.Interval)) <= now {
					atomic.StoreInt64(&currentTimestamp, now)
					GoWithCrashLog("CheckUpdate", CheckUpdate)

					if LoadInitConfig(false) {
						Task()
						GC()
					} else {
						Log("RunConsole", GetLangText("Error-Task_AuthFailed"), true)
					}
				}
			}
		}
	}()

	WaitStop()
	close(stopChan)
	<-doneChan
	Stop(loopPanic, loopPanicStack)
}
