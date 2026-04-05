package main

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 参考: https://stackoverflow.com/questions/51459083/deep-copying-maps-in-golang.
func DeepCopyIPMap(src map[string]IPInfoStruct, dest map[string]IPInfoStruct) {
	if src != nil && dest != nil {
		for k := range dest {
			delete(dest, k)
		}
		for k, v := range src {
			newPortMap := make(map[int]bool)
			for pk, pv := range v.Port {
				newPortMap[pk] = pv
			}
			newTorrentUploadedMap := make(map[string]int64)
			for tk, tv := range v.TorrentUploaded {
				newTorrentUploadedMap[tk] = tv
			}
			dest[k] = IPInfoStruct{
				Net:             v.Net,
				Port:            newPortMap,
				TorrentUploaded: newTorrentUploadedMap,
			}
		}
	}
}
func DeepCopyTorrentMap(src map[string]TorrentInfoStruct, dest map[string]TorrentInfoStruct) {
	if src != nil && dest != nil {
		for k := range dest {
			delete(dest, k)
		}
		for k, v := range src {
			newPeers := make(map[string]PeerInfoStruct)
			for pk, pv := range v.Peers {
				newPortMap := make(map[int]bool)
				for ppk, ppv := range pv.Port {
					newPortMap[ppk] = ppv
				}
				newPeers[pk] = PeerInfoStruct{
					Net:        pv.Net,
					Port:       newPortMap,
					Progress:   pv.Progress,
					Downloaded: pv.Downloaded,
					Uploaded:   pv.Uploaded,
				}
			}
			dest[k] = TorrentInfoStruct{
				Size:  v.Size,
				Peers: newPeers,
			}
		}
	}
}
func IsUnix(path string) bool {
	return !strings.Contains(path, "\\")
}
func IsIPv6(ip string) bool {
	return (strings.Count(ip, ":") >= 2)
}
func ProcessRemark(str string) string {
	// 删除所有注释内容.
	return StrTrim(strings.SplitN(strings.SplitN(str, "#", 2)[0], "//", 2)[0])
}
func StrTrim(str string) string {
	return strings.Trim(str, "  \n\r")
}
func GetDateTime(withTime bool) string {
	formatStr := "2006-01-02"
	if withTime {
		formatStr += " 15:04:05"
	}
	return time.Now().Format(formatStr)
}
func CheckPrivateIP(ip string) bool {
	ipParsed := net.ParseIP(ip)
	if ipParsed == nil {
		return false
	}
	return (ipParsed.IsLoopback() || ipParsed.IsPrivate())
}
func ParseIPCIDR(ip string) *net.IPNet {
	if !strings.Contains(ip, "/") {
		if IsIPv6(ip) {
			ip += "/128"
		} else {
			ip += "/32"
		}
	}

	_, cidr, err := net.ParseCIDR(ip)
	if err != nil {
		return nil
	}

	return cidr
}
func ParseIPCIDRByConfig(ip string) *net.IPNet {
	cidr := ""

	if IsIPv6(ip) {
		if config.BanIP6CIDR != "/128" {
			cidr = config.BanIP6CIDR
		}
	} else {
		if config.BanIPCIDR != "/32" {
			cidr = config.BanIPCIDR
		}
	}

	if cidr == "" {
		return nil
	}

	cidrNet := ParseIPCIDR(ip + cidr)

	if cidrNet == nil {
		return nil
	}

	return cidrNet
}
func ProcessIP(ip string) string {
	ip = strings.ToLower(ip)

	if strings.HasPrefix(ip, "::ffff:") {
		return ip[7:]
	}

	return ip
}
func GenIPFilter(datType uint32, blockPeerMap map[string]BlockPeerInfoStruct) (int, string) {
	ipfilterCount := 0
	ipfilterStr := ""

	if datType != 1 && datType != 2 {
		return ipfilterCount, ipfilterStr
	}

	for peerIP := range blockPeerMap {
		if !IsIPv6(peerIP) {
			ipfilterCount += 2
			if datType == 1 {
				ipfilterStr += peerIP + "/32\n"
				ipfilterStr += "::ffff:" + peerIP + "/128\n"
			} else if datType == 2 {
				ipfilterStr += peerIP + " - " + peerIP + " , 000\n"
				ipfilterStr += "::ffff:" + peerIP + " - ::ffff:" + peerIP + " , 000\n"
			}
		} else {
			ipfilterCount++
			if datType == 1 {
				ipfilterStr += peerIP + "/128\n"
			} else if datType == 2 {
				ipfilterStr += peerIP + " - " + peerIP + " , 000\n"
			}
		}
	}

	return ipfilterCount, ipfilterStr
}
func SaveIPFilter(ipfilterStr string) string {
	err := os.WriteFile("ipfilter.dat", []byte(ipfilterStr), 0666)
	if err != nil {
		return err.Error()
	}

	return ""
}
func DeleteIPFilter() bool {
	err := os.Remove("ipfilter.dat")
	if err != nil {
		return false
	}

	return true
}
func ParseCommand(command string) []string {
	var matchQuote rune = -1
	escaped := false
	commandPart := []string{""}
	commandIndex := 0

	for _, char := range command {
		if char == '\\' && matchQuote == -1 {
			escaped = true
			continue
		} else if char == ' ' && matchQuote == -1 {
			if escaped {
				commandPart[commandIndex] += "\\"
				escaped = false
			}
			if commandPart[commandIndex] != "" {
				commandIndex++
				commandPart = append(commandPart, "")
			}
			continue
		} else if !escaped && char == '\'' || char == '"' {
			if char == matchQuote {
				matchQuote = -1
				continue
			} else if matchQuote == -1 {
				matchQuote = char
				continue
			}
		}
		if escaped {
			commandPart[commandIndex] += "\\"
			escaped = false
		}
		commandPart[commandIndex] += string(char)
	}

	return commandPart
}
func ExecCommand(command string) (bool, string, string) {
	commandSplit := ParseCommand(command)
	Log("Debug-ExecCommand", "Raw: %s, Split (|): %s", false, command, strings.Join(commandSplit, "|"))

	var cmd *exec.Cmd
	if len(commandSplit) == 1 {
		cmd = exec.Command(commandSplit[0])
	} else {
		cmd = exec.Command(commandSplit[0], commandSplit[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, string(out), err.Error()
	}

	return true, string(out), ""
}

// EraseSyncMap 是一个辅助函数, 用于删除 sync.Map 中的所有元素.
// 该函数可在 Go1.23 及以上版本替换为 `Clear` 方法.
//
// go:build !go1.23
func EraseSyncMap(m *sync.Map) {
	m.Range(func(key, value any) bool {
		m.Delete(key)
		return true
	})
}

// CompareVersion 按照语义化版本 (Semantic Versioning) 比较两个版本号.
// 返回 1 表示 v1 > v2, -1 表示 v1 < v2, 0 表示相等.
func CompareVersion(v1, v2 string) int {
	// 清理可选的 "v" 前缀.
	v1 = strings.TrimPrefix(strings.ToLower(v1), "v")
	v2 = strings.TrimPrefix(strings.ToLower(v2), "v")

	if v1 == v2 {
		return 0
	}

	v1Part := strings.Split(v1, ".")
	v2Part := strings.Split(v2, ".")

	len1 := len(v1Part)
	len2 := len(v2Part)
	maxLen := len1
	if len2 > maxLen {
		maxLen = len2
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len1 {
			n1, _ = strconv.Atoi(v1Part[i])
		}
		if i < len2 {
			n2, _ = strconv.Atoi(v2Part[i])
		}

		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	return 0
}
