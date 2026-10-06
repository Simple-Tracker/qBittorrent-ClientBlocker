//go:build !darwin && !windows && !linux

package proxy

import (
	"net/http"
	"net/url"
)

func (p *Resolver) GetProxy(r *http.Request) (*url.URL, error) {
	if r == nil {
		if !p.notified {
			p.notified = true
			p.log("GetProxy", p.text("GetProxy_UseEnvVar"), true)
		}
		return nil, nil
	}

	return http.ProxyFromEnvironment(r)
}
