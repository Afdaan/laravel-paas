package infrastructure

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/pkg/metrics"
)

type GithubService struct {
	cfg          *config.Config
	redisService *RedisService
	httpClient   *http.Client
}

func NewGithubService(cfg *config.Config, redisService *RedisService) *GithubService {
	return &GithubService{
		cfg:          cfg,
		redisService: redisService,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (s *GithubService) doRequestWithRetry(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	maxRetries := 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		metrics.GetCollector().IncrGithubApiRequests()

		// Reset request body on retry if it supports it
		if attempt > 1 && req.GetBody != nil {
			if body, bodyErr := req.GetBody(); bodyErr == nil {
				req.Body = body
			}
		}

		resp, err = s.httpClient.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
				remaining := resp.Header.Get("X-RateLimit-Remaining")
				if remaining == "0" || resp.StatusCode == http.StatusTooManyRequests {
					resetHeader := resp.Header.Get("X-RateLimit-Reset")
					retryAfterHeader := resp.Header.Get("Retry-After")

					var waitDuration time.Duration
					if retryAfterHeader != "" {
						if sec, convErr := strconv.Atoi(retryAfterHeader); convErr == nil {
							waitDuration = time.Duration(sec) * time.Second
						}
					}
					if waitDuration <= 0 && resetHeader != "" {
						if resetUnix, convErr := strconv.ParseInt(resetHeader, 10, 64); convErr == nil {
							waitDuration = time.Until(time.Unix(resetUnix, 0))
						}
					}
					if waitDuration <= 0 {
						waitDuration = backoff
					}

					if waitDuration > 10*time.Second {
						waitDuration = 10 * time.Second
					}

					slog.Warn("GitHub API rate limit hit, backing off", "url", req.URL.String(), "wait", waitDuration, "attempt", attempt)
					if attempt < maxRetries {
						resp.Body.Close()
					}
					time.Sleep(waitDuration)
					backoff *= 2
					continue
				}
			}

			if resp.StatusCode >= 500 && resp.StatusCode <= 504 {
				slog.Warn("GitHub API transient server error, retrying", "url", req.URL.String(), "status", resp.StatusCode, "attempt", attempt)
				if attempt < maxRetries {
					resp.Body.Close()
				}
				time.Sleep(backoff)
				backoff *= 2
				continue
			}

			return resp, nil
		}

		slog.Warn("GitHub API network error, retrying", "url", req.URL.String(), "error", err, "attempt", attempt)
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}

	metrics.GetCollector().IncrGithubApiFailures()

	if err != nil {
		return nil, err
	}
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	return nil, fmt.Errorf("request failed after %d attempts, last status=%d", maxRetries, resp.StatusCode)
}

func (s *GithubService) getAppID() (int64, error) {
	appIDStr := strings.TrimSpace(s.cfg.GithubAppID)
	if appIDStr == "" {
		appIDStr = strings.TrimSpace(os.Getenv("GITHUB_APP_ID"))
	}
	if appIDStr == "" {
		return 0, fmt.Errorf("GITHUB_APP_ID environment variable is not set")
	}
	appID, err := strconv.ParseInt(appIDStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid GITHUB_APP_ID: %w", err)
	}
	return appID, nil
}

func (s *GithubService) getPrivateKey() (*rsa.PrivateKey, error) {
	pemPath := strings.TrimSpace(s.cfg.GithubAppPrivateKeyPath)
	if pemPath == "" {
		pemPath = strings.TrimSpace(os.Getenv("GITHUB_APP_PRIVATE_KEY_PATH"))
	}
	if pemPath == "" {
		pemPath = "keys/github-app.pem"
	}

	var candidatePaths []string
	if filepath.IsAbs(pemPath) {
		candidatePaths = append(candidatePaths, pemPath)
	} else {
		cleanPath := filepath.Clean(pemPath)
		if cleanPath == "." || strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator)) || cleanPath == ".." {
			return nil, fmt.Errorf("invalid GITHUB_APP_PRIVATE_KEY_PATH: relative path must stay within DATA_PATH")
		}
		if strings.HasPrefix(cleanPath, "storage/data"+string(os.PathSeparator)) {
			cleanPath = strings.TrimPrefix(cleanPath, "storage/data"+string(os.PathSeparator))
		}
		candidatePaths = append(candidatePaths,
			filepath.Join(s.cfg.DataPath, cleanPath),
			filepath.Join("/app/data", cleanPath),
			filepath.Join("/app/storage/data", cleanPath),
		)
	}

	var pemBytes []byte
	var lastErr error
	for _, candidatePath := range candidatePaths {
		if candidatePath == "" {
			continue
		}
		bytes, err := os.ReadFile(candidatePath)
		if err == nil {
			pemBytes = bytes
			break
		}
		lastErr = err
	}
	if len(pemBytes) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("failed to read private key PEM file: %w", lastErr)
		}
		return nil, fmt.Errorf("failed to read private key PEM file")
	}

	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("failed to parse PEM block from private key")
	}

	privKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Try PKCS8 format if PKCS1 fails
		key, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			return nil, fmt.Errorf("failed to parse private key: pkcs1=%v, pkcs8=%v", err, err8)
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("not an RSA private key")
		}
		return rsaKey, nil
	}

	return privKey, nil
}

func (s *GithubService) GenerateAppJWT() (string, error) {
	appID, err := s.getAppID()
	if err != nil {
		return "", err
	}

	privKey, err := s.getPrivateKey()
	if err != nil {
		return "", err
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iat": now.Add(-60 * time.Second).Unix(), // account for clock drift
		"exp": now.Add(10 * time.Minute).Unix(),  // max 10 mins
		"iss": strconv.FormatInt(appID, 10),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tokenString, err := token.SignedString(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT token: %w", err)
	}

	return tokenString, nil
}

type InstallationTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func (s *GithubService) GetInstallationToken(installationID int64) (string, error) {
	cacheKey := fmt.Sprintf("github:token:%d", installationID)

	// Try to get cached token from Redis
	if s.redisService != nil {
		if cachedToken, err := s.redisService.GetString(cacheKey); err == nil && cachedToken != "" {
			return cachedToken, nil
		}
	}

	// Generate App JWT
	appJWT, err := s.GenerateAppJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installationID)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+appJWT)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.doRequestWithRetry(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request to exchange installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		// Evict any stale cached token so the next call forces a fresh exchange
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusUnauthorized {
			if s.redisService != nil {
				_ = s.redisService.DeleteCache(cacheKey)
			}
		}
		return "", fmt.Errorf("failed to exchange installation token, status=%d, response=%s", resp.StatusCode, string(bodyBytes))
	}

	var tokenResp InstallationTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode installation token response: %w", err)
	}

	expiresTime, err := time.Parse(time.RFC3339, tokenResp.ExpiresAt)
	if err != nil {
		expiresTime = time.Now().Add(55 * time.Minute) // fallback
	}

	// Cache in Redis leaving a 5-minute buffer
	ttl := time.Until(expiresTime) - 5*time.Minute
	if ttl > 0 && s.redisService != nil {
		_ = s.redisService.SetCache(cacheKey, tokenResp.Token, ttl)
	}

	return tokenResp.Token, nil
}

// InvalidateInstallationToken evicts the cached token for an installation.
// Used by handlers to bust a stale cache before retrying API calls.
func (s *GithubService) InvalidateInstallationToken(installationID int64) {
	cacheKey := fmt.Sprintf("github:token:%d", installationID)
	if s.redisService != nil {
		_ = s.redisService.DeleteCache(cacheKey)
	}
}

type GithubRepository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Description   string `json:"description"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
}

type RepositoriesListResponse struct {
	TotalCount   int                `json:"total_count"`
	Repositories []GithubRepository `json:"repositories"`
}

func (s *GithubService) ListRepositories(installationID int64) ([]GithubRepository, error) {
	token, err := s.GetInstallationToken(installationID)
	if err != nil {
		return nil, err
	}

	url := "https://api.github.com/installation/repositories?per_page=100"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to list repositories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list repositories, status=%d, response=%s", resp.StatusCode, string(bodyBytes))
	}

	var listResp RepositoriesListResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return nil, fmt.Errorf("failed to decode repositories list response: %w", err)
	}

	return listResp.Repositories, nil
}

type GithubBranch struct {
	Name string `json:"name"`
}

func (s *GithubService) ListBranches(installationID int64, owner, repo string) ([]GithubBranch, error) {
	token, err := s.GetInstallationToken(installationID)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?per_page=100", owner, repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.doRequestWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to list branches: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to list branches, status=%d, response=%s", resp.StatusCode, string(bodyBytes))
	}

	var branches []GithubBranch
	if err := json.NewDecoder(resp.Body).Decode(&branches); err != nil {
		return nil, fmt.Errorf("failed to decode branches response: %w", err)
	}

	return branches, nil
}

type CommitStatusRequest struct {
	State       string `json:"state"`
	TargetURL   string `json:"target_url,omitempty"`
	Description string `json:"description"`
	Context     string `json:"context"`
}

func (s *GithubService) UpdateCommitStatus(installationID int64, owner, repo, sha, state, targetURL, description string) error {
	err := s.updateCommitStatusRaw(installationID, owner, repo, sha, state, targetURL, description)
	if err != nil && (strings.Contains(err.Error(), "status=401") || strings.Contains(err.Error(), "status=404")) {
		slog.Warn("GitHub API auth error updating commit status, retrying with fresh token", "installation_id", installationID, "error", err)
		s.InvalidateInstallationToken(installationID)
		err = s.updateCommitStatusRaw(installationID, owner, repo, sha, state, targetURL, description)
	}
	return err
}

func (s *GithubService) updateCommitStatusRaw(installationID int64, owner, repo, sha, state, targetURL, description string) error {
	token, err := s.GetInstallationToken(installationID)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/statuses/%s", owner, repo, sha)
	body := CommitStatusRequest{
		State:       state,
		TargetURL:   targetURL,
		Description: description,
		Context:     "paas/deployment",
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(bodyBytes)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.doRequestWithRetry(req)
	if err != nil {
		return fmt.Errorf("failed to send request to update commit status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to update commit status, status=%d, response=%s", resp.StatusCode, string(respBytes))
	}

	slog.Info("Successfully updated GitHub commit status", "owner", owner, "repo", repo, "sha", sha, "state", state)
	return nil
}

type GithubInstallationInfo struct {
	Account struct {
		Login     string `json:"login"`
		AvatarURL string `json:"avatar_url"`
	} `json:"account"`
}

func (s *GithubService) GetInstallationDetails(installationID int64) (*GithubInstallationInfo, error) {
	jwtToken, err := s.GenerateAppJWT()
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.github.com/app/installations/%d", installationID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := s.doRequestWithRetry(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to fetch installation, status=%d, response=%s", resp.StatusCode, string(bodyBytes))
	}

	var info GithubInstallationInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}
