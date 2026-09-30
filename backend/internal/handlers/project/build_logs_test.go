package project

import "testing"

func TestTailLines(t *testing.T) {
	cases := []struct {
		name         string
		input        string
		n            int
		partialFirst bool
		want         string
	}{
		{
			name:         "drops the leading fragment of a mid-file window",
			input:        "ent of a line\nsecond\nthird\n",
			n:            5,
			partialFirst: true,
			want:         "second\nthird",
		},
		{
			name:  "returns everything when fewer lines exist than requested",
			input: "one\ntwo\n",
			n:     10,
			want:  "one\ntwo",
		},
		{
			name:  "a trailing newline produces no empty final line",
			input: "one\ntwo\nthree\n",
			n:     2,
			want:  "two\nthree",
		},
		{
			name:         "a whole-file window keeps its first line",
			input:        "one\ntwo\n",
			n:            5,
			partialFirst: false,
			want:         "one\ntwo",
		},
		{
			name:         "a window with no line break is kept as a truncated line",
			input:        "no break at all",
			n:            3,
			partialFirst: true,
			want:         "no break at all",
		},
		{
			name:  "an empty file yields an empty tail",
			input: "",
			n:     6,
			want:  "",
		},
		{
			name:  "a file of only newlines yields an empty tail",
			input: "\n\n\n",
			n:     6,
			want:  "",
		},
		{
			name:  "n of zero leaves the input untouched",
			input: "one\ntwo\n",
			n:     0,
			want:  "one\ntwo\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tailLines(tc.input, tc.n, tc.partialFirst); got != tc.want {
				t.Fatalf("tailLines(%q, %d, %v) = %q, want %q", tc.input, tc.n, tc.partialFirst, got, tc.want)
			}
		})
	}
}

// The clamp is what keeps the read window bounded; it lives in the handler, so
// pin the arithmetic that depends on it.
func TestTailReadWindowStaysUnderResponseCap(t *testing.T) {
	const maxBytes = 256 * 1024
	if maxTailLines*bytesPerTailLine+tailReadFloor > maxBytes {
		t.Fatalf("a clamped tail request may read %d bytes, above the %d cap",
			maxTailLines*bytesPerTailLine+tailReadFloor, maxBytes)
	}
}
