// Package keyfile owns the on-disk format for local signer credentials.
package keyfile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Type identifies the signing algorithm encoded in a local key file.
type Type string

const (
	// EDDSA identifies an Ed25519 private key.
	EDDSA Type = "eddsa"
	// ECDSA identifies a secp256k1 private key.
	ECDSA Type = "ecdsa"
)

type file struct {
	Type       Type   `json:"type"`
	PrivateKey string `json:"privateKeyBase64"`
}

// Store writes a new local signer file with owner-only permissions.
func Store(path string, keyType Type, privateKey []byte) error {
	if _, err := ParseType(string(keyType)); err != nil {
		return err
	}
	data, err := json.Marshal(file{
		Type:       keyType,
		PrivateKey: base64.StdEncoding.EncodeToString(privateKey),
	})
	if err != nil {
		return fmt.Errorf("encode local signer: %w", err)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return fmt.Errorf("create local signer directory: %w", mkdirErr)
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

// ParseType validates a local signer key type.
func ParseType(raw string) (Type, error) {
	switch Type(raw) {
	case EDDSA, ECDSA:
		return Type(raw), nil
	default:
		return "", fmt.Errorf("invalid key type: %s", raw)
	}
}

// Load reads one explicitly named local signer file.
func Load(path string) (Type, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var credential file
	if decodeErr := json.Unmarshal(data, &credential); decodeErr != nil {
		return "", nil, decodeErr
	}
	keyType, err := ParseType(string(credential.Type))
	if err != nil {
		return "", nil, err
	}
	privateKey, err := base64.StdEncoding.DecodeString(credential.PrivateKey)
	if err != nil {
		return "", nil, err
	}
	return keyType, privateKey, nil
}
