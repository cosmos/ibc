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
