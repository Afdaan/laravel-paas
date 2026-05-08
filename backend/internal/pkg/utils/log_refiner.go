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
	// Pattern to strip ANSI escape sequences
	ansiPattern *regexp.Regexp
	// buffer for partial lines
	buf string
}

// NewLogRefiner creates a new LogRefiner.
func NewLogRefiner(w io.Writer) *LogRefiner {
	return &LogRefiner{
		writer: w,
		hidePatterns: []*regexp.Regexp{
			// 1. Hide ALL Docker/BuildKit step headers and status lines (lines starting with #num but no timestamp)
			regexp.MustCompile(`^#\d+(?:\s+[^0-9]|$)`), 
			
			// 2. Hide specific noise from the output lines (lines that DO have timestamps)
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s+(checking for |checking whether |checking |creating |compiling |/bin/sh | cc |mkdir \.?libs|LD_LIBRARY_PATH|Libraries have been installed|Build complete|Don't forget to run|find \. -name|rm -rf|Purging |OK: |Executing busybox|Get:|Fetched|Reading|Building|Selecting|Preparing|Unpacking|Setting up|Processing|Configuring for:|PHP Api Version:|Zend Module Api No:|Zend Extension Api No:|Appending configuration tag|config\.status:creating|\( *[0-9]+/[0-9]+\) (Upgrading|Installing|Purging))`),
			
			// 3. Hide Composer & NPM specific noise from output lines (Audits, Funding, Vulnerabilities)
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s*(\s+-\s(Downloading|Installing|Extracting archive)|[0-9]+/[0-9]+\s\[|48 packages you are using|Use the .*composer fund|npm notice|run .*npm fund|packages are looking for funding|moderate severity vulnerabilities|To address all issues|npm warn config production|npm audit|Run .*npm audit|audited .* packages|found .* vulnerabilities)`),
			
			// 4. Hide Laravel specific progress dots (the long lines of dots in package discovery)
			regexp.MustCompile(`[90m\.`), 
			
			// 5. Hide Railpack branding and internal metadata headers
			regexp.MustCompile(`(INFO No package manager|╭─|│ Railpack|╰─|↳ Using config|⚠ The config|↳ Detected|↳ Using|↳ Deploying|↳ Output directory|  Packages|  ──────────|node\s+│|caddy\s+│|  Steps|  ▸ |  Deploy|\$ caddy run)`),
			
			// 6. Hide final Docker metadata and build times
			regexp.MustCompile(`(Loaded image:|Successfully built image in|Run with \x60docker run|built in [0-9.]+(s|ms))`),
			
			// 7. General Infrastructure & Hash Noise
			regexp.MustCompile(`sha256:[a-f0-9]{64}`), 
			regexp.MustCompile(`(resolving|extracting|transferring context|loading secrets|docker-image://|\[2mmise\[0m)`),
			
			// 8. Hide lines that are just a Docker timestamp and nothing else (empty lines from build)
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s*$`),
		},
		stripPattern: regexp.MustCompile(`^#\d+\s+[0-9.]+\s*`),
		ansiPattern:  regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`),
	}
}

func (r *LogRefiner) Write(p []byte) (n int, err error) {
	// Add current write to buffer
	r.buf += string(p)
	
	// Split into lines
	lines := strings.Split(r.buf, "\n")
	
	// Keep the last partial line in the buffer
	r.buf = lines[len(lines)-1]
	
	// Process all complete lines
	var filteredLines []string
	hasWritten := false

	for i := 0; i < len(lines)-1; i++ {
		line := lines[i]
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
			
			// Strip ANSI escape sequences (e.g., color codes like [37;44m) 
			// which the user might perceive as "time" noise.
			cleanedLine = r.ansiPattern.ReplaceAllString(cleanedLine, "")
			
			// Aggressively skip any line that is empty or just whitespace
			// This fixes the "ngebug ada yang kosong" issue by ensuring no blank lines enter the log
			if strings.TrimSpace(cleanedLine) == "" {
				continue
			}
			
			filteredLines = append(filteredLines, cleanedLine)
			hasWritten = true
		}
	}

	if hasWritten {
		output := strings.Join(filteredLines, "\n") + "\n"
		_, err = r.writer.Write([]byte(output))
		if err != nil {
			return 0, err
		}
	}

	return len(p), nil
}
