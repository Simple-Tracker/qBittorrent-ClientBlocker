//go:build darwin || windows || linux

package proxy

import (
	"net/http"
	"net/url"

	"github.com/bdwyertech/go-get-proxied/proxy"
)

func (p *Resolver) GetProxy(r *http.Request) (*url.URL, error) {
	if r == nil && p.httpProxyURL == nil && p.httpsProxyURL == nil {
		proxyProvider := proxy.NewProvider("")

		httpProxy := proxyProvider.GetHTTPProxy("")
		if httpProxy != nil {
			p.httpProxyURL = httpProxy.URL()
			if p.httpProxyURL.Scheme == "" {
				p.httpProxyURL.Scheme = "http"
			}

			p.log("GetProxy", p.text("GetProxy_ProxyFound"), true, "HTTP", p.httpProxyURL.String(), httpProxy.Src())
		}

		httpsProxy := proxyProvider.GetHTTPSProxy("")
		if httpsProxy != nil {
			p.httpsProxyURL = httpsProxy.URL()
			if p.httpsProxyURL.Scheme == "" {
				p.httpsProxyURL.Scheme = "http"
			}

			p.log("GetProxy", p.text("GetProxy_ProxyFound"), true, "HTTPS", p.httpsProxyURL.String(), httpsProxy.Src())
		}

		if p.httpProxyURL == nil || p.httpsProxyURL == nil {
			p.log("GetProxy", p.text("GetProxy_ProxyNotFound"), true, "HTTP/HTTPS")
		}
	} else if r != nil {
		if r.URL.Scheme == "https" {
			return p.httpsProxyURL, nil
		} else if r.URL.Scheme == "http" {
			return p.httpProxyURL, nil
		}
	}

	return nil, nil
}
