package docker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/laravel-paas/shared/config"
	"github.com/laravel-paas/shared/models"
	"github.com/laravel-paas/shared/pkg/utils"
)

func TestResolveRailpackNodeVersion(t *testing.T) {
	cases := []struct {
		framework       string
		nodeVersion     string
		languageVersion string
		want            string
	}{
		{framework: "PHP", languageVersion: "8.4"},
		{framework: "Laravel", languageVersion: "8.4"},
		{framework: "PHP", nodeVersion: "22", languageVersion: "8.4", want: "22"},
		{framework: "React", nodeVersion: "20", languageVersion: "22", want: "20"},
		{framework: "Other", languageVersion: "8.4"},
		{framework: "Other", nodeVersion: "22", languageVersion: "8.4", want: "22"},
		{languageVersion: "8.4"},
		{nodeVersion: " 22 \n", languageVersion: "8.4", want: "22"},
		{framework: "Node.js", nodeVersion: " \t", languageVersion: " 22 \n", want: "22"},
		{framework: "PHP", nodeVersion: " \t", languageVersion: "8.4"},
		{framework: "Node.js"},
		{framework: "Node.js", languageVersion: "22", want: "22"},
		{framework: "Next.js", languageVersion: "22", want: "22"},
		{framework: "Vite", languageVersion: "22", want: "22"},
		{framework: "React", languageVersion: "22", want: "22"},
		{framework: "Vue", languageVersion: "22", want: "22"},
		{framework: "Nuxt.js", languageVersion: "22", want: "22"},
		{framework: "Svelte", languageVersion: "22", want: "22"},
		{framework: "Angular", languageVersion: "22", want: "22"},
		{framework: "TypeScript", languageVersion: "22", want: "22"},
	}
	for _, testCase := range cases {
		t.Run(testCase.framework+"/"+testCase.nodeVersion+"/"+testCase.languageVersion, func(t *testing.T) {
			project := &models.Project{
				Framework: testCase.framework, NodeVersion: testCase.nodeVersion, LanguageVersion: testCase.languageVersion,
			}
			if got := resolveRailpackNodeVersion(project); got != testCase.want {
				t.Fatalf("resolveRailpackNodeVersion() = %q, want %q", got, testCase.want)
			}
		})
	}
}

func TestRailpackBuildNodeVersion(t *testing.T) {
	cases := []struct {
		name            string
		framework       string
		nodeVersion     string
		languageVersion string
		envVersion      string
		want            string
	}{
		{name: "php defers to railpack", framework: "PHP", languageVersion: "8.4", want: "unset"},
		{name: "php explicit node", framework: "PHP", nodeVersion: "22", languageVersion: "8.4", want: "22"},
		{name: "node language fallback", framework: "Node.js", languageVersion: "22", want: "22"},
		{name: "unknown defers to railpack", framework: "Other", languageVersion: "8.4", want: "unset"},
		{name: "empty framework defers to railpack", languageVersion: "8.4", want: "unset"},
		{name: "preserve project env", framework: "PHP", languageVersion: "8.4", envVersion: "24", want: "24"},
		{name: "model overrides project env", framework: "PHP", nodeVersion: "22", languageVersion: "8.4", envVersion: "24", want: "22"},
	}
	for _, testCase := range cases {
		for _, exitCode := range []string{"0", "1"} {
			for _, nodeFile := range []string{"", "24\n"} {
				t.Run(testCase.name+"/exit="+exitCode+"/file="+nodeFile, func(t *testing.T) {
					root := t.TempDir()
					project := &models.Project{
						Framework: testCase.framework, NodeVersion: testCase.nodeVersion, LanguageVersion: testCase.languageVersion,
						Subdomain: "test", StartCommand: "npm start",
					}
					service := &DockerService{cfg: &config.Config{
						ProjectsPath: root, RailpacksPath: filepath.Join("..", "..", "..", "..", "railpacks"),
					}}
					buildPath := project.GetProjectPath(root)
					if err := os.MkdirAll(buildPath, 0755); err != nil {
						t.Fatal(err)
					}
					files := map[string]string{
						"package.json":      `{"scripts":{"start":"node server.js"}}`,
						".env":              "RAILPACK_NODE_VERSION=" + testCase.envVersion + "\n",
						"bun.lock":          "bun lockfile\n",
						"yarn.lock":         "yarn lockfile\n",
						"pnpm-lock.yaml":    "pnpm lockfile\n",
						"package-lock.json": "npm lockfile\n",
					}
					if testCase.envVersion == "" {
						files[".env"] = ""
					}
					files[".env"] += "API_KEY=do-not-log-my-api-key\n"
					if nodeFile != "" {
						files[".node-version"] = nodeFile
					}
					for name, content := range files {
						if err := os.WriteFile(filepath.Join(buildPath, name), []byte(content), 0644); err != nil {
							t.Fatal(err)
						}
					}
					capturePath := filepath.Join(root, "node-version-env")
					commands := map[string]string{
						"railpack": "#!/bin/sh\nprintf '%s' \"${RAILPACK_NODE_VERSION-unset}\" > \"$RAILPACK_TEST_CAPTURE\"\nprintf '%s\\n' \"$@\" > \"$RAILPACK_TEST_ARGS\"\nprintf 'INFO cache imports: []\\nINFO cache exports: []\\n' >&2\nif [ " + exitCode + " -ne 0 ]; then printf 'SyntaxError: Unexpected token {\\n'; fi\nexit " + exitCode + "\n",
						"docker":   "#!/bin/sh\nexit 0\n",
					}
					for name, script := range commands {
						if err := os.WriteFile(filepath.Join(root, name), []byte(script), 0755); err != nil {
							t.Fatal(err)
						}
					}
					t.Setenv("RAILPACK_NODE_VERSION", "")
					if err := os.Unsetenv("RAILPACK_NODE_VERSION"); err != nil {
						t.Fatal(err)
					}
					t.Setenv("PATH", root)
					t.Setenv("RAILPACK_TEST_CAPTURE", capturePath)
					argsPath := filepath.Join(root, "railpack-args")
					t.Setenv("RAILPACK_TEST_ARGS", argsPath)
					err := service.railpackBuild(context.Background(), project, buildPath, "test-image", filepath.Join(root, "build.log"), false, nil)
					if (err != nil) != (exitCode != "0") {
						t.Fatalf("railpackBuild() error = %v, exit code = %s", err, exitCode)
					}
					if err != nil && (!strings.Contains(err.Error(), "[BUILD_FAILED] SyntaxError: Unexpected token {") || strings.Contains(err.Error(), "INFO cache")) {
						t.Fatalf("build error must include stdout diagnostic, not stderr cache metadata: %v", err)
					}
					captured, err := os.ReadFile(capturePath)
					if err != nil || string(captured) != testCase.want {
						t.Fatalf("Node override = %q, want %q, read error = %v", captured, testCase.want, err)
					}
					arguments, err := os.ReadFile(argsPath)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(arguments), "--env\nRAILPACK_NODE_VERSION\n") != (testCase.want != "unset") || strings.Contains(string(arguments), "do-not-log-my-api-key") {
						t.Fatalf("Railpack must receive environment names, never secret values: %s", arguments)
					}
					content, err := os.ReadFile(filepath.Join(buildPath, ".node-version"))
					if nodeFile == "" {
						if !os.IsNotExist(err) {
							t.Fatalf(".node-version must stay absent, got %q, error = %v", content, err)
						}
					} else if err != nil || string(content) != nodeFile {
						t.Fatalf(".node-version changed: got %q, want %q, error = %v", content, nodeFile, err)
					}
					for _, name := range []string{"bun.lock", "yarn.lock", "pnpm-lock.yaml", "package-lock.json"} {
						content, err := os.ReadFile(filepath.Join(buildPath, name))
						if err != nil || string(content) != files[name] {
							t.Fatalf("%s changed: got %q, want %q, error = %v", name, content, files[name], err)
						}
					}
				})
			}
		}
	}
}

func TestRailpackEnvArgs(t *testing.T) {
	args, values := RailpackEnvArgs(map[string]string{"SECRET": "private-value", "RAILPACK_NODE_VERSION": "22"})
	if strings.Join(args, " ") != "--env RAILPACK_NODE_VERSION --env SECRET" || strings.Join(values, " ") != "RAILPACK_NODE_VERSION=22 SECRET=private-value" {
		t.Fatalf("unexpected environment forwarding: args=%q, environment count=%d", args, len(values))
	}
}

func TestRailpackNativeRuntimePlan(t *testing.T) {
	binary := os.Getenv("RAILPACK_TEST_BINARY")
	if binary == "" {
		t.Skip("set RAILPACK_TEST_BINARY to verify plans with the pinned Railpack CLI")
	}
	for _, testCase := range []struct {
		name    string
		project models.Project
		want    string
		phpOnly bool
	}{
		{name: "node override differs from language version", project: models.Project{Framework: "Node.js", NodeVersion: "20", LanguageVersion: "22"}, want: "20"},
		{name: "PHP without a Node override", project: models.Project{Framework: "PHP", LanguageVersion: "8.4"}},
		{name: "PHP with an explicit Node override", project: models.Project{Framework: "PHP", NodeVersion: "22", LanguageVersion: "8.4"}, want: "22"},
		{name: "PHP without frontend assets", project: models.Project{Framework: "PHP", LanguageVersion: "8.4"}, phpOnly: true},
		{name: "unknown framework without a Node override", project: models.Project{Framework: "Other", LanguageVersion: "8.4"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{}
			if !testCase.phpOnly {
				files["package.json"] = `{"name":"runtime-check","scripts":{"start":"node server.js"}}`
			}
			if testCase.project.Framework == "PHP" {
				files["composer.json"] = `{"require":{"php":"^8.4","laravel/framework":"^12.0"}}`
				files["artisan"] = ""
			} else {
				service := &DockerService{cfg: &config.Config{RailpacksPath: filepath.Join("..", "..", "..", "..", "railpacks")}}
				service.injectDefaultRailpackConfig(root, "", "node server.js", "nodejs", &testCase.project, "", false)
			}
			for name, content := range files {
				if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			envs := map[string]string{}
			if version := resolveRailpackNodeVersion(&testCase.project); version != "" {
				envs["RAILPACK_NODE_VERSION"] = version
			}
			envArgs, envSlice := RailpackEnvArgs(envs)
			args := append([]string{"info", "--format", "json", "--show-plan"}, envArgs...)
			result, err := utils.RunWithEnvCtx(context.Background(), time.Minute, envSlice, binary, append(args, root)...)
			if err != nil {
				t.Fatalf("Railpack planning failed: %v: %s", err, result.Stderr)
			}
			var info struct {
				Success          bool `json:"success"`
				ResolvedPackages map[string]struct {
					RequestedVersion string `json:"requestedVersion"`
					ResolvedVersion  string `json:"resolvedVersion"`
				} `json:"resolvedPackages"`
				Plan struct {
					Deploy struct {
						Inputs []struct {
							Step string `json:"step"`
						} `json:"inputs"`
					} `json:"deploy"`
				} `json:"plan"`
			}
			if err := json.Unmarshal([]byte(result.Stdout), &info); err != nil {
				t.Fatal(err)
			}
			node := info.ResolvedPackages["node"]
			if testCase.phpOnly {
				if !info.Success || node.ResolvedVersion != "" || info.ResolvedPackages["php"].ResolvedVersion == "" {
					t.Fatal("PHP-only plan must resolve PHP without injecting Node")
				}
				return
			}
			if !info.Success || node.ResolvedVersion == "" || strings.HasPrefix(node.ResolvedVersion, "8.") || (testCase.want != "" && node.RequestedVersion != testCase.want) {
				t.Fatalf("incorrect resolved Node version: requested=%q resolved=%q success=%v", node.RequestedVersion, node.ResolvedVersion, info.Success)
			}
			if testCase.project.Framework != "PHP" {
				hasNodeRuntime := false
				for _, input := range info.Plan.Deploy.Inputs {
					if input.Step == "packages:mise" {
						hasNodeRuntime = true
					}
				}
				if !hasNodeRuntime {
					t.Fatal("deploy image must import the same runtime packages used by the build")
				}
			}
		})
	}
}

func TestInjectDefaultRailpackNodeConfig(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		project models.Project
	}{
		{name: "explicit node wins", project: models.Project{Framework: "Node.js", NodeVersion: "20", LanguageVersion: "22"}},
		{name: "unknown framework cannot inject PHP", project: models.Project{Framework: "Other", LanguageVersion: "8.4"}},
		{name: "native version resolution", project: models.Project{Framework: "Node.js"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			service := &DockerService{cfg: &config.Config{RailpacksPath: filepath.Join("..", "..", "..", "..", "railpacks")}}
			service.injectDefaultRailpackConfig(root, "", "node server.js", "nodejs", &testCase.project, "", false)
			content, err := os.ReadFile(filepath.Join(root, "railpack.json"))
			if err != nil {
				t.Fatal(err)
			}
			var generated struct {
				Deploy struct {
					Base         json.RawMessage `json:"base"`
					StartCommand string          `json:"startCommand"`
				} `json:"deploy"`
			}
			if err := json.Unmarshal(content, &generated); err != nil {
				t.Fatal(err)
			}
			if len(generated.Deploy.Base) != 0 || generated.Deploy.StartCommand != "node server.js" || strings.Contains(string(content), "{{VERSION}}") {
				t.Fatalf("config must preserve native runtime selection and the start command: %s", content)
			}
		})
	}
}

func TestCommandReferencesPath(t *testing.T) {
	cases := []struct {
		name       string
		entrypoint []string
		command    []string
		want       bool
	}{
		{name: "shell command", command: []string{"/bin/bash", "-c", "/start-container.sh"}, want: true},
		{name: "entrypoint", entrypoint: []string{"/start-container.sh"}, want: true},
		{name: "quoted shell command", command: []string{"/bin/bash", "-c", "exec '/start-container.sh'"}, want: true},
		{name: "similar path", command: []string{"/start-container.sh.bak"}, want: false},
		{name: "unrelated command", command: []string{"node", "server.js"}, want: false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := commandReferencesPath(testCase.entrypoint, testCase.command, "/start-container.sh"); got != testCase.want {
				t.Fatalf("commandReferencesPath() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestShouldUseStaticNodeHosting(t *testing.T) {
	if shouldUseStaticNodeHosting(true, true, true, true) {
		t.Fatal("start script must prevent static hosting")
	}
	if !shouldUseStaticNodeHosting(false, false, true, false) {
		t.Fatal("build-only package should use static hosting")
	}
}

func TestUsesNodeBuildTemplate(t *testing.T) {
	if !usesNodeBuildTemplate("Next.js") {
		t.Fatal("Next.js should use Node build template")
	}
	for _, framework := range []string{"Laravel", "PHP", "Ruby", "Rust", "Java", ".NET", "Deno"} {
		if usesNodeBuildTemplate(framework) {
			t.Fatalf("%s must use native Railpack planning", framework)
		}
	}
}
