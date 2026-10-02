package optionalproofs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/client/proofnode"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const (
	// payloadFetchAttempts and payloadFetchDelay bound how long the prover
	// waits for the beacon node to be able to serve a revealed payload.
	payloadFetchAttempts = 6
	payloadFetchDelay    = 1 * time.Second
)

// payloadAvailable reports that a beacon block's execution payload has been
// revealed and can now be proved.
type payloadAvailable struct {
	slot      primitives.Slot
	blockRoot [32]byte
}

// beaconClient is the prover's view of its beacon node.
type beaconClient struct {
	endpoint string
	hc       *http.Client

	// genesis is fetched lazily and retried until it succeeds.
	genesisMu sync.Mutex
	genesis   time.Time
}

func newBeaconClient(endpoint string) (*beaconClient, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("beacon API endpoint is empty")
	}

	u, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, fmt.Errorf("beacon API endpoint %q is invalid: %w", endpoint, err)
	}

	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("beacon API endpoint must be an absolute http(s) URL, got %q", endpoint)
	}

	return &beaconClient{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		hc:       &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// subscribePayloadAvailable streams execution_payload_available events until
// the context is cancelled, reconnecting when the stream drops.
func (c *beaconClient) subscribePayloadAvailable(ctx context.Context, out chan<- payloadAvailable) {
	for ctx.Err() == nil {
		c.streamPayloadAvailable(ctx, out)

		// The stream ended. Pause before reconnecting so a beacon node that is
		// down does not get hammered.
		select {
		case <-ctx.Done():
			return
		case <-time.After(params.BeaconConfig().SlotDuration()):
		}
	}
}

func (c *beaconClient) streamPayloadAvailable(ctx context.Context, out chan<- payloadAvailable) {
	events := make(chan *event.Event, 1)
	stream, err := event.NewEventStream(ctx, c.hc, c.endpoint, []string{event.EventExecutionPayloadAvailable})
	if err != nil {
		log.WithError(err).Error("Could not create the payload availability event stream")
		return
	}

	go func() {
		if err := stream.Subscribe(events); err != nil {
			log.WithError(err).Debug("Payload availability event stream ended")
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-events:
			switch ev.Type {
			case event.EventExecutionPayloadAvailable:
				available, err := decodePayloadAvailable(ev.Data)
				if err != nil {
					log.WithError(err).Error("Could not decode payload availability event")
					continue
				}
				select {
				case out <- available:
				case <-ctx.Done():
					return
				}
			case event.EventConnectionError, event.EventError:
				log.WithField("error", string(ev.Data)).Debug("Payload availability event stream error")
				return
			}
		}
	}
}

func decodePayloadAvailable(data []byte) (payloadAvailable, error) {
	var payload structs.ExecutionPayloadAvailableEvent
	if err := json.Unmarshal(data, &payload); err != nil {
		return payloadAvailable{}, fmt.Errorf("unmarshal event: %w", err)
	}

	slot, err := strconv.ParseUint(payload.Slot, 10, 64)
	if err != nil {
		return payloadAvailable{}, fmt.Errorf("parse slot: %w", err)
	}

	blockRoot, err := hexutil.Decode(payload.BlockRoot)
	if err != nil {
		return payloadAvailable{}, fmt.Errorf("decode block root: %w", err)
	}

	if len(blockRoot) != 32 {
		return payloadAvailable{}, fmt.Errorf("block root has length %d, want 32", len(blockRoot))
	}
	return payloadAvailable{slot: primitives.Slot(slot), blockRoot: [32]byte(blockRoot)}, nil
}

// newPayloadRequest rebuilds the new payload request for the payload revealed
// for a beacon block, so that the proof node proves exactly what consensus
// accepted.
func (c *beaconClient) newPayloadRequest(ctx context.Context, blockRoot [32]byte) (*enginev1.SSZNewPayloadRequest, error) {
	var lastErr error
	for attempt := 0; attempt < payloadFetchAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(payloadFetchDelay):
			}
		}

		newPayloadRequest, err := c.buildNewPayloadRequest(ctx, blockRoot)
		if err == nil {
			return newPayloadRequest, nil
		}

		lastErr = err
		log.WithError(err).WithField("attempt", attempt+1).Debug("Could not read revealed payload yet, retrying")
	}

	return nil, lastErr
}

func (c *beaconClient) buildNewPayloadRequest(ctx context.Context, blockRoot [32]byte) (*enginev1.SSZNewPayloadRequest, error) {
	root := hexutil.Encode(blockRoot[:])

	envelope, err := c.executionPayloadEnvelope(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("could not get execution payload envelope: %w", err)
	}

	commitments, err := c.blobKzgCommitments(ctx, root)
	if err != nil {
		return nil, fmt.Errorf("could not get blob KZG commitments: %w", err)
	}

	newPayloadRequest, err := blocks.NewPayloadRequest(envelope, commitments)
	if err != nil {
		return nil, fmt.Errorf("cnew payload request: %w", err)
	}
	return newPayloadRequest, nil
}

// executionPayloadEnvelope fetches the revealed payload envelope as SSZ.
func (c *beaconClient) executionPayloadEnvelope(ctx context.Context, blockRoot string) (interfaces.ROExecutionPayloadEnvelope, error) {
	body, err := c.getSSZ(ctx, "/eth/v1/beacon/execution_payload_envelopes/"+blockRoot)
	if err != nil {
		return nil, fmt.Errorf("get ssz: %w", err)
	}

	signed := &ethpb.SignedExecutionPayloadEnvelope{}
	if err := signed.UnmarshalSSZ(body); err != nil {
		return nil, fmt.Errorf("unmarshal execution payload envelope: %w", err)
	}

	wrapped, err := blocks.WrappedROSignedExecutionPayloadEnvelope(signed)
	if err != nil {
		return nil, fmt.Errorf("wrap execution payload envelope: %w", err)
	}

	return wrapped.Envelope()
}

// blobKzgCommitments reads the blob commitments from the execution payload bid
// the block committed to. The versioned hashes of the new payload request must
// be the ones consensus accepted, not the ones the envelope happens to carry.
func (c *beaconClient) blobKzgCommitments(ctx context.Context, blockRoot string) ([][]byte, error) {
	body, err := c.getSSZ(ctx, "/eth/v2/beacon/blocks/"+blockRoot)
	if err != nil {
		return nil, fmt.Errorf("get ssz: %w", err)
	}

	block := &ethpb.SignedBeaconBlockGloas{}
	if err := block.UnmarshalSSZ(body); err != nil {
		return nil, fmt.Errorf("unmarshal beacon block: %w", err)
	}

	bid := block.GetBlock().GetBody().GetSignedExecutionPayloadBid().GetMessage()
	if bid == nil {
		return nil, fmt.Errorf("beacon block carries no execution payload bid")
	}

	return bid.BlobKzgCommitments, nil
}

// submitExecutionProof hands a signed envelope to the beacon node to broadcast.
func (c *beaconClient) submitExecutionProof(ctx context.Context, signed *ethpb.SignedExecutionProofEnvelope) error {
	body, err := json.Marshal(structs.SignedExecutionProofEnvelopeFromConsensus(signed))
	if err != nil {
		return fmt.Errorf("marshal execution proof envelope: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.endpoint+"/eth/v1/prover/execution_proofs", bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", api.JsonMediaType)

	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("submit execution proof: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return errorFor(resp)
	}
	return nil
}

// chainConfig describes this chain to the proof node. The genesis time comes
// from the beacon node rather than the preset, so that the fork activation
// timestamp is right on a devnet with its own genesis.
func (c *beaconClient) chainConfig(ctx context.Context) (proofnode.ChainConfig, error) {
	genesisTime, err := c.genesisTime(ctx)
	if err != nil {
		return proofnode.ChainConfig{}, err
	}

	return proofnode.ChainConfigAt(genesisTime), nil
}

// genesisTime fetches and caches this chain's genesis time.
func (c *beaconClient) genesisTime(ctx context.Context) (time.Time, error) {
	c.genesisMu.Lock()
	defer c.genesisMu.Unlock()

	if !c.genesis.IsZero() {
		return c.genesis, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/eth/v1/beacon/genesis", nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", api.JsonMediaType)

	resp, err := c.hc.Do(req)
	if err != nil {
		return time.Time{}, fmt.Errorf("call beacon node: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return time.Time{}, errorFor(resp)
	}

	var genesis struct {
		Data struct {
			GenesisTime string `json:"genesis_time"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&genesis); err != nil {
		return time.Time{}, fmt.Errorf("decode genesis: %w", err)
	}
	seconds, err := strconv.ParseInt(genesis.Data.GenesisTime, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse genesis time: %w", err)
	}

	c.genesis = time.Unix(seconds, 0)
	return c.genesis, nil
}

func (c *beaconClient) getSSZ(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", api.OctetStreamMediaType)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call beacon node: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, errorFor(resp)
	}
	return io.ReadAll(resp.Body)
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		log.WithError(err).Debug("Could not close beacon node response body")
	}
}

func errorFor(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return fmt.Errorf("beacon node returned %s", resp.Status)
	}

	return fmt.Errorf("beacon node returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}
