// SPDX-License-Identifier: Apache-2.0

//go:build e2e

package e2e_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
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
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

func TestOutboundTLS(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "ibc")
	buildCtx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	build := exec.CommandContext(buildCtx, "go", "build", "-o", bin, "./cmd/ibc")
	build.Dir = ".."
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build ibc: %s", out)

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
	ca := certs.NewCA(t)
	dir := t.TempDir()
	cert, key := ca.WriteLeaf(t, dir, "client")
	f := &tlsFixture{
		ca: ca.WriteCA(t, dir), otherCA: certs.NewCA(t).WriteCA(t, t.TempDir()),
		cert: cert, key: key, mutual: mutual,
		signer: &kmsService{}, attestor: &attestorService{}, prover: &proverService{},
	}
	serverTLS := ca.ServerTLS(t, "localhost", mutual)
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
	f.chain = startChain(t)
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

func startChain(t *testing.T) string {
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
