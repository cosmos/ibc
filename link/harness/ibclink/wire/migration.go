package wire

// MigrationUpResult is emitted by `ibc migrate up`.
type MigrationUpResult struct {
	DB      string `json:"db"`
	Applied int    `json:"applied"`
}
