package dockercli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissingContainer(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "missing container", err: errors.New("docker rm -f c: No such container: c"), want: true},
		{name: "real failure", err: errors.New("docker rm -f c: permission denied"), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, MissingContainer(tc.err))
		})
	}
}
