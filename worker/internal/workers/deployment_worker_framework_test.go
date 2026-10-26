package workers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/worker/internal/infrastructure/docker"
)

func TestDetectProjectRuntimeEnvironment(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "project environment", true: "manual override"}[manual], func(t *testing.T) {
			root := t.TempDir()
			project := &models.Project{Framework: "Node.js", Subdomain: "test", NodeVersion: "20", IsManualVersion: manual}
			cfg := &config.Config{ProjectsPath: root}
			buildPath := project.GetProjectPath(root)
			if err := os.MkdirAll(buildPath, 0755); err != nil {
				t.Fatal(err)
			}
			for name, content := range map[string]string{
				".env":         "RAILPACK_NODE_VERSION=22\nAPI_KEY=do-not-log-my-api-key\n",
				"package.json": `{"scripts":{"start":"node server.js"}}`,
			} {
				if err := os.WriteFile(filepath.Join(buildPath, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			for name, script := range map[string]string{
				"docker":   "#!/bin/sh\nexit 0\n",
				"railpack": "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$RAILPACK_TEST_ARGS\"\nprintf '{\"success\":true,\"detectedProviders\":[\"node\"],\"resolvedPackages\":{\"node\":{\"requestedVersion\":\"%s\"}}}' \"$RAILPACK_NODE_VERSION\"\n",
			} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0755); err != nil {
					t.Fatal(err)
				}
			}
			argsPath := filepath.Join(root, "args")
			t.Setenv("PATH", root)
			t.Setenv("RAILPACK_TEST_ARGS", argsPath)
			worker := &DeploymentWorker{cfg: cfg, dockerService: docker.NewDockerService(cfg, nil, nil)}
			got := worker.detectProjectRuntime(context.Background(), project, buildPath)
			want := "22"
			if manual {
				want = "20"
			}
			if got.Framework != "Node.js" || got.RuntimeVersion != want || got.Source != "railpack" {
				t.Fatalf("runtime inspection = %#v, want Node %s through Railpack", got, want)
			}
			arguments, err := os.ReadFile(argsPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(arguments), "--env\nRAILPACK_NODE_VERSION\n") || strings.Contains(string(arguments), "do-not-log-my-api-key") {
				t.Fatalf("inspection must forward environment names without secrets: %s", arguments)
			}
		})
	}
}

func TestDetectJSFrameworkFromPackage(t *testing.T) {
	cases := map[string]struct {
		manifest string
		expect   string
	}{
		"next dependency": {
			manifest: `{"dependencies":{"next":"15.0.0","react":"19.0.0"}}`,
			expect:   "Next.js",
		},
		"react dependency": {
			manifest: `{"dependencies":{"react":"19.0.0","react-dom":"19.0.0"}}`,
			expect:   "React",
		},
		"plain node package": {
			manifest: `{"dependencies":{"express":"5.0.0"}}`,
			expect:   "Node.js",
		},
		"node server with react client": {
			manifest: `{"scripts":{"start":"node server.js"},"dependencies":{"express":"5.0.0","react":"19.0.0"}}`,
			expect:   "Node.js",
		},
		"typescript node package": {
			manifest: `{"devDependencies":{"typescript":"5.9.0"}}`,
			expect:   "Node.js",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "package.json")
			if err := os.WriteFile(path, []byte(testCase.manifest), 0644); err != nil {
				t.Fatal(err)
			}
			if got := detectJSFrameworkFromPackage(path); got != testCase.expect {
				t.Fatalf("detectJSFrameworkFromPackage() = %s, want %s", got, testCase.expect)
			}
		})
	}
}

func TestRuntimeDetectionFromRailpack(t *testing.T) {
	cases := []struct {
		name string
		info railpackProjectInfo
		want projectRuntimeDetection
	}{
		{
			name: "laravel wins over node tooling",
			info: railpackProjectInfo{
				DetectedProviders: []string{"node", "php"},
				Metadata:          map[string]string{"phpLaravel": "true"},
				ResolvedPackages: map[string]railpackPackage{
					"php":  {ResolvedVersion: "8.4.8"},
					"node": {ResolvedVersion: "22.16.0"},
				},
			},
			want: projectRuntimeDetection{Framework: "Laravel", Provider: "php", Runtime: "laravel", RuntimeVersion: "8.4", Source: "railpack"},
		},
		{
			name: "current Laravel metadata wins over node tooling",
			info: railpackProjectInfo{
				DetectedProviders: []string{"node", "php"},
				Metadata:          map[string]string{"phpFramework": "laravel"},
				ResolvedPackages:  map[string]railpackPackage{"php": {ResolvedVersion: "8.4.8"}},
			},
			want: projectRuntimeDetection{Framework: "PHP", Provider: "php", Runtime: "laravel", RuntimeVersion: "8.4", Source: "railpack"},
		},
		{
			name: "plain PHP stays native",
			info: railpackProjectInfo{
				DetectedProviders: []string{"php"},
				ResolvedPackages:  map[string]railpackPackage{"php": {ResolvedVersion: "8.4.8"}},
			},
			want: projectRuntimeDetection{Framework: "PHP", Provider: "php", Runtime: "php", RuntimeVersion: "8.4", Source: "railpack"},
		},
		{
			name: "next runtime",
			info: railpackProjectInfo{
				DetectedProviders: []string{"node"},
				Metadata:          map[string]string{"nodeRuntime": "next"},
				ResolvedPackages:  map[string]railpackPackage{"node": {RequestedVersion: "22"}},
			},
			want: projectRuntimeDetection{Framework: "Next.js", Provider: "node", Runtime: "next", RuntimeVersion: "22", Source: "railpack"},
		},
		{
			name: "python runtime metadata",
			info: railpackProjectInfo{
				DetectedProviders: []string{"python"},
				Metadata:          map[string]string{"pythonRuntime": "fastapi"},
				ResolvedPackages:  map[string]railpackPackage{"python": {ResolvedVersion: "3.13.5"}},
			},
			want: projectRuntimeDetection{Framework: "Python", Provider: "python", Runtime: "fastapi", RuntimeVersion: "3.13", Source: "railpack"},
		},
		{
			name: "dotnet native provider",
			info: railpackProjectInfo{
				DetectedProviders: []string{"dotnet"},
				ResolvedPackages:  map[string]railpackPackage{"dotnet": {ResolvedVersion: "8.0.18"}},
			},
			want: projectRuntimeDetection{Framework: ".NET", Provider: "dotnet", Runtime: "dotnet", RuntimeVersion: "8.0", Source: "railpack"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := runtimeDetectionFromRailpack(testCase.info, t.TempDir())
			if got != testCase.want {
				t.Fatalf("runtimeDetectionFromRailpack() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

func TestIsLaravelManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "artisan"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	composer := `{"require":{"php":"^8.3","laravel/framework":"^12.0"}}`
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(composer), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vite":"6.0.0"}}`), 0644); err != nil {
		t.Fatal(err)
	}

	if !isLaravelManifest(root) {
		t.Fatal("isLaravelManifest() = false, want true")
	}
	info := railpackProjectInfo{
		DetectedProviders: []string{"node", "php"},
		ResolvedPackages:  map[string]railpackPackage{"php": {ResolvedVersion: "8.4.8"}},
	}
	want := projectRuntimeDetection{Framework: "PHP", Provider: "php", Runtime: "laravel", RuntimeVersion: "8.4", Source: "railpack"}
	if got := runtimeDetectionFromRailpack(info, root); got != want {
		t.Fatalf("Laravel manifest fallback = %#v, want %#v", got, want)
	}
}
