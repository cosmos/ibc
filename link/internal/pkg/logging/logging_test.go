package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestReplaceErrorAttr(t *testing.T) {
	t.Run("normalizes error key and value", func(t *testing.T) {
		var output bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{
			ReplaceAttr: ReplaceErrorAttr,
		}))
		err := pkgerrors.WithStack(errors.New("boom"))

		logger.Error("failed", "err", err)

		assert.Contains(t, output.String(), "err=boom")
		assert.NotContains(t, output.String(), "logging_test.go")
	})

	t.Run("leaves non-errors unchanged", func(t *testing.T) {
		attr := slog.String("status", "failed")

		assert.Equal(t, attr, ReplaceErrorAttr(nil, attr))
	})

	t.Run("leaves nil unchanged", func(t *testing.T) {
		attr := slog.Any("err", nil)

		assert.Equal(t, attr, ReplaceErrorAttr(nil, attr))
	})
}
