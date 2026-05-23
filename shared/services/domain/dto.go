package domain

import (
	"encoding/json"
	"time"
)

type DomainStatusDTO struct {
	ID              uint      `json:"id"`
	ProjectID       uint      `json:"project_id"`
	Domain          string    `json:"domain"`
	Status          string    `json:"status"`
	HealthStatus    string    `json:"health_status"`
	LatencyMs       int64     `json:"latency_ms"`
	CurrentSequence int       `json:"current_sequence"`
	SnapshotVersion int       `json:"snapshot_version"`
	ErrorCode       string    `json:"error_code,omitempty"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type DomainEventDTO struct {
	EventVersion int             `json:"event_version"`
	EventType    string          `json:"event_type"`
	DomainID     uint            `json:"domain_id"`
	ProjectID    uint            `json:"project_id"`
	Sequence     int             `json:"sequence"`
	Payload      json.RawMessage `json:"payload"`
	Error        string          `json:"error,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
}
