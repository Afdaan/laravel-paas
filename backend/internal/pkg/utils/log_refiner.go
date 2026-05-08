package utils

import (
	"io"
	"regexp"
	"strings"
)

// LogRefiner is an io.Writer that filters out noise from build logs.
type LogRefiner struct {
	writer io.Writer
	// Patterns to completely hide
	hidePatterns []*regexp.Regexp
}

// NewLogRefiner creates a new LogRefiner.
func NewLogRefiner(w io.Writer) *LogRefiner {
	return &LogRefiner{
		writer: w,
		hidePatterns: []*regexp.Regexp{
			// Docker BuildKit internal/infra steps (headers and generic setup)
			regexp.MustCompile(`^#\d+ (\[internal\]|FROM|WORKDIR|COPY|RUN mkdir|RUN cat|RUN if \[ -f|RUN \{ echo|CACHED|transferring context|transferring dockerfile|resolve|extracting|sending tarball|unpacking|naming to|\[railpack\]|loading|merge|create mise|install mise|install apt|mkfile|caddy fmt)`),
			
			// Compilation & Package Manager Noise (Alpine/PHP/Apt/Caddy)
			regexp.MustCompile(`^#\d+ [0-9.]+ (checking for |checking whether |checking |creating |compiling |/bin/sh | cc |mkdir \.?libs|LD_LIBRARY_PATH|Libraries have been installed|Build complete|Don't forget to run|find \. -name|rm -rf|Purging |OK: |Executing busybox|Get:|Fetched|Reading|Building|Selecting|Preparing|Unpacking|Setting up|Processing|Configuring for:|PHP Api Version:|Zend Module Api No:|Zend Extension Api No:|Appending configuration tag|config\.status:creating|\( *[0-9]+/[0-9]+\) (Upgrading|Installing|Purging))`),
			
			// Composer & NPM Progress/Auditing Noise
			regexp.MustCompile(`^#\d+ [0-9.]+ (\s+-\s(Downloading|Installing|Extracting archive)|[0-9]+/[0-9]+\s\[|48 packages you are using|Use the .*composer fund|npm notice|run .*npm fund|packages are looking for funding|moderate severity vulnerabilities|To address all issues|npm warn config production)`),
			
			// General Infrastructure & Hash Noise
			regexp.MustCompile(`sha256:[a-f0-9]{64}`), 
			regexp.MustCompile(`(resolving|extracting|transferring context|loading secrets|docker-image://|\[2mmise\[0m)`),
		},
	}
}

func (r *LogRefiner) Write(p []byte) (n int, err error) {
	// Note: This is a simple implementation that assumes p contains full lines.
	// For production-grade streaming, we would use a line-buffer.
	lines := strings.Split(string(p), "\n")
	var filteredLines []string

	for i, line := range lines {
		// Skip the last element if it's empty (trailing newline)
		if i == len(lines)-1 && line == "" {
			continue
		}

		shouldHide := false
		for _, pattern := range r.hidePatterns {
			if pattern.MatchString(line) {
				shouldHide = true
				break
			}
		}

		if !shouldHide {
			filteredLines = append(filteredLines, line)
		}
	}

	if len(filteredLines) > 0 {
		output := strings.Join(filteredLines, "\n")
		if strings.HasSuffix(string(p), "\n") {
			output += "\n"
		}
		_, err = r.writer.Write([]byte(output))
		if err != nil {
			return 0, err
		}
	}

	return len(p), nil
}
