package config

import (
	"fmt"
	"os"
	"regexp"
)

var envRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ExpandEnvRefs resolves explicit ${NAME} references. Other dollar-sign forms are literals, and a
// reference to an unset variable is a configuration error.
func ExpandEnvRefs(value string) (string, error) {
	var expandErr error
	expanded := envRef.ReplaceAllStringFunc(value, func(ref string) string {
		name := ref[2 : len(ref)-1]
		resolved, ok := os.LookupEnv(name)
		if !ok && expandErr == nil {
			expandErr = fmt.Errorf("environment variable %s is not set", name)
		}
		return resolved
	})
	if expandErr != nil {
		return "", expandErr
	}
	return expanded, nil
}
