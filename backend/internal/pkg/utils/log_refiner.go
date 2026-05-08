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
	// buffer for partial lines
	buf string
}

// NewLogRefiner creates a new LogRefiner.
func NewLogRefiner(w io.Writer) *LogRefiner {
	return &LogRefiner{
		writer: w,
		hidePatterns: []*regexp.Regexp{
			// 1. Hide ALL Docker/BuildKit step headers and status lines (lines starting with #num but no timestamp)
			// Example: "#10 [composer 6/14] RUN...", "#1 DONE", "#13 copy...", "#10"
			// This preserves output lines like "#20 1.168 Installing..." because they have a digit (timestamp) after the number.
			regexp.MustCompile(`^#\d+(?:\s+[^0-9]|$)`), 
			
			// 2. Hide specific noise from the output lines (lines that DO have timestamps)
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s+(checking for |checking whether |checking |creating |compiling |/bin/sh | cc |mkdir \.?libs|LD_LIBRARY_PATH|Libraries have been installed|Build complete|Don't forget to run|find \. -name|rm -rf|Purging |OK: |Executing busybox|Get:|Fetched|Reading|Building|Selecting|Preparing|Unpacking|Setting up|Processing|Configuring for:|PHP Api Version:|Zend Module Api No:|Zend Extension Api No:|Appending configuration tag|config\.status:creating|\( *[0-9]+/[0-9]+\) (Upgrading|Installing|Purging))`),
			
			// 3. Hide Composer & NPM specific noise from output lines
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s+(\s+-\s(Downloading|Installing|Extracting archive)|[0-9]+/[0-9]+\s\[|48 packages you are using|Use the .*composer fund|npm notice|run .*npm fund|packages are looking for funding|moderate severity vulnerabilities|To address all issues|npm warn config production)`),
			
			// 4. General Infrastructure & Hash Noise
			regexp.MustCompile(`sha256:[a-f0-9]{64}`), 
			regexp.MustCompile(`(resolving|extracting|transferring context|loading secrets|docker-image://|\[2mmise\[0m)`),
			
			// 5. Hide lines that are just a Docker timestamp and nothing else (empty lines from build)
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s*$`),
		},
		stripPattern: regexp.MustCompile(`^#\d+\s+[0-9.]+\s*`),
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
