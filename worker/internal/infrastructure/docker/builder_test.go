package docker

import "testing"

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
