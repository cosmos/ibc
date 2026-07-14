package wire

type MigrationUpResult struct {
	DB      string `json:"db"`
	Applied int    `json:"applied"`
}
