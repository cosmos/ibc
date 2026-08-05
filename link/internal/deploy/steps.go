package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"

	"github.com/cosmos/ibc/link/internal/deploy/manifest"
)

// Step is one idempotent deployment phase. A nil Done means never
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
			// The manifest is a local record of chain state, not the source
			// of truth for it. If the client is already registered on-chain
			// but the manifest doesn't reflect that (e.g. after `deploy
			// import` left counterparty fields empty, or a crash between
			// RegisterClient and m.Save), sync it here. This is repair, not
			// deployment, so it also runs under --dry-run.
			candidate, err := specToClient(spec, address)
			if err != nil {
				return false, err
			}
			existing, _ := m.Client(spec.ClientID)
			candidate = mergeClientDefaults(existing, candidate)
			if !clientsEqual(existing, candidate) {
				m.UpsertClient(candidate)
				if err := m.Save(dir); err != nil {
					return false, err
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
			ref, err := t.ProvisionClient(ctx, spec)
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

// mergeClientDefaults fills candidate's empty fields from existing, so a
// precheck sync doesn't blank out fields the spec doesn't carry.
func mergeClientDefaults(existing, candidate manifest.Client) manifest.Client {
	if candidate.Type == "" {
		candidate.Type = existing.Type
	}
	if candidate.CounterpartyChainID == "" {
		candidate.CounterpartyChainID = existing.CounterpartyChainID
	}
	if candidate.CounterpartyClientID == "" {
		candidate.CounterpartyClientID = existing.CounterpartyClientID
	}
	if candidate.Params == nil {
		candidate.Params = existing.Params
	}
	return candidate
}

// clientsEqual compares via their JSON encoding so a candidate built from
// native Go types (e.g. uint8 params) compares equal to one round-tripped
// through the manifest file (where params decode as float64).
func clientsEqual(a, b manifest.Client) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}
