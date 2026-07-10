package rpcsafe

import "testing"

func TestRedactURLs(t *testing.T) {
	for _, tt := range []struct {
		name, in, want string
	}{
		{
			name: "https userinfo",
			in:   `dial error: Post "https://user:s3cret@node.example:8545/rpc?apikey=abc": EOF`,
			want: `dial error: Post "https://node.example:8545": EOF`,
		},
		{
			name: "no credential untouched",
			in:   "connect to https://node.example:8545 refused",
			want: "connect to https://node.example:8545 refused",
		},
		{
			name: "plain text untouched",
			in:   "block 42 not found",
			want: "block 42 not found",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactURLs(tt.in); got != tt.want {
				t.Fatalf("RedactURLs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEndpointUnparseable(t *testing.T) {
	if got := Endpoint("://not a url"); got != "<redacted>" {
		t.Fatalf("Endpoint = %q, want <redacted>", got)
	}
}
