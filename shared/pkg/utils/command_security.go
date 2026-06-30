package utils

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// BaseCommand returns the base executable name (first token) from a command string.
func BaseCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "<empty>"
	}

	var first strings.Builder
	inQuote := false
	var quoteChar rune

	for _, r := range trimmed {
		if inQuote {
			if r == quoteChar {
				inQuote = false
			}
			first.WriteRune(r)
			continue
		}
		if r == '\'' || r == '"' {
			inQuote = true
			quoteChar = r
			first.WriteRune(r)
			continue
		}
		if unicode.IsSpace(r) {
			break
		}
		first.WriteRune(r)
	}

	result := first.String()
	if len(result) > 64 {
		result = result[:64]
	}
	return strings.ToLower(result)
}

var blockedCommandNames = []string{
	// Container/runtime control. Docker socket or privileged runtime access can become host control.
	"docker",
	"podman",
	"kubectl",
	"helm",
	"crictl",
	"ctr",
	"nerdctl",
	"runc",

	// Namespace, kernel, mount, and network mutation.
	"nsenter",
	"unshare",
	"mount",
	"umount",
	"swapon",
	"swapoff",
	"modprobe",
	"insmod",
	"rmmod",
	"sysctl",
	"iptables",
	"ip6tables",
	"nft",
	"tc",
	"ip",
	"route",
	"ifconfig",

	// Init/service control.
	"systemctl",
	"service",
	"journalctl",
	"reboot",
	"shutdown",
	"halt",
	"poweroff",
	"init",

	// Privilege/account changes.
	"sudo",
	"su",
	"passwd",
	"useradd",
	"usermod",
	"groupadd",
	"groupmod",
}

var blockedCommandNameRegex = regexp.MustCompile(
	`(?:^|[\s'` + "`" + `";|&()])` +
		`(?:` + strings.Join(blockedCommandNames, "|") + `)` +
		`(?:[\s'` + "`" + `";|&()]|$)`,
)

var blockedCommandFragments = []string{
	// Resource exhaustion.
	":(){:|:&};:",

	// Cloud metadata endpoints.
	"169.254.169.254",
	"fd00:ec2::254",
	"fd20:ce::254",
	"100.100.100.200",
	"metadata.google.internal",

	// Runtime sockets, host namespaces, Kubernetes tokens, and kernel escape surfaces.
	"/var/run/docker.sock",
	"/run/docker.sock",
	"/proc/1/ns",
	"/var/run/secrets/kubernetes.io/serviceaccount",
	"/sys/fs/cgroup",
	"release_agent",
	"/proc/sys/kernel/core_pattern",
	"/dev/mem",
	"/dev/kmsg",

	// Destructive root-level filesystem/device operations (multi-word, substring matched).
	"rm -rf /",
	"rm -fr /",
	"rm -rf /*",
	"rm -fr /*",
	"chmod -R 777 /",
	"mkfs",
	"dd if=",
}

// ValidateCommand blocks commands that can damage the PaaS runtime or escape the project container.
// Application-level commands are allowed; frontend confirmation and audit logs handle risky app actions.
func ValidateCommand(command string) error {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return fmt.Errorf("command cannot be empty")
	}

	lower := strings.ToLower(trimmed)
	for _, fragment := range blockedCommandFragments {
		if strings.Contains(lower, fragment) {
			return fmt.Errorf("command contains restricted pattern '%s' for platform security reasons", fragment)
		}
	}

	if blockedCommandNameRegex.MatchString(lower) {
		return fmt.Errorf("command contains a restricted platform binary operand")
	}

	return nil
}
