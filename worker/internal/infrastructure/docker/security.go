package docker

// TenantHardeningArgs returns Docker runtime hardening flags for tenant app containers.
// Internal worker-manager containers (which mount the Docker socket) must NOT use these.
func TenantHardeningArgs(memoryLimit string) []string {
	return []string{
		"--memory-swap", memoryLimit,
		"--security-opt=no-new-privileges:true",
		"--pids-limit=250",
	}
}