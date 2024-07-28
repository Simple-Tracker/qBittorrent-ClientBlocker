//go:build darwin || windows || linux

package main

import (
	"github.com/bdwyertech/go-get-proxied/proxy"
	"net/http"
	"net/url"
)

var getproxy_httpProxyURL *url.URL
var getproxy_httpsProxyURL *url.URL

func GetProxy(r *http.Request) (*url.URL, error) {
	if r == nil && getproxy_httpProxyURL == nil && getproxy_httpsProxyURL == nil {
		proxyProvider := proxy.NewProvider("")

		httpProxy := proxyProvider.GetHTTPProxy("")
		if httpProxy != nil {
			getproxy_httpProxyURL = httpProxy.URL()
			if getproxy_httpProxyURL.Scheme == "" {
				getproxy_httpProxyURL.Scheme = "http"
			}

			Log("GetProxy", "发现 HTTP 代理: %s (来源: %s)", true, getproxy_httpProxyURL.String(), httpProxy.Src())
		}

		httpsProxy := proxyProvider.GetHTTPSProxy("")
		if httpsProxy != nil {
			getproxy_httpsProxyURL = httpsProxy.URL()
			if getproxy_httpsProxyURL.Scheme == "" {
				getproxy_httpsProxyURL.Scheme = "http"
			}

			Log("GetProxy", "发现 HTTPS 代理: %s (来源: %s)", true, getproxy_httpsProxyURL.String(), httpsProxy.Src())
		}

		if getproxy_httpProxyURL == nil || getproxy_httpsProxyURL == nil {
			Log("GetProxy", "未能发现 HTTP/HTTPS 代理", true)
		}
	} else if r != nil {
		if r.URL.Scheme == "https" {
			return getproxy_httpsProxyURL, nil
		} else if r.URL.Scheme == "http" {
			return getproxy_httpProxyURL, nil
		}
	}

	return nil, nil
}
