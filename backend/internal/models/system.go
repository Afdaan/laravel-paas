package models

import "time"

// SystemStats represents the host machine's resource usage
type SystemStats struct {
	Hostname    string  `json:"hostname"`
	OS          string  `json:"os"`
	CPUUsage    float64 `json:"cpu_usage"`
	CPUCores    int     `json:"cpu_cores"`
	MemoryUsed  uint64  `json:"memory_used"`
	MemoryTotal uint64  `json:"memory_total"`
	DiskUsed    uint64  `json:"disk_used"`
	DiskTotal   uint64  `json:"disk_total"`
	DiskPath    string  `json:"disk_path"`
}

// DockerContainer represents a container running on the host
type DockerContainer struct {
	ID            string    `json:"id"`
	Names         []string  `json:"names"`
	Image         string    `json:"image"`
	State         string    `json:"state"`
	Status        string    `json:"status"`
	Ports         []string  `json:"ports"`
	IPAddress     string    `json:"ip_address"`
	CPUPercent    float64   `json:"cpu_percent"`
	MemoryUsage   float64   `json:"memory_usage"`
	CreatedAt     time.Time `json:"created_at"`
}

// DockerImage represents an image stored on the host
type DockerImage struct {
	ID          string    `json:"id"`
	RepoTags    []string  `json:"repo_tags"`
	Size        int64     `json:"size"`
	Created     time.Time `json:"created"`
	Status      string    `json:"status"` // "In Use" or "Unused"
	Repository  string    `json:"repository"`
	Tag         string    `json:"tag"`
	SizeHuman   string    `json:"size_human"`
}

// DockerVolume represents a docker volume
type DockerVolume struct {
	Name       string    `json:"name"`
	Driver     string    `json:"driver"`
	Mountpoint string    `json:"mountpoint"`
	Status     string    `json:"status"` // "In Use" or "Unused"
	Size       string    `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
}

// DockerNetwork represents a docker network
type DockerNetwork struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Driver string `json:"driver"`
	Scope  string `json:"scope"`
	Status string `json:"status"` // "In Use" or "Unused"
}
