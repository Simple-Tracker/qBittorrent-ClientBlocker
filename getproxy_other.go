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
			Log("GetProxy", GetLangText("GetProxy_UseEnvVar"), true)
		}
		return nil, nil
	}

	return http.ProxyFromEnvironment(r)
}
