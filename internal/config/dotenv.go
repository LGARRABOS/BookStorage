package config

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

// LoadDotEnvFile reads KEY=value lines from path and sets os.Setenv for each pair.
// It returns applied=true only if the file was read successfully.
// Missing file, or unreadable file (e.g. permission denied for the service user), returns (false, nil) so that
// processes started by systemd with EnvironmentFile= still work when the unit user cannot read the .env file on disk.
// Lines starting with # and empty lines are skipped.
func LoadDotEnvFile(path string) (applied bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, fs.ErrPermission) {
			return false, nil
		}
		return false, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if len(val) >= 2 {
			if val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			} else if val[0] == '\'' && val[len(val)-1] == '\'' {
				val = val[1 : len(val)-1]
			}
		}
		_ = os.Setenv(key, val)
	}
	return true, nil
}
