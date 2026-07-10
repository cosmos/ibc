package dockercli

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMissing(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "missing container", err: errors.New("docker rm -f c: No such container: c"), want: true},
		{name: "missing network old wording", err: errors.New("docker network rm n: No such network: n"), want: true},
		{
			name: "missing network current wording",
			err:  errors.New("docker network rm n: Error response from daemon: network n not found"),
			want: true,
		},
		{name: "real failure", err: errors.New("docker network rm n: permission denied"), want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, Missing(tc.err))
		})
	}
}
