package rules

import (
	"net"
	"strings"
)

func ProcessRemark(str string) string {
	// 删除所有注释内容.
	return StrTrim(strings.SplitN(strings.SplitN(str, "#", 2)[0], "//", 2)[0])
}
func StrTrim(str string) string {
	return strings.Trim(str, "  \n\r")
}
func ParseIPCIDR(ip string) *net.IPNet {
	if !strings.Contains(ip, "/") {
		if strings.Count(ip, ":") >= 2 {
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
