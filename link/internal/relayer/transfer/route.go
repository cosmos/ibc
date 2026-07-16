package transfer

// Route identifies the client pair a pipeline relays.
type Route struct {
	SourceChainID       string
	SourceClientID      string
	DestinationChainID  string
	DestinationClientID string
}
