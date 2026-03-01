package execution

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
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
	// sszRestProtocol is the protocol identifier for SSZ-REST in EIP-8160 communication channels.
	sszRestProtocol = "ssz_rest"

	// sszContentType is the Content-Type header used for SSZ-REST requests/responses.
	sszContentType = "application/octet-stream"

	// channelRefreshInterval is how often to refresh communication channels from the EL.
	channelRefreshInterval = 5 * time.Minute

	// maxResponseSize is the maximum allowed SSZ response body size (32 MB).
	maxResponseSize = 32 * 1024 * 1024
)

// sszRestClient handles SSZ-REST communication with the execution layer per EIP-8161.
type sszRestClient struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.RWMutex
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
	url := c.baseURL + path
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, errors.Wrap(err, "create SSZ-REST request")
	}
	req.Header.Set("Content-Type", sszContentType)

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
		if jsonErr := json.Unmarshal(respBody, &restErr); jsonErr == nil {
			return nil, handleSSZRestError(&restErr)
		}
		return nil, fmt.Errorf("SSZ-REST request to %s returned status %d", path, resp.StatusCode)
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

// sszRestAvailableURL returns the SSZ-REST base URL from the discovered communication channels,
// or empty string if SSZ-REST is not available or disabled.
func sszRestAvailableURL(channels []*structs.CommunicationChannel) string {
	if flags.Get().DisableSSZRest {
		return ""
	}
	for _, ch := range channels {
		if ch.Protocol == sszRestProtocol {
			return ch.URL
		}
	}
	return ""
}

// refreshCommunicationChannels re-fetches communication channels from the EL
// and updates the SSZ-REST client state accordingly.
// Prefers engine_exchangeCapabilitiesV2 (EIP-8160), falls back to V1 channels method.
func (s *Service) refreshCommunicationChannels() {
	ctx, cancel := context.WithTimeout(s.ctx, defaultEngineTimeout)
	defer cancel()

	// Try V2 first.
	var v2Result structs.ExchangeCapabilitiesV2Response
	if err := s.rpcClient.CallContext(ctx, &v2Result, ExchangeCapabilitiesV2, supportedEngineEndpoints); err == nil && len(v2Result.SupportedProtocols) > 0 {
		s.communicationChannels = v2Result.SupportedProtocols
		s.setupSSZRestClient()
		return
	}

	// Fall back to old method.
	channels, err := s.GetClientCommunicationChannelsV1(ctx)
	if err != nil {
		log.WithError(err).Debug("Could not refresh execution client communication channels")
		return
	}
	s.communicationChannels = channels
	s.setupSSZRestClient()
}

// setupSSZRestClient checks the discovered communication channels for ssz_rest
// and creates an HTTP client for SSZ-REST communication if available.
func (s *Service) setupSSZRestClient() {
	baseURL := sszRestAvailableURL(s.communicationChannels)
	if baseURL == "" {
		s.sszRestClient = nil
		return
	}

	// The EL advertises the SSZ-REST URL using its listen address (often 0.0.0.0).
	// In containerized environments (Docker, Kurtosis), we need to replace the host
	// with the one we already use for JSON-RPC (which routes correctly).
	baseURL = s.resolveSSZRestURL(baseURL)

	// Reuse the same JWT authentication as JSON-RPC.
	httpClient := s.cfg.currHttpEndpoint.HttpClient()
	s.sszRestClient = newSSZRestClient(baseURL, httpClient)
	log.WithField("url", baseURL).Info("SSZ-REST Engine API transport enabled (EIP-8161)")
}

// resolveSSZRestURL replaces the host in the SSZ-REST URL with the host from the
// current engine endpoint, keeping the port from the advertised URL. This handles
// containerized environments where the EL advertises 0.0.0.0 as its listen address.
func (s *Service) resolveSSZRestURL(advertisedURL string) string {
	sszURL, err := url.Parse(advertisedURL)
	if err != nil {
		return advertisedURL
	}

	// Extract the engine endpoint host (e.g., "el-1-erigon-prysm" from "el-1-erigon-prysm:8551")
	engineURL := s.cfg.currHttpEndpoint.Url
	engineParsed, err := url.Parse(engineURL)
	if err != nil {
		return advertisedURL
	}

	engineHost := engineParsed.Hostname()
	if engineHost == "" {
		return advertisedURL
	}

	// Only replace if the advertised host is a wildcard address
	sszHost := sszURL.Hostname()
	if sszHost == "0.0.0.0" || sszHost == "::" || sszHost == "" {
		sszPort := sszURL.Port()
		if sszPort != "" {
			sszURL.Host = engineHost + ":" + sszPort
		} else {
			sszURL.Host = engineHost
		}
		return sszURL.String()
	}

	return advertisedURL
}

// isSSZRestAvailable returns true if the SSZ-REST client is configured and ready to use.
func (s *Service) isSSZRestAvailable() bool {
	return s.sszRestClient != nil
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
	default:
		return nil, errors.New("unknown execution data type for SSZ-REST")
	}

	// Build the SSZ-encoded request body.
	reqBody, err := marshalNewPayloadRequest(payload, versionedHashes, parentBlockRoot, executionRequests)
	if err != nil {
		return nil, errors.Wrap(err, "marshal SSZ-REST new_payload request")
	}

	respBody, err := s.sszRestClient.doRequest(ctx, versionPath, reqBody)
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

	// Build the SSZ-encoded request body.
	reqBody, err := marshalForkchoiceUpdatedRequest(state, attrs)
	if err != nil {
		return nil, nil, errors.Wrap(err, "marshal SSZ-REST forkchoice_updated request")
	}

	respBody, err := s.sszRestClient.doRequest(ctx, "/engine/v3/forkchoice_updated", reqBody)
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

// isNetworkError returns true if the error is a network-level error (connection refused,
// timeout, DNS failure, etc.) that warrants falling back to JSON-RPC.
// Non-network errors (invalid payload, protocol-level errors) should not trigger fallback.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	// Timeout errors are network errors.
	if isTimeout(err) {
		return true
	}
	// Check for common network error patterns.
	errStr := err.Error()
	for _, pattern := range []string{
		"connection refused",
		"connection reset",
		"no such host",
		"network is unreachable",
		"SSZ-REST request failed",
		"create SSZ-REST request",
		"read SSZ-REST response",
	} {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
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

	// Determine version path based on slot/epoch.
	epoch := slots.ToEpoch(slot)
	cfg := params.BeaconConfig()
	var versionPath string
	switch {
	case epoch >= cfg.FuluForkEpoch:
		versionPath = "/engine/v5/get_payload"
	case epoch >= cfg.ElectraForkEpoch:
		versionPath = "/engine/v4/get_payload"
	case epoch >= cfg.DenebForkEpoch:
		versionPath = "/engine/v3/get_payload"
	case epoch >= cfg.CapellaForkEpoch:
		versionPath = "/engine/v2/get_payload"
	default:
		versionPath = "/engine/v1/get_payload"
	}

	// Request body: 8-byte uint64 payload ID (LE).
	reqBody := make([]byte, 8)
	binary.LittleEndian.PutUint64(reqBody, binary.LittleEndian.Uint64(payloadId[:]))

	respBody, err := s.sszRestClient.doRequest(ctx, versionPath, reqBody)
	if err != nil {
		return nil, err
	}

	// Parse the GetPayloadResponse SSZ.
	parsed, err := unmarshalGetPayloadResponseSSZ(respBody)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal SSZ-REST get_payload response")
	}

	// Reconstruct the proto types from the SSZ data.
	payload := &pb.ExecutionPayloadDeneb{}
	if err := payload.UnmarshalSSZ(parsed.ExecutionPayloadSSZ); err != nil {
		return nil, errors.Wrap(err, "unmarshal execution payload SSZ")
	}

	ed, err := blocks.WrappedExecutionPayloadDeneb(payload)
	if err != nil {
		return nil, errors.Wrap(err, "wrap execution payload")
	}

	// Use BlobsBundleV2 for Fulu and later, BlobsBundle (v1) otherwise.
	var bundler pb.BlobsBundler
	if epoch >= cfg.FuluForkEpoch {
		bundle := &pb.BlobsBundleV2{}
		if len(parsed.BlobsBundleSSZ) > 0 {
			if err := bundle.UnmarshalSSZ(parsed.BlobsBundleSSZ); err != nil {
				return nil, errors.Wrap(err, "unmarshal blobs bundle v2 SSZ")
			}
		}
		bundler = bundle
	} else {
		bundle := &pb.BlobsBundle{}
		if len(parsed.BlobsBundleSSZ) > 0 {
			if err := bundle.UnmarshalSSZ(parsed.BlobsBundleSSZ); err != nil {
				return nil, errors.Wrap(err, "unmarshal blobs bundle SSZ")
			}
		}
		bundler = bundle
	}

	resp := &blocks.GetPayloadResponse{
		ExecutionData:     ed,
		BlobsBundler:      bundler,
		OverrideBuilder:   parsed.OverrideBuilder,
		Bid:               blockValueToWei(parsed.BlockValue),
		ExecutionRequests: parsed.ExecutionRequests,
	}

	return resp, nil
}

// getBlobsSSZRest sends a GetBlobs request via SSZ-REST.
func (s *Service) getBlobsSSZRest(ctx context.Context, versionedHashes []common.Hash) ([]*pb.BlobAndProof, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetBlobsSSZRest")
	defer span.End()

	reqBody := marshalGetBlobsRequest(versionedHashes)

	respBody, err := s.sszRestClient.doRequest(ctx, "/engine/v1/get_blobs", reqBody)
	if err != nil {
		return nil, err
	}

	return unmarshalGetBlobsResponseSSZ(respBody)
}

// exchangeCapabilitiesSSZRest sends an ExchangeCapabilities request via SSZ-REST.
func (s *Service) exchangeCapabilitiesSSZRest(ctx context.Context, capabilities []string) ([]string, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.ExchangeCapabilitiesSSZRest")
	defer span.End()

	reqBody := marshalExchangeCapabilitiesRequest(capabilities)

	respBody, err := s.sszRestClient.doRequest(ctx, "/engine/v1/exchange_capabilities", reqBody)
	if err != nil {
		return nil, err
	}

	return unmarshalExchangeCapabilitiesResponse(respBody)
}

// getClientVersionSSZRest sends a GetClientVersion request via SSZ-REST.
func (s *Service) getClientVersionSSZRest(ctx context.Context) ([]*structs.ClientVersionV1, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetClientVersionSSZRest")
	defer span.End()

	commit := version.GitCommit()
	if len(commit) >= 8 {
		commit = commit[:8]
	}

	reqBody := marshalClientVersionRequest("PM", "Prysm", version.SemanticVersion(), commit)

	respBody, err := s.sszRestClient.doRequest(ctx, "/engine/v1/get_client_version", reqBody)
	if err != nil {
		return nil, err
	}

	return unmarshalClientVersionResponse(respBody)
}

// getClientCommunicationChannelsSSZRest sends a GetClientCommunicationChannels request via SSZ-REST.
func (s *Service) getClientCommunicationChannelsSSZRest(ctx context.Context) ([]*structs.CommunicationChannel, error) {
	ctx, span := trace.StartSpan(ctx, "powchain.engine-api-client.GetClientCommunicationChannelsSSZRest")
	defer span.End()

	// Request body is empty per EIP-8161.
	respBody, err := s.sszRestClient.doRequest(ctx, "/engine/v1/get_client_communication_channels", nil)
	if err != nil {
		return nil, err
	}

	return unmarshalCommunicationChannelsResponse(respBody)
}

