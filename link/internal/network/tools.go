// Package network provides network-related tools.
package network

import (
	"fmt"
	"net"
	"strconv"
)

// ValidateListenAddr validates a listen address in the format host:port or :port
func ValidateListenAddr(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty string provided")
	}

	_, port, err := net.SplitHostPort(raw)
	if err != nil {
		return fmt.Errorf("expected address in host:port or :port form: %w", err)
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("port must be numeric: %w", err)
	}

	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", portNum)
	}

	return nil
}
