// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/cosmos/kms/gen/signerservice"
	"github.com/cosmos/solidity-ibc-eureka/packages/go-abigen/ics26router"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	attestorapi "github.com/cosmos/ibc/cli/api/v2/attestor"
	proverapi "github.com/cosmos/ibc/cli/api/v2/prover"
	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/ibccli"
)

// TestOutboundTLS is a black-box test of CLI configuration loading and
// outbound TLS: it runs the real ibc binary's `config validate --live`
// against local gRPC/HTTP/2 service fixtures over server-authenticated TLS
// and mTLS. It does not relay real packets or need Docker; that coverage
// belongs to the rest of this suite. See the "Outbound TLS" section of
// ../cli/README.md for the feature this exercises.
func TestOutboundTLS(t *testing.T) {
	// Unlike every other test here, this one has no chain topology to
	// record, and check-matrix's discovery run doesn't build cli/bin/ibc:
	// exec-ing it for real would fail there. Return instead of skipping so
	// discovery sees a pass rather than a skip.
	if e2etest.UnderMatrixDiscovery() {
		return
	}
	t.Parallel()

	bin := ibccli.ResolvedBin()

	for _, mutual := range []bool{false, true} {
		name := "TLS"
		if mutual {
			name = "mTLS"
		}
		t.Run(name, func(t *testing.T) {
			f := newTLSFixture(t, mutual)
			if !mutual {
				t.Run("slow-prover", func(t *testing.T) {
					f.reset()
					f.prover.delay.Store(int64(6 * time.Second))
					defer f.prover.delay.Store(0)
					output, runErr := runValidate(t, bin, f.config("", ""))
					require.NoError(t, runErr, "%s", output)
					require.Equal(t, int32(2), f.prover.calls.Load())
				})
			}
			t.Run("prover-path-prefix", func(t *testing.T) {
				f.reset()
				cfg := strings.ReplaceAll(f.config("", ""), "https://"+f.api, "https://"+f.api+"/grpc")
				output, runErr := runValidate(t, bin, cfg)
				require.NoError(t, runErr, "%s", output)
				require.Equal(t, int32(2), f.prover.calls.Load())
			})
			for _, suffix := range []string{"?token=abc", "#fragment"} {
				t.Run("prover-url/"+suffix, func(t *testing.T) {
					f.reset()
					cfg := strings.ReplaceAll(f.config("", ""), "https://"+f.api, "https://"+f.api+suffix)
					output, runErr := runValidate(t, bin, cfg)
					require.Error(t, runErr, "%s", output)
					require.Contains(t, output, "params.url: must not contain")
					require.Zero(t, f.signer.calls.Load(), "invalid config must fail before connecting")
					require.Zero(t, f.prover.calls.Load())
				})
			}
			t.Run("valid", func(t *testing.T) {
				f.reset()
				output, runErr := runValidate(t, bin, f.config("", ""))
				require.NoError(t, runErr, "%s", output)
				require.Contains(t, output, `"status": "valid"`)
				require.NotContains(t, output, "Skipping unresolvable")
				require.Equal(t, int32(1), f.signer.calls.Load())
				require.Equal(t, int32(1), f.attestor.calls.Load())
				require.Equal(t, int32(2), f.prover.calls.Load())
			})

			faults := []string{"wrong-ca", "wrong-name", "plaintext"}
			if mutual {
				faults = append(faults, "missing-client-cert")
			}
			for _, service := range []string{"signer", "attestor", "prover"} {
				for _, fault := range faults {
					t.Run(service+"/"+fault, func(t *testing.T) {
						f.reset()
						output, runErr := runValidate(t, bin, f.config(service, fault))
						if service == "attestor" {
							require.NoError(t, runErr, "%s", output)
							require.Contains(t, output, "Skipping unresolvable configured attestor")
							require.Equal(t, int32(0), f.attestor.calls.Load())
							require.Equal(t, int32(2), f.prover.calls.Load())
						} else {
							require.Error(t, runErr, "%s", output)
							require.Contains(t, output, service)
							if service == "signer" {
								require.Equal(t, int32(0), f.signer.calls.Load())
							} else {
								require.Equal(t, int32(0), f.prover.calls.Load())
							}
						}
					})
				}
			}
		})
	}
}

type tlsFixture struct {
	ca, otherCA, cert, key string
	kms, api, chain        string
	mutual                 bool
	signer                 *kmsService
	attestor               *attestorService
	prover                 *proverService
}

func newTLSFixture(t *testing.T, mutual bool) *tlsFixture {
	t.Helper()
	ca := newTestCA(t)
	dir := t.TempDir()
	cert, key := ca.writeLeaf(t, dir, "client")
	f := &tlsFixture{
		ca: ca.writeCA(t, dir), otherCA: newTestCA(t).writeCA(t, t.TempDir()),
		cert: cert, key: key, mutual: mutual,
		signer: &kmsService{}, attestor: &attestorService{}, prover: &proverService{},
	}
	serverTLS := ca.serverTLS(t, "localhost", mutual)
	serverTLS.MinVersion = tls.VersionTLS13
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(serverTLS)),
		grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo,
			handler grpc.UnaryHandler,
		) (any, error) {
			p, ok := peer.FromContext(ctx)
			if !ok {
				return nil, fmt.Errorf("missing TLS peer")
			}
			info, ok := p.AuthInfo.(credentials.TLSInfo)
			if !ok || !validTLS(info.State, mutual) {
				return nil, fmt.Errorf("unexpected TLS identity")
			}
			return handler(ctx, req)
		}))
	signerservice.RegisterSignerServiceServer(server, f.signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	f.kms = ln.Addr().String()
	go func() { _ = server.Serve(ln) }()
	t.Cleanup(server.Stop)

	mux := http.NewServeMux()
	path, handler := attestorapi.NewAttestationServiceHandler(f.attestor)
	mux.Handle(path, handler)
	path, handler = proverapi.NewProverServiceHandler(f.prover)
	mux.Handle(path, handler)
	mux.Handle("/grpc/"+strings.TrimPrefix(path, "/"), http.StripPrefix("/grpc", handler))
	httpServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.ProtoMajor != 2 || !validTLS(*r.TLS, mutual) {
			http.Error(w, "unexpected TLS identity or protocol", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	httpServer.EnableHTTP2 = true
	httpServer.TLS = serverTLS.Clone()
	httpServer.StartTLS()
	t.Cleanup(httpServer.Close)
	f.api = strings.TrimPrefix(httpServer.URL, "https://")
	f.chain = startFakeEVMChain(t)
	return f
}

func validTLS(state tls.ConnectionState, mutual bool) bool {
	return state.Version == tls.VersionTLS13 && (!mutual ||
		(len(state.VerifiedChains) > 0 && state.PeerCertificates[0].Subject.CommonName == "client"))
}

func (f *tlsFixture) reset() {
	f.signer.calls.Store(0)
	f.attestor.calls.Store(0)
	f.prover.calls.Store(0)
}

func (f *tlsFixture) config(service, fault string) string {
	tlsBlock := func(target string, indent int) string {
		if service == target && fault == "plaintext" {
			return ""
		}
		ca, name := f.ca, "localhost"
		if service == target && fault == "wrong-ca" {
			ca = f.otherCA
		}
		if service == target && fault == "wrong-name" {
			name = "wrong.example"
		}
		pad := strings.Repeat(" ", indent)
		block := fmt.Sprintf("%stls:\n%s  caFile: %q\n%s  serverName: %q\n%s  minVersion: \"1.3\"\n",
			pad, pad, ca, pad, name, pad)
		if f.mutual && (service != target || fault != "missing-client-cert") {
			block += fmt.Sprintf("%s  certFile: %q\n%s  keyFile: %q\n", pad, f.cert, pad, f.key)
		}
		return block
	}
	scheme := "https://"
	if service == "prover" && fault == "plaintext" {
		scheme = "http://"
	}
	config := fmt.Sprintf(`signers:
  - alias: kms
    type: remote
    grpc: %q
    remoteKeyId: key-1
%sattestors:
  - name: remote-attestor
    type: remote
    grpc: %q
%schains:
`, f.kms, tlsBlock("signer", 4), f.api, tlsBlock("attestor", 4))
	for _, id := range []string{"1", "2"} {
		config += fmt.Sprintf(`  - chainId: %q
    evm:
      rpc: %q
      ics26Router: "0x0000000000000000000000000000000000000001"
`, id, f.chain)
	}
	config += "relayer:\n  connections:\n    - alias: tls-test\n"
	for i, end := range []string{"clientA", "clientB"} {
		config += fmt.Sprintf(`      %s:
        chainId: "%d"
        clientId: client-0
        signer: kms
        type: remote
        params:
          url: %q
%s`, end, i+1, scheme+f.api, tlsBlock("prover", 10))
	}
	return config
}

func runValidate(t *testing.T, bin, config string) (string, error) {
	t.Helper()
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, "ibc.yml"), []byte(config), 0o600))
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--home", home, "config", "validate", "--live")
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	require.NoError(t, ctx.Err(), "CLI timed out: %s", out)
	return string(out), err
}

type kmsService struct {
	signerservice.UnimplementedSignerServiceServer
	calls atomic.Int32
}

func (s *kmsService) GetKey(
	_ context.Context,
	req *signerservice.GetKeyRequest,
) (*signerservice.GetKeyResponse, error) {
	if req.GetId() != "key-1" {
		return nil, fmt.Errorf("unexpected key ID %q", req.GetId())
	}
	s.calls.Add(1)
	return &signerservice.GetKeyResponse{Key: &signerservice.Key{
		Id: req.GetId(), Pubkey: []byte("test-public-key"), Scheme: signerservice.SignatureScheme_ED25519,
	}}, nil
}

type attestorService struct {
	attestorapi.UnimplementedAttestationServiceHandler
	calls atomic.Int32
}

func (s *attestorService) Info(_ context.Context, req *connect.Request[attestorapi.InfoRequest]) (
	*connect.Response[attestorapi.InfoResponse], error,
) {
	if req.Msg.GetAttestor() != "remote-attestor" {
		return nil, fmt.Errorf("unexpected attestor %q", req.Msg.GetAttestor())
	}
	s.calls.Add(1)
	return connect.NewResponse(&attestorapi.InfoResponse{ChainId: "1", Address: "0xabc"}), nil
}

type proverService struct {
	proverapi.UnimplementedProverServiceHandler
	calls atomic.Int32
	delay atomic.Int64
}

func (s *proverService) LatestProvableHeight(ctx context.Context,
	req *connect.Request[proverapi.LatestProvableHeightRequest],
) (*connect.Response[proverapi.LatestProvableHeightResponse], error) {
	client := req.Msg.GetClient()
	if client.GetClientId() != "client-0" || (client.GetChainId() != "1" && client.GetChainId() != "2") {
		return nil, fmt.Errorf("unexpected prover target %v", client)
	}
	if delay := time.Duration(s.delay.Load()); delay > 0 && client.GetChainId() == "1" {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	s.calls.Add(1)
	return connect.NewResponse(&proverapi.LatestProvableHeightResponse{Height: 42, Timestamp: 1}), nil
}

// startFakeEVMChain fakes just enough JSON-RPC (a single eth_call, answering
// the router counterparty lookup live validation makes) to validate a config
// without a real chain: this test is about outbound TLS, not chain state.
func startFakeEVMChain(t *testing.T) string {
	t.Helper()
	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)
	method := routerABI.Methods["getCounterparty"]
	result, err := method.Outputs.Pack(ics26router.IICS02ClientMsgsCounterpartyInfo{ClientId: "client-0"})
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil || req.Method != "eth_call" {
			http.Error(w, "expected eth_call", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID, "result": hexutil.Encode(result),
		})
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// Certificates below are generated locally rather than shared with the cli
// module's cli/internal/testutil/certs: this e2e module is separate and,
// per the harness wall (see internal/harness/AGENTS.md), must not import
// cli/internal.

const testCertValidity = time.Hour

// testCA is a self-signed authority that issues the server and client
// certificates this test needs.
type testCA struct {
	cert *x509.Certificate
	der  []byte
	key  *ecdsa.PrivateKey
}

func newTestCA(t testing.TB) *testCA {
	t.Helper()

	key := newTestCertKey(t)

	tpl := &x509.Certificate{
		SerialNumber:          testCertSerial(),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-testCertValidity),
		NotAfter:              time.Now().Add(testCertValidity),
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

	return &testCA{cert: cert, der: der, key: key}
}

// pool returns a pool trusting only this CA.
func (c *testCA) pool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.cert)

	return pool
}

// leaf issues a certificate valid for both server and client auth, carrying
// commonName as a DNS name and 127.0.0.1 as an IP SAN so it verifies against
// a loopback listener.
func (c *testCA) leaf(t testing.TB, commonName string) tls.Certificate {
	t.Helper()

	key := newTestCertKey(t)

	tpl := &x509.Certificate{
		SerialNumber: testCertSerial(),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-testCertValidity),
		NotAfter:     time.Now().Add(testCertValidity),
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

// serverTLS returns a server config presenting a leaf for commonName. When
// requireClientCert is set the server demands and verifies a client
// certificate issued by this CA.
func (c *testCA) serverTLS(t testing.TB, commonName string, requireClientCert bool) *tls.Config {
	t.Helper()

	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{c.leaf(t, commonName)},
	}

	if requireClientCert {
		cfg.ClientCAs = c.pool()
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	}

	return cfg
}

// writeCA writes the CA certificate into dir and returns its path, suitable
// for a caFile config field.
func (c *testCA) writeCA(t testing.TB, dir string) string {
	t.Helper()

	path := filepath.Join(dir, "ca.crt")
	writeTestPEM(t, path, "CERTIFICATE", c.der)

	return path
}

// writeLeaf issues a leaf for name and writes it into dir, returning paths
// suitable for certFile and keyFile config fields.
func (c *testCA) writeLeaf(t testing.TB, dir, name string) (certFile, keyFile string) {
	t.Helper()

	return writeTestCertificate(t, dir, name, c.leaf(t, name))
}

func writeTestCertificate(t testing.TB, dir, name string, cert tls.Certificate) (certFile, keyFile string) {
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

	writeTestPEM(t, certFile, "CERTIFICATE", cert.Certificate[0])
	writeTestPEM(t, keyFile, "EC PRIVATE KEY", keyDER)

	return certFile, keyFile
}

func writeTestPEM(t testing.TB, path, blockType string, der []byte) {
	t.Helper()

	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newTestCertKey(t testing.TB) *ecdsa.PrivateKey {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	return key
}

func testCertSerial() *big.Int {
	return big.NewInt(time.Now().UnixNano())
}
