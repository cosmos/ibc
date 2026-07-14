package environment

import (
	"cmp"
	"fmt"
	"slices"
	"sync"
)

type resourceKey struct {
	kind ResourceKind
	id   string
}

// journal is mutable realization machinery. It never escapes as the public
// lookup interface; callers receive deterministic immutable Manifest snapshots.
type journal struct {
	mu               sync.Mutex
	resources        map[resourceKey]ResourceRecord
	hostDependencies map[resourceKey][]ChainID
	cleanup          []CleanupRecord
}

func newJournal() *journal {
	return &journal{
		resources:        make(map[resourceKey]ResourceRecord),
		hostDependencies: make(map[resourceKey][]ChainID),
	}
}

func (j *journal) recordAcquired(
	kind ResourceKind,
	id string,
	ownership Ownership,
	hosts ...ChainID,
) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	key := resourceKey{kind: kind, id: id}
	if _, exists := j.resources[key]; exists {
		return fmt.Errorf("environment journal: duplicate %s resource %q", kind, id)
	}
	if ownership == OwnershipOwnedHostScoped && len(hosts) == 0 {
		return fmt.Errorf("environment journal: host-scoped %s resource %q has no host dependency", kind, id)
	}
	if ownership != OwnershipOwnedHostScoped && len(hosts) != 0 {
		return fmt.Errorf("environment journal: non-host-scoped %s resource %q has host dependencies", kind, id)
	}
	if len(hosts) != 0 {
		unique := make(map[ChainID]struct{}, len(hosts))
		for _, host := range hosts {
			if host == "" {
				return fmt.Errorf(
					"environment journal: host-scoped %s resource %q has an empty host dependency",
					kind,
					id,
				)
			}
			unique[host] = struct{}{}
		}
		normalizedHosts := make([]ChainID, 0, len(unique))
		for host := range unique {
			normalizedHosts = append(normalizedHosts, host)
		}
		slices.Sort(normalizedHosts)
		j.hostDependencies[key] = normalizedHosts
	}
	j.resources[key] = ResourceRecord{
		Kind:      kind,
		ID:        id,
		Ownership: ownership,
		State:     ResourceStateAcquired,
	}
	return nil
}

func (j *journal) resourceHosts(kind ResourceKind, id string) []ChainID {
	j.mu.Lock()
	defer j.mu.Unlock()
	return slices.Clone(j.hostDependencies[resourceKey{kind: kind, id: id}])
}

func (j *journal) setResourceState(kind ResourceKind, id string, state ResourceState) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	key := resourceKey{kind: kind, id: id}
	record, exists := j.resources[key]
	if !exists {
		return fmt.Errorf("environment journal: unknown %s resource %q", kind, id)
	}
	record.State = state
	j.resources[key] = record
	return nil
}

func (j *journal) recordCleanup(kind ResourceKind, id string, action CleanupAction, outcome CleanupOutcome) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.cleanup = append(j.cleanup, CleanupRecord{
		Kind:    kind,
		ID:      id,
		Action:  action,
		Outcome: outcome,
	})
}

func (j *journal) snapshot() Manifest {
	j.mu.Lock()
	defer j.mu.Unlock()

	resources := make([]ResourceRecord, 0, len(j.resources))
	for _, record := range j.resources {
		resources = append(resources, record)
	}
	slices.SortFunc(resources, func(a, b ResourceRecord) int {
		if n := cmp.Compare(string(a.Kind), string(b.Kind)); n != 0 {
			return n
		}
		return cmp.Compare(a.ID, b.ID)
	})

	cleanup := slices.Clone(j.cleanup)
	slices.SortFunc(cleanup, func(a, b CleanupRecord) int {
		for _, pair := range [][2]string{
			{string(a.Kind), string(b.Kind)},
			{a.ID, b.ID},
			{string(a.Action), string(b.Action)},
			{string(a.Outcome), string(b.Outcome)},
		} {
			if n := cmp.Compare(pair[0], pair[1]); n != 0 {
				return n
			}
		}
		return 0
	})

	return Manifest{resources: resources, cleanup: cleanup}
}
