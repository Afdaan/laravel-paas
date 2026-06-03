package domain

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	httpClient         *http.Client
	clientOnce         sync.Once
	localPublicIPs     []string
	localPublicIPsLock sync.RWMutex
	appMode            string = "local"
	appModeLock        sync.RWMutex
	defaultGateway     string = "127.0.0.1"
	defaultGatewayOnce sync.Once
)

// SetLocalPublicIPs registers the server's public IPs.
// When checking custom domains, if the domain resolves to one of these IPs,
// the health checker will route the request internally to prevent hairpin NAT / loopback timeout issues.
func SetLocalPublicIPs(ips []string) {
	localPublicIPsLock.Lock()
	localPublicIPs = ips
	localPublicIPsLock.Unlock()
}

func getLocalPublicIPs() []string {
	localPublicIPsLock.RLock()
	defer localPublicIPsLock.RUnlock()
	return localPublicIPs
}

// SetAppMode registers the app mode ("local" or "docker").
func SetAppMode(mode string) {
	appModeLock.Lock()
	appMode = mode
	appModeLock.Unlock()
}

func getAppMode() string {
	appModeLock.RLock()
	defer appModeLock.RUnlock()
	return appMode
}

// getDefaultGateway returns the cached default route gateway IP.
func getDefaultGateway() string {
	defaultGatewayOnce.Do(func() {
		gw, err := detectDefaultGateway()
		if err == nil && gw != "" {
			defaultGateway = gw
		}
	})
	return defaultGateway
}

// detectDefaultGateway parses /proc/net/route to find the default route gateway (usually the host IP in docker networks).
func detectDefaultGateway() (string, error) {
	file, err := os.Open("/proc/net/route")
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && fields[1] == "00000000" {
			ipHex := fields[2]
			if len(ipHex) != 8 {
				continue
			}
			b, err := hex.DecodeString(ipHex)
			if err != nil {
				return "", err
			}
			ip := net.IPv4(b[3], b[2], b[1], b[0])
			return ip.String(), nil
		}
	}
	return "", fmt.Errorf("default gateway not found")
}

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

					// Check if any resolved IP matches our cached local public IPs.
					// If it does, we rewrite the target IP to route internally.
					targetIP := ips[0].String()
					localIPs := getLocalPublicIPs()
					isLocalPublic := false
					for _, lip := range localIPs {
						if targetIP == lip {
							isLocalPublic = true
							break
						}
					}

					dialIP := targetIP
					// SSRF Protection: Only permit loopback/gateway rewrite for standard web ports (80 and 443).
					// This prevents attackers from mapping a domain to the local public IP and proxying requests 
					// to internal databases/services on unexposed ports (e.g. 6379, 5432).
					if isLocalPublic && (port == "80" || port == "443") {
						if getAppMode() == "local" {
							dialIP = "127.0.0.1"
						} else {
							dialIP = getDefaultGateway()
						}
					}

					// Safely dial with secure resolved target address
					return dialer.DialContext(ctx, network, net.JoinHostPort(dialIP, port))
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
