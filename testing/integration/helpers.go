package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/testing/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
)

// WaitForSlot blocks until at least one beacon node reports the given slot.
func (h *Harness) WaitForSlot(slot uint64) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.Timeout)
	defer cancel()

	h.t.Logf("Waiting for slot %d...", slot)
	ticker := time.NewTicker(time.Duration(h.cfg.SecondsPerSlot/2) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.t.Fatalf("Timed out waiting for slot %d", slot)
		case <-ticker.C:
			for i := 0; i < h.cfg.NumBeaconNodes; i++ {
				s, err := h.headSlot(ctx, i)
				if err != nil {
					continue
				}
				if s >= slot {
					h.t.Logf("Reached slot %d on beacon-%d", s, i)
					return
				}
			}
		}
	}
}

// WaitForFinalizedEpoch blocks until at least one beacon node reports the given finalized epoch.
func (h *Harness) WaitForFinalizedEpoch(epoch uint64) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.Timeout)
	defer cancel()

	h.t.Logf("Waiting for finalized epoch %d...", epoch)
	ticker := time.NewTicker(time.Duration(h.cfg.SecondsPerSlot) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.t.Fatalf("Timed out waiting for finalized epoch %d", epoch)
		case <-ticker.C:
			for i := 0; i < h.cfg.NumBeaconNodes; i++ {
				e, err := h.finalizedEpoch(ctx, i)
				if err != nil {
					continue
				}
				if e >= epoch {
					h.t.Logf("Finalized epoch %d on beacon-%d", e, i)
					return
				}
			}
		}
	}
}

// WaitForCondition polls until the predicate returns true for any beacon node.
// The predicate receives the beacon node index.
func (h *Harness) WaitForCondition(description string, check func(ctx context.Context, beaconIndex int) bool) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), h.cfg.Timeout)
	defer cancel()

	h.t.Logf("Waiting for condition: %s", description)
	ticker := time.NewTicker(time.Duration(h.cfg.SecondsPerSlot/2) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.t.Fatalf("Timed out waiting for condition: %s", description)
		case <-ticker.C:
			for i := 0; i < h.cfg.NumBeaconNodes; i++ {
				if check(ctx, i) {
					h.t.Logf("Condition met on beacon-%d: %s", i, description)
					return
				}
			}
		}
	}
}

// GRPCConn returns a gRPC connection to the beacon node at the given index.
func (h *Harness) GRPCConn(index int) *grpc.ClientConn {
	h.t.Helper()
	endpoint := beaconGRPCEndpoint(index)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, endpoint, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	require.NoError(h.t, err, "failed to connect to beacon-%d at %s", index, endpoint)
	h.t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// BeaconNodeClient returns a BeaconChain gRPC client for the given node.
func (h *Harness) BeaconNodeClient(index int) ethpb.BeaconChainClient {
	return ethpb.NewBeaconChainClient(h.GRPCConn(index))
}

// NodeClient returns a Node gRPC client for the given node.
func (h *Harness) NodeClient(index int) ethpb.NodeClient {
	return ethpb.NewNodeClient(h.GRPCConn(index))
}

// --- internal gRPC helpers ---

func (h *Harness) headSlot(ctx context.Context, beaconIndex int) (uint64, error) {
	conn, err := grpc.DialContext(ctx, beaconGRPCEndpoint(beaconIndex),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	client := ethpb.NewBeaconChainClient(conn)
	resp, err := client.GetChainHead(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, fmt.Errorf("GetChainHead: %w", err)
	}
	return uint64(resp.HeadSlot), nil
}

func (h *Harness) finalizedEpoch(ctx context.Context, beaconIndex int) (uint64, error) {
	conn, err := grpc.DialContext(ctx, beaconGRPCEndpoint(beaconIndex),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 0, err
	}
	defer func() { _ = conn.Close() }()

	client := ethpb.NewBeaconChainClient(conn)
	resp, err := client.GetChainHead(ctx, &emptypb.Empty{})
	if err != nil {
		return 0, fmt.Errorf("GetChainHead: %w", err)
	}
	return uint64(resp.FinalizedEpoch), nil
}

// BlockVersion queries the beacon API for the block at the given slot and returns its version string.
func (h *Harness) BlockVersion(beaconIndex int, slot uint64) (string, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/eth/v2/beacon/blocks/%d", beaconGRPCPort(beaconIndex), slot)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}
	var result struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Version, nil
}
