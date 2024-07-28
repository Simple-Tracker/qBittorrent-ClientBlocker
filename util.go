package main

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Source: https://stackoverflow.com/questions/51459083/deep-copying-maps-in-golang.
func DeepCopyIPMap(src map[string]IPInfoStruct, dest map[string]IPInfoStruct) {
	if src != nil && dest != nil {
		if jsonStr, err := json.Marshal(src); err == nil {
			json.Unmarshal(jsonStr, &dest)
		}
	}
}
func DeepCopyTorrentMap(src map[string]TorrentInfoStruct, dest map[string]TorrentInfoStruct) {
	if src != nil && dest != nil {
		if jsonStr, err := json.Marshal(src); err == nil {
			json.Unmarshal(jsonStr, &dest)
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
	// Remove all remarks.
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
