package models

import (
	"testing"
)

func TestGetTargetHostname(t *testing.T) {
	tests := []struct {
		name        string
		subdomain   string
		containerID *string
		expected    string
	}{
		{
			name:        "nil container ID returns network alias",
			subdomain:   "my-app",
			containerID: nil,
			expected:    "project-my-app",
		},
		{
			name:        "empty container ID returns network alias",
			subdomain:   "my-app",
			containerID: strPtr(""),
			expected:    "project-my-app",
		},
		{
			name:        "64-char hex container ID returns 12-char short ID",
			subdomain:   "my-app",
			containerID: strPtr("d4e5f67890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"),
			expected:    "d4e5f67890ab",
		},
		{
			name:        "container name (paas-project-subdomain-timestamp) returns full container name",
			subdomain:   "my-app",
			containerID: strPtr("paas-project-my-app-1723456789"),
			expected:    "paas-project-my-app-1723456789",
		},
		{
			name:        "short project alias (project-my-app) returns full string",
			subdomain:   "my-app",
			containerID: strPtr("project-my-app"),
			expected:    "project-my-app",
		},
		{
			name:        "12-char hex container ID returns as-is",
			subdomain:   "my-app",
			containerID: strPtr("d4e5f67890ab"),
			expected:    "d4e5f67890ab",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Project{
				Subdomain:   tt.subdomain,
				ContainerID: tt.containerID,
			}
			got := p.GetTargetHostname()
			if got != tt.expected {
				t.Errorf("GetTargetHostname() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
