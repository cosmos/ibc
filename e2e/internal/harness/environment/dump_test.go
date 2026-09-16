// SPDX-License-Identifier: Apache-2.0

package environment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRunDumpDir(t *testing.T) {
	for _, tt := range []struct {
		name        string
		runID       string
		expected    string
		errContains string
	}{
		{
			name:     "valid pid-timestamp",
			runID:    "1234-9876543210",
			expected: filepath.Join("pid-1234", "9876543210"),
		},
		{
			name:     "single digit pid",
			runID:    "1-2",
			expected: filepath.Join("pid-1", "2"),
		},
		{
			name:     "hyphen in timestamp preserved",
			runID:    "42-abc-def",
			expected: filepath.Join("pid-42", "abc-def"),
		},
		{
			name:        "no separator",
			runID:       "noseparator",
			errContains: `invalid run ID "noseparator"`,
		},
		{
			name:        "empty string",
			runID:       "",
			errContains: `invalid run ID ""`,
		},
		{
			name:        "leading separator only",
			runID:       "-trailing",
			errContains: `invalid run ID "-trailing"`,
		},
		{
			name:        "trailing separator only",
			runID:       "leading-",
			errContains: `invalid run ID "leading-"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			result, err := getRunDumpDir(tt.runID)

			// ASSERT
			if tt.errContains != "" {
				require.ErrorContains(t, err, tt.errContains)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSanitizeDumpSegment(t *testing.T) {
	for _, tt := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no special characters",
			input:    "simple-name",
			expected: "simple-name",
		},
		{
			name:     "forward slashes replaced",
			input:    "Test/SubTest/Case",
			expected: "Test_SubTest_Case",
		},
		{
			name:     "backslashes replaced",
			input:    `Test\SubTest\Case`,
			expected: "Test_SubTest_Case",
		},
		{
			name:     "trailing whitespace trimmed",
			input:    "  name  ",
			expected: "name",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "all slashes",
			input:    "///",
			expected: "___",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// ACT
			result := sanitizeDumpSegment(tt.input)

			// ASSERT
			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("list separator replaced", func(t *testing.T) {
		// filepath.ListSeparator is ':' on unix, ';' on windows.
		input := string([]rune{'a', filepath.ListSeparator, 'b'})

		// ACT
		result := sanitizeDumpSegment(input)

		// ASSERT
		assert.Equal(t, "a_b", result)
	})

	t.Run("list separator platform specific", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			assert.Equal(t, "a_b", sanitizeDumpSegment("a;b"))
		} else {
			assert.Equal(t, "a_b", sanitizeDumpSegment("a:b"))
		}
	})
}

func TestSetupDumpEnabled(t *testing.T) {
	t.Run("not set", func(t *testing.T) {
		// ARRANGE
		t.Setenv(EnvDumpEnabled, "")
		require.NoError(t, os.Unsetenv(EnvDumpEnabled))

		// ACT
		enabled, dir, err := setupDumpEnabled()

		// ASSERT
		require.NoError(t, err)
		assert.False(t, enabled)
		assert.Empty(t, dir)
	})

	t.Run("set to true without dir", func(t *testing.T) {
		// ARRANGE
		t.Setenv(EnvDumpEnabled, "true")
		require.NoError(t, os.Unsetenv(EnvDumpDir))

		// ACT
		enabled, dir, err := setupDumpEnabled()

		// ASSERT
		require.NoError(t, err)
		assert.True(t, enabled)

		absDefault, absErr := filepath.Abs(defaultDumpDir)
		require.NoError(t, absErr)
		assert.Equal(t, absDefault, dir)
	})

	t.Run("set to true with custom dir", func(t *testing.T) {
		// ARRANGE
		customDir := t.TempDir()
		t.Setenv(EnvDumpEnabled, "1")
		t.Setenv(EnvDumpDir, customDir)

		// ACT
		enabled, dir, err := setupDumpEnabled()

		// ASSERT
		require.NoError(t, err)
		assert.True(t, enabled)
		assert.Equal(t, customDir, dir)
	})

	t.Run("set to false", func(t *testing.T) {
		// ARRANGE
		t.Setenv(EnvDumpEnabled, "false")

		// ACT
		enabled, dir, err := setupDumpEnabled()

		// ASSERT
		require.NoError(t, err)
		assert.False(t, enabled)
		assert.Empty(t, dir)
	})

	t.Run("invalid boolean", func(t *testing.T) {
		// ARRANGE
		t.Setenv(EnvDumpEnabled, "notabool")

		// ACT
		_, _, err := setupDumpEnabled()

		// ASSERT
		require.ErrorContains(t, err, "failed to parse "+EnvDumpEnabled)
	})
}

func TestTryDumpDirectory(t *testing.T) {
	t.Run("errors on empty dumpDir", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = ""
		t.Cleanup(func() { dumpDir = saved })

		// ACT
		dest, err := tryDumpDirectory("1-2", t.TempDir(), "")

		// ASSERT
		require.ErrorContains(t, err, "dump directory is not configured")
		assert.Empty(t, dest)
	})

	t.Run("errors on empty runID", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		// ACT
		dest, err := tryDumpDirectory("", t.TempDir(), "")

		// ASSERT
		require.ErrorContains(t, err, "run ID is empty")
		assert.Empty(t, dest)
	})

	t.Run("errors on empty path", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		// ACT
		dest, err := tryDumpDirectory("1-2", "", "")

		// ASSERT
		require.ErrorContains(t, err, "dump source path is empty")
		assert.Empty(t, dest)
	})

	t.Run("errors on nonexistent source", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		// ACT
		dest, err := tryDumpDirectory("1-2", filepath.Join(t.TempDir(), "missing"), "")

		// ASSERT
		require.ErrorContains(t, err, "stat dump source")
		assert.Empty(t, dest)
	})

	t.Run("errors when source is a file", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		src := filepath.Join(t.TempDir(), "afile")
		require.NoError(t, os.WriteFile(src, []byte("data"), 0o644))

		// ACT
		dest, err := tryDumpDirectory("1-2", src, "")

		// ASSERT
		require.ErrorContains(t, err, "dump source is not a directory")
		assert.Empty(t, dest)
	})

	t.Run("returns empty for empty directory", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		src := t.TempDir()

		// ACT
		dest, err := tryDumpDirectory("1-2", src, "")

		// ASSERT
		require.NoError(t, err)
		assert.Empty(t, dest)
	})

	t.Run("copies populated directory", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		src := filepath.Join(t.TempDir(), "logs")
		require.NoError(t, os.MkdirAll(src, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(src, "output.log"), []byte("hello"), 0o644))

		// ACT
		dest, err := tryDumpDirectory("42-1000", src, "")

		// ASSERT
		require.NoError(t, err)
		require.NotEmpty(t, dest)

		expected := filepath.Join(dumpDir, "pid-42", "1000", "logs")
		assert.Equal(t, expected, dest)

		content, readErr := os.ReadFile(filepath.Join(dest, "output.log"))
		require.NoError(t, readErr)
		assert.Equal(t, "hello", string(content))
	})

	t.Run("includes test name in path", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		src := filepath.Join(t.TempDir(), "data")
		require.NoError(t, os.MkdirAll(src, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(src, "trace.log"), []byte("trace"), 0o644))

		// ACT
		dest, err := tryDumpDirectory("7-500", src, "TestFoo/subcase")

		// ASSERT
		require.NoError(t, err)
		require.NotEmpty(t, dest)

		expected := filepath.Join(dumpDir, "pid-7", "500", "TestFoo_subcase", "data")
		assert.Equal(t, expected, dest)

		content, readErr := os.ReadFile(filepath.Join(dest, "trace.log"))
		require.NoError(t, readErr)
		assert.Equal(t, "trace", string(content))
	})

	t.Run("copies subdirectory tree", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		src := filepath.Join(t.TempDir(), "nested")
		sub := filepath.Join(src, "sub")
		require.NoError(t, os.MkdirAll(sub, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(src, "root.txt"), []byte("root"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(sub, "child.txt"), []byte("child"), 0o644))

		// ACT
		dest, err := tryDumpDirectory("1-1", src, "")

		// ASSERT
		require.NoError(t, err)
		require.NotEmpty(t, dest)

		rootContent, err := os.ReadFile(filepath.Join(dest, "root.txt"))
		require.NoError(t, err)
		assert.Equal(t, "root", string(rootContent))

		childContent, err := os.ReadFile(filepath.Join(dest, "sub", "child.txt"))
		require.NoError(t, err)
		assert.Equal(t, "child", string(childContent))
	})

	t.Run("errors on invalid runID", func(t *testing.T) {
		// ARRANGE
		saved := dumpDir
		dumpDir = t.TempDir()
		t.Cleanup(func() { dumpDir = saved })

		src := filepath.Join(t.TempDir(), "d")
		require.NoError(t, os.MkdirAll(src, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(src, "f"), []byte("x"), 0o644))

		// ACT
		dest, err := tryDumpDirectory("badid", src, "")

		// ASSERT
		require.ErrorContains(t, err, `invalid run ID "badid"`)
		assert.Empty(t, dest)
	})
}
