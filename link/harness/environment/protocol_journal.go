package environment

import (
	"cmp"
	"slices"
	"sync"
)

// protocolReceiptJournal records typed setup evidence as soon as transactions
// are submitted or existing state is resolved. It is separate from Manifest:
// lifecycle state is generic, while protocol outputs remain domain-shaped.
type protocolReceiptJournal struct {
	mu          sync.Mutex
	instances   map[IBCInstanceID]IBCInstanceReceipt
	connections map[ConnectionID]IBCConnectionReceipt
}

type protocolReceiptSnapshot struct {
	instances   []IBCInstanceReceipt
	connections []IBCConnectionReceipt
}

func newProtocolReceiptJournal() *protocolReceiptJournal {
	return &protocolReceiptJournal{
		instances:   make(map[IBCInstanceID]IBCInstanceReceipt),
		connections: make(map[ConnectionID]IBCConnectionReceipt),
	}
}

func (j *protocolReceiptJournal) recordInstance(receipt IBCInstanceReceipt) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.instances[receipt.ID] = cloneIBCInstanceReceipt(receipt)
}

func (j *protocolReceiptJournal) recordConnectionEnd(
	id ConnectionID,
	end string,
	receipt IBCClientReceipt,
) {
	j.mu.Lock()
	defer j.mu.Unlock()
	connection := j.connections[id]
	connection.ID = id
	switch end {
	case "A":
		connection.A = cloneIBCClientReceipt(&receipt)
	case "B":
		connection.B = cloneIBCClientReceipt(&receipt)
	default:
		panic("environment: invalid IBC Connection end " + end)
	}
	j.connections[id] = connection
}

func (j *protocolReceiptJournal) snapshot() protocolReceiptSnapshot {
	j.mu.Lock()
	defer j.mu.Unlock()

	out := protocolReceiptSnapshot{
		instances:   make([]IBCInstanceReceipt, 0, len(j.instances)),
		connections: make([]IBCConnectionReceipt, 0, len(j.connections)),
	}
	for _, receipt := range j.instances {
		out.instances = append(out.instances, cloneIBCInstanceReceipt(receipt))
	}
	for _, receipt := range j.connections {
		out.connections = append(out.connections, cloneIBCConnectionReceipt(receipt))
	}
	slices.SortFunc(out.instances, func(a, b IBCInstanceReceipt) int {
		return cmp.Compare(string(a.ID), string(b.ID))
	})
	slices.SortFunc(out.connections, func(a, b IBCConnectionReceipt) int {
		return cmp.Compare(string(a.ID), string(b.ID))
	})
	return out
}

func cloneProtocolReceiptSnapshot(in protocolReceiptSnapshot) protocolReceiptSnapshot {
	out := protocolReceiptSnapshot{
		instances:   make([]IBCInstanceReceipt, len(in.instances)),
		connections: make([]IBCConnectionReceipt, len(in.connections)),
	}
	for i, receipt := range in.instances {
		out.instances[i] = cloneIBCInstanceReceipt(receipt)
	}
	for i, receipt := range in.connections {
		out.connections[i] = cloneIBCConnectionReceipt(receipt)
	}
	return out
}
