package docker

import (
	"strings"
	"testing"
)

func TestSanitizeBuildError(t *testing.T) {
	cases := []struct {
		name    string
		output  string
		want    string
		omitted string
	}{
		{
			name: "root cause outranks generic npm failure across streams",
			output: "INFO cache imports: []\nINFO cache exports: []\nnpm ERR! code ELIFECYCLE\n" +
				"npm ERR! errno 1\n\n/app/node_modules/esbuild/install.js:122\n} catch {\n^\n" +
				"SyntaxError: Unexpected token {\n    at createScript (vm.js:74:10)\n    at Object.runInThisContext (vm.js:116:10)\n",
			want:    "SyntaxError: Unexpected token {",
			omitted: "ELIFECYCLE",
		},
		{
			name:    "docker prefix and terminal formatting removed",
			output:  "#8 1.23 \x1b[31mSyntaxError: Unexpected token {\x1b[0m\n#8 1.24     at createScript (vm.js:74:10)",
			want:    "SyntaxError: Unexpected token {\nat createScript (vm.js:74:10)",
			omitted: "#8",
		},
		{
			name:    "empty cache metadata is not an actionable error",
			output:  "INFO cache imports: []\nINFO cache exports: []\n",
			want:    "Build failed. Check the build logs for details.",
			omitted: "INFO cache",
		},
		{
			name:    "unknown diagnostics preserved",
			output:  "INFO cache imports: []\nDependency installation aborted: disk full\nINFO cache exports: []",
			want:    "Dependency installation aborted: disk full",
			omitted: "INFO cache",
		},
		{
			name:    "typescript error outranks npm wrapper",
			output:  "npm ERR! code 1\nsrc/main.ts(3,7): error TS2322: Type 'string' is not assignable to type 'number'.\n",
			want:    "error TS2322:",
			omitted: "INFO cache",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			message := sanitizeBuildError(testCase.output)
			if !strings.Contains(message, testCase.want) || strings.Contains(message, testCase.omitted) || strings.Contains(message, "\x1b") {
				t.Fatalf("sanitizeBuildError() = %q", message)
			}
		})
	}
}
