// Package keyfile implements the local-signer file contract used by e2e binaries.
package keyfile

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Type string

const (
	EDDSA Type = "eddsa"
	ECDSA Type = "ecdsa"
)

type credential struct {
	Type       Type   `json:"type"`
	PrivateKey string `json:"privateKeyBase64"`
}

func Store(path string, keyType Type, privateKey []byte) error {
	if _, err := parseType(string(keyType)); err != nil {
		return err
	}
	data, err := json.Marshal(credential{
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

func Load(path string) (Type, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var stored credential
	if decodeErr := json.Unmarshal(data, &stored); decodeErr != nil {
		return "", nil, decodeErr
	}
	keyType, err := parseType(string(stored.Type))
	if err != nil {
		return "", nil, err
	}
	privateKey, err := base64.StdEncoding.DecodeString(stored.PrivateKey)
	if err != nil {
		return "", nil, err
	}
	return keyType, privateKey, nil
}

func parseType(raw string) (Type, error) {
	switch Type(raw) {
	case EDDSA, ECDSA:
		return Type(raw), nil
	default:
		return "", fmt.Errorf("invalid key type: %s", raw)
	}
}
