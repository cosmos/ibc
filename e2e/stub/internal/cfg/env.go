package cfg

import (
	"fmt"
	"os"
	"regexp"
)

var exactEnvRef = regexp.MustCompile(`^\$\{([A-Za-z_][A-Za-z0-9_]*)\}$`)

func resolveEnvRef(value string) (string, error) {
	match := exactEnvRef.FindStringSubmatch(value)
	if match == nil {
		return value, nil
	}

	resolved, ok := os.LookupEnv(match[1])
	if !ok {
		return "", fmt.Errorf("environment variable %s is not set", match[1])
	}
	return resolved, nil
}
