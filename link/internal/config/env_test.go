package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExpandEnvRefs(t *testing.T) {
	t.Setenv("IBC_LINK_TEST_TOKEN", "secret")
	t.Setenv("IBC_LINK_TEST_DOLLARS", "$UNCHANGED")
	t.Setenv("IBC_LINK_TEST_EMPTY", "")

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "explicit reference", value: "before-${IBC_LINK_TEST_TOKEN}-after", want: "before-secret-after"},
		{name: "double dollar is literal", value: "$$", want: "$$"},
		{name: "url dollar is literal", value: "http://host/?token=$X", want: "http://host/?token=$X"},
		{name: "invalid name is literal", value: "${1INVALID}", want: "${1INVALID}"},
		{name: "replacement is not recursive", value: "${IBC_LINK_TEST_DOLLARS}", want: "$UNCHANGED"},
		{name: "set empty value", value: "${IBC_LINK_TEST_EMPTY}", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExpandEnvRefs(tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}

	_, err := ExpandEnvRefs("${IBC_LINK_TEST_MISSING_VALUE}")
	require.EqualError(t, err, "environment variable IBC_LINK_TEST_MISSING_VALUE is not set")
}
