package main

import (
	"time"
	"net"
	"strings"
)

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
