//go:build (!darwin && !windows && !linux)
package main

import (
	"net/url"
	"net/http"
)

func GetProxy(r *http.Request) (*url.URL, error) {
	if r == nil {
		Log("GetProxy", "当前平台通过环境变量设置代理, 若要使用, 请确保已正确设置环境变量", true)
		return nil, nil
	}

	return http.ProxyFromEnvironment(r)
}
