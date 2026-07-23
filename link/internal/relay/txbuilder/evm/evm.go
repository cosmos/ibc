// Package evm implements txbuilder.TxBuilder for EVM chains: it packs an
// ICS26Router.updateClient call plus one recvPacket/ackPacket/timeoutPacket
// call per packet relay item into a single ICS26Router.multicall transaction.
package evm

import (
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"

	"github.com/cosmos/ibc/link/internal/chains/evm/contracts/ics26router"
	"github.com/cosmos/ibc/link/internal/config"
	"github.com/cosmos/ibc/link/internal/relay/txbuilder"
	v2 "github.com/cosmos/ibc/link/internal/types/v2"
)

var routerABI = mustRouterABI()

func mustRouterABI() *abi.ABI {
	parsed, err := ics26router.ContractMetaData.GetAbi()
	if err != nil {
		panic(errors.Wrap(err, "parsing ics26 router abi"))
	}

	return parsed
}

// Client implements txbuilder.TxBuilder for one EVM chain's ICS26Router.
type Client struct {
	router common.Address
}

func New(router common.Address) *Client {
	return &Client{router: router}
}

var _ txbuilder.TxBuilder = (*Client)(nil)

// BuildRelayTxs packs clientUpdate and every packetRelayItems entry into a
// single ICS26Router.multicall transaction. EVM router calldata has no
// meaningful size limit for the batch sizes the relayer forms, so this
// always returns exactly one tx.
func (c *Client) BuildRelayTxs(clientUpdate txbuilder.ClientUpdate, packetRelayItems []txbuilder.PacketRelayItem) ([]txbuilder.RelayTx, error) {
	calls := make([][]byte, 0, len(packetRelayItems)+1)

	updateCall, err := packUpdateClient(clientUpdate.ClientID, clientUpdate.StateProof)
	if err != nil {
		return nil, err
	}

	calls = append(calls, updateCall)

	for _, item := range packetRelayItems {
		call, err := packRelayItem(item)
		if err != nil {
			return nil, errors.Wrapf(err, "packing relay item for sequence %d", item.Packet.Sequence)
		}

		calls = append(calls, call)
	}

	tx, err := packMulticall(calls)
	if err != nil {
		return nil, err
	}

	return []txbuilder.RelayTx{{To: c.router.Bytes(), Data: tx}}, nil
}

func packRelayItem(item txbuilder.PacketRelayItem) ([]byte, error) {
	packet := toRouterPacket(item.Packet)

	switch item.Kind {
	case txbuilder.KindRecv:
		return packRecvPacket(packet, item.Proof, item.ProofHeight)
	case txbuilder.KindAck:
		if len(item.Acks) == 0 {
			return nil, errors.Errorf("no acknowledgement recorded for sequence %d", item.Packet.Sequence)
		}

		return packAckPacket(packet, item.Acks[0], item.Proof, item.ProofHeight)
	case txbuilder.KindTimeout:
		return packTimeoutPacket(packet, item.Proof, item.ProofHeight)
	default:
		return nil, errors.Errorf("unsupported relay kind %v", item.Kind)
	}
}

// height converts a plain attested height into the router's versioned height
// type. EVM chains have no notion of a revision/epoch, so RevisionNumber is
// always 0.
func height(h uint64) ics26router.IICS02ClientMsgsHeight {
	return ics26router.IICS02ClientMsgsHeight{RevisionNumber: 0, RevisionHeight: h}
}

// packUpdateClient packs a call to updateClient(clientId, updateMsg), where
// updateMsg is the already-encoded proof produced by proofgen.ProofGenerator.StateProof.
func packUpdateClient(clientID string, updateMsg []byte) ([]byte, error) {
	packed, err := routerABI.Pack("updateClient", clientID, updateMsg)
	if err != nil {
		return nil, errors.Wrap(err, "packing updateClient call")
	}

	return packed, nil
}

// packRecvPacket packs a call to recvPacket for one packet.
func packRecvPacket(packet ics26router.IICS26RouterMsgsPacket, proof []byte, proofHeight uint64) ([]byte, error) {
	packed, err := routerABI.Pack("recvPacket", ics26router.IICS26RouterMsgsMsgRecvPacket{
		Packet:          packet,
		ProofCommitment: proof,
		ProofHeight:     height(proofHeight),
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing recvPacket call")
	}

	return packed, nil
}

// packAckPacket packs a call to ackPacket for one packet.
func packAckPacket(
	packet ics26router.IICS26RouterMsgsPacket,
	acknowledgement []byte,
	proof []byte,
	proofHeight uint64,
) ([]byte, error) {
	packed, err := routerABI.Pack("ackPacket", ics26router.IICS26RouterMsgsMsgAckPacket{
		Packet:          packet,
		Acknowledgement: acknowledgement,
		ProofAcked:      proof,
		ProofHeight:     height(proofHeight),
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing ackPacket call")
	}

	return packed, nil
}

// packTimeoutPacket packs a call to timeoutPacket for one packet.
func packTimeoutPacket(packet ics26router.IICS26RouterMsgsPacket, proof []byte, proofHeight uint64) ([]byte, error) {
	packed, err := routerABI.Pack("timeoutPacket", ics26router.IICS26RouterMsgsMsgTimeoutPacket{
		Packet:       packet,
		ProofTimeout: proof,
		ProofHeight:  height(proofHeight),
	})
	if err != nil {
		return nil, errors.Wrap(err, "packing timeoutPacket call")
	}

	return packed, nil
}

// packMulticall wraps a batch of already-packed calls (one updateClient plus
// one packet-operation call per packet) as a single multicall(bytes[]) call,
// the one transaction the batch is actually submitted as.
func packMulticall(calls [][]byte) ([]byte, error) {
	packed, err := routerABI.Pack("multicall", calls)
	if err != nil {
		return nil, errors.Wrap(err, "packing multicall")
	}

	return packed, nil
}

func toRouterPacket(packet v2.Packet) ics26router.IICS26RouterMsgsPacket {
	payloads := make([]ics26router.IICS26RouterMsgsPayload, len(packet.Payloads))
	for i, p := range packet.Payloads {
		payloads[i] = ics26router.IICS26RouterMsgsPayload{
			SourcePort: p.SourcePort,
			DestPort:   p.DestPort,
			Version:    p.Version,
			Encoding:   p.Encoding,
			Value:      p.Value,
		}
	}

	return ics26router.IICS26RouterMsgsPacket{
		Sequence:         packet.Sequence,
		SourceClient:     packet.SourceClient,
		DestClient:       packet.DestClient,
		TimeoutTimestamp: packet.TimeoutTimestamp,
		Payloads:         payloads,
	}
}

// NewSetFromConfig builds a txbuilder.Set with one EVM Client per configured
// EVM chain, bound to that chain's configured ICS26Router address.
func NewSetFromConfig(cfg config.Config) (*txbuilder.Set, error) {
	builders := make(map[string]txbuilder.TxBuilder, len(cfg.Chains))

	for _, chainCfg := range cfg.Chains {
		if chainCfg.EVM == nil {
			continue
		}

		if !common.IsHexAddress(chainCfg.EVM.ICS26Router) {
			return nil, errors.Errorf("chain %q: invalid ics26 router address %q", chainCfg.ChainID, chainCfg.EVM.ICS26Router)
		}

		builders[chainCfg.ChainID] = New(common.HexToAddress(chainCfg.EVM.ICS26Router))
	}

	return txbuilder.NewSet(builders), nil
}
