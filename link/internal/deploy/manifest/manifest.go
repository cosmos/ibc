// Package manifest stores generated per-chain deployment records. Manifests
// are machine-written only; user-supplied inputs belong in the config file.
package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const schemaVersion = 1

// Manifest is the deployment record for one chain.
type Manifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	ChainID       string            `json:"chainId"`
	Target        string            `json:"target"`
	Core          Core              `json:"core"`
	Clients       []Client          `json:"clients"`
	TargetData    map[string]string `json:"targetData,omitempty"`
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
