package wire

import (
	"fmt"
	"strings"
)

type AppType string

const (
	AppTypeIFT AppType = "IFT"
	AppTypeGMP AppType = "GMP"
)

// App type separates the independent IFT and GMP sequence spaces.
func PacketID(routeID string, app AppType, seq uint64) string {
	return fmt.Sprintf("%s-%s-%d", routeID, strings.ToLower(string(app)), seq)
}
