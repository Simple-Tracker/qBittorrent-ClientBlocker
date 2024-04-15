package main

import (
	"bytes"
	"strings"
	"strconv"
	"github.com/PuerkitoBio/goquery"
)

type BC_TorrentStruct struct {
	TotalSize int64
	UpSpeed   int64
}
type BC_PeerStruct struct {
	IP         string
	Port       int
	Client     string
//	PeerID     string
	Progress   float64
	Downloaded int64
	Uploaded   int64
	DlSpeed    int64
	UpSpeed    int64
}

func BC_ParseTorrentLink(torrentLinkStr string) int {
	torrentIDSplit1 := strings.SplitN(StrTrim(torrentLinkStr), "?id=", 2)
	if len(torrentIDSplit1) < 2 {
		return -2
	}

	torrentIDStr := strings.SplitN(torrentIDSplit1[1], "&", 2)[0]
	torrentID, err := strconv.Atoi(torrentIDStr)
	if err != nil {
		return -3
	}

	return torrentID
}
func BC_ParseSize(sizeStr string) int64 {
	sizeStr = StrTrim(sizeStr)
	if sizeStr == "" {
		return 0
	}

	sizeStrSplit := strings.SplitN(sizeStr, " ", 2)
	if len(sizeStrSplit) < 2 || len(sizeStrSplit[1]) < 2 {
		return -1
	}

	rawSize, err := strconv.ParseFloat(sizeStrSplit[0], 64)
	if err != nil {
		return -2
	}

	matched := false
	multipler := 1
	switch sizeStrSplit[1] {
		case "EB":
			multipler *= 1024
			fallthrough
		case "PB":
			multipler *= 1024
			fallthrough
		case "TB":
			multipler *= 1024
			fallthrough
		case "GB":
			multipler *= 1024
			fallthrough
		case "MB":
			multipler *= 1024
			fallthrough
		case "KB":
			multipler *= 1024
			fallthrough
		case "B":
			matched = true
	}

	if !matched {
		return -3
	}

	return int64(rawSize * float64(multipler))
}
func BC_ParseSpeed(speedStr string) int64 {
	speedStr = StrTrim(speedStr)
	if speedStr == "" {
		return 0
	}

	speedStrSplit := strings.SplitN(speedStr, "/", 2)

	if len(speedStrSplit) < 2 || len(speedStrSplit[1]) != 1 {
		return -1
	}

	return BC_ParseSize(speedStrSplit[0])
}
func BC_ParsePrecent(precentStr string) float64 {
	precentStr = StrTrim(precentStr)
	if len(precentStr) < 2 {
		return -1
	}

	precentStr = precentStr[:(len(precentStr) - 1)]
	precent, err := strconv.ParseFloat(precentStr, 64)
	if err != nil {
		return -2
	}

	return precent
}
func BC_ParseIP(ipStr string) (string, int) {
	ipStr = StrTrim(ipStr)
	if ipStr == "Myself" {
		return "", -1
	}

	lastColonIndex := strings.LastIndex(ipStr, ":")
	if lastColonIndex == -1 || len(ipStr) < (lastColonIndex + 2) {
		return "", -2
	}

	ipWithoutPortStr := ipStr[:lastColonIndex]
	portStr := ipStr[(lastColonIndex + 1):]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", -3
	}

	return ipWithoutPortStr, port
}
func BC_DetectClient() bool {
	apiResponseStatusCode, apiResponseHeaders, _ := Fetch(config.ClientURL + "/panel/", false, false, nil)
	return (apiResponseStatusCode == 401 && strings.Contains(apiResponseHeaders.Get("WWW-Authenticate"), "BitComet"))
}
func BC_Login() bool {
	// BitComet 通过 Basic Auth 进行认证, 因此此处只进行验证.
	apiResponseStatusCode, _, _ := Fetch(config.ClientURL + "/panel/", false, true, nil)
	return (apiResponseStatusCode == 200)
}
func BC_FetchTorrents() *map[int]BC_TorrentStruct {
	_, _, torrentsResponseBody := Fetch(config.ClientURL + "/panel/task_list?group=active", true, true, nil)
	if torrentsResponseBody == nil {
		Log("FetchTorrents", GetLangText("Error"), true)
		return nil
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(torrentsResponseBody))
	if err != nil {
		Log("FetchTorrents", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	torrentsMap := make(map[int]BC_TorrentStruct)
	document.Find("table:last-child > tbody > tr").Each(func (index int, element *goquery.Selection) {
		if index == 0 {
			return
		}

		torrentStatus := ""
		torrentID := 0
		var torrentSize int64 = -233
		var torrentUpSpeed int64 = -233
		element.Find("td").EachWithBreak(func (tdIndex int, tdElement *goquery.Selection) bool {
			switch tdIndex {
				case 0:
					if StrTrim(tdElement.Text()) != "BT" {
						return false
					}
				case 1:
					href, exists := tdElement.Find("a").Attr("href")
					if !exists {
						return false
					}

					torrentID = BC_ParseTorrentLink(href)
				case 2:
					torrentStatus = StrTrim(tdElement.Text())
				case 4:
					torrentSize = BC_ParseSize(tdElement.Text())
				case 7:
					torrentUpSpeed = BC_ParseSpeed(tdElement.Text())
			}

			return true
		})

		if torrentStatus == "" || torrentID <= 0 || torrentSize <= 0 || torrentUpSpeed < 0 {
			return
		}

		torrentsMap[torrentID] = BC_TorrentStruct { TotalSize: torrentSize, UpSpeed: torrentUpSpeed }
	})

	return &torrentsMap
}
func BC_FetchTorrentPeers(infoHash string) *[]BC_PeerStruct {
	_, _, torrentPeersResponseBody := Fetch(config.ClientURL + "/panel/task_detail?id=" + infoHash + "&show=peers", true, true, nil)
	if torrentPeersResponseBody == nil {
		Log("FetchTorrentPeers", GetLangText("Error"), true)
		return nil
	}

	document, err := goquery.NewDocumentFromReader(bytes.NewReader(torrentPeersResponseBody))
	if err != nil {
		Log("FetchTorrentPeers", GetLangText("Error-Parse"), true, err.Error())
		return nil
	}

	torrentsMap := []BC_PeerStruct {}
	document.Find("table:last-child > tbody > tr").Each(func (index int, element *goquery.Selection) {
		if index == 0 {
			return
		}

		peerIP := ""
		peerPort := -233
		var peerProgress float64 = -233
		var peerDlSpeed int64 = -233
		var peerUpSpeed int64 = -233
		var peerDownloaded int64 = -233
		var peerUploaded int64 = -233
		peerClient := ""
		//peerID := ""
		element.Find("td").EachWithBreak(func (tdIndex int, tdElement *goquery.Selection) bool {
			switch tdIndex {
				case 0:
					peerIP, peerPort = BC_ParseIP(tdElement.Text())
				case 1:
					peerProgress = BC_ParsePrecent(tdElement.Text())
				case 2:
					peerDlSpeed = BC_ParseSpeed(tdElement.Text())
				case 3:
					peerUpSpeed = BC_ParseSpeed(tdElement.Text())
				case 4:
					peerDownloaded = BC_ParseSize(tdElement.Text())
				case 5:
					peerUploaded = BC_ParseSize(tdElement.Text())
				case 9:
					peerClient = tdElement.Text()
				case 10:
					// 错误的信息.
					// peerID = tdElement.Text()
			}

			return true
		})

		if peerIP == "" || peerPort < 0 || peerProgress < 0 || peerDlSpeed < 0 || peerUpSpeed < 0 || peerDownloaded < 0 || peerUploaded < 0 {
			return
		}

		peerStruct := BC_PeerStruct { IP: peerIP, Port: peerPort, Client: peerClient, Progress: peerProgress, Downloaded: peerDownloaded, Uploaded: peerUploaded, DlSpeed: peerDlSpeed, UpSpeed: peerUpSpeed }
		torrentsMap = append(torrentsMap, peerStruct)
	})

	return &torrentsMap
}
