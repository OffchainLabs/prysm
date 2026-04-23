package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	payloadattribute "github.com/OffchainLabs/prysm/v7/consensus-types/payload-attribute"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	pb "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pkg/errors"
)

const (
	// sszContentType is the Content-Type header used for SSZ-REST requests/responses.
	sszContentType = "application/octet-stream"

	// maxResponseSize is the maximum allowed SSZ response body size (32 MB).
	maxResponseSize = 32 * 1024 * 1024
)

// sszRestClient handles SSZ-REST communication with the execution layer per EIP-8161.
type sszRestClient struct {
	baseURL    string
	httpClient *http.Client
}

// newSSZRestClient creates a new SSZ-REST client with the given base URL and HTTP client.
func newSSZRestClient(baseURL string, httpClient *http.Client) *sszRestClient {
	return &sszRestClient{
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

// sszRestError represents an error response from the SSZ-REST API.
type sszRestError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *sszRestError) Error() string {
	return fmt.Sprintf("SSZ-REST error (code %d): %s", e.Code, e.Message)
}

// doRequest sends an SSZ-encoded POST request and returns the SSZ-encoded response body.
func (c *sszRestClient) doRequest(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.doHTTP(ctx, http.MethodPost, path, body)
}

// doGetRequest sends a GET request (no body) and returns the SSZ-encoded response body.
func (c *sszRestClient) doGetRequest(ctx context.Context, path string) ([]byte, error) {
	return c.doHTTP(ctx, http.MethodGet, path, nil)
}

func (c *sszRestClient) doHTTP(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	url := c.baseURL + path
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "create SSZ-REST request")
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", sszContentType)
	}
	req.Header.Set("Accept", sszContentType)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "SSZ-REST request failed")
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return nil, errors.Wrap(err, "read SSZ-REST response")
	}

	if resp.StatusCode != http.StatusOK {
		var restErr sszRestError
		if jsonErr := json.Unmarshal(respBody, &restErr); jsonErr == nil && restErr.Code != 0 {
			return nil, handleSSZRestError(&restErr)
		}

		// Error responses use text/plain per execution-apis SSZ spec.
		errMsg := string(respBody)
		if errMsg == "" {
			errMsg = resp.Status
		}
		return nil, fmt.Errorf("SSZ-REST %s %s returned status %d: %s", method, path, resp.StatusCode, errMsg)
	}

	return respBody, nil
}

// handleSSZRestError maps SSZ-REST error codes to existing engine API errors.
func handleSSZRestError(e *sszRestError) error {
	switch e.Code {
	case -32700:
		errParseCount.Inc()
		return ErrParse
	case -32600:
		errInvalidRequestCount.Inc()
		return ErrInvalidRequest
	case -32601:
		errMethodNotFoundCount.Inc()
		return ErrMethodNotFound
	case -32602:
		errInvalidParamsCount.Inc()
		return ErrInvalidParams
	case -32603:
		errInternalCount.Inc()
		return ErrInternal
	case -38001:
		errUnknownPayloadCount.Inc()
		return ErrUnknownPayload
	case -38002:
		errInvalidForkchoiceStateCount.Inc()
		return ErrInvalidForkchoiceState
	case -38003:
		errInvalidPayloadAttributesCount.Inc()
		return ErrInvalidPayloadAttributes
	case -38004:
		errRequestTooLargeCount.Inc()
		return ErrRequestTooLarge
	case -32000:
		errServerErrorCount.Inc()
		return errors.Wrapf(ErrServer, "%s", e.Message)
	default:
		return e
	}
}

// setupSSZRestClient creates an SSZ-REST client using the same URL as the
// Engine API JSON-RPC endpoint. SSZ-REST routes are served on the same port
// under /engine/* paths. Auto-probes availability on first use.
func (s *Service) setupSSZRestClient() {
	engineURL := s.cfg.currHttpEndpoint.Url
	if engineURL == "" {
		s.sszRestClientLock.Lock()
		defer s.sszRestClientLock.Unlock()
		s.sszRestClient = nil
		return
	}

	// Derive SSZ-REST base URL from the execution endpoint (same host:port).
	baseURL := strings.TrimRight(engineURL, "/")
	httpClient := s.cfg.currHttpEndpoint.HttpClient()
	s.sszRestClientLock.Lock()
	defer s.sszRestClientLock.Unlock()
	s.sszRestClient = newSSZRestClient(baseURL, httpClient)
	log.WithField("url", baseURL).Info("SSZ-REST Engine API transport enabled (EIP-8161)")
}

func (s *Service) getSSZRestClient() *sszRestClient {
	s.sszRestClientLock.RLock()
	defer s.sszRestClientLock.RUnlock()
	return s.sszRestClient
}

// isSSZRestAvailable returns true if the SSZ-REST client is configured and ready to use.
func (s *Service) isSSZRestAvailable() bool {
	return s.getSSZRestClient() != nil
}

// SSZ-REST Engine API method implementations.
// These methods correspond to the EIP-8161 REST endpoints.

// newPayloadSSZRest sends a NewPayload request via SSZ-REST.
func (s *Service) newPayloadSSZRest(
	ctx context.Context,
	payload interfaces.ExecutionData,
	versionedHashes []common.Hash,
	parentBlockRoot *common.Hash,
	executionRequests *pb.ExecutionRequests,
) ([]byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.NewPayloadSSZRest")
	defer span.End()

	client := s.getSSZRestClient()
	if client == nil {
		return nil, errors.New("SSZ-REST client unavailable")
	}

	// Determine the version path based on payload type.
	var versionPath string
	switch payload.Proto().(type) {
	case *pb.ExecutionPayload:
		versionPath = "/engine/v1/new_payload"
	case *pb.ExecutionPayloadCapella:
		versionPath = "/engine/v2/new_payload"
	case *pb.ExecutionPayloadDeneb:
		if executionRequests != nil {
			versionPath = "/engine/v4/new_payload"
		} else {
			versionPath = "/engine/v3/new_payload"
		}
	case *pb.ExecutionPayloadGloas:
		versionPath = "/engine/v5/new_payload"
	default:
		return nil, errors.New("unknown execution data type for SSZ-REST")
	}

	// Build the SSZ-encoded request body.
	reqBody, err := marshalNewPayloadRequest(payload, versionedHashes, parentBlockRoot, executionRequests)
	if err != nil {
		return nil, errors.Wrap(err, "marshal SSZ-REST new_payload request")
	}

	respBody, err := client.doRequest(ctx, versionPath, reqBody)
	if err != nil {
		return nil, err
	}

	// Parse the PayloadStatusSSZ response.
	status, err := unmarshalPayloadStatusSSZ(respBody)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ-REST new_payload response")
	}

	return status.LatestValidHash, handlePayloadStatus(status)
}

// forkchoiceUpdatedSSZRest sends a ForkchoiceUpdated request via SSZ-REST.
func (s *Service) forkchoiceUpdatedSSZRest(
	ctx context.Context,
	state *pb.ForkchoiceState,
	attrs payloadattribute.Attributer,
) (*pb.PayloadIDBytes, []byte, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ForkchoiceUpdatedSSZRest")
	defer span.End()

	client := s.getSSZRestClient()
	if client == nil {
		return nil, nil, errors.New("SSZ-REST client unavailable")
	}

	// Build the SSZ-encoded request body.
	reqBody, err := marshalForkchoiceUpdatedRequest(state, attrs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal SSZ-REST forkchoice_updated request")
	}

	respBody, err := client.doRequest(ctx, "/engine/v3/forkchoice_updated", reqBody)
	if err != nil {
		return nil, nil, err
	}

	// Parse the ForkchoiceUpdatedResponse.
	fcuResp, err := unmarshalForkchoiceUpdatedResponseSSZ(respBody)
	if err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal SSZ-REST forkchoice_updated response")
	}

	return fcuResp.PayloadId, fcuResp.Status.LatestValidHash, handlePayloadStatus(fcuResp.Status)
}

// handlePayloadStatus converts a PayloadStatus proto into the standard error handling.
func handlePayloadStatus(status *pb.PayloadStatus) error {
	if status.ValidationError != "" {
		log.WithField("status", status.Status.String()).
			WithError(errors.New(status.ValidationError)).
			Error("Got a validation error in SSZ-REST payload response")
	}
	switch status.Status {
	case pb.PayloadStatus_INVALID_BLOCK_HASH:
		return ErrInvalidBlockHashPayloadStatus
	case pb.PayloadStatus_ACCEPTED, pb.PayloadStatus_SYNCING:
		return ErrAcceptedSyncingPayloadStatus
	case pb.PayloadStatus_INVALID:
		return ErrInvalidPayloadStatus
	case pb.PayloadStatus_VALID:
		return nil
	default:
		return errors.Wrapf(ErrUnknownPayloadStatus, "unknown payload status: %s", status.Status.String())
	}
}

// getPayloadSSZRest sends a GetPayload request via SSZ-REST.
func (s *Service) getPayloadSSZRest(ctx context.Context, payloadId [8]byte, slot primitives.Slot) (*blocks.GetPayloadResponse, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetPayloadSSZRest")
	defer span.End()

	client := s.getSSZRestClient()
	if client == nil {
		return nil, errors.New("SSZ-REST client unavailable")
	}

	// Determine version path based on slot/epoch.
	epoch := slots.ToEpoch(slot)
	cfg := params.BeaconConfig()
	var version int
	switch {
	case epoch >= cfg.GloasForkEpoch:
		version = 6
	case epoch >= cfg.FuluForkEpoch:
		version = 5
	case epoch >= cfg.ElectraForkEpoch:
		version = 4
	case epoch >= cfg.DenebForkEpoch:
		version = 3
	case epoch >= cfg.CapellaForkEpoch:
		version = 2
	default:
		version = 1
	}

	// POST /engine/v{N}/get_payload with 8-byte payload ID body
	versionPath := fmt.Sprintf("/engine/v%d/get_payload", version)

	respBody, err := client.doRequest(ctx, versionPath, payloadId[:])
	if err != nil {
		return nil, err
	}

	resp, err := unmarshalGetPayloadResponseSSZ(respBody, version)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ-REST get_payload response")
	}
	return resp, nil
}

// getBlobsSSZRest sends a GetBlobs request via SSZ-REST.
func (s *Service) getBlobsSSZRest(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobsSSZRest")
	defer span.End()

	client := s.getSSZRestClient()
	if client == nil {
		return nil, errors.New("SSZ-REST client unavailable")
	}

	reqBody := marshalGetBlobsRequest(versionedHashes)

	respBody, err := client.doRequest(ctx, "/engine/v1/get_blobs", reqBody)
	if err != nil {
		return nil, err
	}

	return unmarshalGetBlobsResponseSSZ(respBody)
}

// exchangeCapabilitiesSSZRest sends an ExchangeCapabilities request via SSZ-REST.
func (s *Service) exchangeCapabilitiesSSZRest(ctx context.Context, capabilities []string) ([]string, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExchangeCapabilitiesSSZRest")
	defer span.End()

	client := s.getSSZRestClient()
	if client == nil {
		return nil, errors.New("SSZ-REST client unavailable")
	}

	reqBody := marshalExchangeCapabilitiesRequest(capabilities)

	respBody, err := client.doRequest(ctx, "/engine/v1/exchange_capabilities", reqBody)
	if err != nil {
		return nil, err
	}

	return unmarshalExchangeCapabilitiesResponse(respBody)
}

// getClientVersionSSZRest sends a GetClientVersion request via SSZ-REST.
func (s *Service) getClientVersionSSZRest(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetClientVersionSSZRest")
	defer span.End()

	client := s.getSSZRestClient()
	if client == nil {
		return nil, errors.New("SSZ-REST client unavailable")
	}

	commit := version.GitCommit()
	if len(commit) >= 8 {
		commit = commit[:8]
	}

	reqBody := marshalClientVersionRequest("PM", "Prysm", version.SemanticVersion(), commit)

	respBody, err := client.doRequest(ctx, "/engine/v1/get_client_version", reqBody)
	if err != nil {
		return nil, err
	}

	return unmarshalClientVersionResponse(respBody)
}
