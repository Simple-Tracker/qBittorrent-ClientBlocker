package main

import (
	"os"
	"time"
	"strings"
	"strconv"
	"syscall"
	"runtime"
	"os/signal"
	"encoding/json"
)

var loopTicker *time.Ticker
var currentTimestamp int64 = 0
var lastCheckUpdateTimestamp int64 = 0
var githubAPIHeader = map[string]string { "Accept": "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28" }
var isRunning bool

type ReleaseStruct struct {
	URL        string `json:"html_url"`
	TagName    string `json:"tag_name"`
	Name       string `json:"name"`
	Body       string `json:"body"`
	PreRelease bool   `json:"prerelease`
}

func ProcessVersion(version string) (int, int, int, int, string) {
	version = strings.SplitN(version, " ", 2)[0]
	versionSplit := strings.SplitN(version, ".", 2)

	if versionSplit[0] == "Unknown" || len(versionSplit) != 2 {
		return -1, 0, 0, 0, ""
	}

	if strings.Contains(version, "(Nightly)") {
		return -2, 0, 0, 0, ""
	}

	if strings.Contains(version, "-") {
		return -3, 0, 0, 0, ""
	}

	mainVersion, err1 := strconv.Atoi(versionSplit[0])

	versionType := 0 // 0: Public, 1: Beta.
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
 
	return versionType, mainVersion, subVersion, sub2Version, version
}
func CheckUpdate() {
	if !config.CheckUpdate || (lastCheckUpdateTimestamp + 86400) > currentTimestamp {
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

	_, _, listReleaseContent := Fetch("https://api.github.com/repos/Simple-Tracker/qBittorrent-ClientBlocker/releases?per_page=5", false, false, &githubAPIHeader)
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

	if matchLatestPreReleaseVersion {
		versionType, mainVersion, subVersion, sub2Version, _ := ProcessVersion(latestPreReleaseStruct.TagName)

		if versionType == 1 {
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
	}

	Log("CheckUpdate", GetLangText("CheckUpdate-ShowVersion"), true, currentVersion, latestReleaseStruct.TagName, latestPreReleaseStruct.TagName)

	if hasNewReleaseVersion {
		Log("CheckUpdate", GetLangText("CheckUpdate-DetectNewVersion"), true, latestReleaseStruct.TagName, ("https://github.com/Simple-Tracker/" + programName + "/releases/tag/" + latestReleaseStruct.TagName), strings.Replace(latestReleaseStruct.Body, "\r", "", -1))
	}

	if hasNewPreReleaseVersion {
		Log("CheckUpdate", GetLangText("CheckUpdate-DetectNewBetaVersion"), true, latestPreReleaseStruct.TagName, ("https://github.com/Simple-Tracker/" + programName + "/releases/tag/" + latestPreReleaseStruct.TagName), strings.Replace(latestPreReleaseStruct.Body, "\r", "", -1))
	}
}
func Task() {
	if config.ClientURL == "" {
		Log("Task", GetLangText("Error-Task_EmptyURL"), true)
		return
	}
	if !IsSupportClient() {
		Log("Task", GetLangText("Error-Task_NotSupportClient"), true, currentClientType)
		return
	}
	
	torrents := FetchTorrents()
	if torrents == nil {
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

	switch currentClientType {
		case "qBittorrent":
			torrents2 := torrents.(*[]qB_TorrentStruct)
			for _, torrentInfo := range *torrents2 {
				ProcessTorrent(torrentInfo.InfoHash, torrentInfo.Tracker, torrentInfo.NumLeechs, torrentInfo.TotalSize, nil, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount, &emptyPeersCount)
			}
		case "Transmission":
			torrents2 := torrents.(*Tr_TorrentsStruct)
			for _, torrentInfo := range torrents2.Torrents {
				// 手动判断有无 Peer 正在下载.
				var leecherCount int64 = 0
				for _, torrentPeer := range torrentInfo.Peers {
					if torrentPeer.IsUploading {
						leecherCount++
					}
				}

				tracker := ""
				if torrentInfo.Private {
					tracker = "Private"
				}

				ProcessTorrent(torrentInfo.InfoHash, tracker, leecherCount, torrentInfo.TotalSize, torrentInfo.Peers, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount, &emptyPeersCount)
			}
		case "BitComet":
			// BitComet 无法通过 Torrent 列表取得 TorrentInfoHash, 因此使用 TorrentID 取代.
			torrents2 := torrents.(*map[int]BC_TorrentStruct)
			for torrentID, torrentInfo := range *torrents2 {
				var leecherCount int64 = 233
				if torrentInfo.UpSpeed > 0 {
					leecherCount = 233
				}
				ProcessTorrent(strconv.Itoa(torrentID), "Unsupported", leecherCount, torrentInfo.TotalSize, nil, &emptyHashCount, &noLeechersCount, &badTorrentInfoCount, &ptTorrentCount, &blockCount, &ipBlockCount, &badPeersCount, &emptyPeersCount)
			}
	}

	currentIPBlockCount := CheckAllIP(ipMap, lastIPMap)
	torrentBlockCount, torrentIPBlockCount := CheckAllTorrent(torrentMap, lastTorrentMap)
	blockCount += torrentBlockCount
	ipBlockCount += torrentIPBlockCount

	Log("Debug-Task_IgnoreEmptyHashCount", "%d", false, emptyHashCount)
	Log("Debug-Task_IgnoreNoLeechersCount", "%d", false, noLeechersCount)
	Log("Debug-Task_IgnorePTTorrentCount", "%d", false, ptTorrentCount)
	Log("Debug-Task_IgnoreBadTorrentInfoCount", "%d", false, badTorrentInfoCount)
	Log("Debug-Task_IgnoreBadPeersCount", "%d", false, badPeersCount)
	Log("Debug-Task_IgnoreEmptyPeersCount", "%d", false, emptyPeersCount)

	if cleanCount != 0 || blockCount != 0 {
		SubmitBlockPeer(blockPeerMap)
		if !config.IPUploadedCheck && len(ipBlockListCompiled) <= 0 && len(ipBlockListFromURLCompiled) <= 0 {
			Log("Task", GetLangText("Task_BanInfo"), true, blockCount, len(blockPeerMap))
		} else {
			Log("Task", GetLangText("Task_BanInfoWithIP"), true, blockCount, len(blockPeerMap), currentIPBlockCount, ipBlockCount)
		}
	}
}
func GC() {
	ipMapGCCount := (len(ipMap) - 23333333)

	if ipMapGCCount > 0 {
		Log("GC", GetLangText("GC_IPMap"), true, ipMapGCCount)
		for ip, _ := range ipMap {
			ipMapGCCount--
			delete(ipMap, ip)
			if ipMapGCCount <= 0 {
				break
			}
		}
		runtime.GC()
	}

	for torrentInfoHash, torrentInfo := range torrentMap {
		torrentInfoGCCount := (len(torrentInfo.Peers) - 2333333)
		if torrentInfoGCCount > 0 {
			Log("GC", GetLangText("GC_TorrentMap"), true, torrentInfoHash, torrentInfoGCCount)
			for peerIP, _ := range torrentInfo.Peers {
				torrentInfoGCCount--
				delete(torrentMap[torrentInfoHash].Peers, peerIP)
				if torrentInfoGCCount <= 0 {
					break
				}
			}
			runtime.GC()
		}
	}
}
func WaitStop() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM)

	<-signalChan
		Log("WaitStop", GetLangText("WaitStop_Stoping"), true)
		isRunning = false
}
func RunConsole() {
	if config.StartDelay > 0 {
		Log("RunConsole", GetLangText("RunConsole_StartDelay"), false, config.StartDelay)
		time.Sleep(time.Duration(config.StartDelay) * time.Second)
	}
	if !LoadInitConfig(true) {
		Log("RunConsole", GetLangText("RunConsole_AuthFailed"), true)
		os.Exit(1)
	}
	isRunning = true
	Log("RunConsole", GetLangText("RunConsole_ProgramHasStarted"), true)
	go WaitStop()
	loopTicker = time.NewTicker(1 * time.Second)
	for ; true; <- loopTicker.C {
		if !isRunning {
			loopTicker.Stop()
			SubmitBlockPeer(nil)
			httpClient.CloseIdleConnections()
			httpClientWithoutCookie.CloseIdleConnections()
			StopServer()
			Platform_Stop()
			break
		}
		tmpCurrentTimestamp := time.Now().Unix()
		if (currentTimestamp + int64(config.Interval)) <= tmpCurrentTimestamp {
			currentTimestamp = tmpCurrentTimestamp
			LoadInitConfig(false)
			go CheckUpdate()
			Task()
			GC()
		}
	}
}
