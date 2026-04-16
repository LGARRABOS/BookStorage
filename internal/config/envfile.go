package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveEnvFilePath returns the absolute path to the active .env file.
func ResolveEnvFilePath(root, flagConfig string) string {
	if strings.TrimSpace(flagConfig) != "" {
		p, err := filepath.Abs(flagConfig)
		if err != nil {
			return flagConfig
		}
		return p
	}
	p, err := filepath.Abs(filepath.Join(root, ".env"))
	if err != nil {
		return filepath.Join(root, ".env")
	}
	return p
}

// MergeEnvKeys updates or appends KEY=value lines in envFilePath.
func MergeEnvKeys(envFilePath string, updates map[string]string) error {
	if envFilePath == "" {
		return fmt.Errorf("empty env file path")
	}
	pending := map[string]string{}
	for k, v := range updates {
		pending[strings.ToUpper(strings.TrimSpace(k))] = fmt.Sprintf("%s=%s", strings.TrimSpace(k), escapeEnvValue(v))
	}

	var out strings.Builder
	raw, err := os.ReadFile(envFilePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(raw)))
		for sc.Scan() {
			line := sc.Text()
			s := strings.TrimSpace(line)
			if s == "" || strings.HasPrefix(s, "#") {
				out.WriteString(line)
				out.WriteByte('\n')
				continue
			}
			i := strings.IndexByte(s, '=')
			if i <= 0 {
				out.WriteString(line)
				out.WriteByte('\n')
				continue
			}
			key := strings.ToUpper(strings.TrimSpace(s[:i]))
			if rep, ok := pending[key]; ok {
				out.WriteString(rep)
				out.WriteByte('\n')
				delete(pending, key)
			} else {
				out.WriteString(line)
				out.WriteByte('\n')
			}
		}
	}
	for _, line := range pending {
		out.WriteString(line)
		out.WriteByte('\n')
	}

	tmp := envFilePath + ".tmp"
	if err := os.WriteFile(tmp, []byte(out.String()), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, envFilePath)
}

func escapeEnvValue(v string) string {
	if strings.ContainsAny(v, " \t\n\"'#") || v == "" {
		v = strings.ReplaceAll(v, `\`, `\\`)
		v = strings.ReplaceAll(v, `"`, `\"`)
		return `"` + v + `"`
	}
	return v
}
