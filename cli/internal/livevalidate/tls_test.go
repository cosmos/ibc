// SPDX-License-Identifier: Apache-2.0

package livevalidate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
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

	attestorapi "github.com/cosmos/ibc/cli/api/v2/attestor"
	proverapi "github.com/cosmos/ibc/cli/api/v2/prover"
	"github.com/cosmos/ibc/cli/internal/config"
	"github.com/cosmos/ibc/cli/internal/livevalidate"
	"github.com/cosmos/ibc/cli/internal/testutil/certs"
)

// TestValidateOutboundMutualTLS loads a config file whose remote signer,
// attestor and provers all use mTLS, and runs live validation against local
// fixtures: the path from yaml to an authenticated request for every service.
func TestValidateOutboundMutualTLS(t *testing.T) {
	f := newTLSFixture(t)

	t.Run("valid", func(t *testing.T) {
		f.reset()
		require.NoError(t, f.validate(t, ""))
		require.Equal(t, int32(1), f.signer.calls.Load())
		require.Equal(t, int32(1), f.attestor.calls.Load())
		require.Equal(t, int32(2), f.prover.calls.Load())
	})

	t.Run("untrusted signer fails", func(t *testing.T) {
		f.reset()
		require.ErrorContains(t, f.validate(t, "signer"), "signer")
		require.Zero(t, f.signer.calls.Load())
	})

	t.Run("untrusted prover fails", func(t *testing.T) {
		f.reset()
		require.ErrorContains(t, f.validate(t, "prover"), "prover")
		require.Zero(t, f.prover.calls.Load())
	})

	t.Run("untrusted attestor fails", func(t *testing.T) {
		f.reset()
		require.ErrorContains(t, f.validate(t, "attestor"), "attestor")
		require.Zero(t, f.attestor.calls.Load())
	})

	t.Run("expired client certificate fails", func(t *testing.T) {
		f.reset()
		cert, key := f.cert, f.key
		f.cert, f.key = certs.WriteExpiredSelfSigned(t, t.TempDir(), "expired")
		t.Cleanup(func() { f.cert, f.key = cert, key })

		require.ErrorContains(t, f.validate(t, ""), "client certificate "+f.cert+" expired")
		require.Zero(t, f.signer.calls.Load())
	})
}

type tlsFixture struct {
	ca, otherCA, cert, key string
	kms, api, chain        string
	signer                 *kmsService
	attestor               *attestorService
	prover                 *proverService
}

func newTLSFixture(t *testing.T) *tlsFixture {
	t.Helper()

	ca := certs.NewCA(t)
	dir := t.TempDir()
	cert, key := ca.WriteLeaf(t, dir, "client")

	f := &tlsFixture{
		ca:       ca.WriteCA(t, dir),
		otherCA:  certs.NewCA(t).WriteCA(t, t.TempDir()),
		cert:     cert,
		key:      key,
		signer:   &kmsService{},
		attestor: &attestorService{},
		prover:   &proverService{},
	}

	grpcServer := grpc.NewServer(grpc.Creds(credentials.NewTLS(ca.ServerTLS(t, "localhost", true))))
	signerservice.RegisterSignerServiceServer(grpcServer, f.signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { _ = grpcServer.Serve(ln) }()
	t.Cleanup(grpcServer.Stop)

	f.kms = ln.Addr().String()

	mux := http.NewServeMux()
	mux.Handle(attestorapi.NewAttestationServiceHandler(f.attestor))
	mux.Handle(proverapi.NewProverServiceHandler(f.prover))

	httpServer := httptest.NewUnstartedServer(mux)
	httpServer.EnableHTTP2 = true
	httpServer.TLS = ca.ServerTLS(t, "localhost", true)
	httpServer.StartTLS()
	t.Cleanup(httpServer.Close)

	f.api = strings.TrimPrefix(httpServer.URL, "https://")
	f.chain = startFakeEVMChain(t)

	return f
}

func (f *tlsFixture) reset() {
	f.signer.calls.Store(0)
	f.attestor.calls.Store(0)
	f.prover.calls.Store(0)
}

// validate writes a config in which untrusted names the one service whose
// tls block trusts the wrong CA, loads it, and runs live validation.
func (f *tlsFixture) validate(t *testing.T, untrusted string) error {
	t.Helper()

	tlsBlock := func(service string, indent int) string {
		ca := f.ca
		if service == untrusted {
			ca = f.otherCA
		}

		pad := strings.Repeat(" ", indent)

		return fmt.Sprintf("%stls:\n%s  caFile: %q\n%s  certFile: %q\n%s  keyFile: %q\n%s  serverName: localhost\n",
			pad, pad, ca, pad, f.cert, pad, f.key, pad)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "signers:\n  - alias: kms\n    type: remote\n    grpc: %q\n    remoteKeyId: key-1\n%s",
		f.kms, tlsBlock("signer", 4))
	fmt.Fprintf(&b, "attestors:\n  - name: remote-attestor\n    type: remote\n    grpc: %q\n%s",
		f.api, tlsBlock("attestor", 4))
	b.WriteString("chains:\n")

	for _, id := range []string{"1", "2"} {
		fmt.Fprintf(&b, "  - chainId: %q\n    evm:\n      rpc: %q\n"+
			"      ics26Router: \"0x0000000000000000000000000000000000000001\"\n", id, f.chain)
	}

	b.WriteString("relayer:\n  connections:\n    - alias: tls-test\n")

	for i, end := range []string{"clientA", "clientB"} {
		fmt.Fprintf(&b, "      %s:\n        chainId: \"%d\"\n        clientId: client-0\n        signer: kms\n"+
			"        type: remote\n        params:\n          url: %q\n%s",
			end, i+1, "https://"+f.api, tlsBlock("prover", 10))
	}

	path := filepath.Join(t.TempDir(), "ibc.yml")
	require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o600))

	cfg, err := config.LoadFromFile(path, true)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	return livevalidate.Validate(ctx, cfg)
}

type kmsService struct {
	signerservice.UnimplementedSignerServiceServer
	calls atomic.Int32
}

func (s *kmsService) GetKey(
	_ context.Context,
	req *signerservice.GetKeyRequest,
) (*signerservice.GetKeyResponse, error) {
	s.calls.Add(1)

	return &signerservice.GetKeyResponse{Key: &signerservice.Key{
		Id: req.GetId(), Pubkey: []byte("test-public-key"), Scheme: signerservice.SignatureScheme_ED25519,
	}}, nil
}

type attestorService struct {
	attestorapi.UnimplementedAttestationServiceHandler
	calls atomic.Int32
}

func (s *attestorService) Info(
	context.Context, *connect.Request[attestorapi.InfoRequest],
) (*connect.Response[attestorapi.InfoResponse], error) {
	s.calls.Add(1)

	return connect.NewResponse(&attestorapi.InfoResponse{ChainId: "1", Address: "0xabc"}), nil
}

type proverService struct {
	proverapi.UnimplementedProverServiceHandler
	calls atomic.Int32
}

func (s *proverService) LatestProvableHeight(
	context.Context, *connect.Request[proverapi.LatestProvableHeightRequest],
) (*connect.Response[proverapi.LatestProvableHeightResponse], error) {
	s.calls.Add(1)

	return connect.NewResponse(&proverapi.LatestProvableHeightResponse{Height: 42, Timestamp: 1}), nil
}

// startFakeEVMChain answers the single eth_call live validation makes (the
// router counterparty lookup), so no real chain is needed.
func startFakeEVMChain(t *testing.T) string {
	t.Helper()

	routerABI, err := ics26router.ContractMetaData.GetAbi()
	require.NoError(t, err)

	result, err := routerABI.Methods["getCounterparty"].Outputs.Pack(
		ics26router.IICS02ClientMsgsCounterpartyInfo{ClientId: "client-0"},
	)
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
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": hexutil.Encode(result)})
	}))
	t.Cleanup(server.Close)

	return server.URL
}
