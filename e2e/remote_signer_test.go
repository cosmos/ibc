// SPDX-License-Identifier: Apache-2.0

package e2e_test

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"net"
	"testing"

	"github.com/cosmos/kms/gen/signerservice"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/ibc/e2e/internal/e2etest"
	"github.com/cosmos/ibc/e2e/internal/harness/environment"
	"github.com/cosmos/ibc/e2e/internal/harness/ibclink"
	relayerv2 "github.com/cosmos/ibc/link/api/v2/relayer"
)

const (
	remoteSignerKeyID         = "e2e-relayer"
	remoteSignerPrivateKeyHex = "000000000000000000000000000000000000000000000000000000000000000a"
)

type remoteSignerService struct {
	signerservice.UnimplementedSignerServiceServer
	privateKey *ecdsa.PrivateKey
}

func newRemoteSignerService() (*remoteSignerService, error) {
	privateKey, err := crypto.HexToECDSA(remoteSignerPrivateKeyHex)
	if err != nil {
		return nil, err
	}
	return &remoteSignerService{privateKey: privateKey}, nil
}

func (s *remoteSignerService) requireKey(id string) error {
	if id != remoteSignerKeyID {
		return status.Errorf(codes.NotFound, "key %q not found", id)
	}
	return nil
}

func (s *remoteSignerService) GetKey(
	_ context.Context,
	request *signerservice.GetKeyRequest,
) (*signerservice.GetKeyResponse, error) {
	if err := s.requireKey(request.GetId()); err != nil {
		return nil, err
	}
	return &signerservice.GetKeyResponse{Key: &signerservice.Key{
		Id:     remoteSignerKeyID,
		Pubkey: crypto.CompressPubkey(&s.privateKey.PublicKey),
		Scheme: signerservice.SignatureScheme_ECDSA_SECP256K1ETH,
	}}, nil
}

func (s *remoteSignerService) Sign(
	_ context.Context,
	request *signerservice.SignRequest,
) (*signerservice.SignResponse, error) {
	if err := s.requireKey(request.GetKeyId()); err != nil {
		return nil, err
	}
	digest := request.GetPayload().GetGeneric()
	if len(digest) != 32 {
		return nil, status.Error(
			codes.InvalidArgument,
			"ECDSA_SECP256K1ETH payload must be a 32-byte digest",
		)
	}
	signature, err := crypto.Sign(digest, s.privateKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "sign: %v", err)
	}
	return &signerservice.SignResponse{Signature: signature}, nil
}

func startRemoteSignerFixture(t testing.TB) string {
	t.Helper()
	service, err := newRemoteSignerService()
	require.NoError(t, err, "e2e: create remote signer service")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "e2e: listen for remote signer")

	server := grpc.NewServer()
	signerservice.RegisterSignerServiceServer(server, service)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)
	return listener.Addr().String()
}

func TestRemoteSignerFixtureRequiresKeyID(t *testing.T) {
	t.Parallel()
	service, err := newRemoteSignerService()
	require.NoError(t, err)
	digest := make([]byte, 32)

	for _, test := range []struct {
		name, id string
	}{
		{"missing ID", ""},
		{"wrong ID", "wrong"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := service.GetKey(t.Context(), &signerservice.GetKeyRequest{Id: test.id})
			require.Equal(t, codes.NotFound, status.Code(err), "GetKey")

			_, err = service.Sign(t.Context(), &signerservice.SignRequest{
				KeyId: test.id,
				Payload: &signerservice.Payload{
					Kind: &signerservice.Payload_Generic{Generic: digest},
				},
			})
			require.Equal(t, codes.NotFound, status.Code(err), "Sign")
		})
	}
}

func TestIFTTransfer_RemoteSigner(t *testing.T) {
	t.Parallel()
	spec, runtime := attestedMesh(e2etest.EVMChains(
		t,
		e2etest.EVMRequirements{},
		e2etest.ChainA,
		e2etest.ChainB,
	))
	env := e2etest.Start(t, spec, runtime)
	remoteSignerEndpoint := startRemoteSignerFixture(t)
	sender := e2etest.NewSigner(t)
	relayerSigner := e2etest.NewSignerFromHex(t, remoteSignerPrivateKeyHex)
	route := e2etest.AtoB(e2etest.ChainA, e2etest.ChainB)
	driver, deployment := e2etest.DeployWithRelayerConfig(
		t,
		env,
		sender,
		relayerSigner,
		func(config *ibclink.RelayerConfig) {
			config.SignerType = ibclink.RelayerSignerRemote
			config.SignerGRPC = remoteSignerEndpoint
			config.SignerRemoteKeyID = remoteSignerKeyID
		},
		route,
	)
	iftApp := e2etest.NewIFT(t, env, deployment, sender, route)
	relayer := e2etest.StartRelayer(t, driver, env)
	ctx := t.Context()

	transfer, err := iftApp.Send(ctx, e2etest.IFTRequest{Amount: big.NewInt(1_234_000)})
	require.NoError(t, err)
	status, err := e2etest.AwaitState(
		ctx,
		relayer,
		transfer.PacketTx(),
		relayerv2.PacketState_PACKET_STATE_SUCCEEDED,
	)
	require.NoError(t, err)
	require.NoError(t, transfer.VerifyDelivered(ctx))
	require.NoError(t, transfer.VerifyBurned(ctx), "a successful ack must not also refund")

	transactions := []struct {
		name    string
		chainID environment.ChainID
		hash    string
	}{
		{"receive", route.Destination, status.GetRecvTx().GetTxHash()},
		{"acknowledgement", route.Source, status.GetAckTx().GetTxHash()},
	}
	for _, transaction := range transactions {
		t.Run(transaction.name, func(t *testing.T) {
			chain, err := env.Chain(transaction.chainID)
			require.NoError(t, err)
			evmAccess, err := chain.EVM()
			require.NoError(t, err)
			tx, pending, err := evmAccess.TransactionByHash(ctx, common.HexToHash(transaction.hash))
			require.NoError(t, err)
			require.False(t, pending, "%s transaction must be mined", transaction.name)
			got, err := types.Sender(
				types.LatestSignerForChainID(new(big.Int).SetUint64(chain.EVMChainID())),
				tx,
			)
			require.NoError(t, err)
			require.Equal(t, relayerSigner.Address(), got)
		})
	}
}
