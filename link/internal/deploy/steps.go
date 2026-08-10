// SPDX-License-Identifier: Apache-2.0

package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// Step is one idempotent deployment step. A nil Done means never
// pre-satisfied.
type Step struct {
	Name string
	Done func(ctx context.Context) (bool, error)
	Run  func(ctx context.Context) error
}

// StepResult actions.
const (
	ActionSkipped  = "skipped"
	ActionExecuted = "executed"
	ActionPlanned  = "planned"
)

// StepResult records what happened to one step.
type StepResult struct {
	Name   string `json:"name"`
	Action string `json:"action"`
}

// RunSteps executes steps in order, skipping satisfied ones. With dryRun it
// only reports what would run. Stops at the first error.
func RunSteps(ctx context.Context, log *slog.Logger, dryRun bool, steps []Step) ([]StepResult, error) {
	results := make([]StepResult, 0, len(steps))
	for _, step := range steps {
		if step.Done != nil {
			done, err := step.Done(ctx)
			if err != nil {
				return results, fmt.Errorf("step %q precheck: %w", step.Name, err)
			}
			if done {
				log.Info("step satisfied, skipping", "step", step.Name)
				results = append(results, StepResult{Name: step.Name, Action: ActionSkipped})
				continue
			}
		}
		if dryRun {
			log.Info("would execute", "step", step.Name)
			results = append(results, StepResult{Name: step.Name, Action: ActionPlanned})
			continue
		}
		log.Info("executing", "step", step.Name)
		if err := step.Run(ctx); err != nil {
			return results, fmt.Errorf("step %q: %w", step.Name, err)
		}
		results = append(results, StepResult{Name: step.Name, Action: ActionExecuted})
	}
	return results, nil
}

// CoreSteps provisions the core routing stack on one chain and records it in
// the chain's manifest.
func CoreSteps(t Target, dir, chainID string) []Step {
	return []Step{{
		Name: fmt.Sprintf("core stack on chain %s", chainID),
		Done: func(ctx context.Context) (bool, error) {
			m, err := manifest.Load(dir, chainID)
			if err != nil || m == nil || m.Core.Router == "" {
				return false, err
			}
			return t.HasCode(ctx, m.Core.Router)
		},
		Run: func(ctx context.Context) error {
			ref, err := t.ProvisionCore(ctx, CoreParams{ChainID: chainID})
			if err != nil {
				return err
			}
			m, err := manifest.Load(dir, chainID)
			if err != nil {
				return err
			}
			if m == nil {
				// single-target simplification: thread the target name
				// through CoreSteps when a second target type exists
				m = manifest.New(chainID, "evm")
			}
			m.Core.Router = ref.Router
			m.TargetData = ref.TargetData
			return m.Save(dir)
		},
	}}
}

// ClientSteps provisions and registers one light client on chainID's router
// and records it in the manifest. Requires the core step to have run.
func ClientSteps(t Target, dir, chainID string, spec ClientSpec) []Step {
	return []Step{{
		Name: fmt.Sprintf("client %s on chain %s tracking chain %s", spec.ClientID, chainID, spec.CounterpartyChainID),
		Done: func(ctx context.Context) (bool, error) {
			m, err := manifest.Load(dir, chainID)
			if err != nil || m == nil || m.Core.Router == "" {
				return false, err
			}
			address, registered, err := t.ClientRegistered(ctx, m.Core.Router, spec.ClientID)
			if err != nil || !registered {
				return false, err
			}
			existing, ok := m.Client(spec.ClientID)
			if !ok {
				// a deployment that died between RegisterClient and Save: the
				// client exists on-chain but its constructor parameters were
				// never recorded and cannot be recovered reliably, so the
				// rerun's spec must not be trusted as its record
				return false, fmt.Errorf(
					"client %q is registered on-chain but missing from the manifest on chain %s "+
						"(likely an interrupted deployment); pass a new --client-id to deploy a new client",
					spec.ClientID, chainID,
				)
			}
			// On-chain client params are constructor-fixed: a spec that
			// disagrees with the recorded deployment cannot be satisfied by
			// skipping, and rewriting the manifest would desync it from the
			// chain. initialHeight/initialTimestamp are launch-time trusted
			// state (defaults track the live counterparty head), not client
			// identity, so they are not compared.
			if diffs := clientConflicts(existing, spec); len(diffs) > 0 {
				return false, fmt.Errorf(
					"client %q on chain %s is already deployed with different values (%s); "+
						"pass a new --client-id to deploy another client pair",
					spec.ClientID, chainID, strings.Join(diffs, "; "),
				)
			}
			slog.Info("client already registered, continuing",
				"client", spec.ClientID, "chain", chainID, "address", address)
			if existing.Address != address {
				existing.Address = address
				m.UpsertClient(existing)
				if saveErr := m.Save(dir); saveErr != nil {
					return false, saveErr
				}
			}
			return true, nil
		},
		Run: func(ctx context.Context) error {
			if !slices.Contains(t.SupportedClientTypes(), spec.Type) {
				return fmt.Errorf(
					"client type %q not supported by target (supported: %v)",
					spec.Type,
					t.SupportedClientTypes(),
				)
			}
			m, err := manifest.Load(dir, chainID)
			if err != nil {
				return err
			}
			if m == nil || m.Core.Router == "" {
				return fmt.Errorf("no core deployment recorded for chain %s: run `ibc deploy core` first", chainID)
			}
			ref, err := t.ProvisionClient(ctx, m.Core.Router, spec)
			if err != nil {
				return err
			}
			if _, regErr := t.RegisterClient(ctx, m.Core.Router, spec, ref); regErr != nil {
				return regErr
			}
			client, err := specToClient(spec, ref.Address)
			if err != nil {
				return err
			}
			m.UpsertClient(client)
			return m.Save(dir)
		},
	}}
}

// GMPSteps provisions the ICS27-GMP app on chainID's router and records it in
// the manifest. Requires the core step to have run.
func GMPSteps(t Target, dir, chainID string) []Step {
	return []Step{{
		Name: fmt.Sprintf("gmp app on chain %s", chainID),
		Done: func(ctx context.Context) (bool, error) {
			m, err := manifest.Load(dir, chainID)
			if err != nil {
				return false, err
			}
			if m == nil || m.Core.Router == "" {
				return false, nil // Run reports the missing-core error
			}
			if m.GMP == nil || m.GMP.Address == "" {
				// an interrupted deploy can leave the app registered on-chain
				// but unrecorded; the port is fixed, so a rerun cannot
				// re-register it and would revert — surface it instead
				_, registered, regErr := t.AppRegistered(ctx, m.Core.Router, GMPPortID)
				if regErr != nil {
					return false, regErr
				}
				if registered {
					return false, fmt.Errorf(
						"gmp app is registered at port %q on chain %s but missing from the manifest "+
							"(likely an interrupted deployment); reconcile the manifest before rerunning",
						GMPPortID, chainID,
					)
				}
				return false, nil
			}
			hasCode, err := t.HasCode(ctx, m.GMP.Address)
			if err != nil || !hasCode {
				return false, err
			}
			addr, registered, err := t.AppRegistered(ctx, m.Core.Router, m.GMP.Port)
			if err != nil || !registered {
				return false, err
			}
			if !strings.EqualFold(addr, m.GMP.Address) {
				return false, fmt.Errorf(
					"gmp port %q on chain %s is registered to %s but the manifest has %s",
					m.GMP.Port, chainID, addr, m.GMP.Address,
				)
			}
			return true, nil
		},
		Run: func(ctx context.Context) error {
			m, err := manifest.Load(dir, chainID)
			if err != nil {
				return err
			}
			if m == nil || m.Core.Router == "" {
				return fmt.Errorf("no core deployment recorded for chain %s: run `ibc deploy core` first", chainID)
			}
			am := m.TargetData["accessManager"]
			if am == "" {
				return fmt.Errorf("core manifest for chain %s has no accessManager recorded", chainID)
			}
			ref, err := t.ProvisionGMP(ctx, m.Core.Router, am)
			if err != nil {
				return err
			}
			if regErr := t.RegisterApp(ctx, m.Core.Router, ref.Address, GMPPortID); regErr != nil {
				return regErr
			}
			m.GMP = &manifest.GMP{Address: ref.Address, AccountLogic: ref.AccountLogic, Port: GMPPortID}
			return m.Save(dir)
		},
	}}
}

// IFTSteps provisions one IFT token on chainID and records it in the manifest.
// Requires the gmp step to have run.
func IFTSteps(t Target, dir, chainID string, spec IFTSpec) []Step {
	return []Step{{
		Name: fmt.Sprintf("ift token %s on chain %s", spec.Symbol, chainID),
		Done: func(ctx context.Context) (bool, error) {
			m, err := manifest.Load(dir, chainID)
			if err != nil || m == nil {
				return false, err
			}
			tok, ok := m.Token(spec.Symbol)
			if !ok {
				return false, nil
			}
			// token symbol identifies the deployment; a rerun that disagrees on
			// name or owner cannot be satisfied by skipping (constructor-fixed)
			if diffs := tokenConflicts(tok, spec); len(diffs) > 0 {
				return false, fmt.Errorf(
					"token %q on chain %s is already deployed with different values (%s); pass a new --symbol",
					spec.Symbol, chainID, strings.Join(diffs, "; "),
				)
			}
			if tok.Address == "" {
				return false, nil
			}
			return t.HasCode(ctx, tok.Address)
		},
		Run: func(ctx context.Context) error {
			m, err := manifest.Load(dir, chainID)
			if err != nil {
				return err
			}
			if m == nil || m.GMP == nil || m.GMP.Address == "" {
				return fmt.Errorf("no gmp deployment recorded for chain %s: run `ibc deploy gmp` first", chainID)
			}
			ref, err := t.ProvisionIFT(ctx, m.GMP.Address, spec)
			if err != nil {
				return err
			}
			m.UpsertToken(manifest.Token{
				Symbol:  spec.Symbol,
				Name:    spec.Name,
				Address: ref.Address,
				Owner:   spec.Owner,
			})
			return m.Save(dir)
		},
	}}
}

// IFTBridgeSteps deploys the EVM send-call constructor (unless an override is
// supplied or one is already recorded) and registers a bridge on the symbol's
// token. Requires the ift and client steps to have run.
func IFTBridgeSteps(t Target, dir, chainID, symbol, ctorOverride string, spec BridgeSpec) []Step {
	return []Step{
		{
			Name: fmt.Sprintf("send call constructor on chain %s", chainID),
			Done: func(_ context.Context) (bool, error) {
				if ctorOverride != "" {
					return true, nil
				}
				m, err := manifest.Load(dir, chainID)
				if err != nil || m == nil {
					return false, err
				}
				return m.SendCallConstructor != "", nil
			},
			Run: func(ctx context.Context) error {
				m, err := manifest.Load(dir, chainID)
				if err != nil {
					return err
				}
				if m == nil {
					return fmt.Errorf("no manifest for chain %s", chainID)
				}
				addr, err := t.ProvisionSendCallConstructor(ctx)
				if err != nil {
					return err
				}
				m.SendCallConstructor = addr
				return m.Save(dir)
			},
		},
		{
			Name: fmt.Sprintf("ift bridge %s to client %s on chain %s", symbol, spec.ClientID, chainID),
			Done: func(ctx context.Context) (bool, error) {
				m, err := manifest.Load(dir, chainID)
				if err != nil || m == nil {
					return false, err
				}
				tok, ok := m.Token(symbol)
				if !ok || tok.Address == "" {
					return false, nil // Run reports the missing-token error
				}
				cp, registered, err := t.IFTBridge(ctx, tok.Address, spec.ClientID)
				if err != nil || !registered {
					return false, err
				}
				if cp != spec.CounterpartyIFT {
					return false, fmt.Errorf(
						"bridge for client %s on token %s (chain %s) is already registered to counterparty %s, requested %s; "+
							"register a new client pair",
						spec.ClientID,
						symbol,
						chainID,
						cp,
						spec.CounterpartyIFT,
					)
				}
				return true, nil
			},
			Run: func(ctx context.Context) error {
				m, err := manifest.Load(dir, chainID)
				if err != nil {
					return err
				}
				if m == nil || m.Core.Router == "" {
					return fmt.Errorf("no core deployment recorded for chain %s", chainID)
				}
				tok, ok := m.Token(symbol)
				if !ok || tok.Address == "" {
					return fmt.Errorf(
						"no ift token %q recorded on chain %s: run `ibc deploy ift` first",
						symbol,
						chainID,
					)
				}
				_, registered, err := t.ClientRegistered(ctx, m.Core.Router, spec.ClientID)
				if err != nil {
					return err
				}
				if !registered {
					return fmt.Errorf(
						"client %q not registered on chain %s: run `ibc deploy client` first", spec.ClientID, chainID)
				}
				ctor, err := resolveConstructor(m, ctorOverride)
				if err != nil {
					return err
				}
				full := spec
				full.SendCallConstructor = ctor
				if regErr := t.RegisterIFTBridge(ctx, tok.Address, full); regErr != nil {
					return regErr
				}
				m.UpsertBridge(symbol, manifest.Bridge{
					ClientID:            spec.ClientID,
					CounterpartyIFT:     spec.CounterpartyIFT,
					SendCallConstructor: ctor,
				})
				return m.Save(dir)
			},
		},
	}
}

// resolveConstructor returns the send-call constructor address: the override
// when set, else the manifest's recorded singleton.
func resolveConstructor(m *manifest.Manifest, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if m.SendCallConstructor != "" {
		return m.SendCallConstructor, nil
	}
	return "", fmt.Errorf("no send call constructor recorded; rerun without --send-call-constructor to deploy one")
}

// tokenConflicts reports the identity fields on which spec disagrees with an
// already-recorded token. Address is deployed-derived, not compared.
func tokenConflicts(existing manifest.Token, spec IFTSpec) []string {
	var diffs []string
	if existing.Name != spec.Name {
		diffs = append(diffs, fmt.Sprintf("name: deployed %q, requested %q", existing.Name, spec.Name))
	}
	if !strings.EqualFold(existing.Owner, spec.Owner) {
		diffs = append(diffs, fmt.Sprintf("owner: deployed %s, requested %s", existing.Owner, spec.Owner))
	}
	return diffs
}

// specToClient converts a ClientSpec and its provisioned address into the
// manifest.Client record. Shared by Run and Done so the two can't drift on
// how params are marshaled.
func specToClient(spec ClientSpec, address string) (manifest.Client, error) {
	client := manifest.Client{
		ClientID:             spec.ClientID,
		Type:                 spec.Type,
		Address:              address,
		CounterpartyChainID:  spec.CounterpartyChainID,
		CounterpartyClientID: spec.CounterpartyClientID,
	}
	if spec.Type == ClientTypeAttestation {
		p, ok := spec.Params.(AttestationParams)
		if !ok {
			return manifest.Client{}, fmt.Errorf(
				"client %q: params type %T does not match client type %q",
				spec.ClientID,
				spec.Params,
				spec.Type,
			)
		}
		client.Params = map[string]any{
			"attestors":        p.Attestors,
			"threshold":        p.Threshold,
			"initialHeight":    p.InitialHeight,
			"initialTimestamp": p.InitialTimestamp,
		}
	}
	return client, nil
}

// clientConflicts reports the identity fields on which spec disagrees with
// an already-recorded deployment. Values are compared via their JSON
// encoding so native spec types (uint8, []string) match their file
// round-tripped forms (float64, []any).
func clientConflicts(existing manifest.Client, spec ClientSpec) []string {
	var diffs []string
	conflict := func(field string, deployed, requested any) {
		db, errD := json.Marshal(deployed)
		rb, errR := json.Marshal(requested)
		if errD != nil || errR != nil || string(db) != string(rb) {
			diffs = append(diffs, fmt.Sprintf("%s: deployed %s, requested %s", field, db, rb))
		}
	}
	conflict("type", existing.Type, spec.Type)
	conflict("counterpartyChainId", existing.CounterpartyChainID, spec.CounterpartyChainID)
	conflict("counterpartyClientId", existing.CounterpartyClientID, spec.CounterpartyClientID)
	if p, ok := spec.Params.(AttestationParams); ok && spec.Type == ClientTypeAttestation {
		conflict("attestors", existing.Params["attestors"], p.Attestors)
		conflict("threshold", existing.Params["threshold"], p.Threshold)
	}
	return diffs
}
