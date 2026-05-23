package utils

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

var (
	ErrUnterminatedQuote = errors.New("unterminated quote or escape sequence in command")
	ErrDangerousOperator = errors.New("dangerous shell control operator detected in command")
	ErrEmptyCommand      = errors.New("command cannot be empty")
)

// dangerousOperators defines unescaped shell control tokens forbidden in restricted execution mode
// to eliminate command injection vulnerabilities.
var dangerousOperators = []string{";", "&&", "||", "|", ">", "<", ">>"}

// SecureCommandParser provides quote-aware tokenization and validation of arbitrary command strings.
// Tokenizing arguments enables direct execution via syscalls (execve), bypassing intermediate shells (sh -c).
type SecureCommandParser struct {
	restricted bool
}

func NewSecureCommandParser(restricted bool) *SecureCommandParser {
	return &SecureCommandParser{restricted: restricted}
}

// Tokenize parses a command string into an exact argument slice respecting single and double quotes.
// If restricted mode is enabled, it actively rejects shell control operators and subshell expansions.
func (p *SecureCommandParser) Tokenize(command string) ([]string, error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return nil, ErrEmptyCommand
	}

	if p.restricted {
		if strings.Contains(trimmed, "$(") || strings.Contains(trimmed, "`") {
			return nil, fmt.Errorf("%w: subshell execution is restricted", ErrDangerousOperator)
		}
	}

	var args []string
	var current strings.Builder

	inSingleQuote := false
	inDoubleQuote := false
	escaped := false

	for _, r := range trimmed {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}

		if r == '\\' {
			if inSingleQuote {
				current.WriteRune(r)
			} else {
				escaped = true
			}
			continue
		}

		if r == '\'' {
			if inDoubleQuote {
				current.WriteRune(r)
			} else {
				inSingleQuote = !inSingleQuote
			}
			continue
		}

		if r == '"' {
			if inSingleQuote {
				current.WriteRune(r)
			} else {
				inDoubleQuote = !inDoubleQuote
			}
			continue
		}

		if unicode.IsSpace(r) {
			if inSingleQuote || inDoubleQuote {
				current.WriteRune(r)
			} else if current.Len() > 0 {
				token := current.String()
				if err := p.validateToken(token); err != nil {
					return nil, err
				}
				args = append(args, token)
				current.Reset()
			}
			continue
		}

		current.WriteRune(r)
	}

	if inSingleQuote || inDoubleQuote || escaped {
		return nil, ErrUnterminatedQuote
	}

	if current.Len() > 0 {
		token := current.String()
		if err := p.validateToken(token); err != nil {
			return nil, err
		}
		args = append(args, token)
	}

	return args, nil
}

func (p *SecureCommandParser) validateToken(token string) error {
	if !p.restricted {
		return nil
	}

	for _, op := range dangerousOperators {
		if token == op {
			return fmt.Errorf("%w: '%s'", ErrDangerousOperator, op)
		}
	}
	return nil
}

// ExpandEnv safely replaces ${VAR} or $VAR within a token against an explicit environment map.
func ExpandEnv(token string, env map[string]string) string {
	return osExpand(token, func(key string) string {
		val, exists := env[key]
		if !exists {
			return ""
		}
		return val
	})
}

func osExpand(s string, mapping func(string) string) string {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '$' && i+1 < len(s) {
			if s[i+1] == '{' {
				end := strings.IndexByte(s[i+2:], '}')
				if end != -1 {
					key := s[i+2 : i+2+end]
					buf.WriteString(mapping(key))
					i += 2 + end + 1
					continue
				}
			} else {
				j := i + 1
				for j < len(s) && (s[j] >= 'a' && s[j] <= 'z' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= '0' && s[j] <= '9' || s[j] == '_') {
					j++
				}
				key := s[i+1 : j]
				buf.WriteString(mapping(key))
				i = j
				continue
			}
		}
		buf.WriteByte(s[i])
		i++
	}
	return buf.String()
}
