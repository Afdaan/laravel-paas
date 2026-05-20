package domain

import (
	"net"
	"net/http"
	"sync"
	"time"
)

var (
	httpClient *http.Client
	clientOnce sync.Once
)

func GetHTTPClient() *http.Client {
	clientOnce.Do(func() {
		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   5 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   5 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			Timeout: 10 * time.Second,
		}
	})
	return httpClient
}
