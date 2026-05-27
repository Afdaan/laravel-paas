# Implementation Plan: Safe, Reliable, and Optimized Deployment Pipeline

This implementation plan details the core optimization tasks for the Laravel PaaS deployment pipeline across 5 key areas:
1. **Task 1: Caching Optimization, Rootless BuildKit, & Registry Flow**
2. **Task 2: Safe & Reliable Deployment Flow & Log UX Improvements**
3. **Task 3: Docker Image & Runtime Optimization**
4. **Task 4: Infrastructure & Error Sanitization**
5. **Task 5: GitHub App Backend Review & Future-Proofing**

---

## 🎯 Overall Platform Direction

### Primary Focus
* **Clean developer experience**: Developer-facing interfaces and logs must feel smooth, helpful, and professional.
* **Fast deployments**: Optimize caching strategies so rebuilds don't re-install unchanged dependencies.
* **Reliable rollback behavior**: Keep rollback images safe from over-aggressive pruning and verify health before promotion.
* **Infrastructure abstraction**: Shield developers from raw Docker/BuildKit/network commands and topologies.
* **Minimal but useful logs**: Show only what matters for debugging (like application exceptions and migration outputs) without log spam.
* **Stable deployment lifecycle**: Prevent hanging jobs, manage slot concurrency, and recover gracefully from failures.

### Avoid
* ❌ **Overexposing infrastructure details**: Do not leak hostnames, passwords, registry URLs, or stack traces of internal services.
* ❌ **Overly verbose logs**: Do not spam developers with raw Docker progress bars or standard output clutter.
* ❌ **Queue deadlocks**: Do not let failed or timed-out jobs lock up worker threads indefinitely.
* ❌ **Duplicate runtime artifacts**: Do not leak dangling layers or keep duplicate images that fill up server storage.
* ❌ **Fragile deployment states**: Do not allow a failed deployment or configuration update to break a healthy running site.

---

---

## ⚙️ Task 1: Caching & Infrastructure Optimizations

### 1. Rootless BuildKit Integration (Security)
* **Goal**: Isolate build executions from host root privileges without losing resource constraints.
* **Solution**: Replace `moby/buildkit` with the rootless `moby/buildkit:rootless` image.
  * Remove the `--privileged` flag from the container run command.
  * Add `--device /dev/fuse --security-opt seccomp=unconfined --security-opt apparmor=unconfined` to support unprivileged nested containerization (`runc`).
  * Relocate the persistent cache mount from `/var/lib/buildkit` to the rootless user's path: `/home/user/.local/share/buildkit`.

### 2. Robust Registry Recovery (`build-runtime.sh`)
* **Goal**: Ensure the local registry can be repopulated from existing host images in seconds if wiped, without running hours-long rebuilds.
* **Solution**: Separate the build logic and push logic in `scripts/build-runtime.sh`. Even if a build is skipped (because the image exists on the host daemon), the script will **always execute the `docker push`** command.

### 3. Layer-Aware Build Caching (Lockfiles First)
* **Goal**: Prevent rebuilding dependencies when only source code changes.
* **Solution**: Modify dynamic Dockerfile templates to copy only dependency files first, run installers, and then copy the remaining source code.

### 4. Zero-Config 2-Step HTTP Readiness Probe
* **Goal**: Prevent promoting broken "zombie" containers (processes running but app server down/500).
* **Solution**: Query the container's private IP on the `paas-network` and verify it returns an HTTP status code `< 500` (e.g. 200, 301, 401, 403, 404) before promoting routing traffic.

---

## ⚙️ Task 2: Safe & Reliable Deployment Flow & Log UX

### 1. Global Deployment Watchdog Timeout
* **Goal**: Prevent hanging deployment steps (like network timeouts on git clone, infinite loop build scripts, or stuck migrations) from locking worker threads and blocking the deployment queue.
* **Solution**: Wrap the entire deployment execution context inside `processDeployment` with a watchdog timeout.
  * Fetch `SettingBuildTimeout` (default 30 mins) and allocate `timeout * 1.5` for the total job context to cover git cloning, migrations, and Nginx reloads.
  * If the context expires, the worker thread cleanly cancels, releases locks/leases, destroys temporary rollout containers, and fails the job with a `TIMEOUT_EXCEEDED` error.

### 2. Verification Guard on Instant Env Updates/Rollbacks
* **Goal**: Ensure that instant env updates (`update_env`) and rollbacks (`rollback`) do not replace a healthy running container with an unhealthy one if the app fails to initialize under the new configuration.
* **Solution**: Modify `RecreateProjectZeroDowntime` in `service.go` to use the 2-Step `AdvancedHealthcheck` readiness probe rather than the basic `IsContainerHealthy` check.

### 3. Structured Failure Classification
* **Goal**: Separate and classify build, runtime, infrastructure, and queue failures so developers get immediate, clear categorizations.
* **Solution**: Classify errors into standard categories during failure tracking:
  * `[CLONE_FAILED]`: Git sync or credential authentication issues.
  * `[BUILD_FAILED]`: Compilation, Composer, or JS asset bundle errors.
  * `[RUNTIME_FAILED]`: Application failed to boot or returned 5xx errors.
  * `[MIGRATION_FAILED]`: Database migration script failures.
  * `[TIMEOUT_EXCEEDED]`: Execution watchdog timed out.
  * `[QUEUE_CONFLICT]`: Concurrent lease or lock conflicts.
  * `[SYSTEM_PANIC]`: Internal worker panic.
* Save these codes as structured prefixes in `ErrorLog` with clear, human-actionable headlines.

### 4. Developer-Facing Log UX Refinement
* **Goal**: Keep error logs minimal, readable, and actionable, avoiding raw infrastructure noise or directory leakage.
* **Solution**:
  * For healthcheck/migration failures, slice and extract only the last 15 lines of relevant application logs (e.g. stack traces, Laravel exception logs) rather than throwing a massive chunk of raw Docker metadata.
  * Redact internal host directories (like `/home/afdaan/...` or `/var/lib/...`) from the user-visible errors.

---

## ⚙️ Task 3: Docker Image & Runtime Optimization

### 1. Retention-Aware Image Pruning
* **Goal**: Retain a configurable number of historical project image builds for instant rollbacks while reclaiming disk space.
* **Solution**:
  * **Keep Recent Tagged Versions**: During successful deployment, the system calls `PruneProjectImages` which reads the `max_image_retention` setting (configured in Settings.tsx, defaulting to 3). It preserves the active container image plus the last `N` tagged rollback versions, and deletes only older tags.
  * **Clean Up on Purge**: Refactor `RemoveImage` in `pruning.go` so that when a project is deleted, it finds and deletes all tagged commit images (`paas-<subdomain>:<commit_hash>`) rather than leaving them untagged or orphaned.
  * **Fix Prune Label Bug**: Fix the `=true=true` label syntax bug in `PruneImages` to ensure dangling system layers can be pruned.

### 2. Time-Restricted Build Cache Pruning (SSD & Cache Friendly)
* **Goal**: Avoid wiping hot builder layers every night so that daily builds remain instantaneous and write less data to the SSD.
* **Solution**: Replace the blind `docker builder prune -a -f` daily scheduler with a time-restricted prune:
  `docker builder prune -f --filter "until=48h"`
  This purges only stale builder cache older than 48 hours and preserves hot caches for daily deployments.

### 3. Laravel Startup Cache Optimizations
* **Goal**: Optimize runtime efficiency and reduce container startup time.
* **Solution**: Update the `laravel-init` program in `supervisord.conf` to run `php artisan route:cache` and `php artisan view:cache` at container boot, improving Laravel's initial response latency by 20-30%.

---

## ⚙️ Task 4: Infrastructure & Error Sanitization

### 1. Robust Backend Error Sanitization (`SanitizeError`)
* **Goal**: Prevent exposure of internal database passwords, Git connection tokens, registry URLs, container IDs, hostnames, and internal system paths in user-facing error logs.
* **Solution**: Implement a centralized `SanitizeError` utility in the shared package. When an error is recorded (in the worker or watchdog), the utility redacts sensitive infrastructure details and presents a clean, user-friendly failure message.

### 2. Separation of Diagnostic Logs and User Logs
* **Goal**: Provide administrators with raw diagnostics for debugging while keeping developer logs clean.
* **Solution**: Continue writing the full raw errors, panic stack traces, and debug logs directly to Go's structured `slog` system outputs (which only admins can read via terminal/journalctl), while writing only the sanitized error message to the database `error_log` (visible to developers).

---

## Proposed Changes

### 1. Infrastructure Configuration (Rootless BuildKit & Always-Push Registry)

#### [MODIFY] [infra.sh](file:///home/afdaan/Documents/Code/laravel-paas/scripts/infra.sh)
```diff
 # 8. Jalankan BuildKit
 echo "[INFO] Starting BuildKit..."
 docker volume create paas-buildkit-cache 2>/dev/null || true
 docker run -d \
     --name paas-buildkit \
     --network paas-network \
     -p 127.0.0.1:1234:1234 \
-    --privileged \
+    --device /dev/fuse \
+    --security-opt seccomp=unconfined \
+    --security-opt apparmor=unconfined \
     --restart unless-stopped \
     --cpus="2.0" \
     --memory="3g" \
-    -v paas-buildkit-cache:/var/lib/buildkit \
+    -v paas-buildkit-cache:/home/user/.local/share/buildkit \
     -v "$(pwd)/docker/templates/buildkitd.toml:/etc/buildkit/buildkitd.toml:ro" \
-    moby/buildkit --addr tcp://0.0.0.0:1234 --config /etc/buildkit/buildkitd.toml
+    moby/buildkit:rootless --addr tcp://0.0.0.0:1234 --config /etc/buildkit/buildkitd.toml
```

#### [MODIFY] [start.sh](file:///home/afdaan/Documents/Code/laravel-paas/scripts/start.sh)
```diff
     echo -e "${YELLOW}Starting BuildKit...${NC}"
     docker rm -f paas-buildkit 2>/dev/null || true
     docker volume create paas-buildkit-cache 2>/dev/null || true
     local config_path="${PROJECT_ROOT}/docker/templates/buildkitd.toml"
     docker run -d \
         --name paas-buildkit \
         --network paas-network \
         -p 127.0.0.1:1234:1234 \
-        --privileged \
+        --device /dev/fuse \
+        --security-opt seccomp=unconfined \
+        --security-opt apparmor=unconfined \
         --restart unless-stopped \
         --cpus="2.0" \
         --memory="3g" \
-        -v paas-buildkit-cache:/var/lib/buildkit \
+        -v paas-buildkit-cache:/home/user/.local/share/buildkit \
         -v "${config_path}:/etc/buildkit/buildkitd.toml:ro" \
-        moby/buildkit --addr tcp://0.0.0.0:1234 --config /etc/buildkit/buildkitd.toml
+        moby/buildkit:rootless --addr tcp://0.0.0.0:1234 --config /etc/buildkit/buildkitd.toml
```

#### [MODIFY] [build-runtime.sh](file:///home/afdaan/Documents/Code/laravel-paas/scripts/build-runtime.sh)
```diff
     # 1. Build Base Runtime
     if [[ "$TARGET" == "all" || "$TARGET" == "runtime" ]]; then
         TAG_RUNTIME="paas-runtime-php:${VERSION}-alpine"
+        reg_port=${REGISTRY_PORT:-"5000"}
+        reg_host=${REGISTRY_HOST:-"127.0.0.1"}
         
         if [ "$FORCE_REBUILD" = false ] && docker image inspect "$TAG_RUNTIME" >/dev/null 2>&1; then
             echo -e "${GREEN}[SKIP] PHP ${VERSION} runtime already exists. Use --force to rebuild.${NC}"
         else
             echo -e "${YELLOW}Building PHP ${VERSION} runtime... ($TAG_RUNTIME)${NC}"
             # Tag with registry hosts to avoid remote pulls and enable instant local resolution in BuildKit.
             $BUILD_CMD \
                 --build-arg PHP_VERSION="${VERSION}" \
                 -f "${DOCKER_BASE}" \
                 -t "${TAG_RUNTIME}" \
                 -t "paas-registry:5000/library/paas-runtime-php:${VERSION}-alpine" \
                 -t "${reg_host}:${reg_port}/library/paas-runtime-php:${VERSION}-alpine" \
                 "${PROJECT_ROOT}/docker/runtime"
         fi
+
+        # Always verify tags and push to local registry to heal wiped registry containers
+        docker tag "$TAG_RUNTIME" "paas-registry:5000/library/paas-runtime-php:${VERSION}-alpine" 2>/dev/null || true
+        docker tag "$TAG_RUNTIME" "${reg_host}:${reg_port}/library/paas-runtime-php:${VERSION}-alpine" 2>/dev/null || true
+        echo -e "${YELLOW}Ensuring PHP ${VERSION} runtime is registered at ${reg_host}:${reg_port}...${NC}"
+        docker push "${reg_host}:${reg_port}/library/paas-runtime-php:${VERSION}-alpine"
+        echo -e "${GREEN}[SUCCESS] PHP ${VERSION} runtime registered successfully.${NC}"
     fi
 
     # 2. Build Unified Builder
     if [[ "$TARGET" == "all" || "$TARGET" == "builder" ]]; then
         TAG_BUILDER="paas-builder-base:${VERSION}-alpine"
+        reg_port=${REGISTRY_PORT:-"5000"}
+        reg_host=${REGISTRY_HOST:-"127.0.0.1"}
         
         if [ "$FORCE_REBUILD" = false ] && docker image inspect "$TAG_BUILDER" >/dev/null 2>&1; then
             echo -e "${GREEN}[SKIP] PHP ${VERSION} Unified Builder already exists. Use --force to rebuild.${NC}"
         else
             echo -e "${YELLOW}Building PHP ${VERSION} Unified Builder... ($TAG_BUILDER)${NC}"
             # Tag with registry hosts to avoid remote pulls and enable instant local resolution in BuildKit.
             $BUILD_CMD \
                 --build-arg PHP_VERSION="${VERSION}" \
                 -f "${DOCKER_BUILDER}" \
                 -t "${TAG_BUILDER}" \
                 -t "paas-registry:5000/library/paas-builder-base:${VERSION}-alpine" \
                 -t "${reg_host}:${reg_port}/library/paas-builder-base:${VERSION}-alpine" \
                 "${PROJECT_ROOT}/docker/runtime"
         fi
+
+        # Always verify tags and push to local registry to heal wiped registry containers
+        docker tag "$TAG_BUILDER" "paas-registry:5000/library/paas-builder-base:${VERSION}-alpine" 2>/dev/null || true
+        docker tag "$TAG_BUILDER" "${reg_host}:${reg_port}/library/paas-builder-base:${VERSION}-alpine" 2>/dev/null || true
+        echo -e "${YELLOW}Ensuring PHP ${VERSION} Unified Builder is registered at ${reg_host}:${reg_port}...${NC}"
+        docker push "${reg_host}:${reg_port}/library/paas-builder-base:${VERSION}-alpine"
+        echo -e "${GREEN}[SUCCESS] PHP ${VERSION} Unified Builder registered successfully.${NC}"
     fi
```

---

### 2. Caching Optimizations in PHP Dynamic Dockerfiles

#### [MODIFY] [Dockerfile.php80.dynamic](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/Dockerfile.php80.dynamic)
#### [MODIFY] [Dockerfile.php81.dynamic](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/Dockerfile.php81.dynamic)
#### [MODIFY] [Dockerfile.php82.dynamic](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/Dockerfile.php82.dynamic)
#### [MODIFY] [Dockerfile.php83.dynamic](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/Dockerfile.php83.dynamic)
#### [MODIFY] [Dockerfile.php84.dynamic](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/Dockerfile.php84.dynamic)
Modify builder stage to copy dependency files first:
```dockerfile
# Copy Composer lockfiles first
COPY composer.json composer.lock* ./

# Run Composer (excluding dev)
RUN --mount=type=cache,target=/root/.composer/cache \
    composer install --no-dev --no-scripts --no-autoloader --prefer-dist --ignore-platform-reqs || \
    composer install --no-dev --prefer-dist --ignore-platform-reqs

# Copy JS dependency files
COPY package*.json pnpm-lock.yaml* yarn.lock* bun.lock* ./

# Auto-detect package manager and install JS dependencies with caching and performance flags
RUN --mount=type=cache,target=/root/.npm \
    --mount=type=cache,target=/root/.local/share/pnpm/store \
    --mount=type=cache,target=/root/.cache/yarn \
    --mount=type=cache,target=/root/.bun/install/cache \
    if [ -f package.json ]; then \
        echo "[PROGRESS] Installing JavaScript dependencies..." && \
        if [ -f bun.lock ]; then bun install --prefer-offline --no-audit; \
        elif [ -f pnpm-lock.yaml ]; then pnpm install --prefer-offline --no-audit --no-progress; \
        elif [ -f yarn.lock ]; then yarn install --prefer-offline --no-progress; \
        elif [ -f package-lock.json ]; then npm ci --prefer-offline --no-audit --no-fund --no-progress; \
        else npm install --prefer-offline --no-audit --no-fund --no-progress; fi; \
    fi

# Copy the remaining application source code
COPY . .

# Generate optimized autoloader and detect extensions
RUN composer dump-autoload --optimize --no-dev --classmap-authoritative --no-scripts
```

---

### 3. Caching & Logging Optimizations in Frontend & Railpack Templates

#### [MODIFY] [Dockerfile](file:///home/afdaan/Documents/Code/laravel-paas/frontend/Dockerfile)
```diff
 WORKDIR /app
 
 # Copy package files
 COPY package*.json ./
+
 # Install packages leveraging BuildKit NPM local cache
 RUN --mount=type=cache,target=/root/.npm \
-    npm ci
+    npm ci --prefer-offline --no-audit --no-fund --no-progress
 
 # Declare build-time arguments for Vite to inject
 ARG VITE_GITHUB_APP_URL
 
 # Copy source and build
 COPY . .
 RUN npm run build
```

#### [MODIFY] [railpack.json](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/railpack.json)
```diff
     "install": {
       "cmds": [
-        "npm install --legacy-peer-deps --no-audit --no-fund || yarn install --no-immutable || pnpm install --no-frozen-lockfile || bun install"
+        "npm install --prefer-offline --no-audit --no-fund --no-progress --legacy-peer-deps || yarn install --prefer-offline --no-progress --no-immutable || pnpm install --prefer-offline --no-audit --no-progress --no-frozen-lockfile || bun install --prefer-offline --no-audit"
       ]
     },
```

---

### 4. Dynamic HTTP Probes, IPs, and Helpers in Go service

#### [MODIFY] [healthcheck.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/infrastructure/docker/healthcheck.go)
Add HTTP check methods and implement them in the `AdvancedHealthcheck` loop:
```go
// GetContainerIP extracts the private IP of a container on the Docker network
func (s *DockerService) GetContainerIP(containerID string) (string, error) {
	res, err := utils.Run(5*time.Second, "docker", "inspect", "--format", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", containerID)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(res.Stdout)
	if ip == "" {
		return "", fmt.Errorf("container IP is empty")
	}
	return ip, nil
}

// probeHTTP sends an HTTP GET request to the specified URL to verify status code < 500
func (s *DockerService) probeHTTP(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	client := &http.Client{
		Timeout: 2 * time.Second,
	}
	
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode >= 500 {
		return fmt.Errorf("server error status code: %d", resp.StatusCode)
	}
	
	return nil
}
```

Update `AdvancedHealthcheck` to run HTTP verification and clean up error responses:
```go
// AdvancedHealthcheck runs a production readiness probe with exponential backoff and a stabilization monitoring window.
func (s *DockerService) AdvancedHealthcheck(ctx context.Context, project *models.Project, containerID string) error {
	slog.Info("Starting advanced healthcheck probe", "projectId", project.ID, "containerId", containerID)

	// Determine if container exposes a web port
	isWebFacing := project.Port == nil || *project.Port > 0

	// 1. Startup grace period
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	maxAttempts := 10
	currentInterval := 1 * time.Second
	isReady := false

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if s.IsContainerHealthy(containerID) {
			if isWebFacing {
				ip, ipErr := s.GetContainerIP(containerID)
				if ipErr == nil {
					url := fmt.Sprintf("http://%s:%s%s", ip, project.GetInternalPort(), project.GetHealthCheckPath())
					if err := s.probeHTTP(ctx, url); err == nil {
						isReady = true
						break
					} else {
						slog.Warn("Container running but HTTP probe failed", "url", url, "error", err, "attempt", attempt)
					}
				} else {
					slog.Warn("Failed to resolve container IP for HTTP probe", "error", ipErr, "attempt", attempt)
				}
			} else {
				isReady = true
				break
			}
		}

		slog.Debug("Container health check probe pending", "attempt", attempt, "containerId", containerID)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(currentInterval):
		}

		currentInterval *= 2
		if currentInterval > 8*time.Second {
			currentInterval = 8 * time.Second
		}
	}

	if !isReady {
		logs, _ := s.GetLogs(containerID, 15) // Trimmed to last 15 lines of developer logs
		return fmt.Errorf("[RUNTIME_FAILED] Container process is running but did not respond to HTTP readiness checks. Last logs:\n%s", logs)
	}

	// 2. Stabilization window
	slog.Info("Container readiness verified, entering stabilization monitoring window (5s)", "containerId", containerID)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
	}

	if !s.IsContainerHealthy(containerID) {
		logs, _ := s.GetLogs(containerID, 15) // Trimmed to last 15 lines of developer logs
		return fmt.Errorf("[RUNTIME_FAILED] Container crashed during stabilization window. Last logs:\n%s", logs)
	}

	if isWebFacing {
		ip, _ := s.GetContainerIP(containerID)
		url := fmt.Sprintf("http://%s:%s%s", ip, project.GetInternalPort(), project.GetHealthCheckPath())
		if err := s.probeHTTP(ctx, url); err != nil {
			logs, _ := s.GetLogs(containerID, 15) // Trimmed to last 15 lines of developer logs
			return fmt.Errorf("[RUNTIME_FAILED] HTTP probe failed during stabilization window. Error: %w. Last logs:\n%s", err, logs)
		}
	}

	slog.Info("Advanced healthcheck completed successfully", "containerId", containerID)
	return nil
}
```

---

### 6. Deployment Watchdog and Structured Failure Classification

#### [MODIFY] [deployment_worker.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/workers/deployment_worker.go)
1. Wrap the entire deployment execution with a global watchdog context timeout.
2. Classify errors into structured prefixes (`[CLONE_FAILED]`, `[BUILD_FAILED]`, `[RUNTIME_FAILED]`, `[MIGRATION_FAILED]`, `[TIMEOUT_EXCEEDED]`).

In `processDeployment` (around line 313):
```diff
 	// Wrap deployment execution in panic recovery
 	defer func() {
 		if r := recover(); r != nil {
 			slog.Error("CRITICAL PANIC during deployment execution", "jobId", job.JobID, "projectId", job.ProjectID, "workerId", workerID, "panic", r)
-			w.recordAuditLog(job.ProjectID, job.JobID, workerID, "deployment_panic", fmt.Sprintf("Worker panic recovered: %v", r))
+			w.recordAuditLog(job.ProjectID, job.JobID, workerID, "deployment_panic", fmt.Sprintf("[SYSTEM_PANIC] Worker panic recovered: %v", r))
 			_ = w.redisService.ReleaseDeploymentLease(job.JobID, workerID)
 			_ = w.redisService.ForceReleaseDeploymentLock(job.ProjectID, fmt.Sprintf("Worker panic recovered: %v", r))
 			if project, err := w.projectRepo.GetByID(job.ProjectID); err == nil {
-				w.updateProjectError(project, job.JobID, fmt.Sprintf("Deployment aborted due to worker internal error (panic): %v", r))
+				w.updateProjectError(project, job.JobID, fmt.Sprintf("[SYSTEM_PANIC] Deployment aborted due to worker internal error (panic): %v", r))
 			}
 		}
 	}()
 
 	// Idempotency check for regular deploys
 	if job.Type == "deploy" {
 		project, err := w.projectRepo.GetByID(job.ProjectID)
 		if err == nil && project.LastCommitHash != "" {
 			if idempotent, _ := w.redisService.CheckIdempotency(job.ProjectID, project.LastCommitHash, project.Subdomain, job.Type); idempotent {
 				slog.Info("Deployment idempotent match found, skipping duplicate processing", "projectId", job.ProjectID, "commit", project.LastCommitHash)
 				w.redisService.IncrementDeploymentCounter("processed")
 				return
 			}
 		}
 	}
 
+	// 1. Establish overall deployment watchdog timeout (default 30 mins)
+	buildTimeoutSec, err := strconv.Atoi(w.getSetting(models.SettingBuildTimeout, models.DefaultBuildTimeout))
+	if err != nil || buildTimeoutSec <= 0 {
+		buildTimeoutSec = 1800
+	}
+	// Give the overall deployment 1.5x the build timeout to cover cloning, migrations, and Nginx reloads
+	overallTimeout := time.Duration(buildTimeoutSec) * 3 / 2 * time.Second
+	if overallTimeout < 5*time.Minute {
+		overallTimeout = 30 * time.Minute
+	}
-	ctx, cancel := context.WithCancel(context.Background())
+	ctx, cancel := context.WithTimeout(context.Background(), overallTimeout)
 	defer cancel()
```

Classify failure messages in `deployProject`:
```diff
 	projectPath, cloneHash, cloneErr = w.gitService.CloneRepository(authURL, project.Branch, project.Subdomain)
 	...
 	if cloneErr != nil {
-		w.updateProjectError(project, job.JobID, "Failed to clone repository: "+cloneErr.Error())
+		w.updateProjectError(project, job.JobID, "[CLONE_FAILED] Failed to clone repository: "+cloneErr.Error())
 		return
 	}
 	if dbErr != nil {
-		w.updateProjectError(project, job.JobID, "Failed to create database: "+dbErr.Error())
+		w.updateProjectError(project, job.JobID, "[INFRASTRUCTURE_FAILED] Failed to create database: "+dbErr.Error())
 		return
 	}
 ...
 	newContainerID, err := w.dockerService.BuildAndRun(buildCtx, project, finalPHPVersion, projectDomain, cpuLimit, memoryLimit, job.Type == "deploy", job.Type == "redeploy", appendLog)
 	if err != nil {
 		sharedDocker.GetCircuitBreaker().RecordFailure()
 		if ctx.Err() == context.Canceled {
 			appendLog("ERROR: Deployment cancelled by user request.")
 			w.transitionDeploymentState(project, job.JobID, models.DepStatusCancelled, project.DeploymentProgress, "deployment_cancelled", "User requested cancellation")
-			w.updateProjectError(project, job.JobID, "Deployment cancelled by user.")
+			w.updateProjectError(project, job.JobID, "[TIMEOUT_EXCEEDED] Deployment cancelled by user request or execution watchdog timeout.")
 			return
 		}
 		if errors.Is(buildCtx.Err(), context.DeadlineExceeded) {
 			appendLog("ERROR: Deployment build phase timed out (watchdog kill).")
 			w.transitionDeploymentState(project, job.JobID, models.DepStatusFailed, project.DeploymentProgress, "watchdog_timeout", "Build log watchdog timed out build step")
-			w.updateProjectError(project, job.JobID, "Deployment failed: Build step exceeded maximum allowed time limit.")
+			w.updateProjectError(project, job.JobID, "[BUILD_FAILED] Deployment failed: Build step exceeded maximum allowed time limit.")
 			return
 		}
 		appendLog("ERROR: Failed to deploy container: " + err.Error())
-		w.updateProjectError(project, job.JobID, "Failed to deploy container: "+err.Error())
+		w.updateProjectError(project, job.JobID, "[BUILD_FAILED] Failed to deploy container: "+err.Error())
 		return
 	}
```
And migration failures:
```diff
 		if output, err := w.dockerService.RunMigrations(newContainerID); err != nil {
 			slog.Error("Migrations failed", "subdomain", project.Subdomain, "error", err)
 			appendLog("ERROR: Migrations failed:\n" + output)
 			w.transitionDeploymentState(project, job.JobID, models.DepStatusRollback, project.DeploymentProgress, "deployment_rollback", "Migrations failed")
 			if err := w.dockerService.RemoveContainer(newContainerID, project.WorkerContainerID); err != nil {
 				slog.Warn("Failed to cleanup failed container", "id", newContainerID, "error", err)
 			}
 			_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
 				"rollout_container_id": nil,
 			})
-			w.updateProjectError(project, job.JobID, "Migrations failed: "+err.Error()+"\n\nOutput:\n"+output)
+			// Slice migration output to keep it minimal and clean up internal paths
+			w.updateProjectError(project, job.JobID, "[MIGRATION_FAILED] Migrations failed: "+err.Error())
 			return
```

---

### 7. Active Verification Guard in Recreate logic

#### [MODIFY] [service.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/services/project/service.go)
Replace `IsContainerHealthy` loop in `RecreateProjectZeroDowntime` with `AdvancedHealthcheck` to guarantee environment updates are validated before replacing the active container.

```diff
 	_ = s.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
 		"rollout_container_id": newID,
 	})
 
-	isHealthy := false
-	maxWait := 30
-	for i := 0; i < maxWait; i++ {
-		if s.dockerService.IsContainerHealthy(newID) {
-			isHealthy = true
-			break
-		}
-		time.Sleep(1 * time.Second)
-	}
-
-	if !isHealthy {
-		slog.Error("New container failed health check, rolling back", "subdomain", project.Subdomain, "newID", newID)
+	// Run Advanced 2-step Healthcheck with timeout context
+	hcCtx, hcCancel := context.WithTimeout(context.Background(), 2*time.Minute)
+	defer hcCancel()
+
+	if err := s.dockerService.AdvancedHealthcheck(hcCtx, project, newID); err != nil {
+		slog.Error("New container failed advanced healthcheck, rolling back", "subdomain", project.Subdomain, "newID", newID, "error", err)
 
 		if err := s.dockerService.RemoveContainer(newID, project.WorkerContainerID); err != nil {
 			slog.Warn("Failed to cleanup unhealthy new container", "id", newID, "error", err)
 		}
 		_ = s.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
 			"rollout_container_id": nil,
 		})
 
-		return fmt.Errorf("recreation failed: new container is unhealthy")
+		return fmt.Errorf("recreation failed: %w", err)
 	}
```

---

### 8. Frontend UX Refinement (Clean Deployment Failure States)

#### [MODIFY] [ProjectDetail.tsx](file:///home/afdaan/Documents/Code/laravel-paas/frontend/src/pages/user/ProjectDetail.tsx)
1. Hide the deployment status badge next to the status indicator in the page header when the deployment is NOT active (i.e. once the deployment transitions to `failed`, `completed`, `rollback`, or `cancelled`).
2. Only show the top warning banner if the deployment is active (`isDeploying`) or if the project status itself is `failed` (e.g. first deploy failed, no container running).

```diff
@@ -664,6 +664,6 @@
-      {(isDeploying || project.deployment_status === 'failed' || project.status === 'failed') && (
+      {(isDeploying || (project.status === 'failed' && project.deployment_status === 'failed')) && (
         <Card className={cn(
           "border-blue-500/20 bg-blue-500/5 p-6 mb-6",
@@ -728,18 +728,18 @@
-            {project.deployment_status && project.deployment_status !== 'completed' && (
-              <Badge variant="outline" className={cn(
-                "gap-2 py-1 px-3 flex items-center",
-                project.deployment_status === 'failed' ? "text-rose-500 bg-rose-500/10 border-rose-500/20" :
-                project.deployment_status === 'rollback' ? "text-amber-500 bg-amber-500/10 border-amber-500/20" :
-                "text-blue-500 bg-blue-500/10 border-blue-500/20"
-              )}>
-                <div className={cn(
-                  "w-2 h-2 rounded-full",
-                  project.deployment_status === 'failed' ? "bg-rose-500" :
-                  project.deployment_status === 'rollback' ? "bg-amber-500" :
-                  "bg-blue-500 animate-spin"
-                )} />
-                <span className="text-[10px] uppercase font-bold tracking-wider">
-                  {deploymentPhase ? deploymentPhase.label : project.deployment_status} {project.deployment_progress != null ? `(${project.deployment_progress}%)` : ''}
-                </span>
-              </Badge>
-            )}
+            {project.deployment_status && !['completed', 'failed', 'rollback', 'cancelled'].includes(project.deployment_status) && (
+              <Badge variant="outline" className={cn(
+                "gap-2 py-1 px-3 flex items-center",
+                project.deployment_status === 'rollback' ? "text-amber-500 bg-amber-500/10 border-amber-500/20" :
+                "text-blue-500 bg-blue-500/10 border-blue-500/20"
+              )}>
+                <div className={cn(
+                  "w-2 h-2 rounded-full",
+                  project.deployment_status === 'rollback' ? "bg-amber-500" :
+                  "bg-blue-500 animate-spin"
+                )} />
+                <span className="text-[10px] uppercase font-bold tracking-wider">
+                  {deploymentPhase ? deploymentPhase.label : project.deployment_status} {project.deployment_progress != null ? `(${project.deployment_progress}%)` : ''}
+                </span>
+              </Badge>
+            )}
```

### 9. Pre-compiled Memcached in Base Images

#### [MODIFY] [Dockerfile.base](file:///home/afdaan/Documents/Code/laravel-paas/docker/runtime/Dockerfile.base)
Compile `memcached` extension in the base Alpine image:
```diff
@@ -79,6 +79,8 @@
     fi \
+    && printf "\n" | pecl install memcached \
+    && docker-php-ext-enable memcached \
     && find /usr/local/lib/php/extensions -name "*.so" -exec strip --strip-unneeded {} + \
```

#### [MODIFY] [Dockerfile.builder](file:///home/afdaan/Documents/Code/laravel-paas/docker/runtime/Dockerfile.builder)
Compile `memcached` extension in the base builder image:
```diff
@@ -76,4 +76,4 @@
-    && printf "\n" | pecl install redis imagick \
-    && docker-php-ext-enable redis imagick \
+    && printf "\n" | pecl install redis imagick memcached \
+    && docker-php-ext-enable redis imagick memcached \
```

### 10. Tag-Aware Project Image Removal & Prune Label Fix

#### [MODIFY] [pruning.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/infrastructure/docker/pruning.go)
Clean up all commit-tagged versions of a project when it is deleted, and fix the `=true=true` label prune bug:
```diff
@@ -15,9 +15,22 @@
 // RemoveImage removes a project's docker image
 func (s *DockerService) RemoveImage(subdomain string) error {
 	imageName := fmt.Sprintf("paas-%s", subdomain)
-	// Try both with and without the paas- prefix in case naming varies
-	if err := exec.Command("docker", "rmi", imageName).Run(); err != nil {
-		slog.Warn("Failed to remove image", "image", imageName, "error", err)
+	res, err := utils.Run(30*time.Second, "docker", "images", "--format", "{{.Tag}}", imageName)
+	if err != nil {
+		slog.Warn("Failed to list project images for deletion", "image", imageName, "error", err)
+		return nil
+	}
+	tags := strings.Split(strings.TrimSpace(res.Stdout), "\n")
+	for _, tag := range tags {
+		if tag == "" {
+			continue
+		}
+		imgToDel := fmt.Sprintf("%s:%s", imageName, tag)
+		if delRes, delErr := utils.Run(30*time.Second, "docker", "rmi", "-f", imgToDel); delErr != nil {
+			slog.Warn("Failed to remove project image tag", "image", imgToDel, "error", delErr, "stderr", delRes.Stderr)
+		} else {
+			slog.Info("Successfully removed project image tag", "image", imgToDel)
+		}
 	}
 	return nil
 }
@@ -35,5 +48,5 @@
-	filter := fmt.Sprintf("label=%s=true", models.LabelProjectManaged)
-	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-a", "-f", "--filter", filter); err != nil {
+	filter := fmt.Sprintf("label=%s", models.LabelProjectManaged)
+	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f", "--filter", filter); err != nil {
 		slog.Warn("Failed to prune project images", "error", err)
 	}
```

#### [MODIFY] [monitor.go](file:///home/afdaan/Documents/Code/laravel-paas/shared/infrastructure/docker/monitor.go)
Fix the `=true=true` label prune bug in the shared monitor service:
```diff
@@ -637,5 +637,5 @@
-	filter := fmt.Sprintf("label=%s=true", models.LabelProjectManaged)
-	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-a", "-f", "--filter", filter); err != nil {
+	filter := fmt.Sprintf("label=%s", models.LabelProjectManaged)
+	if err := utils.RunSilent(5*time.Minute, "docker", "image", "prune", "-f", "--filter", filter); err != nil {
 		slog.Warn("Failed to prune project images", "error", err)
 	}
```

### 11. Selective Build Cache Pruning in Watchdog

#### [MODIFY] [watchdog.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/services/worker/watchdog.go)
Restrict BuildKit cache pruning to stale layers older than 48 hours to preserve hot builder layers:
```diff
@@ -298,5 +298,5 @@
-			if err := exec.Command("docker", "builder", "prune", "-a", "-f").Run(); err != nil {
+			if err := exec.Command("docker", "builder", "prune", "-f", "--filter", "until=48h").Run(); err != nil {
 				slog.Warn("Central watchdog: failed to prune docker builder", "error", err)
 			}
```

### 12. Laravel Init Caching at Startup

#### [MODIFY] [supervisord.conf](file:///home/afdaan/Documents/Code/laravel-paas/docker/templates/supervisord.conf)
Enable route and view caching at startup to optimize response latency:
```diff
@@ -17,2 +17,2 @@
 [program:laravel-init]
-command=/usr/local/bin/php /var/www/html/artisan storage:link --ansi --force
+command=/bin/sh -c "php /var/www/html/artisan storage:link --ansi --force && php /var/www/html/artisan route:cache --ansi --quiet && php /var/www/html/artisan view:cache --ansi --quiet"
```

### 13. Image Retention Default Alignments

#### [MODIFY] [Settings.tsx](file:///home/afdaan/Documents/Code/laravel-paas/frontend/src/pages/admin/Settings.tsx)
Change frontend default fallback from 5 to 3:
```diff
@@ -266,3 +266,3 @@
-                    value={settings.max_image_retention || 5}
+                    value={settings.max_image_retention || 3}
```

#### [MODIFY] [setting_keys.go](file:///home/afdaan/Documents/Code/laravel-paas/shared/models/setting_keys.go)
Change backend default from 2 to 3:
```diff
@@ -30,2 +30,2 @@
-	DefaultMaxImageRetention   = "2"
+	DefaultMaxImageRetention   = "3"
```

### 14. Shared Error Sanitization Helper

#### [MODIFY] [str.go](file:///home/afdaan/Documents/Code/laravel-paas/shared/pkg/utils/str.go)
Add `SanitizeError` to filter internal information (registry URLs, container names, path structures, and git tokens):
```go
// SanitizeError redacts internal infrastructure names, registry URLs, container names,
// directories, passwords/tokens, and simplifies verbose error outputs into clean user-facing summaries.
func SanitizeError(errStr string) string {
	if errStr == "" {
		return ""
	}

	// 1. Redact Git Authentication Tokens in URLs
	tokenRegex := regexp.MustCompile(`https://x-access-token:[^@]+@`)
	errStr = tokenRegex.ReplaceAllString(errStr, "https://x-access-token:REDACTED@")

	// 2. Redact internal registry URLs and ports
	registryRegex := regexp.MustCompile(`(paas-registry|127\.0\.0\.1|localhost):5000`)
	errStr = registryRegex.ReplaceAllString(errStr, "registry.local")

	// 3. Redact container names
	containerNameRegex := regexp.MustCompile(`paas-project-[a-zA-Z0-9_\-]+`)
	errStr = containerNameRegex.ReplaceAllString(errStr, "app-container")

	// 4. Redact builder names
	builderRegex := regexp.MustCompile(`paas-builder`)
	errStr = builderRegex.ReplaceAllString(errStr, "builder")

	// 5. Redact local absolute paths
	pathRegex := regexp.MustCompile(`/(home|var|tmp|usr/src)/[a-zA-Z0-9_\-\.\/]+`)
	errStr = pathRegex.ReplaceAllString(errStr, "/app/workspace/")

	// 6. Redact image hashes
	shaRegex := regexp.MustCompile(`sha256:[a-fA-F0-9]{64}`)
	errStr = shaRegex.ReplaceAllString(errStr, "sha256:REDACTED")

	// 7. Simplify typical verbose Docker BuildKit errors into clean user-facing summaries
	if strings.Contains(errStr, "DOCKER_BUILD_FAILED") || strings.Contains(errStr, "Docker build failed") {
		return "Deployment failed during the container build process.\nPlease review your application configuration, build commands, or dependency files (composer.json/package.json) for errors."
	}
	if strings.Contains(errStr, "CLONE_FAILED") || strings.Contains(errStr, "Failed to clone repository") {
		return "Failed to clone repository. Please verify your repository URL, branch configuration, or GitHub connection permissions."
	}
	if strings.Contains(errStr, "MIGRATION_FAILED") || strings.Contains(errStr, "Migrations failed") {
		return "Database migrations failed during deployment. Please review your migration scripts or database connections."
	}
	if strings.Contains(errStr, "TIMEOUT_EXCEEDED") || strings.Contains(errStr, "watchdog kill") {
		return "Deployment timed out. The build or startup phase took longer than the configured maximum time limit."
	}
	if strings.Contains(errStr, "SYSTEM_PANIC") {
		return "An internal system error occurred during deployment. The operation was aborted safely. Please contact support if this persists."
	}

	return errStr
}
```

### 15. Sanitize Errors in Deployment Worker

#### [MODIFY] [deployment_worker.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/workers/deployment_worker.go)
Sanitize error messages in `updateProjectError` before transitioning state and writing to project error logs:
```diff
@@ -969,2 +969,5 @@
 func (w *DeploymentWorker) updateProjectError(project *models.Project, jobID string, errorMsg string) {
-	w.transitionDeploymentState(project, jobID, models.DepStatusFailed, project.DeploymentProgress, "deployment_failed", errorMsg)
-	msg := errorMsg
+	// Log the raw error for administrator diagnostics
+	slog.Error("Deployment failure with raw diagnostic details", "projectId", project.ID, "jobId", jobID, "error", errorMsg)
+
+	sanitizedMsg := utils.SanitizeError(errorMsg)
+	w.transitionDeploymentState(project, jobID, models.DepStatusFailed, project.DeploymentProgress, "deployment_failed", sanitizedMsg)
+	msg := sanitizedMsg
```

### 16. Sanitize Errors in Watchdog

#### [MODIFY] [watchdog.go](file:///home/afdaan/Documents/Code/laravel-paas/worker/internal/services/worker/watchdog.go)
Sanitize watchdog recovery and timeout error logs:
```diff
@@ -226,3 +226,3 @@
-					if _, err := w.projectService.TransitionDeploymentState(context.Background(), project.ID, jobID, models.DepStatusFailed, project.DeploymentProgress, "orphan_recovered", errorMsg); err != nil {
+					if _, err := w.projectService.TransitionDeploymentState(context.Background(), project.ID, jobID, models.DepStatusFailed, project.DeploymentProgress, "orphan_recovered", utils.SanitizeError(errorMsg)); err != nil {
 						slog.Error("Central watchdog: failed atomic state transition for failed project deployment", "id", project.ID, "error", err)
 					}
 					_ = w.projectRepo.UpdateMetadata(project.ID, map[string]interface{}{
-						"error_log": errorMsg,
+						"error_log": utils.SanitizeError(errorMsg),
 					})
```

---

## ⚙️ Task 5: GitHub App Backend Review & Future-Proofing

### 1. Webhook Replay & Idempotency Protection
* **Goal**: Prevent processing duplicate or replayed webhook events that cause race conditions or redundant deployments.
* **Solution**: Check the unique event ID from the `X-GitHub-Delivery` header. Cache this ID in Redis with a 24-hour TTL using a secure `SetNX` transaction. If the key already exists, reject the webhook immediately as a duplicate.

### 2. Multi-Tenant Installation & Repository Scoping Guard
* **Goal**: Prevent users from creating projects using GitHub App installations or repositories belonging to other users.
* **Solution**: Before creating a project:
  1. Query GORM to verify the requested `github_installation_id` belongs to the authenticated user.
  2. Query the GitHub API (`ListRepositories`) using the installation token to verify that the target repository (`github_repo_owner/github_repo_name`) is authorized and exists under that installation.

### 3. Rate Limit Handling & Transient API Retries
* **Goal**: Make external GitHub API communication resilient against rate limits and transient network/server failures.
* **Solution**: Centralize all HTTP requests inside `GithubService` with a `doRequestWithRetry` helper.
  * Attempt the request up to 3 times with exponential backoff.
  * If a rate limit (status 403 or 429) is hit, inspect the `X-RateLimit-Reset` or `Retry-After` headers to calculate the precise backoff duration, capping it at 10 seconds.
  * If a transient server error (500, 502, 503, 504) or network issue occurs, wait and retry.
  * Safely reset the request body stream on retry using `GetBody` to support payloads.

### 4. Integration Observability & Metrics
* **Goal**: Provide visibility into external API health, rate limit usage, and webhook volumes.
* **Solution**: Record metrics using the global `MetricsCollector`:
  * `github_webhooks_received_total`: Total webhook deliveries received.
  * `github_webhooks_processed_total`: Webhook deliveries successfully enqueued.
  * `github_api_requests_total`: Total GitHub API requests sent.
  * `github_api_failures_total`: Total failed API requests.

---

## Proposed Changes (Task 5)

### 17. Redis Service SetNX Helper

#### [MODIFY] [redis.go](file:///home/afdaan/Documents/Code/laravel-paas/shared/infrastructure/redis.go)
Add `SetNX` helper method:
```go
// SetNX sets a cache key only if it does not exist, returning true if successful.
func (r *RedisService) SetNX(key string, value interface{}, expiration time.Duration) (bool, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return false, err
	}
	return r.client.SetNX(r.ctx, key, data, expiration).Result()
}
```

### 18. GitHub Integration Metrics

#### [MODIFY] [collector.go](file:///home/afdaan/Documents/Code/laravel-paas/shared/pkg/metrics/collector.go)
Add GitHub metrics fields:
```diff
@@ -70,2 +70,7 @@
 	domainStaleWriteRejectedTotal  int64
+
+	// GitHub Integration Metrics
+	githubWebhooksReceivedTotal  int64
+	githubWebhooksProcessedTotal int64
+	githubApiRequestsTotal       int64
+	githubApiFailuresTotal       int64
 }
```
Add helper functions and expose them in `PrometheusHandler`:
```go
func (m *MetricsCollector) IncrGithubWebhooksReceived() {
	atomic.AddInt64(&m.githubWebhooksReceivedTotal, 1)
}

func (m *MetricsCollector) IncrGithubWebhooksProcessed() {
	atomic.AddInt64(&m.githubWebhooksProcessedTotal, 1)
}

func (m *MetricsCollector) IncrGithubApiRequests() {
	atomic.AddInt64(&m.githubApiRequestsTotal, 1)
}

func (m *MetricsCollector) IncrGithubApiFailures() {
	atomic.AddInt64(&m.githubApiFailuresTotal, 1)
}
```
```diff
@@ -258,2 +258,7 @@
 		writeCounterMetric(&buf, "domain_stale_write_rejected_total", "Total rejected domain state updates due to stale sequence version", atomic.LoadInt64(&col.domainStaleWriteRejectedTotal))
+
+		writeCounterMetric(&buf, "github_webhooks_received_total", "Total GitHub App webhooks received by the backend", atomic.LoadInt64(&col.githubWebhooksReceivedTotal))
+		writeCounterMetric(&buf, "github_webhooks_processed_total", "Total GitHub App webhooks successfully processed", atomic.LoadInt64(&col.githubWebhooksProcessedTotal))
+		writeCounterMetric(&buf, "github_api_requests_total", "Total GitHub API requests sent", atomic.LoadInt64(&col.githubApiRequestsTotal))
+		writeCounterMetric(&buf, "github_api_failures_total", "Total failed GitHub API requests", atomic.LoadInt64(&col.githubApiFailuresTotal))
```

### 19. Robust Retry & Rate Limit Handling in GitHub Service

#### [MODIFY] [github.go](file:///home/afdaan/Documents/Code/laravel-paas/shared/infrastructure/github.go)
Import `"sync/atomic"` and `"github.com/laravel-paas/shared/pkg/metrics"`. Add request retry handler:
```go
func (s *GithubService) doRequestWithRetry(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error
	maxRetries := 3
	backoff := 1 * time.Second

	for attempt := 1; attempt <= maxRetries; attempt++ {
		atomic.AddInt64(&metrics.GetCollector().githubApiRequestsTotal, 1)

		// Reset request body on retry if it supports it
		if attempt > 1 && req.GetBody != nil {
			if body, bodyErr := req.GetBody(); bodyErr == nil {
				req.Body = body
			}
		}

		resp, err = s.httpClient.Do(req)
		if err == nil {
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
				remaining := resp.Header.Get("X-RateLimit-Remaining")
				if remaining == "0" || resp.StatusCode == http.StatusTooManyRequests {
					resetHeader := resp.Header.Get("X-RateLimit-Reset")
					retryAfterHeader := resp.Header.Get("Retry-After")
					
					var waitDuration time.Duration
					if retryAfterHeader != "" {
						if sec, convErr := strconv.Atoi(retryAfterHeader); convErr == nil {
							waitDuration = time.Duration(sec) * time.Second
						}
					}
					if waitDuration <= 0 && resetHeader != "" {
						if resetUnix, convErr := strconv.ParseInt(resetHeader, 10, 64); convErr == nil {
							waitDuration = time.Until(time.Unix(resetUnix, 0))
						}
					}
					if waitDuration <= 0 {
						waitDuration = backoff
					}
					
					if waitDuration > 10*time.Second {
						waitDuration = 10 * time.Second
					}
					
					slog.Warn("GitHub API rate limit hit, backing off", "url", req.URL.String(), "wait", waitDuration, "attempt", attempt)
					time.Sleep(waitDuration)
					backoff *= 2
					resp.Body.Close()
					continue
				}
			}

			if resp.StatusCode >= 500 && resp.StatusCode <= 504 {
				slog.Warn("GitHub API transient server error, retrying", "url", req.URL.String(), "status", resp.StatusCode, "attempt", attempt)
				time.Sleep(backoff)
				backoff *= 2
				resp.Body.Close()
				continue
			}

			return resp, nil
		}

		slog.Warn("GitHub API network error, retrying", "url", req.URL.String(), "error", err, "attempt", attempt)
		if attempt < maxRetries {
			time.Sleep(backoff)
			backoff *= 2
		}
	}

	atomic.AddInt64(&metrics.GetCollector().githubApiFailuresTotal, 1)

	if err != nil {
		return nil, err
	}
	return resp, fmt.Errorf("request failed after %d attempts, last status=%d", maxRetries, resp.StatusCode)
}
```
Replace all occurrences of `s.httpClient.Do(req)` in `github.go` with `s.doRequestWithRetry(req)`.

### 20. Webhook Replay Protection & Duplicate Account linking check

#### [MODIFY] [github_app.go](file:///home/afdaan/Documents/Code/laravel-paas/backend/internal/handlers/github_app.go)
Import `"github.com/laravel-paas/shared/pkg/metrics"`.

1. Add replay checks to `Webhook`:
```go
	metrics.GetCollector().IncrGithubWebhooksReceived()

	deliveryID := c.Get("X-GitHub-Delivery")
	if deliveryID != "" {
		key := fmt.Sprintf("github:webhook:processed:%s", deliveryID)
		ok, err := h.redisService.SetNX(key, true, 24*time.Hour)
		if err != nil {
			slog.Warn("Failed to check webhook delivery cache", "delivery_id", deliveryID, "error", err)
		} else if !ok {
			slog.Info("Duplicate webhook delivery detected, ignoring", "delivery_id", deliveryID)
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Duplicate event ignored"})
		}
	}
```
And increment `IncrGithubWebhooksProcessed()` before returning `fiber.StatusOK`.

2. Professional double account guard in `LinkInstallation`:
```go
	var existing models.GithubAppInstallation
	if err := h.db.Where("installation_id = ?", req.InstallationID).First(&existing).Error; err == nil {
		if existing.UserID != userID {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"error": "This GitHub account is already connected to another user profile. Please disconnect it from the other profile first or use a different GitHub account.",
			})
		}
	}
```

### 21. Multi-Tenant Installation & Repo Scoping Guard

#### [MODIFY] [actions.go](file:///home/afdaan/Documents/Code/laravel-paas/backend/internal/handlers/project/actions.go)
Import `"strings"` and `"github.com/laravel-paas/shared/infrastructure"`. Insert checking guard to `Create`:
```go
	if req.GithubInstallationID != nil && *req.GithubInstallationID != 0 {
		var localInst models.GithubAppInstallation
		if err := h.db.Where("installation_id = ? AND user_id = ?", *req.GithubInstallationID, userID).First(&localInst).Error; err != nil {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "The specified GitHub installation does not belong to your account",
			})
		}

		githubService := infrastructure.NewGithubService(h.cfg, h.redisService)
		repos, err := githubService.ListRepositories(*req.GithubInstallationID)
		if err != nil {
			slog.Warn("Failed to list repositories for validation", "installation_id", *req.GithubInstallationID, "error", err)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to verify repository access with GitHub. Please check your GitHub App configuration.",
			})
		}

		expectedFullName := fmt.Sprintf("%s/%s", req.GithubRepoOwner, req.GithubRepoName)
		found := false
		for _, r := range repos {
			if strings.EqualFold(r.FullName, expectedFullName) {
				found = true
				break
			}
		}

		if !found {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "The repository is not authorized or does not exist under the specified GitHub App installation.",
			})
		}
	}
```

---

## Verification Plan

### Automated Tests
* **Task 2 Failure Classification**: Trigger a deployment with a broken build command. Verify that the build failure is classified as `[BUILD_FAILED]` and does not interrupt the active running app.
* **Task 2 Database Rollback**: Trigger a deployment with a database migration syntax error. Verify that the failure is classified as `[MIGRATION_FAILED]` and rolls back correctly.
* **Task 2 Build Timeout**: Run a slow network build that exceeds the timeout. Verify that the context terminates, releases locks/leases, and prints `[TIMEOUT_EXCEEDED]` in the error logs.
* **Task 2 Advanced Healthcheck**: Modify a project environment variable and check the logs during the update to verify `AdvancedHealthcheck` HTTP verification is triggered.
* **Task 5 Webhook Replay Protection**: Send two HTTP POST requests to `/api/webhooks/github-app` with the same `X-GitHub-Delivery` header. Verify that the second request returns `200 OK` with the message `"Duplicate event ignored"`, and does not trigger a second deployment.
* **Task 5 Multi-Tenant Isolation**: Attempt to create a project specifying a `github_installation_id` owned by another user. Verify the request is rejected with a `403 Forbidden` error.
* **Task 5 GitHub API Retries**: Mock a transient 502 Bad Gateway response from the GitHub API. Verify that the system retries the request up to 3 times and logs the attempts before succeeding or reporting a failure.

