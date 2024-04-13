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
	return (strings.Count(ip, ":") >= 2)
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
