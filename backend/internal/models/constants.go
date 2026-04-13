package models

const (
	// Default Project Resource Limits (Docker Specific)
	DefaultDockerCPULimit    = "0.5"
	DefaultDockerMemoryLimit = "512m"

	// Docker Infrastructure
	LabelProjectManaged = "com.paas.project=true"
	NetworkName         = "paas-network"
)
