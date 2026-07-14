package environment

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"
)

type workspace struct {
	privateDir     string
	diagnosticsDir string
	runID          string
}

func newWorkspace() (workspace, error) {
	privateDir, err := os.MkdirTemp("", "ibc-environment-private-")
	if err != nil {
		return workspace{}, fmt.Errorf("create private environment work directory: %w", err)
	}
	diagnosticsDir, err := os.MkdirTemp("", "ibc-environment-diagnostics-")
	if err != nil {
		return workspace{}, errors.Join(
			fmt.Errorf("create environment diagnostics directory: %w", err),
			removeDirectory("private environment work", privateDir),
		)
	}
	return workspace{
		privateDir:     privateDir,
		diagnosticsDir: diagnosticsDir,
		runID:          fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano()),
	}, nil
}

func (w workspace) remove() error {
	return errors.Join(w.removePrivate(), w.removeDiagnostics())
}

func (w workspace) removePrivate() error {
	return removeDirectory("private environment work", w.privateDir)
}

func (w workspace) removeDiagnostics() error {
	return removeDirectory("environment diagnostics", w.diagnosticsDir)
}

func removeDirectory(label, path string) error {
	if path == "" {
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove %s directory: %w", label, err)
	}
	return nil
}

// resourcePathToken keeps authored identities out of filesystem paths. IDs
// are domain values and may legitimately contain separators or dot segments.
func resourcePathToken(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:8])
}
