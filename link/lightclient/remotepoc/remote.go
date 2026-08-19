// SPDX-License-Identifier: Apache-2.0

// Package remotepoc provides an experimental HTTP prover.
package remotepoc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	channeltypesv2 "github.com/cosmos/ibc-go/v11/modules/core/04-channel/v2/types"
	"github.com/cosmos/ibc/link/lightclient"
)

// Type is the remote prover's config type.
const Type = "remote"

// ClientParams configures the remote service.
type ClientParams struct {
	URL string `yaml:"url"`
}

// Factory constructs remote provers.
type Factory struct {
	HTTPClient *http.Client
}

func (Factory) Type() string { return Type }

func (f Factory) New(
	_ context.Context,
	options lightclient.ProverFactoryOptions,
) (lightclient.Prover, error) {
	var p ClientParams
	if err := options.Client.ClientParams.Decode(&p); err != nil {
		return nil, err
	}
	u, err := url.ParseRequestURI(p.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("url must be an absolute HTTP(S) URL")
	}

	client := f.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Generator{url: strings.TrimRight(p.URL, "/") + "/proof", client: client}, nil
}

// Generator forwards proof generation to an HTTP service.
type Generator struct {
	url    string
	client *http.Client
}

type request struct {
	Operation string                  `json:"operation"`
	Height    uint64                  `json:"height,omitempty"`
	Kind      lightclient.ProofKind   `json:"kind,omitempty"`
	Packets   []channeltypesv2.Packet `json:"packets,omitempty"`
}

type response struct {
	Height    uint64    `json:"height,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	Proof     []byte    `json:"proof,omitempty"`
	Proofs    [][]byte  `json:"proofs,omitempty"`
	Error     string    `json:"error,omitempty"`
}

func (g *Generator) LatestProvableHeight(ctx context.Context) (uint64, time.Time, error) {
	res, err := g.call(ctx, request{Operation: "latest"})
	return res.Height, res.Timestamp, err
}

func (g *Generator) StateProof(ctx context.Context, height uint64) ([]byte, error) {
	res, err := g.call(ctx, request{Operation: "state", Height: height})
	return res.Proof, err
}

func (g *Generator) PacketProofs(
	ctx context.Context,
	height uint64,
	kind lightclient.ProofKind,
	packets []channeltypesv2.Packet,
) ([][]byte, error) {
	res, err := g.call(ctx, request{Operation: "packets", Height: height, Kind: kind, Packets: packets})
	return res.Proofs, err
}

func (g *Generator) call(ctx context.Context, req request) (response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, g.url, strings.NewReader(string(body)))
	if err != nil {
		return response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpRes, err := g.client.Do(httpReq)
	if err != nil {
		return response{}, err
	}
	defer func() { _ = httpRes.Body.Close() }()

	var res response
	if err := json.NewDecoder(httpRes.Body).Decode(&res); err != nil {
		return response{}, fmt.Errorf("remote proof service: %w", err)
	}
	if res.Error != "" {
		return response{}, errors.New(res.Error)
	}

	return res, nil
}
