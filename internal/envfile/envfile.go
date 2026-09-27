// Package envfile loads KEY=VALUE files into the process environment.
// Variables already set in the process are left unchanged.
package envfile

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"unicode"
)

// Load reads path and exports each assignment that is not already present.
// A missing file is not an error.
func Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		key, val, ok, err := parseLine(sc.Text())
		if err != nil {
			return fmt.Errorf("env file line %d: %w", lineNo, err)
		}
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, val); err != nil {
			return fmt.Errorf("set env %s: %w", key, err)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read env file: %w", err)
	}
	return nil
}

func parseLine(raw string) (key, val string, ok bool, err error) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	name, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false, fmt.Errorf("missing '='")
	}
	name = strings.TrimSpace(name)
	if name == "" || !validName(name) {
		return "", "", false, fmt.Errorf("invalid name")
	}
	value = strings.TrimSpace(value)
	value, err = unquote(value)
	if err != nil {
		return "", "", false, err
	}
	return name, value, true, nil
}

func validName(name string) bool {
	for i, r := range name {
		if r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r)) {
			continue
		}
		return false
	}
	return true
}

func unquote(value string) (string, error) {
	if len(value) < 2 {
		return value, nil
	}
	quote := value[0]
	if (quote != '"' && quote != '\'') || value[len(value)-1] != quote {
		return value, nil
	}
	inner := value[1 : len(value)-1]
	if quote == '\'' {
		return inner, nil
	}
	var b strings.Builder
	b.Grow(len(inner))
	escaped := false
	for _, r := range inner {
		if escaped {
			switch r {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteRune(r)
			}
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	if escaped {
		return "", fmt.Errorf("dangling escape")
	}
	return b.String(), nil
}
