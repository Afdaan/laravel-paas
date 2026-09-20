package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// NormalizeHostname validates a deployment hostname, not a URL.
func NormalizeHostname(value string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
	if host == "" || net.ParseIP(host) != nil || strings.ContainsAny(host, "/?#:@") || strings.Contains(host, "*") {
		return "", fmt.Errorf("invalid hostname")
	}
	if len(host) > 253 {
		return "", fmt.Errorf("invalid hostname")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", fmt.Errorf("invalid hostname")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", fmt.Errorf("invalid hostname")
			}
		}
	}
	return host, nil
}

// NormalizeOrigin validates a browser origin, not a general URL.
func NormalizeOrigin(value string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("invalid origin")
	}
	host, err := NormalizeHostname(u.Hostname())
	if err != nil {
		return "", err
	}
	port := u.Port()
	if port != "" {
		if _, _, err := net.SplitHostPort(u.Host); err != nil {
			return "", fmt.Errorf("invalid origin")
		}
		host += ":" + port
	}
	return u.Scheme + "://" + host, nil
}

func ValidInternalAPIToken(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func ValidJWTKeyID(value string) bool {
	if len(value) == 0 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func distinctRegistrableDomains(baseDomain, projectDomain string) error {
	base, err := publicsuffix.EffectiveTLDPlusOne(baseDomain)
	if err != nil {
		return fmt.Errorf("invalid BASE_DOMAIN")
	}
	project, err := publicsuffix.EffectiveTLDPlusOne(projectDomain)
	if err != nil {
		return fmt.Errorf("invalid PROJECT_DOMAIN")
	}
	if base == project {
		return fmt.Errorf("BASE_DOMAIN and PROJECT_DOMAIN must use separate registrable domains")
	}
	return nil
}
