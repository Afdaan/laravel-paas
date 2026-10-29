package utils

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestLogRefinerKeepsDiagnosticsWithoutEmptyCacheMetadata(t *testing.T) {
	for _, trailingNewline := range []string{"", "\n"} {
		t.Run("trailing="+trailingNewline, func(t *testing.T) {
			var output bytes.Buffer
			refiner := NewLogRefiner(&output)
			input := "INFO cache imports: []\n#7 0.1 INFO cache exports: []\n" +
				"#7 0.2 /app/node_modules/esbuild/install.js:122\n" +
				"#7 0.3   } catch {\n#7 0.4     ^\n" +
				"#7 0.5 SyntaxError: Unexpected token {\n" +
				"#7 0.6     at createScript (vm.js:74:10)\nINFO cache imports: []" + trailingNewline
			for _, part := range []string{input[:17], input[17:]} {
				if _, err := refiner.Write([]byte(part)); err != nil {
					t.Fatal(err)
				}
			}
			if err := refiner.Flush(); err != nil {
				t.Fatal(err)
			}
			want := "/app/node_modules/esbuild/install.js:122\n  } catch {\n    ^\n" +
				"SyntaxError: Unexpected token {\n    at createScript (vm.js:74:10)\n"
			if output.String() != want {
				t.Fatalf("output = %q, want %q", output.String(), want)
			}
		})
	}
}

func TestLogRefinerHidesRailpackNoiseAndKeepsDependencySummary(t *testing.T) {
	var output bytes.Buffer
	refiner := NewLogRefiner(&output)
	input := "⚠ The `railpack.json` format is still evolving\n" +
		"→ Applying default exclude patterns: `.venv`, `venv`\n" +
		"Deploy\nStarting Docker Build...\n" +
		"  $ pip install -r requirements.txt\n" +
		"Collecting fastapi\n" +
		"Installing collected packages: typing-extensions, fastapi, uvicorn\n" +
		"Successfully installed fastapi-0.1 uvicorn-0.2\n"

	if _, err := refiner.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	if err := refiner.Flush(); err != nil {
		t.Fatal(err)
	}

	want := "> pip install\nDependencies: typing-extensions, fastapi, uvicorn\n✓ Python dependencies installed\n"
	if output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestLogRefinerKeepsRailpackFailureDiagnostics(t *testing.T) {
	var output bytes.Buffer
	refiner := NewLogRefiner(&output)
	line := "ERROR: Railpack failed to build image"

	if _, err := refiner.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	if output.String() != line+"\n" {
		t.Fatalf("failure diagnostic lost: %q", output.String())
	}
}

func TestLogRefinerKeepsNonemptyCacheMetadata(t *testing.T) {
	var output bytes.Buffer
	refiner := NewLogRefiner(&output)
	line := "INFO cache imports: [registry-cache]"
	if _, err := refiner.Write([]byte(line)); err != nil {
		t.Fatal(err)
	}
	if err := refiner.Flush(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), line) {
		t.Fatal("nonempty cache metadata lost")
	}
}

func TestBuildRunnerWritesSingleLineSummary(t *testing.T) {
	var lines []string
	_, err := RunWithRefinedLogAndEnvCtx(context.Background(), time.Second*5, nil, "", func(line string) {
		lines = append(lines, line)
	}, nil, "sh", "-c", "printf 'SyntaxError: Unexpected token {\\n'; exit 1")
	if err == nil {
		t.Fatal("expected failed build")
	}
	output := strings.Join(lines, "\n")
	if strings.Count(output, "[BUILD SUMMARY] Build failed in ") != 1 || strings.Contains(output, "========") {
		t.Fatalf("unexpected summary: %s", output)
	}
	if !strings.Contains(output, "SyntaxError: Unexpected token {") {
		t.Fatal("build diagnostic lost")
	}
}
