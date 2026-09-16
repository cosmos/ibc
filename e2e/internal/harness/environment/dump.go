// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	EnvDumpEnabled = "E2E_DUMP"
	EnvDumpDir     = "E2E_DUMP_DIR"
)

const defaultDumpDir = "./dumps/"

var (
	dumpEnabled bool
	dumpDir     string
)

func DumpEnabled() bool {
	return dumpEnabled
}

func init() {
	enabled, dir, err := setupDumpEnabled()
	if err != nil {
		slog.Error("E2E dumps: failed to initialize", "error", err)
		return
	}

	dumpEnabled = enabled
	dumpDir = dir

	if enabled {
		slog.Info("E2E_DUMP enabled", "dumpDir", dumpDir)
	}
}

func setupDumpEnabled() (bool, string, error) {
	boolStr, exists := os.LookupEnv(EnvDumpEnabled)
	if !exists {
		return false, "", nil
	}

	enabled, err := strconv.ParseBool(boolStr)
	if err != nil {
		return false, "", fmt.Errorf("failed to parse %s: %w", EnvDumpEnabled, err)
	}
	if !enabled {
		return false, "", nil
	}

	dir, exists := os.LookupEnv(EnvDumpDir)
	if !exists {
		dir = defaultDumpDir
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false, "", fmt.Errorf("failed to get absolute path of %s: %w", dir, err)
	}

	return true, absDir, nil
}

func dumpDirectory(runID, path string) {
	dest, err := tryDumpDirectory(runID, path)
	switch {
	case err != nil:
		slog.Error("E2E dumps: failed to dump directory", "runID", runID, "path", path, "error", err)
		return
	case dest == "":
		slog.Info("E2E dumps: no files to dump", "runID", runID, "path", path)
	default:
		slog.Info("E2E dumps: dumped directory", "runID", runID, "dest", dest)
	}
}

func tryDumpDirectory(runID, path string) (string, error) {
	switch {
	case dumpDir == "":
		return "", fmt.Errorf("dump directory is not configured")
	case runID == "":
		return "", fmt.Errorf("run ID is empty")
	case path == "":
		return "", fmt.Errorf("dump source path is empty")
	}

	src, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve dump source: %w", err)
	}

	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("stat dump source: %w", err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("dump source is not a directory: %s", src)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return "", fmt.Errorf("read dump source: %w", err)
	}
	if len(entries) == 0 {
		return "", nil
	}

	runDir, err := getRunDumpDir(runID)
	if err != nil {
		return "", err
	}

	dest := filepath.Join(dumpDir, runDir, filepath.Base(src))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", fmt.Errorf("create dump run directory: %w", err)
	}

	if err := os.CopyFS(dest, os.DirFS(src)); err != nil {
		return "", fmt.Errorf("copy dump directory: %w", err)
	}

	return dest, nil
}

// getRunDumpDir maps workspace runID "<pid>-<unix-nano>" to "pid-<pid>/<unix-nano>"
// so dumps from one go test process share a directory.
func getRunDumpDir(runID string) (string, error) {
	pid, ts, ok := strings.Cut(runID, "-")
	if !ok || pid == "" || ts == "" {
		return "", fmt.Errorf("invalid run ID %q", runID)
	}

	return filepath.Join("pid-"+pid, ts), nil
}
