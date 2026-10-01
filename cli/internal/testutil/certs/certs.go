// SPDX-License-Identifier: Apache-2.0

// Package certs issues throwaway certificates for tests that need a real TLS
// or mTLS handshake, so each package does not carry its own copy.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validity = time.Hour

// CA is a self-signed authority that issues the server and client
// certificates a test needs.
type CA struct {
	cert *x509.Certificate
	der  []byte
	key  *ecdsa.PrivateKey
}

// NewCA creates a self-signed authority.
func NewCA(t testing.TB) *CA {
	t.Helper()

	key := newKey(t)

	tpl := &x509.Certificate{
		SerialNumber:          serial(),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-validity),
		NotAfter:              time.Now().Add(validity),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse CA certificate: %v", err)
	}

	return &CA{cert: cert, der: der, key: key}
}

// Pool returns a pool trusting only this CA.
func (c *CA) Pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.cert)

	return pool
}

// Leaf issues a certificate valid for both server and client auth, carrying
// commonName as a DNS name and 127.0.0.1 as an IP SAN so it verifies against a
// loopback listener.
func (c *CA) Leaf(t testing.TB, commonName string) tls.Certificate {
	t.Helper()

	key := newKey(t)

	tpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-validity),
		NotAfter:     time.Now().Add(validity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     []string{commonName},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// ServerTLS returns a server config presenting a leaf for commonName. When
// requireClientCert is set the server demands and verifies a client
// certificate issued by this CA.
func (c *CA) ServerTLS(t testing.TB, commonName string, requireClientCert bool) *tls.Config {
	t.Helper()

	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{c.Leaf(t, commonName)},
	}

	if requireClientCert {
		cfg.ClientCAs = c.Pool()
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg
}

// WriteCA writes the CA certificate into dir and returns its path, suitable
// for a caFile config field.
func (c *CA) WriteCA(t testing.TB, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "ca.crt")
	writePEM(t, path, "CERTIFICATE", c.der)

	return path
}

// WriteLeaf issues a leaf for name and writes it into dir, returning paths
// suitable for certFile and keyFile config fields.
func (c *CA) WriteLeaf(t testing.TB, dir, name string) (certFile, keyFile string) {
	t.Helper()

	return writeCertificate(t, dir, name, c.Leaf(t, name))
}

// WriteSelfSigned writes a standalone self-signed pair into dir, for tests
// that need a loadable certificate rather than a verifiable chain.
func WriteSelfSigned(t testing.TB, dir, name string) (certFile, keyFile string) {
	t.Helper()

	key := newKey(t)

	tpl := &x509.Certificate{
		SerialNumber: serial(),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-validity),
		NotAfter:     time.Now().Add(validity),
		IsCA:         true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create self-signed certificate: %v", err)
	}

	return writeCertificate(t, dir, name, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key})
}

func writeCertificate(t testing.TB, dir, name string, cert tls.Certificate) (certFile, keyFile string) {
	t.Helper()

	key, ok := cert.PrivateKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatalf("unexpected private key type %T", cert.PrivateKey)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}

	certFile = filepath.Join(dir, name+".crt")
	keyFile = filepath.Join(dir, name+".key")

	writePEM(t, certFile, "CERTIFICATE", cert.Certificate[0])
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
