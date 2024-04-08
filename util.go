package main

import (
	"net"
	"time"
	"strings"
	"encoding/json"
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
	return strings.Contains(ip, "::")
}
func StrTrim(str string) string {
	return strings.Trim(str, " \n\r")
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
	return ipParsed.IsPrivate()
}
func ParseIP(ip string) *net.IPNet {
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
func ProcessIP(ip string) string {
	ip = strings.ToLower(ip)

	if strings.HasPrefix(ip, "::ffff:") {
		return ip[7:]
	}

	return ip
}
