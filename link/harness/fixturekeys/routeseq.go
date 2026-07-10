package fixturekeys

import "hash/fnv"

// RouteScopedSeq maps a route's raw source sequence to a destination-unique sequence. Source sequences are
// assigned per source fixture (each EVM MockIFT/MockGMP has its own counter), so two routes into one
// destination whose sequence spaces both start at 1 would
// otherwise collide on a shared destination mock fixture or light client — cross-matching
// one route's delivery with another's. The relayer performs every destination-side effect (and its
// idempotency check) under this value, and the harness readers await it; source-side effects (escrow,
// refund) keep the raw sequence, which is already unique per source fixture.
//
// The high 32 bits are a stable hash of the route id and the low 32 bits the raw sequence (test sequences
// are far below 2^32), so the mapping is deterministic and injective in practice. Like the fixture keys, it
// is mock mechanism, not wire contract: real contracts identify a packet by client/route natively, and only
// the mock fixtures key events on a bare sequence. It lives here so the stub and the harness — which never
// import each other — derive it identically, and it goes away with this package at the deploy swap instead
// of lingering in the permanent wire schema.
func RouteScopedSeq(routeID string, seq uint64) uint64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(routeID))
	return uint64(h.Sum32())<<32 | (seq & 0xFFFFFFFF)
}
