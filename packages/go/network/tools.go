package network

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

// ValidateListenAddr validates a listen address in the format host:port or :port
func ValidateListenAddr(raw string) error {
	if raw == "" {
		return fmt.Errorf("empty string provided")
	}

	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		return fmt.Errorf("expected address in host:port or :port form: %w", err)
	}

	if host != "" {
		if _, err := netip.ParseAddr(host); err != nil {
			return fmt.Errorf("host must be a valid IP address: %w", err)
		}
	}

	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("port must be numeric: %w", err)
	}

	if portNum < 0 || portNum > 65535 {
		return fmt.Errorf("port must be between 0 and 65535, got %d", portNum)
	}

	return nil
}
