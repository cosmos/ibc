// SPDX-License-Identifier: Apache-2.0

// Package manifest stores generated per-chain deployment records. Manifests
// are machine-written only; user-supplied inputs belong in the config file.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const schemaVersion = 1

// Manifest is the deployment record for one chain.
type Manifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	ChainID       string   `json:"chainId"`
	Target        string   `json:"target"`
	Core          Core     `json:"core"`
	Clients       []Client `json:"clients"`
	GMP           *GMP     `json:"gmp,omitempty"`
	Tokens        []Token  `json:"tokens,omitempty"`
	// EVMSendCallConstructor is the chain's reusable stateless EVM send-call
	// constructor. A per-counterparty constructor is recorded for each Bridge.
	EVMSendCallConstructor string            `json:"evmSendCallConstructor,omitempty"`
	TargetData             map[string]string `json:"targetData,omitempty"`
}

// Core holds the ICS26 routing endpoint. The address format is target-specific.
type Core struct {
	Router string `json:"router"`
}

// Client is one light client registered on the chain's router.
type Client struct {
	ClientID             string         `json:"clientId"`
	Type                 string         `json:"type"`
	Address              string         `json:"address"`
	CounterpartyChainID  string         `json:"counterpartyChainId"`
	CounterpartyClientID string         `json:"counterpartyClientId"`
	Params               map[string]any `json:"params,omitempty"`
}

func New(chainID, target string) *Manifest {
	return &Manifest{SchemaVersion: schemaVersion, ChainID: chainID, Target: target}
}

func Path(dir, chainID string) string {
	return filepath.Join(dir, chainID+".json")
}

// Load reads the manifest for chainID, or returns (nil, nil) if none exists.
func Load(dir, chainID string) (*Manifest, error) {
	bz, err := os.ReadFile(Path(dir, chainID))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(bz, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Save writes the manifest atomically.
func (m *Manifest) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	bz, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".manifest-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // Best-effort cleanup; errors are acceptable
	if _, err := tmp.Write(bz); err != nil {
		_ = tmp.Close() // Ignore error; file will be cleaned up on next restart
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), Path(dir, m.ChainID))
}

func (m *Manifest) Client(clientID string) (Client, bool) {
	for _, c := range m.Clients {
		if c.ClientID == clientID {
			return c, true
		}
	}
	return Client{}, false
}

// UpsertClient replaces the client with the same ID or appends it.
func (m *Manifest) UpsertClient(c Client) {
	for i := range m.Clients {
		if m.Clients[i].ClientID == c.ClientID {
			m.Clients[i] = c
			return
		}
	}
	m.Clients = append(m.Clients, c)
}

// GMP is the deployed ICS27-GMP app for a chain. Address is the proxy;
// AccountLogic is the beacon logic impl; Port is the router port it registered
// under (always ICS27's "gmpport").
type GMP struct {
	Address      string `json:"address"`
	AccountLogic string `json:"accountLogic"`
	Port         string `json:"port"`
}

// Token is one deployed IFT token keyed by symbol+name+owner
type Token struct {
	Symbol  string   `json:"symbol"`
	Name    string   `json:"name"`
	Address string   `json:"address"`
	Owner   string   `json:"owner"`
	Bridges []Bridge `json:"bridges,omitempty"`
}

// Bridge is one registered IFT bridge from a token to a counterparty token
// over a client.
type Bridge struct {
	ClientID            string `json:"clientId"`
	CounterpartyIFT     string `json:"counterpartyIft"`
	SendCallConstructor string `json:"sendCallConstructor"`
}

// TokenByAddress returns the token deployed at address (case-insensitive)
func (m *Manifest) TokenByAddress(address string) (Token, bool) {
	for _, t := range m.Tokens {
		if strings.EqualFold(t.Address, address) {
			return t, true
		}
	}
	return Token{}, false
}

// sameToken compares a token against symbol, name, and owner.
func sameToken(t Token, symbol, name, owner string) bool {
	return t.Symbol == symbol && t.Name == name && strings.EqualFold(t.Owner, owner)
}

// TokenByIdentity returns the token matching symbol+name+owner.
func (m *Manifest) TokenByIdentity(symbol, name, owner string) (Token, bool) {
	for _, t := range m.Tokens {
		if sameToken(t, symbol, name, owner) {
			return t, true
		}
	}
	return Token{}, false
}

// UpsertToken replaces the token with the same symbol+name+owner or appends it.
func (m *Manifest) UpsertToken(t Token) {
	for i := range m.Tokens {
		if sameToken(m.Tokens[i], t.Symbol, t.Name, t.Owner) {
			m.Tokens[i] = t
			return
		}
	}
	m.Tokens = append(m.Tokens, t)
}

// UpsertBridge adds or replaces a bridge on the token at iftAddr
// (case-insensitive), returning false if no such token exists.
func (m *Manifest) UpsertBridge(iftAddr string, b Bridge) bool {
	for i := range m.Tokens {
		if strings.EqualFold(m.Tokens[i].Address, iftAddr) {
			m.Tokens[i].upsertBridge(b)
			return true
		}
	}
	return false
}

func (t Token) Bridge(clientID string) (Bridge, bool) {
	for _, b := range t.Bridges {
		if b.ClientID == clientID {
			return b, true
		}
	}
	return Bridge{}, false
}

func (t *Token) upsertBridge(b Bridge) {
	for i := range t.Bridges {
		if t.Bridges[i].ClientID == b.ClientID {
			t.Bridges[i] = b
			return
		}
	}
	t.Bridges = append(t.Bridges, b)
}
