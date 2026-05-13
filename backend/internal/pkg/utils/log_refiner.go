package utils

import (
	"io"
	"regexp"
	"strings"
	"sync"
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
	// started indicates if the build header has been written
	started bool
	// lastStep tracks the last status message to avoid duplicates
	lastStep string
	// mutex for thread safety (Stdout and Stderr use the same refiner)
	mu sync.Mutex
}

type logTransformation struct {
	pattern     *regexp.Regexp
	replacement string
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
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s*([0-9]+/[0-9]+\s\[|\d+ packages you are using|Use the .*composer fund|npm notice|run .*npm fund|packages are looking for funding|[a-zA-Z]+ severity vulnerabilities|To address all issues|npm warn config production|npm audit|Run .*npm audit|audited .* packages|found .* vulnerabilities)`),

			// 4. Hide Laravel specific progress dots (the long lines of dots in package discovery)
			regexp.MustCompile(`\[90m\.`),

			// 5. Hide Railpack branding, metadata headers, and ALL step commands
			regexp.MustCompile(`(INFO No package manager|╭─|│ Railpack|╰─|⚠ The config|↳ Output directory|  Packages|  ──────────)`),
			// Hide Railpack package table rows (node, caddy, bun, python, go, etc.)
			regexp.MustCompile(`^\s*(node|caddy|bun|python|go|ruby|java|php|deno|mise)\s+│`),
			// Hide ALL Railpack step commands ($ bun install, $ npm ci, $ caddy run, etc.)
			regexp.MustCompile(`^\s+\$\s+`),
			// Hide build steps 
			regexp.MustCompile(`^\s*(↳|▸|Steps)`),

			// 6. Hide final Docker metadata (Except the build time which we will transform)
			regexp.MustCompile(`(Loaded image:|Run with \x60docker run)`),

			// 7. General Infrastructure & Hash Noise
			regexp.MustCompile(`sha256:[a-f0-9]{64}`),
			regexp.MustCompile(`(resolving|transferring context|loading secrets|docker-image://|\[2mmise\[0m)`),

			// 8. Hide lines that are just a Docker timestamp and nothing else (empty lines from build)
			regexp.MustCompile(`^#\d+\s+[0-9.]+\s*$`),

			// 9. Hide Docker/Railpack error blocks and separators
			regexp.MustCompile(`^-{4,}$`),                                          // ------ separators
			regexp.MustCompile(`^\s*>\s+.+:$`),                                     // > bun install --frozen-lockfile:
			regexp.MustCompile(`^[0-9]+\.[0-9]+\s+`),                               // 0.254 bun install... (timestamp-prefixed duplicate lines)
			regexp.MustCompile(`(ERRO failed to solve|unrecognized image format)`), // Internal Docker errors

			// 10. Hide Yarn / pnpm specific noise
			regexp.MustCompile(`(YN\d{4}:)`),          // Yarn Berry status codes
			regexp.MustCompile(`(Packages: \+|Progress:|Already up to date|Lockfile is up to date)`), // pnpm progress

			// 11. Hide pip / Python build noise
			regexp.MustCompile(`(Collecting |Requirement already satisfied|Successfully installed|Using cached|Downloading .+\.whl|Building wheel for|Created wheel for)`),

			// 12. Hide Go module noise
			regexp.MustCompile(`(go: downloading |go: finding |go: extracting )`),

			// 13. Hide Ruby/Bundler noise
			regexp.MustCompile(`(Fetching gem metadata|Resolving dependencies\.\.\.|Using bundler|Installing .+ with native extensions|Bundle complete!|Bundled gems are installed)`),

			// 14. Hide Deno noise
			regexp.MustCompile(`(Download https://deno\.land|Check file://)`),

			// 15. Hide Nixpacks metadata (alternative to Railpack)
			regexp.MustCompile(`(Nixpacks|nixpacks|nix-support|\/nix\/store\/)`),

			// 16. General infrastructure leak prevention
			regexp.MustCompile(`^(#\d+\s+[0-9.]+\s*)?(COPY|RUN|ADD|WORKDIR|FROM|ENV|EXPOSE|CMD|ENTRYPOINT|ARG|LABEL|VOLUME|STOPSIGNAL|HEALTHCHECK|SHELL|ONBUILD)\s`), // Dockerfile instructions
			regexp.MustCompile(`(\/app\/storage\/|\/tmp\/build|\/var\/cache\/|\.docker\/|layer already exists|Pushing image|Pulling from|Build starting\.\.\.)`),      // Internal paths & Docker ops
			regexp.MustCompile(`^[a-f0-9]{12}$`), // Short container/layer IDs

			// 17. Hide Dockerfile source code leaks in error output
			regexp.MustCompile(`^Dockerfile:\d+`),         // Dockerfile:112
			regexp.MustCompile(`^\s*\d+\s+\|`),            // 111 |     # Production asset build  /  112 | >>> RUN ...
			regexp.MustCompile(`^ERROR: failed to build`), // ERROR: failed to build: failed to solve: process...
			regexp.MustCompile(`^\s*>\s+\[.+\]\s+RUN\s`),  // > [frontend 13/15] RUN if [ -f package.json ]...
			regexp.MustCompile(`(shtool|Extension .+ is missing|Installing missing extensions|Configuring extension|Configuring libtool|appending configuration tag|Generating files|configure: |config\.status:)`), // PHP extension build noise

			// 18. Laravel framework output that leaks internal paths and infrastructure
			regexp.MustCompile(`storage/app/public.*public/storage`),                                          // php artisan storage:link output
			regexp.MustCompile(`Discovered Package:`),                                                         // php artisan package:discover internal list
			regexp.MustCompile(`Package manifest generated`),                                                  // package:discover completion
			// (Removed generated optimized autoload from here to allow it)
			regexp.MustCompile(`Configuration cache`),                                                         // php artisan config:cache
			regexp.MustCompile(`Route cache`),                                                                 // php artisan route:cache
			regexp.MustCompile(`(Compiled views cleared|View cache cleared|Views compiled)`),                  // php artisan view:cache/clear
			regexp.MustCompile(`(application key|APP_KEY|key:generate)`),                                      // APP_KEY related warnings
			regexp.MustCompile(`\/var\/www\/html`),                                                            // Internal container path
			regexp.MustCompile(`(Writing .+ to disk|Manifest compiled|Compiling common classes)`),             // Laravel optimization internals
			regexp.MustCompile(`(npm warn|npm WARN)`),                                                         // npm warnings (deprecated packages, peer deps)
			regexp.MustCompile(`\d+ vulnerabilities \(`),                                                      // npm audit summary
			regexp.MustCompile(`To address (all )?issues`),                                                    // npm audit suggestion
			regexp.MustCompile(`(run.*npm audit|npm audit fix)`),                                              // npm audit fix suggestion
			regexp.MustCompile(`Browserslist.*outdated`),                                                      // caniuse-lite outdated warning
			regexp.MustCompile(`npx browserslist`),                                                            // browserslist update suggestion
			regexp.MustCompile(`Can't resolve.*/app/resources`),                                               // webpack resolve errors exposing container paths
			regexp.MustCompile(`in '/app`),                                                                    // webpack/node resolve context path
			regexp.MustCompile(`(libtool: |make: |cc |install: |strip )`),                                     // Build tool noise
			regexp.MustCompile(`(Entering directory|Leaving directory)`),                                      // Make directory noise
			regexp.MustCompile(`(Circular .* dependency dropped)`),                                            // JIT circular dependency noise
			regexp.MustCompile(`(Installing shared extensions|shared_alloc|ZendAccelerator)`),                 // PHP internal build noise

			// 19. Hide Railpack/Nixpacks toolchain management (mise, node, bun install noise)
			regexp.MustCompile(`(mise\b|\[\d+/\d+\]\s+(install|download|extract|generate))`),
			regexp.MustCompile(`^(#\d+\s+[0-9.]+\s*)?(v\d+\.\d+\.\d+|✓ installed)$`),                               // Generic version and success checkmarks
			regexp.MustCompile(`(Hit:\d+ http:\/\/deb\.debian\.org|bookworm InRelease)`),                           // Debian/Apt repository noise
			regexp.MustCompile(`(warn: incorrect peer dependency|note: try re-running without --frozen-lockfile)`), // Bun noise

			// 20. Hide APT/dpkg noise (Reading database, package installation stats, triggers)
			regexp.MustCompile(`(NEW packages will be installed|upgraded, .* newly installed|get [0-9.]+ [kMG]?B of archives|After this operation, [0-9.]+ [kMG]?B|debconf: delaying|Reading database \.\.\.|files and directories currently installed|Selecting previously unselected|Preparing to unpack|Unpacking |Setting up |Processing triggers)`),

			// 21. Hide minimal Vite/Rollup noise but ALLOW asset summaries and transformation progress
			regexp.MustCompile(`(computing gzip size\.\.\.|Some chunks are larger than .* after minification|Using dynamic import\(\)|build\.rollupOptions\.output\.manualChunks|build\.chunkSizeWarningLimit)`),

			// 22. Hide internal env vars but ALLOW package manager summaries
			regexp.MustCompile(`(NIXPACKS_|PAAS_|NPM_CONFIG_|NODE_ENV=)`),
		},
		stripPattern: regexp.MustCompile(`^#\d+\s+[0-9.]+\s*`),
		ansiPattern:  regexp.MustCompile(`\x1B\[[0-9;]*[a-zA-Z]`),
	}
}

var buildTransformations = []logTransformation{
	{regexp.MustCompile(`^\s*\$\s*(npm|pnpm|yarn|bun|composer|php|go|python|pip|pip3|ruby|bundle|rake|make|deno|mise)\s+(install|ci|i|get|add|download)`), "> $1 $2"},
	{regexp.MustCompile(`^\s*\$\s*(npm|pnpm|yarn|bun)\s+run\s+(build|prod|production)`), "> $1 run $2"},
	{regexp.MustCompile(`^\s*\$\s*(php|python|go|ruby|node|deno|bun)\s+(artisan|manage\.py|main\.go|app\.rb|index\.js|server\.ts)\s+`), "> $1 $2"},
	{regexp.MustCompile(`^\[\d+/\d+\]\s+(install|download|extracting).*`), "Installing dependencies..."},
	{regexp.MustCompile(`^\[\d+/\d+\]\s+(build|generate|compiling).*`), "Building application..."},
	{regexp.MustCompile(`^↳ Using config.*`), "Detected configuration..."},
	{regexp.MustCompile(`^Successfully built image in\s+(.*)`), "Build completed in $1"},
	{regexp.MustCompile(`^built in\s+(.*)`), "Build completed in $1"},
	{regexp.MustCompile(`^Deploy.*`), "Deploying application..."},
	{regexp.MustCompile(`^[-*]?\s*(Installing|Downloading|Extracting)\s+([^:(\s]+).*`), "- $1 $2"},
	{regexp.MustCompile(`^(Generating|Generated) optimized autoload files.*`), "Optimizing autoload files..."},
}

func (r *LogRefiner) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add current write to buffer
	r.buf += string(p)

	// Process the buffer
	for {
		// Find next separator (\n or \r)
		idxN := strings.Index(r.buf, "\n")
		idxR := strings.Index(r.buf, "\r")
		
		var idx int
		var sepLen int
		
		if idxN == -1 && idxR == -1 {
			break // No more complete lines
		}
		
		if idxN != -1 && (idxR == -1 || idxN < idxR) {
			idx = idxN
			sepLen = 1
		} else {
			idx = idxR
			sepLen = 1
		}
		
		line := r.buf[:idx]
		r.buf = r.buf[idx+sepLen:]
		
		shouldHide := false
		for _, pattern := range r.hidePatterns {
			if pattern.MatchString(line) {
				shouldHide = true
				break
			}
		}

		if !shouldHide {
			cleanedLine := r.stripPattern.ReplaceAllString(line, "")
			cleanedLine = r.ansiPattern.ReplaceAllString(cleanedLine, "")
			cleanedLine = strings.TrimSpace(cleanedLine)

			if cleanedLine != "" {
				// Apply transformations
				isStatus := false
				for _, t := range buildTransformations {
					if t.pattern.MatchString(cleanedLine) {
						cleanedLine = t.pattern.ReplaceAllString(cleanedLine, t.replacement)
						isStatus = true
						break
					}
				}

				if isStatus {
					if cleanedLine == r.lastStep {
						continue // Skip duplicate status message
					}
					r.lastStep = cleanedLine
				} else {
					r.lastStep = "" // Reset last step if it's a real log line
				}

				if !r.started {
					if _, err := r.writer.Write([]byte("Build process started\n")); err != nil {
						return len(p), err
					}
					r.started = true
				}

				if _, err := r.writer.Write([]byte(cleanedLine + "\n")); err != nil {
					return len(p), err
				}
			}
		}
	}

	return len(p), nil
}
