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
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "package.json")
			if err := os.WriteFile(path, []byte(tc.manifest), 0644); err != nil {
				t.Fatal(err)
			}

			if got := detectJSFrameworkFromPackage(path); got != tc.expect {
				t.Fatalf("expected %s, got %s", tc.expect, got)
			}
		})
	}
}
