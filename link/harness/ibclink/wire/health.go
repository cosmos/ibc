package wire

// HealthPath is the daemon liveness/readiness probe endpoint. A readiness line implies this HTTP API is
// already serving status, relay, and health requests.
const HealthPath = "/health"
