package domain

import (
	"context"
	"fmt"
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
		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}

		httpClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					host, port, err := net.SplitHostPort(address)
					if err != nil {
						return nil, err
					}

					ips, err := net.LookupIP(host)
					if err != nil {
						return nil, err
					}

					for _, ip := range ips {
						// Reject connection if target IP resolves to loopback, private, link-local unicast, or multicast addresses to prevent SSRF
						if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsMulticast() {
							return nil, fmt.Errorf("SSRF prevention: connection to private/reserved IP address %s is prohibited", ip.String())
						}
					}

					// Safely dial with secure resolved target address
					return dialer.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
				},
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
