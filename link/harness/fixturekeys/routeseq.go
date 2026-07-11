package fixturekeys

import "hash/fnv"

// RouteScopedSeq prevents routes sharing a destination mock from colliding on their raw sequences.
// Destination effects use this value; source effects retain the raw sequence.
func RouteScopedSeq(routeID string, seq uint64) uint64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(routeID))
	return uint64(h.Sum32())<<32 | (seq & 0xFFFFFFFF)
}
