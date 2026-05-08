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
	// Pattern to strip from the beginning of kept lines (e.g. Docker timestamps)
	stripPattern *regexp.Regexp
}

// NewLogRefiner creates a new LogRefiner.
func NewLogRefiner(w io.Writer) *LogRefiner {
	return &LogRefiner{
		writer: w,
		hidePatterns: []*regexp.Regexp{
			// 1. Hide ALL Docker/BuildKit step headers and status lines (lines starting with #num but no timestamp)
			// Example: "#10 [composer 6/14] RUN...", "#1 DONE", "#13 copy...", "#10"
			// This preserves output lines like "#20 1.168 Installing..." because they have a digit (timestamp) after the number.
			regexp.MustCompile(`^#\d+(?:\s[^0-9]|$)`), 
			
			// 2. Hide specific noise from the output lines (lines that DO have timestamps)
			regexp.MustCompile(`^#\d+ [0-9.]+ (checking for |checking whether |checking |creating |compiling |/bin/sh | cc |mkdir \.?libs|LD_LIBRARY_PATH|Libraries have been installed|Build complete|Don't forget to run|find \. -name|rm -rf|Purging |OK: |Executing busybox|Get:|Fetched|Reading|Building|Selecting|Preparing|Unpacking|Setting up|Processing|Configuring for:|PHP Api Version:|Zend Module Api No:|Zend Extension Api No:|Appending configuration tag|config\.status:creating|\( *[0-9]+/[0-9]+\) (Upgrading|Installing|Purging))`),
			
			// 3. Hide Composer & NPM specific noise from output lines
			regexp.MustCompile(`^#\d+ [0-9.]+ (\s+-\s(Downloading|Installing|Extracting archive)|[0-9]+/[0-9]+\s\[|48 packages you are using|Use the .*composer fund|npm notice|run .*npm fund|packages are looking for funding|moderate severity vulnerabilities|To address all issues|npm warn config production)`),
			
			// 4. General Infrastructure & Hash Noise
			regexp.MustCompile(`sha256:[a-f0-9]{64}`), 
			regexp.MustCompile(`(resolving|extracting|transferring context|loading secrets|docker-image://|\[2mmise\[0m)`),
			
			// 5. Hide lines that are just a Docker timestamp and nothing else (empty lines from build)
			regexp.MustCompile(`^#\d+ [0-9.]+ \s*$`),
		},
		stripPattern: regexp.MustCompile(`^#\d+ [0-9.]+ `),
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
			// Strip the prefix (e.g., "#20 1.234 ") to make the output clean
			cleanedLine := r.stripPattern.ReplaceAllString(line, "")
			
			// Only add if it's not empty after cleaning (unless it was already an empty line without a prefix)
			if cleanedLine != "" || line == "" {
				filteredLines = append(filteredLines, cleanedLine)
			}
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
