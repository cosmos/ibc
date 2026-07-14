package stub

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/ibc/link/cmd/configcmd"
	"github.com/cosmos/ibc/link/cmd/relayercmd"
)

const (
	testDestination = "dst"
	testRouteID     = "r1"
)

func TestPendingReceivedKeysAreRouteScoped(t *testing.T) {
	r := &relayer{cfg: &configcmd.Config{Relayer: configcmd.Relayer{Routes: []configcmd.Route{
		{ID: testRouteID, Destination: testDestination},
	}}}}
	p := storedPacket{RouteID: testRouteID, AppType: relayercmd.AppTypeIFT, Sequence: 7}
	want := receivedKey{
		destination: testDestination,
		appType:     relayercmd.AppTypeIFT,
		routeID:     testRouteID,
		sequence:    7,
	}

	if _, ok := r.pendingReceivedKeys([]storedPacket{p})[want]; !ok {
		t.Fatalf("pending key missing route-scoped destination identity %+v", want)
	}
}

func TestLookupReceivedCachesOnlyPendingKeys(t *testing.T) {
	addr := common.HexToAddress("0x1234")
	first := receivedKey{
		destination: testDestination,
		appType:     relayercmd.AppTypeIFT,
		routeID:     testRouteID,
		sequence:    11,
	}
	second := receivedKey{
		destination: testDestination,
		appType:     relayercmd.AppTypeIFT,
		routeID:     testRouteID,
		sequence:    12,
	}
	r := &relayer{
		recvCursor: map[string]uint64{},
		recvSeen:   map[receivedKey]receivedResult{},
		recvActive: map[receivedKey]struct{}{first: {}, second: {}},
	}

	scans := 0
	scan := func(
		_ context.Context,
		from uint64,
	) (map[receivedEventKey]receivedResult, uint64, error) {
		scans++
		if scans == 1 {
			if from != 0 {
				t.Fatalf("first scan from block %d, want 0", from)
			}
			return map[receivedEventKey]receivedResult{
				{RouteID: testRouteID, Sequence: 11}: {Success: true},
				{RouteID: testRouteID, Sequence: 12}: {Success: true},
				{RouteID: testRouteID, Sequence: 13}: {Success: true},
			}, 20, nil
		}
		if from != 20 {
			t.Fatalf("second scan from block %d, want 20", from)
		}
		return nil, 20, nil
	}

	if _, found, err := r.lookupReceived(t.Context(), first, addr, scan); err != nil || !found {
		t.Fatalf("lookup first pending key = found %v, err %v; want found", found, err)
	}
	if _, found, err := r.lookupReceived(t.Context(), second, addr, scan); err != nil || !found {
		t.Fatalf("lookup second pending key = found %v, err %v; want cached result", found, err)
	}
	if len(r.recvSeen) != 2 {
		t.Fatalf("cached results = %v; want only two pending keys", r.recvSeen)
	}
	historical := receivedKey{
		destination: testDestination,
		appType:     relayercmd.AppTypeIFT,
		routeID:     testRouteID,
		sequence:    13,
	}
	if _, cached := r.recvSeen[historical]; cached {
		t.Fatal("historical key was cached")
	}
}

func TestFinishTerminalEvictsOnlyAfterPersistence(t *testing.T) {
	key := receivedKey{
		destination: testDestination,
		appType:     relayercmd.AppTypeGMP,
		routeID:     testRouteID,
		sequence:    9,
	}
	r := &relayer{
		recvSeen:   map[receivedKey]receivedResult{key: {Success: true}},
		recvActive: map[receivedKey]struct{}{key: {}},
	}

	persistErr := errors.New("persist terminal state")
	if err := r.finishTerminal(key, persistErr); !errors.Is(err, persistErr) {
		t.Fatalf("finish error = %v, want %v", err, persistErr)
	}
	if _, cached := r.recvSeen[key]; !cached {
		t.Fatal("failed persistence evicted cached retry state")
	}
	if _, active := r.recvActive[key]; !active {
		t.Fatal("failed persistence evicted active retry state")
	}

	if err := r.finishTerminal(key, nil); err != nil {
		t.Fatalf("finish successful persistence: %v", err)
	}
	if _, cached := r.recvSeen[key]; cached {
		t.Fatal("successful persistence retained cached result")
	}
	if _, active := r.recvActive[key]; active {
		t.Fatal("successful persistence retained active key")
	}
}
