package optionalproofs

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	api_client "github.com/OffchainLabs/prysm/v7/api/client"
	"github.com/OffchainLabs/prysm/v7/api/client/event"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	enginev1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	validatorpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1/validator-client"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	accountsiface "github.com/OffchainLabs/prysm/v7/validator/accounts/iface"
	"github.com/OffchainLabs/prysm/v7/validator/accounts/wallet"
	beaconApi "github.com/OffchainLabs/prysm/v7/validator/client/beacon-api"
	"github.com/OffchainLabs/prysm/v7/validator/client/iface"
	"github.com/OffchainLabs/prysm/v7/validator/keymanager"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Config for the optional proofs service.
type Config struct {
	BeaconApiEndpoint string
	ProverApiEndpoint string
	Wallet            *wallet.Wallet
}

// Service subscribes to beacon node SSE block events and prover
// SSE proof events to coordinate optional proof generation.
type Service struct {
	ctx             context.Context
	cancel          context.CancelFunc
	cfg             *Config
	keyManager      keymanager.IKeymanager
	nodeClient      iface.NodeClient
	validatorClient iface.ValidatorClient
}

// NewService creates a new optional proofs service.
func NewService(ctx context.Context, cfg *Config) *Service {
	ctx, cancel := context.WithCancel(ctx)

	restHandler := beaconApi.NewBeaconApiRestHandler(
		http.Client{Transport: otelhttp.NewTransport(api_client.NewCustomHeadersTransport(http.DefaultTransport, nil))},
		cfg.BeaconApiEndpoint,
	)

	return &Service{
		ctx:             ctx,
		cancel:          cancel,
		cfg:             cfg,
		nodeClient:      beaconApi.NewNodeClientWithFallback(restHandler, nil),
		validatorClient: beaconApi.NewBeaconApiValidatorClient(restHandler),
	}
}

// Start the optional proofs service.
func (s *Service) Start() {
	log.Info("Starting optional proofs service")
	go s.listenToBlockEvents()
	go s.listenToProverEvents()
}

// Stop the optional proofs service.
func (s *Service) Stop() error {
	s.cancel()
	log.Info("Stopping optional proofs service")
	return nil
}

// Status returns nil if the service is healthy.
func (s *Service) Status() error {
	return nil
}

func (s *Service) listenToBlockEvents() {
	eventsChannel := make(chan *event.Event, 1)

	httpClient := &http.Client{}
	eventStream, err := event.NewEventStream(s.ctx, httpClient, s.cfg.BeaconApiEndpoint, []string{event.EventBlock})
	if err != nil {
		log.WithError(err).Error("Failed to create block event stream")
		return
	}

	go eventStream.Subscribe(eventsChannel)

	for {
		select {
		case <-s.ctx.Done():
			return
		case ev := <-eventsChannel:
			s.processBlockEvent(ev)
		}
	}
}

func (s *Service) processBlockEvent(ev *event.Event) {
	switch ev.EventType {
	case event.EventBlock:
		if err := s.handleBlockEvent(ev.Data); err != nil {
			log.WithError(err).Error("Failed to handle block event")
		}
	case event.EventConnectionError:
		log.WithField("error", string(ev.Data)).Error("Block event stream connection error")
	case event.EventError:
		log.WithField("error", string(ev.Data)).Error("Block event stream error")
	}
}

func (s *Service) handleBlockEvent(data []byte) error {
	blockEvent := &structs.BlockEvent{}
	if err := json.Unmarshal(data, blockEvent); err != nil {
		return fmt.Errorf("unmarshal block event: %w", err)
	}
	log.WithField("slot", blockEvent.Slot).WithField("block", blockEvent.Block).Info("Received block event")

	blockData, err := s.fetchBlock(blockEvent.Slot)
	if err != nil {
		return fmt.Errorf("fetch block for slot %s: %w", blockEvent.Slot, err)
	}

	req, err := buildNewPayloadRequest(blockData)
	if err != nil {
		return fmt.Errorf("build NewPayloadRequest for slot %s: %w", blockEvent.Slot, err)
	}

	if err := s.submitToProver(req); err != nil {
		return fmt.Errorf("submit to prover for slot %s: %w", blockEvent.Slot, err)
	}

	return nil
}

// fetchBlock fetches a full beacon block by slot from the beacon API and extracts the fields
// needed to build a NewPayloadRequest.
func (s *Service) fetchBlock(slot string) (*blockData, error) {
	url := s.cfg.BeaconApiEndpoint + "/eth/v2/beacon/blocks/" + slot

	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch block: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("Failed to close block response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Parse version and signed block envelope.
	var blockResp structs.GetBlockV2Response
	if err := json.Unmarshal(body, &blockResp); err != nil {
		return nil, fmt.Errorf("unmarshal block response: %w", err)
	}

	if blockResp.Data == nil {
		return nil, fmt.Errorf("block response data is nil")
	}

	// Unmarshal the block message based on version.
	// Currently supports Electra; can be extended for other versions.
	switch blockResp.Version {
	case "electra", "fulu":
		return parseElectraBlock(blockResp.Data.Message)
	default:
		return nil, fmt.Errorf("unsupported block version: %s", blockResp.Version)
	}
}

func parseElectraBlock(raw json.RawMessage) (*blockData, error) {
	var block structs.BeaconBlockElectra
	if err := json.Unmarshal(raw, &block); err != nil {
		return nil, fmt.Errorf("unmarshal electra block: %w", err)
	}

	if block.Body == nil {
		return nil, fmt.Errorf("block body is nil")
	}
	if block.Body.ExecutionPayload == nil {
		return nil, fmt.Errorf("execution payload is nil")
	}

	return &blockData{
		ParentRoot:         block.ParentRoot,
		ExecutionPayload:   block.Body.ExecutionPayload,
		BlobKzgCommitments: block.Body.BlobKzgCommitments,
		ExecutionRequests:  block.Body.ExecutionRequests,
	}, nil
}

func buildNewPayloadRequest(data *blockData) (*enginev1.NewPayloadRequest, error) {
	payload, err := data.ExecutionPayload.ToConsensus()
	if err != nil {
		return nil, fmt.Errorf("convert execution payload: %w", err)
	}

	var execRequests *enginev1.ExecutionRequests
	if data.ExecutionRequests != nil {
		execRequests, err = data.ExecutionRequests.ToConsensus()
		if err != nil {
			return nil, fmt.Errorf("convert execution requests: %w", err)
		}
	}

	versionedHashes, err := kzgCommitmentsToVersionedHashes(data.BlobKzgCommitments)
	if err != nil {
		return nil, fmt.Errorf("convert kzg commitments: %w", err)
	}

	parentRoot, err := hex.DecodeString(strings.TrimPrefix(data.ParentRoot, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode parent root: %w", err)
	}

	return &enginev1.NewPayloadRequest{
		ExecutionPayload:  payload,
		VersionedHashes:   versionedHashes,
		ParentBlockRoot:   parentRoot,
		ExecutionRequests: execRequests,
	}, nil
}

// submitToProver SSZ-marshals the NewPayloadRequest and POSTs it to the prover.
func (s *Service) submitToProver(npReq *enginev1.NewPayloadRequest) error {
	sszBytes, err := npReq.MarshalSSZ()
	if err != nil {
		return fmt.Errorf("marshal SSZ: %w", err)
	}

	url := s.cfg.ProverApiEndpoint + "/v1/execution_proof_requests?proof_types=reth-openvm,reth-risc0,reth-sp1,reth-zisk"

	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, url, bytes.NewReader(sszBytes))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("post to prover: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("Failed to close prover response body")
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read prover response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("prover returned status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		NewPayloadRequestRoot string `json:"new_payload_request_root"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("unmarshal prover response: %w", err)
	}

	log.WithField("newPayloadRequestRoot", result.NewPayloadRequestRoot).Info("Submitted NewPayloadRequest to prover")
	return nil
}

// fetchProof downloads a completed proof from the prover.
func (s *Service) fetchProof(root, proofType string) ([]byte, error) {
	url := fmt.Sprintf("%s/v1/execution_proofs/%s/%s", s.cfg.ProverApiEndpoint, root, proofType)

	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch proof: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("Failed to close proof response body")
		}
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read proof response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prover returned status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// getKeyManager returns the cached keymanager, initializing it on first call.
func (s *Service) getKeyManager() (keymanager.IKeymanager, error) {
	if s.keyManager != nil {
		return s.keyManager, nil
	}

	if s.cfg.Wallet == nil {
		return nil, errors.New("no wallet configured")
	}

	keyManager, err := s.cfg.Wallet.InitializeKeymanager(s.ctx, accountsiface.InitKeymanagerConfig{ListenForChanges: false})
	if err != nil {
		return nil, fmt.Errorf("initialize keymanager: %w", err)
	}

	s.keyManager = keyManager
	return keyManager, nil
}

// signExecutionProof creates an ExecutionProof, signs it with a random active validator key,
// and returns the SignedExecutionProof.
func (s *Service) signExecutionProof(proofData []byte, proofType uint8, newPayloadRequestRoot []byte) (*ethpb.SignedExecutionProof, error) {
	km, err := s.getKeyManager()
	if err != nil {
		return nil, fmt.Errorf("get keymanager: %w", err)
	}

	pubkeys, err := km.FetchValidatingPublicKeys(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch validating public keys: %w", err)
	}
	if len(pubkeys) == 0 {
		return nil, fmt.Errorf("no validating keys available")
	}

	selectedPubkey := pubkeys[rand.IntN(len(pubkeys))]

	// Look up validator index from the beacon node.
	indexResp, err := s.validatorClient.ValidatorIndex(s.ctx, &ethpb.ValidatorIndexRequest{PublicKey: selectedPubkey[:]})
	if err != nil {
		return nil, fmt.Errorf("fetch validator index: %w", err)
	}

	executionProof := &ethpb.ExecutionProof{
		ProofData: proofData,
		ProofType: []byte{proofType},
		PublicInput: &ethpb.PublicInput{
			NewPayloadRequestRoot: newPayloadRequestRoot,
		},
	}

	// Get genesis info for current slot computation.
	genesisResp, err := s.nodeClient.Genesis(s.ctx, &emptypb.Empty{})
	if err != nil {
		return nil, fmt.Errorf("fetch genesis: %w", err)
	}

	currentSlot := slots.CurrentSlot(genesisResp.GenesisTime.AsTime())
	currentEpoch := slots.ToEpoch(currentSlot)

	// Get domain data for execution proof signing.
	domainResp, err := s.validatorClient.DomainData(s.ctx, &ethpb.DomainRequest{
		Epoch:  currentEpoch,
		Domain: params.BeaconConfig().DomainExecutionProof[:],
	})
	if err != nil {
		return nil, fmt.Errorf("fetch domain data: %w", err)
	}

	proofRoot, err := blocks.ExecutionProofHashTreeRoot(executionProof)
	if err != nil {
		return nil, fmt.Errorf("execution proof hash tree root: %w", err)
	}
	signingRoot, err := signing.ComputeSigningRootForRoot(proofRoot, domainResp.SignatureDomain)
	if err != nil {
		return nil, fmt.Errorf("compute signing root: %w", err)
	}

	signature, err := km.Sign(s.ctx, &validatorpb.SignRequest{
		PublicKey:       selectedPubkey[:],
		SigningRoot:     signingRoot[:],
		SignatureDomain: domainResp.SignatureDomain,
		SigningSlot:     currentSlot,
	})
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	return &ethpb.SignedExecutionProof{
		Message:        executionProof,
		ValidatorIndex: indexResp.Index,
		Signature:      signature.Marshal(),
	}, nil
}

// submitSignedProofToBeaconNode POSTs the signed execution proof to the beacon node
// so it can be broadcast to the P2P network.
func (s *Service) submitSignedProofToBeaconNode(signedProof *ethpb.SignedExecutionProof) error {
	proof := &structs.SignedExecutionProof{
		Message: &structs.ExecutionProof{
			ProofData: signedProof.Message.ProofData,
			ProofType: signedProof.Message.ProofType[0],
			PublicInput: &structs.PublicInput{
				NewPayloadRequestRoot: signedProof.Message.PublicInput.NewPayloadRequestRoot,
			},
		},
		ValidatorIndex: uint64(signedProof.ValidatorIndex),
		Signature:      signedProof.Signature,
	}

	body, err := json.Marshal(proof)
	if err != nil {
		return fmt.Errorf("marshal proof: %w", err)
	}

	url := s.cfg.BeaconApiEndpoint + "/eth/v1/prover/execution_proofs"
	req, err := http.NewRequestWithContext(s.ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return fmt.Errorf("submit proof: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("Failed to close proof submission response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// connectToProverSSE creates and executes an SSE request to the prover endpoint,
// retrying indefinitely with a 10 second interval until the connection succeeds
// or the service context is cancelled.
func (s *Service) connectToProverSSE() (*http.Response, error) {
	url := s.cfg.ProverApiEndpoint + "/v1/execution_proof_requests"
	log.WithField("url", url).Info("Subscribing to prover events")

	req, err := http.NewRequestWithContext(s.ctx, http.MethodGet, url, nil)
	if err != nil {
		log.WithError(err).Error("Failed to create prover SSE request")
		return nil, err
	}
	req.Header.Set("Accept", api.EventStreamMediaType)
	req.Header.Set("Connection", api.KeepAlive)

	log := log.WithField("url", url)

	for {
		resp, err := (&http.Client{}).Do(req)
		if err == nil {
			log.Info("Connected to prover SSE endpoint")
			return resp, nil
		}

		log.WithError(err).Warning("Failed to connect to prover SSE endpoint, retrying")

		select {
		case <-s.ctx.Done():
			return nil, s.ctx.Err()

		case <-time.After(10 * time.Second):
		}
	}
}

// listenToProverEvents subscribes to the prover's SSE endpoint to receive
// proof_complete and proof_failure events.
func (s *Service) listenToProverEvents() {
	resp, err := s.connectToProverSSE()
	if err != nil {
		log.WithError(err).Error("Could not connect to prover SSE endpoint, giving up")
		return
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.WithError(err).Error("Failed to close prover SSE response body")
		}
	}()

	scanner := bufio.NewScanner(resp.Body)
	var eventType, data string

	for scanner.Scan() {
		select {
		case <-s.ctx.Done():
			return
		default:
			line := scanner.Text()
			if strings.HasPrefix(line, ":") {
				continue
			}
			if line == "" {
				if eventType != "" && data != "" {
					s.processProverEvent(eventType, data)
				}
				eventType, data = "", ""
				continue
			}
			if et, ok := strings.CutPrefix(line, "event: "); ok {
				eventType = et
			}
			if d, ok := strings.CutPrefix(line, "data: "); ok {
				data = d
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.WithError(err).Error("Prover SSE stream scanner error")
	}
}

type proofCompleteEvent struct {
	NewPayloadRequestRoot string `json:"new_payload_request_root"`
	ProofType             string `json:"proof_type"`
}

type proofFailureEvent struct {
	NewPayloadRequestRoot string `json:"new_payload_request_root"`
	ProofType             string `json:"proof_type"`
	Reason                string `json:"reason"`
	Error                 string `json:"error"`
}

func (s *Service) processProverEvent(eventType, data string) {
	switch eventType {
	case "proof_complete":
		ev := &proofCompleteEvent{}
		if err := json.Unmarshal([]byte(data), ev); err != nil {
			log.WithError(err).Error("Failed to unmarshal proof_complete event")
			return
		}

		proofBytes, err := s.fetchProof(ev.NewPayloadRequestRoot, ev.ProofType)
		if err != nil {
			log.WithError(err).WithField("root", ev.NewPayloadRequestRoot).WithField("proofType", ev.ProofType).Error("Failed to fetch proof from prover")
			return
		}

		log.WithField("root", ev.NewPayloadRequestRoot).WithField("proofType", ev.ProofType).WithField("proofSize", len(proofBytes)).Info("Fetched proof from prover")

		rootBytes, err := bytesutil.DecodeHexWithLength(ev.NewPayloadRequestRoot, 32)
		if err != nil {
			log.WithError(err).Error("Failed to decode new_payload_request_root")
			return
		}

		proofTypeID, err := ethpb.ProofTypeIndex(ev.ProofType)
		if err != nil {
			log.WithError(err).WithField("proofType", ev.ProofType).Error("Unknown proof type")
			return
		}

		signedProof, err := s.signExecutionProof(proofBytes, proofTypeID, rootBytes)
		if err != nil {
			log.WithError(err).Error("Failed to sign execution proof")
			return
		}

		if err := s.submitSignedProofToBeaconNode(signedProof); err != nil {
			log.WithError(err).Error("Failed to submit signed proof to beacon node")
			return
		}

		log.WithFields(logrus.Fields{
			"root":           ev.NewPayloadRequestRoot,
			"validatorIndex": signedProof.ValidatorIndex,
		}).Info("Submitted signed execution proof to beacon node")

	case "proof_failure":
		ev := &proofFailureEvent{}
		if err := json.Unmarshal([]byte(data), ev); err != nil {
			log.WithError(err).Error("Failed to unmarshal proof_failure event")
			return
		}

		log.WithField("root", ev.NewPayloadRequestRoot).WithField("proofType", ev.ProofType).WithField("reason", ev.Reason).WithField("error", ev.Error).Warn("Received proof_failure event")
	default:
		log.WithField("eventType", eventType).Warn("Received unknown prover event type")
	}
}
