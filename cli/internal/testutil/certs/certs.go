// SPDX-License-Identifier: Apache-2.0

// Package certs writes throwaway self-signed certificates for tests that load
// TLS settings from files.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validity = time.Hour

// WriteSelfSigned writes a standalone self-signed pair into dir, for tests
// that need a loadable certificate rather than a verifiable chain.
func WriteSelfSigned(t testing.TB, dir, name string) (certFile, keyFile string) {
	t.Helper()

	return writeSelfSigned(t, dir, name, time.Now().Add(-validity), time.Now().Add(validity))
}

// WriteExpiredSelfSigned is WriteSelfSigned with a certificate that has
// already expired, which still loads but fails verification.
func WriteExpiredSelfSigned(t testing.TB, dir, name string) (certFile, keyFile string) {
	t.Helper()

	return writeSelfSigned(t, dir, name, time.Now().Add(-validity), time.Now().Add(-time.Minute))
}

// WriteNotYetValidSelfSigned is WriteSelfSigned with a certificate whose
// validity starts in the future.
func WriteNotYetValidSelfSigned(t testing.TB, dir, name string) (certFile, keyFile string) {
	t.Helper()

	return writeSelfSigned(t, dir, name, time.Now().Add(time.Hour), time.Now().Add(validity))
}

func writeSelfSigned(t testing.TB, dir, name string, notBefore, notAfter time.Time) (certFile, keyFile string) {
	t.Helper()

	key := newKey(t)

	tpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		IsCA:         true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create self-signed certificate: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	certFile = filepath.Join(dir, name+".crt")
	keyFile = filepath.Join(dir, name+".key")

	writePEM(t, certFile, "CERTIFICATE", der)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)

	return certFile, keyFile
}

func writePEM(t testing.TB, path, blockType string, der []byte) {
	t.Helper()

	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return key
}

func serial() *big.Int {
	return big.NewInt(time.Now().UnixNano())
}
