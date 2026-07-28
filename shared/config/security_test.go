package config

import (
	"testing"
	"time"
)

func TestNormalizeHostname(t *testing.T) {
	valid, err := NormalizeHostname("Apps.Example.NET.")
	if err != nil || valid != "apps.example.net" {
		t.Fatalf("got %q, %v", valid, err)
	}
	for _, value := range []string{"https://apps.example.net", "apps.example.net:443", "*.example.net", "127.0.0.1", "apps.example.net/path"} {
		if _, err := NormalizeHostname(value); err == nil {
			t.Fatalf("accepted invalid hostname %q", value)
		}
	}
}

func TestNormalizeOrigin(t *testing.T) {
	valid, err := NormalizeOrigin("https://Console.Example.NET:8443")
	if err != nil || valid != "https://console.example.net:8443" {
		t.Fatalf("got %q, %v", valid, err)
	}
	for _, value := range []string{"console.example.net", "https://console.example.net/path", "https://console.example.net?x=1"} {
		if _, err := NormalizeOrigin(value); err == nil {
			t.Fatalf("accepted invalid origin %q", value)
		}
	}
}

func TestDistinctRegistrableDomains(t *testing.T) {
	if err := distinctRegistrableDomains("console.example.com", "apps.example.net"); err != nil {
		t.Fatal(err)
	}
	if err := distinctRegistrableDomains("console.example.com", "apps.example.com"); err == nil {
		t.Fatal("accepted shared registrable domain")
	}
}

func TestRotationConfigParsing(t *testing.T) {
	keys, err := parseJWTPreviousKeys("prior|abcdefghijklmnopqrstuvwxyz123456|2030-01-01T00:00:00Z")
	if err != nil || len(keys) != 1 || keys[0].ID != "prior" {
		t.Fatalf("JWT keys = %#v, %v", keys, err)
	}
	if _, err := parseJWTPreviousKeys("bad"); err == nil {
		t.Fatal("accepted malformed JWT previous key")
	}
	csrf, err := parseCSRFPreviousSecrets("prior|abcdefghijklmnopqrstuvwxyz123456|2030-01-01T00:00:00Z")
	if err != nil || len(csrf) != 1 || csrf[0].ID != "prior" {
		t.Fatalf("CSRF secrets = %#v, %v", csrf, err)
	}
	if _, err := parseCIDRList("127.0.0.1/32,172.16.0.0/12"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseCIDRList("not-a-cidr"); err == nil {
		t.Fatal("accepted invalid trusted proxy CIDR")
	}
}

func TestValidInternalAPIToken(t *testing.T) {
	valid := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !ValidInternalAPIToken(valid) {
		t.Fatal("rejected valid token")
	}
	for _, value := range []string{"", valid[:63], valid + "0", "0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde\"", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde\\", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde\n", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde "} {
		if ValidInternalAPIToken(value) {
			t.Fatalf("accepted invalid token %q", value)
		}
	}
}

func TestProductionRequiresExplicitTrustedProxyCIDRs(t *testing.T) {
	cfg := productionSecurityConfig()
	cfg.TrustedProxyCIDRsConfigured = false
	if err := cfg.ValidateProductionSecurity(); err == nil || err.Error() != "TRUSTED_PROXY_CIDRS must be explicitly configured in production" {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func productionSecurityConfig() *Config {
	return &Config{
		AppEnv:                      "production",
		JWTSecret:                   "abcdefghijklmnopqrstuvwxyz123456",
		JWTKeyID:                    "current",
		JWTIssuer:                   "runara",
		JWTAudience:                 "runara-api",
		JWTExpiryHours:              24,
		CSRFSecret:                  "abcdefghijklmnopqrstuvwxyz123456",
		UIDSalt:                     "abcdefghijklmnopqrstuvwxyz123456",
		CredentialEncryptionKey:     "abcdefghijklmnopqrstuvwxyz123456",
		BaseDomain:                  "console.example.com",
		ProjectDomain:               "apps.example.net",
		FrontendURL:                 "https://console.example.com",
		TrustedProxyCIDRs:           []string{"127.0.0.1/32"},
		TrustedProxyCIDRsConfigured: true,
		InternalAPIToken:            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	}
}

func TestProductionAllowsExpiredPreviousJWTKey(t *testing.T) {
	cfg := &Config{
		AppEnv:                      "production",
		JWTSecret:                   "abcdefghijklmnopqrstuvwxyz123456",
		JWTKeyID:                    "current",
		JWTIssuer:                   "runara",
		JWTAudience:                 "runara-api",
		JWTExpiryHours:              24,
		CSRFSecret:                  "abcdefghijklmnopqrstuvwxyz123456",
		UIDSalt:                     "abcdefghijklmnopqrstuvwxyz123456",
		CredentialEncryptionKey:     "abcdefghijklmnopqrstuvwxyz123456",
		BaseDomain:                  "console.example.com",
		ProjectDomain:               "apps.example.net",
		FrontendURL:                 "https://console.example.com",
		TrustedProxyCIDRs:           []string{"127.0.0.1/32"},
		TrustedProxyCIDRsConfigured: true,
		InternalAPIToken:            "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		JWTPreviousKeys: []JWTPreviousKey{{
			ID: "prior", Secret: "abcdefghijklmnopqrstuvwxyz123456", NotAfter: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		}},
	}
	if err := cfg.ValidateProductionSecurity(); err != nil {
		t.Fatalf("expired previous key blocked startup: %v", err)
	}
}
