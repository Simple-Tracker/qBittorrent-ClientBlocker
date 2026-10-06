package app

import (
	"fmt"
	"runtime"
)

// SetVersion 同时更新由注入的构建版本生成的用户代理字符串.
func SetVersion(version string) {
	programVersion = version
	programUserAgent = fmt.Sprintf("%s/%s (%s, %s)", programName, version, runtime.GOOS, runtime.GOARCH)
	btnUserAgent = programUserAgent + " " + btnProtocol
}
