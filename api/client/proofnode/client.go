// Package proofnode implements a client for the EIP-8025 proof node API, the
// component the specification calls the proof engine.
//
// It generates execution proofs asynchronously and verifies them on demand.
package proofnode

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/config/params"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// ErrProofInvalid is returned when the proof node ran verification and the
// proof did not pass.
var (
	ErrProofInvalid = errors.New("execution proof is invalid")

	// errProofUnavailable is returned when the proof node has no proof for the
	// requested identifier and type.
	errProofUnavailable = errors.New("execution proof is unavailable")
)

const (
	sszContentType = "application/octet-stream"

	// maxResponseSize bounds the JSON responses this client reads. Proof
	// downloads are read separately and bounded by MAX_PROOF_SIZE.
	maxResponseSize = 1 << 20
)

// sseEvent names an event the proof node emits on the proof request stream.
type sseEvent string

const (
	// sseProofComplete reports that a proof finished generating and can be
	// downloaded.
	sseProofComplete sseEvent = "proof_complete"

	// sseProofFailure reports that a proof will not be generated.
	sseProofFailure sseEvent = "proof_failure"
)

// Client talks to a proof node over HTTP.
type (
	// Client represents a proof node client that communicates over HTTP.
	Client struct {
		endpoint string
		hc       *http.Client
	}

	// proofTypeInfo describes one proof type the node has configured.
	proofTypeInfo struct {
		ProofType string `json:"proof_type"`
		Kind      string `json:"kind"`
		CanProve  bool   `json:"can_prove"`
		CanVerify bool   `json:"can_verify"`
	}

	// proofEvent reports the terminal outcome of a generation request for one proof
	// type. Err is nil when the proof completed and is ready to download.
	proofEvent struct {
		ProofType ethpb.ProofType
		Err       error
	}
)

// NewClient returns a proof node client for the given base endpoint.
func NewClient(endpoint string) (*Client, error) {
	if endpoint == "" {
		return nil, errors.New("proof node endpoint is empty")
	}

	uri, err := url.ParseRequestURI(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse request uri: %w", err)
	}
	if uri.Scheme == "" || uri.Host == "" {
		return nil, fmt.Errorf("proof node endpoint must be an absolute http(s) URL, got %q", endpoint)
	}

	return &Client{
		endpoint: strings.TrimSuffix(endpoint, "/"),
		hc:       &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// ProvableTypes returns the proof types this node can generate, resolved to
// their identifiers. Types the node reports but Prysm does not recognise are
// skipped.
func (c *Client) ProvableTypes(ctx context.Context) ([]ethpb.ProofType, error) {
	infos, err := c.proofTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("proof types: %w", err)
	}

	var out []ethpb.ProofType
	for _, info := range infos {
		if !info.CanProve {
			continue
		}

		proofType, err := ethpb.ParseProofType(info.ProofType)
		if err != nil {
			log.WithField("proofType", info.ProofType).Debug("Skipping proof type the proof node reports but Prysm does not recognise")
			continue
		}

		out = append(out, proofType)
	}

	return out, nil
}

// RequestProofs asks the proof node to start generating the given proof types
// for a payload. It returns the hash tree root of the new payload request,
// which identifies the generation request when retrieving the proofs.
func (c *Client) RequestProofs(
	ctx context.Context,
	newPayloadRequest *enginev1.SSZNewPayloadRequest,
	cfg ChainConfig,
	proofTypes []ethpb.ProofType,
) ([32]byte, error) {
	if len(proofTypes) == 0 {
		return [32]byte{}, errors.New("no proof types requested")
	}

	body, err := marshalRequestBody(protocolFork(), newPayloadRequest, cfg, proofTypes)
	if err != nil {
		return [32]byte{}, err
	}

	resp, err := c.do(ctx, http.MethodPost, "/v1/execution_proof_requests", body, sszContentType)
	if err != nil {
		return [32]byte{}, err
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return [32]byte{}, errorFor(resp)
	}

	var result struct {
		NewPayloadRequestRoot string `json:"new_payload_request_root"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&result); err != nil {
		return [32]byte{}, fmt.Errorf("new decoder: %w", err)
	}

	root, err := hexutil.Decode(result.NewPayloadRequestRoot)
	if err != nil {
		return [32]byte{}, fmt.Errorf("decode: %w", err)
	}
	if len(root) != 32 {
		return [32]byte{}, fmt.Errorf("new payload request root has length %d, want 32", len(root))
	}

	return [32]byte(root), nil
}

// AwaitProof waits for the proof node to finish generating one proof type for
// newPayloadRequestRoot, then downloads it.
//
// The node replays terminal events cached from before the subscription, so it
// is safe to call this after RequestProofs without racing the result.
func (c *Client) AwaitProof(
	ctx context.Context,
	newPayloadRequestRoot [32]byte,
	proofType ethpb.ProofType,
) ([]byte, error) {
	events, err := c.subscribe(ctx, newPayloadRequestRoot)
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return nil, errors.New("proof node event stream closed before the proof completed")
			}

			if event.ProofType != proofType {
				continue
			}

			if event.Err != nil {
				return nil, event.Err
			}

			proof, err := c.getProof(ctx, newPayloadRequestRoot, proofType)
			if err != nil {
				return nil, fmt.Errorf("get proof: %w", err)
			}

			return proof, nil
		}
	}
}

// VerifyProof asks the proof node to verify a single proof against the payload
// identified by newPayloadRequestRoot.
//
// It returns nil when the proof is valid, ErrProofInvalid when verification ran
// and rejected the proof, and any other error when verification could not be
// performed.
func (c *Client) VerifyProof(
	ctx context.Context,
	newPayloadRequestRoot [32]byte,
	proofType ethpb.ProofType,
	proof []byte,
	cfg ChainConfig,
) error {
	body := marshalVerificationBody(protocolFork(), newPayloadRequestRoot, cfg, proofType, proof)

	resp, err := c.do(ctx, http.MethodPost, "/v1/execution_proof_verifications", body, sszContentType)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return errorFor(resp)
	}

	var result struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&result); err != nil {
		return fmt.Errorf("decode verification response: %w", err)
	}
	if result.Status != "VALID" {
		return ErrProofInvalid
	}
	return nil
}

// subscribe opens the server-sent event stream filtered to one generation
// request. The returned channel is closed when the stream ends.
func (c *Client) subscribe(ctx context.Context, newPayloadRequestRoot [32]byte) (<-chan proofEvent, error) {
	path := "/v1/execution_proof_requests?new_payload_request_root=" + hexutil.Encode(newPayloadRequestRoot[:])

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+path, nil)
	if err != nil {
		return nil, fmt.Errorf("new request with context: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	// The stream is long lived, so it must not inherit the client timeout.
	hc := &http.Client{Transport: c.hc.Transport}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		// errorFor reads the body, so close it only once it has.
		err := errorFor(resp)
		closeBody(resp)
		return nil, err
	}

	// From here the stream is handed to the goroutine, which owns the body.
	events := make(chan proofEvent, 1)
	go func() {
		defer close(events)
		defer closeBody(resp)

		// Parse the server-sent event stream. Comment lines, which the proof
		// node sends as keep-alives, are ignored.
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 4096), maxResponseSize)

		var eventName sseEvent
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			switch {
			case line == "":
				eventName = ""
			case strings.HasPrefix(line, ":"):
				// Keep-alive comment.
			case strings.HasPrefix(line, "event:"):
				eventName = sseEvent(strings.TrimSpace(strings.TrimPrefix(line, "event:")))
			case strings.HasPrefix(line, "data:"):
				data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				event, ok := parseProofEvent(eventName, data)
				if !ok {
					continue
				}
				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := scanner.Err(); err != nil && ctx.Err() == nil {
			log.WithError(err).Debug("Proof node event stream ended")
		}
	}()
	return events, nil
}

// proofTypes lists the proof types the node has initialised.
func (c *Client) proofTypes(ctx context.Context) ([]proofTypeInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/proof_types", nil, "")
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, errorFor(resp)
	}

	var result struct {
		ProofTypes []proofTypeInfo `json:"proof_types"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode proof types response: %w", err)
	}
	return result.ProofTypes, nil
}

// getProof downloads a completed proof. It returns ErrProofUnavailable if the
// proof node does not have one for this identifier and type.
func (c *Client) getProof(ctx context.Context, newPayloadRequestRoot [32]byte, proofType ethpb.ProofType) ([]byte, error) {
	path := fmt.Sprintf("/v1/execution_proofs/%s/%s", hexutil.Encode(newPayloadRequestRoot[:]), proofType)

	resp, err := c.do(ctx, http.MethodGet, path, nil, "")
	if err != nil {
		return nil, fmt.Errorf("do: %w", err)
	}
	defer closeBody(resp)

	if resp.StatusCode == http.StatusNotFound {
		return nil, errProofUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errorFor(resp)
	}

	proof, err := io.ReadAll(io.LimitReader(resp.Body, int64(params.BeaconConfig().MaxProofSize)+1))
	if err != nil {
		return nil, fmt.Errorf("read all: %w", err)
	}
	if uint64(len(proof)) > params.BeaconConfig().MaxProofSize {
		return nil, fmt.Errorf("proof exceeds MAX_PROOF_SIZE of %d bytes", params.BeaconConfig().MaxProofSize)
	}
	return proof, nil
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType string) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+path, reader)
	if err != nil {
		return nil, fmt.Errorf("new request with context: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

func parseProofEvent(name sseEvent, data string) (proofEvent, bool) {
	var payload struct {
		ProofType string `json:"proof_type"`
		Reason    string `json:"reason"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		log.WithError(err).Debug("Could not decode proof node event")
		return proofEvent{}, false
	}
	proofType, err := ethpb.ParseProofType(payload.ProofType)
	if err != nil {
		return proofEvent{}, false
	}

	switch name {
	case sseProofComplete:
		return proofEvent{ProofType: proofType}, true
	case sseProofFailure:
		return proofEvent{
			ProofType: proofType,
			Err:       fmt.Errorf("proof generation failed (%s): %s", payload.Reason, payload.Error),
		}, true
	default:
		return proofEvent{}, false
	}
}

// protocolFork returns the proof node's fork discriminant for the active
// stateless input schema. STATELESS_INPUT_SCHEMA_ID encodes the execution-layer
// protocol fork in its high byte and the schema revision in its low byte.
func protocolFork() uint8 {
	return uint8(params.BeaconConfig().StatelessInputSchemaId >> 8)
}

func closeBody(resp *http.Response) {
	if err := resp.Body.Close(); err != nil {
		log.WithError(err).Error("Could not close proof node response body")
	}
}

// errorFor drains a non-2xx response into an error.
func errorFor(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("proof node returned %s", resp.Status)
	}

	return fmt.Errorf("proof node returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}
