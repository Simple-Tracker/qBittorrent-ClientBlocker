//go:build !darwin && !windows && !linux

package main

import (
	"net/http"
	"net/url"
)

var getproxy_notified = false

func GetProxy(r *http.Request) (*url.URL, error) {
	if r == nil {
		if !getproxy_notified {
			getproxy_notified = true
			Log("GetProxy", "当前平台通过环境变量设置代理, 若要使用, 请确保已正确设置环境变量", true)
		}
		return nil, nil
	}

	return http.ProxyFromEnvironment(r)
}
