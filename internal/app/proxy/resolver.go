package proxy

import "net/url"

// Resolver 缓存平台代理结果, 在启动时发现代理, 供 HTTP Transport 使用.
type Resolver struct {
	httpProxyURL, httpsProxyURL *url.URL
	notified                    bool
	log                         func(string, string, bool, ...any)
	text                        func(string) string
}

func NewResolver(log func(string, string, bool, ...any), text func(string) string) *Resolver {
	return &Resolver{log: log, text: text}
}
