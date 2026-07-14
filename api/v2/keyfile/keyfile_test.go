package keyfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalSignerFileContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "signer.json")
	if err := Store(path, ECDSA, []byte{1, 2, 3}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"type":"ecdsa","privateKeyBase64":"AQID"}`; got != want {
		t.Fatalf("stored credential = %s, want %s", got, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Fatalf("stored credential mode = %v, want %v", got, want)
	}

	keyType, privateKey, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if keyType != ECDSA {
		t.Fatalf("loaded key type = %q, want %q", keyType, ECDSA)
	}
	if want := []byte{1, 2, 3}; !bytes.Equal(privateKey, want) {
		t.Fatalf("loaded private key = %v, want %v", privateKey, want)
	}
	if err := Store(path, ECDSA, []byte{4, 5, 6}); err == nil {
		t.Fatal("Store overwrote an existing credential")
	}
}
