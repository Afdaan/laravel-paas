package workers

import (
	"os"
	"path/filepath"
	"testing"
)

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
}
