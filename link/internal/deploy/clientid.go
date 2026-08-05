package deploy

import "strings"

// DefaultClientID is the client ID used when the user does not supply one.
func DefaultClientID(counterpartyChainID string) string {
	return "link-" + counterpartyChainID
}

// ValidClientID reports whether id is a valid custom client identifier:
// 4-128 chars, alphanumerics plus [-._+#<>\[\]], and not in the generated
// "client-"/"channel-" namespaces reserved by routers.
func ValidClientID(id string) bool {
	if len(id) < 4 || len(id) > 128 ||
		strings.HasPrefix(id, "client-") || strings.HasPrefix(id, "channel-") {
		return false
	}
	for _, b := range []byte(id) {
		switch {
		case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		case b == '-', b == '.', b == '_', b == '+', b == '#', b == '[', b == ']', b == '<', b == '>':
		default:
			return false
		}
	}
	return true
}
