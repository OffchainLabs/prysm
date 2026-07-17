package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/api"
	builderAPI "github.com/OffchainLabs/prysm/v7/api/client/builder"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/signing"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	types "github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/network"
	"github.com/OffchainLabs/prysm/v7/network/authorization"
	"github.com/OffchainLabs/prysm/v7/network/httputil"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethTypes "github.com/ethereum/go-ethereum/core/types"
	gethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/pkg/errors"
	ssz "github.com/prysmaticlabs/fastssz"
	"github.com/sirupsen/logrus"
)

const (
	statusPath       = "/eth/v1/builder/status"
	registerPath     = "/eth/v1/builder/validators"
	headerPath       = "/eth/v1/builder/header/{slot}/{parent_hash}/{pubkey}"
	headerPathPrefix = "/eth/v1/builder/header/"
	blindedPath      = "/eth/v1/builder/blinded_blocks"
	fuluBlindedPath  = "/eth/v2/builder/blinded_blocks"

	// ForkchoiceUpdatedMethod v1 request string for JSON-RPC.
	ForkchoiceUpdatedMethod = "engine_forkchoiceUpdatedV1"
	// ForkchoiceUpdatedMethodV2 v2 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV2 = "engine_forkchoiceUpdatedV2"
	// ForkchoiceUpdatedMethodV3 v3 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV3 = "engine_forkchoiceUpdatedV3"
	// GetPayloadMethod v1 request string for JSON-RPC.
	GetPayloadMethod = "engine_getPayloadV1"
	// GetPayloadMethodV2 v2 request string for JSON-RPC.
	GetPayloadMethodV2 = "engine_getPayloadV2"
	// GetPayloadMethodV3 v3 request string for JSON-RPC.
	GetPayloadMethodV3 = "engine_getPayloadV3"
	// GetPayloadMethodV4 v4 request string for JSON-RPC.
	GetPayloadMethodV4 = "engine_getPayloadV4"
)

var (
	defaultBuilderHost = "127.0.0.1"
	defaultBuilderPort = 8551
)

type jsonRPCObject struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      uint64 `json:"id"`
	Result  any    `json:"result"`
}

type ForkchoiceUpdatedResponse struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
	ID      uint64 `json:"id"`
	Result  struct {
		Status    *v1.PayloadStatus  `json:"payloadStatus"`
		PayloadId *v1.PayloadIDBytes `json:"payloadId"`
	} `json:"result"`
}

type ExecPayloadResponse struct {
	Version string               `json:"version"`
	Data    *v1.ExecutionPayload `json:"data"`
}
type Builder struct {
	cfg            *config
	address        string
	execClient     *gethRPC.Client
	currId         *v1.PayloadIDBytes
	prevBeaconRoot []byte
	currVersion    int
	currPayload    interfaces.ExecutionData
	blobBundle     *v1.BlobsBundle
	mux            *http.ServeMux
	validatorMap   map[string]*eth.ValidatorRegistrationV1
	valLock        sync.RWMutex
	srv            *http.Server
}

// New creates a proxy server forwarding requests from a consensus client to an execution client.
func New(opts ...Option) (*Builder, error) {
	p := &Builder{
		cfg: &config{
			builderPort: defaultBuilderPort,
			builderHost: defaultBuilderHost,
			logger:      logrus.New(),
		},
	}
	for _, o := range opts {
		if err := o(p); err != nil {
			return nil, err
		}
	}
	if p.cfg.destinationUrl == nil {
		return nil, errors.New("must provide a destination address for request proxying")
	}
	endpoint := network.HttpEndpoint(p.cfg.destinationUrl.String())
	endpoint.Auth.Method = authorization.Bearer
	endpoint.Auth.Value = p.cfg.secret
	execClient, err := network.NewExecutionRPCClient(context.Background(), endpoint, nil)
	if err != nil {
		return nil, err
	}
	router := http.NewServeMux()
	router.HandleFunc(http.MethodGet+" "+statusPath, func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})
	router.HandleFunc(http.MethodPost+" "+registerPath, p.registerValidators)
	router.HandleFunc(http.MethodGet+" "+headerPath, p.handleHeaderRequest)
	router.HandleFunc(http.MethodPost+" "+blindedPath, p.handleBlindedBlock)
	router.HandleFunc(http.MethodPost+" "+fuluBlindedPath, p.handleBlindedBlockPostFulu)
	addr := net.JoinHostPort(p.cfg.builderHost, strconv.Itoa(p.cfg.builderPort))
	srv := &http.Server{
		Handler:           p,
		Addr:              addr,
		ReadHeaderTimeout: time.Second,
	}
	p.address = addr
	p.srv = srv
	p.execClient = execClient
	p.valLock.Lock()
	p.validatorMap = map[string]*eth.ValidatorRegistrationV1{}
	p.valLock.Unlock()
	p.mux = router
	return p, nil
}

// Address for the proxy server.
func (p *Builder) Address() string {
	return p.address
}

// Start a proxy server.
func (p *Builder) Start(ctx context.Context) error {
	p.srv.BaseContext = func(listener net.Listener) context.Context {
		return ctx
	}
	p.cfg.logger.WithFields(logrus.Fields{
		"executionAddress": p.cfg.destinationUrl.String(),
	}).Infof("Builder now listening on address %s", p.address)
	go func() {
		if err := p.srv.ListenAndServe(); err != nil {
			p.cfg.logger.Error(err)
		}
	}()
	for {
		<-ctx.Done()
		return p.srv.Shutdown(context.Background())
	}
}

// ServeHTTP requests from a consensus client to an execution client, modifying in-flight requests
// and/or responses as desired. It also processes any backed-up requests.
func (p *Builder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.cfg.logger.Infof("Received %s request from beacon with url: %s", r.Method, r.URL.Path)
	if p.isBuilderCall(r) {
		p.mux.ServeHTTP(w, r)
		return
	}
	requestBytes, err := parseRequestBytes(r)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not parse request")
		return
	}
	execRes, err := p.sendHttpRequest(r, requestBytes)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not forward request")
		return
	}
	p.cfg.logger.Infof("Received response for %s request with method %s from %s", r.Method, r.Method, p.cfg.destinationUrl.String())

	defer func() {
		if err = execRes.Body.Close(); err != nil {
			p.cfg.logger.WithError(err).Error("Could not do close proxy responseGen body")
		}
	}()

	buf := bytes.NewBuffer([]byte{})
	if _, err = io.Copy(buf, execRes.Body); err != nil {
		p.cfg.logger.WithError(err).Error("Could not copy proxy request body")
		return
	}
	byteResp := bytesutil.SafeCopyBytes(buf.Bytes())
	p.handleEngineCalls(requestBytes, byteResp)
	// Pipe the proxy responseGen to the original caller.
	if _, err = io.Copy(w, buf); err != nil {
		p.cfg.logger.WithError(err).Error("Could not copy proxy request body")
		return
	}
}

func (p *Builder) handleEngineCalls(req, resp []byte) {
	if !isEngineAPICall(req) {
		return
	}
	rpcObj, err := unmarshalRPCObject(req)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not unmarshal rpc object")
		return
	}
	p.cfg.logger.Infof("Received engine call %s", rpcObj.Method)
	switch rpcObj.Method {
	case ForkchoiceUpdatedMethod, ForkchoiceUpdatedMethodV2, ForkchoiceUpdatedMethodV3:
		result := &ForkchoiceUpdatedResponse{}
		err = json.Unmarshal(resp, result)
		if err != nil {
			p.cfg.logger.Errorf("Could not unmarshal fcu: %v", err)
			return
		}
		if result.Result.PayloadId != nil && *result.Result.PayloadId != [8]byte{} {
			p.currId = result.Result.PayloadId
		}
		if rpcObj.Method == ForkchoiceUpdatedMethodV3 {
			attr := &v1.PayloadAttributesV3{}
			obj, err := json.Marshal(rpcObj.Params[1])
			if err != nil {
				p.cfg.logger.Errorf("Could not marshal attr: %v", err)
				return
			}
			if err := json.Unmarshal(obj, attr); err != nil {
				p.cfg.logger.Errorf("Could not unmarshal attr: %v", err)
				return
			}
			p.prevBeaconRoot = attr.ParentBeaconBlockRoot
		}
		payloadID := [8]byte{}
		status := ""
		var lastValHash []byte
		if result.Result.PayloadId != nil {
			payloadID = *result.Result.PayloadId
		}
		if result.Result.Status != nil {
			status = result.Result.Status.Status.String()
			lastValHash = result.Result.Status.LatestValidHash
		}
		p.cfg.logger.Infof("Received payload id of %#x and status of %s along with a valid hash of %#x", payloadID, status, lastValHash)
	}
}

func (p *Builder) isBuilderCall(req *http.Request) bool {
	switch req.URL.Path {
	case statusPath, registerPath, blindedPath, fuluBlindedPath:
		return true
	default:
		return isHeaderCall(req.URL.Path)
	}
}

// isHeaderCall requires all three header params so malformed header paths fall
// through to the proxy, keeping unknown routes proxied once rather than 404'd.
func isHeaderCall(path string) bool {
	if !strings.HasPrefix(path, headerPathPrefix) {
		return false
	}
	params := strings.Split(strings.TrimPrefix(path, headerPathPrefix), "/")
	if len(params) != 3 {
		return false
	}
	return !slices.Contains(params, "")
}

func (p *Builder) registerValidators(w http.ResponseWriter, req *http.Request) {
	registrations, err := decodeValidatorRegistrations(req)
	if err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	p.valLock.Lock()
	for _, registration := range registrations {
		p.validatorMap[hexutil.Encode(registration.Message.Pubkey)] = registration.Message
	}
	p.valLock.Unlock()
	// TODO: Verify Signatures from validators
	w.WriteHeader(http.StatusOK)
}

func decodeValidatorRegistrations(req *http.Request) ([]*eth.SignedValidatorRegistrationV1, error) {
	if httputil.IsRequestSsz(req) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		registrationSize := (&eth.SignedValidatorRegistrationV1{}).SizeSSZ()
		if len(body) == 0 || len(body)%registrationSize != 0 {
			return nil, errors.New("invalid validator registration SSZ framing")
		}
		registrations := make([]*eth.SignedValidatorRegistrationV1, 0, len(body)/registrationSize)
		for offset := 0; offset < len(body); offset += registrationSize {
			registration := &eth.SignedValidatorRegistrationV1{}
			if err := registration.UnmarshalSSZ(body[offset : offset+registrationSize]); err != nil {
				return nil, err
			}
			registrations = append(registrations, registration)
		}
		return registrations, nil
	}

	jsonRegistrations, err := decodeSingleJSON[[]structs.SignedValidatorRegistration](req.Body)
	if err != nil {
		return nil, err
	}
	if len(jsonRegistrations) == 0 {
		return nil, errors.New("empty validator registration JSON batch")
	}
	registrations := make([]*eth.SignedValidatorRegistrationV1, 0, len(jsonRegistrations))
	for _, registration := range jsonRegistrations {
		consensusRegistration, err := registration.ToConsensus()
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, consensusRegistration)
	}
	return registrations, nil
}

func decodeSingleJSON[T any](reader io.Reader) (T, error) {
	var value T
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return value, errors.New("multiple JSON documents")
	} else if !errors.Is(err, io.EOF) {
		return value, err
	}
	return value, nil
}

func writeBuilderResponse(w http.ResponseWriter, req *http.Request, forkVersion int, jsonResponse any, sszResponse ssz.Marshaler) error {
	w.Header().Set(api.VersionHeader, version.String(forkVersion))
	if httputil.RespondWithSsz(req) {
		body, err := sszResponse.MarshalSSZ()
		if err != nil {
			return err
		}
		w.Header().Set("Content-Type", api.OctetStreamMediaType)
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
		return err
	}
	w.Header().Set("Content-Type", api.JsonMediaType)
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(jsonResponse)
}

func (p *Builder) handleHeaderRequest(w http.ResponseWriter, req *http.Request) {
	pHash := req.PathValue("parent_hash")
	if pHash == "" {
		http.Error(w, "no valid parent hash", http.StatusBadRequest)
		return
	}
	_, err := bytesutil.DecodeHexWithLength(pHash, common.HashLength)
	if err != nil {
		http.Error(w, "invalid parent hash", http.StatusBadRequest)
		return
	}
	reqSlot := req.PathValue("slot")
	if reqSlot == "" {
		http.Error(w, "no valid slot provided", http.StatusBadRequest)
		return
	}
	slot, err := strconv.Atoi(reqSlot)
	if err != nil {
		http.Error(w, "invalid slot provided", http.StatusBadRequest)
		return
	}
	reqPubkey := req.PathValue("pubkey")
	if reqPubkey == "" {
		http.Error(w, "no valid pubkey provided", http.StatusBadRequest)
		return
	}
	_, err = bytesutil.DecodeHexWithLength(reqPubkey, fieldparams.BLSPubkeyLength)
	if err != nil {
		http.Error(w, "invalid pubkey", http.StatusBadRequest)
		return
	}
	ax := types.Slot(slot)
	currEpoch := types.Epoch(ax / params.BeaconConfig().SlotsPerEpoch)
	if currEpoch >= params.BeaconConfig().ElectraForkEpoch {
		p.handleHeaderRequestElectra(w, req)
		return
	}

	if currEpoch >= params.BeaconConfig().DenebForkEpoch {
		p.handleHeaderRequestDeneb(w, req)
		return
	}

	if currEpoch >= params.BeaconConfig().CapellaForkEpoch {
		p.handleHeaderRequestCapella(w, req)
		return
	}

	b, err := p.retrievePendingBlock()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve pending block")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	secKey, err := bls.RandKey()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve secret key")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	wObj, err := blocks.WrappedExecutionPayload(b)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not wrap execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hdr, err := blocks.PayloadToHeader(wObj)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make payload into header")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gEth := big.NewInt(int64(params.BeaconConfig().GweiPerEth))
	weiEth := gEth.Mul(gEth, gEth)
	val := builderAPI.Uint256{Int: weiEth}

	wrappedHdr, err := structs.ExecutionPayloadHeaderFromConsensus(hdr)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not convert wrapped header")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bid := &builderAPI.BuilderBid{
		Header: wrappedHdr,
		Value:  val,
		Pubkey: secKey.PublicKey().Marshal(),
	}
	sszBid := &eth.BuilderBid{
		Header: hdr,
		Value:  val.SSZBytes(),
		Pubkey: secKey.PublicKey().Marshal(),
	}
	d, err := signing.ComputeDomain(params.BeaconConfig().DomainApplicationBuilder,
		nil, /* fork version */
		nil /* genesis val root */)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the domain")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rt, err := signing.ComputeSigningRoot(sszBid, d)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the signing root")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sig := secKey.Sign(rt[:])
	hdrResp := &builderAPI.ExecHeaderResponse{
		Version: "bellatrix",
		Data: struct {
			Signature hexutil.Bytes          `json:"signature"`
			Message   *builderAPI.BuilderBid `json:"message"`
		}{
			Signature: sig.Marshal(),
			Message:   bid,
		},
	}
	err = writeBuilderResponse(w, req, version.Bellatrix, hdrResp, &eth.SignedBuilderBid{Message: sszBid, Signature: sig.Marshal()})
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not encode response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.currVersion = version.Bellatrix
	p.currPayload = wObj
}

func (p *Builder) handleHeaderRequestCapella(w http.ResponseWriter, req *http.Request) {
	b, err := p.retrievePendingBlockCapella()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve pending block")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secKey, err := bls.RandKey()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve secret key")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := big.NewInt(0).SetBytes(bytesutil.ReverseByteOrder(b.Value))
	// we set the payload value as twice its actual one so that it always chooses builder payloads vs local payloads
	v = v.Mul(v, big.NewInt(2))
	wObj, err := blocks.WrappedExecutionPayloadCapella(b.Payload)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not wrap execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	hdr, err := blocks.PayloadToHeaderCapella(wObj)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make payload into header")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	val := builderAPI.Uint256{Int: v}
	wrappedHdr, err := structs.ExecutionPayloadHeaderCapellaFromConsensus(hdr)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bid := &builderAPI.BuilderBidCapella{
		Header: wrappedHdr,
		Value:  val,
		Pubkey: secKey.PublicKey().Marshal(),
	}
	sszBid := &eth.BuilderBidCapella{
		Header: hdr,
		Value:  val.SSZBytes(),
		Pubkey: secKey.PublicKey().Marshal(),
	}
	d, err := signing.ComputeDomain(params.BeaconConfig().DomainApplicationBuilder,
		nil, /* fork version */
		nil /* genesis val root */)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the domain")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rt, err := signing.ComputeSigningRoot(sszBid, d)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the signing root")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sig := secKey.Sign(rt[:])
	hdrResp := &builderAPI.ExecHeaderResponseCapella{
		Version: "capella",
		Data: struct {
			Signature hexutil.Bytes                 `json:"signature"`
			Message   *builderAPI.BuilderBidCapella `json:"message"`
		}{
			Signature: sig.Marshal(),
			Message:   bid,
		},
	}
	err = writeBuilderResponse(w, req, version.Capella, hdrResp, &eth.SignedBuilderBidCapella{Message: sszBid, Signature: sig.Marshal()})
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not encode response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.currVersion = version.Capella
	p.currPayload = wObj
}

func (p *Builder) handleHeaderRequestDeneb(w http.ResponseWriter, req *http.Request) {
	b, err := p.retrievePendingBlockDeneb()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve pending block")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secKey, err := bls.RandKey()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve secret key")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := big.NewInt(0).SetBytes(bytesutil.ReverseByteOrder(b.Value))
	// we set the payload value as twice its actual one so that it always chooses builder payloads vs local payloads
	v = v.Mul(v, big.NewInt(2))
	wObj, err := blocks.WrappedExecutionPayloadDeneb(b.Payload)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not wrap execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hdr, err := blocks.PayloadToHeaderDeneb(wObj)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make payload into header")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	val := builderAPI.Uint256{Int: v}
	var commitments []hexutil.Bytes
	for _, c := range b.BlobsBundle.KzgCommitments {
		copiedC := c
		commitments = append(commitments, copiedC)
	}
	wrappedHdr, err := structs.ExecutionPayloadHeaderDenebFromConsensus(hdr)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	bid := &builderAPI.BuilderBidDeneb{
		Header:             wrappedHdr,
		BlobKzgCommitments: commitments,
		Value:              val,
		Pubkey:             secKey.PublicKey().Marshal(),
	}
	sszBid := &eth.BuilderBidDeneb{
		Header:             hdr,
		BlobKzgCommitments: b.BlobsBundle.KzgCommitments,
		Value:              val.SSZBytes(),
		Pubkey:             secKey.PublicKey().Marshal(),
	}
	d, err := signing.ComputeDomain(params.BeaconConfig().DomainApplicationBuilder,
		nil, /* fork version */
		nil /* genesis val root */)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the domain")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rt, err := signing.ComputeSigningRoot(sszBid, d)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the signing root")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sig := secKey.Sign(rt[:])
	hdrResp := &builderAPI.ExecHeaderResponseDeneb{
		Version: "deneb",
		Data: struct {
			Signature hexutil.Bytes               `json:"signature"`
			Message   *builderAPI.BuilderBidDeneb `json:"message"`
		}{
			Signature: sig.Marshal(),
			Message:   bid,
		},
	}
	err = writeBuilderResponse(w, req, version.Deneb, hdrResp, &eth.SignedBuilderBidDeneb{Message: sszBid, Signature: sig.Marshal()})
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not encode response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.currVersion = version.Deneb
	p.currPayload = wObj
	p.blobBundle = b.BlobsBundle
}

func (p *Builder) handleHeaderRequestElectra(w http.ResponseWriter, req *http.Request) {
	b, err := p.retrievePendingBlockElectra()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve pending block")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	secKey, err := bls.RandKey()
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not retrieve secret key")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	v := big.NewInt(0).SetBytes(bytesutil.ReverseByteOrder(b.Value))
	// we set the payload value as twice its actual one so that it always chooses builder payloads vs local payloads
	v = v.Mul(v, big.NewInt(2))
	wObj, err := blocks.WrappedExecutionPayloadDeneb(b.Payload)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not wrap execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hdr, err := blocks.PayloadToHeaderElectra(wObj)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make payload into header")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	val := builderAPI.Uint256{Int: v}
	var commitments []hexutil.Bytes
	for _, c := range b.BlobsBundle.KzgCommitments {
		copiedC := c
		commitments = append(commitments, copiedC)
	}
	wrappedHdr, err := structs.ExecutionPayloadHeaderDenebFromConsensus(hdr)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not make execution payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	requests, err := b.GetDecodedExecutionRequests(params.BeaconConfig().ExecutionRequestLimits())
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not get decoded execution requests")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rv1 := structs.ExecutionRequestsFromConsensus(requests)

	bid := &builderAPI.BuilderBidElectra{
		Header:             wrappedHdr,
		BlobKzgCommitments: commitments,
		Value:              val,
		Pubkey:             secKey.PublicKey().Marshal(),
		ExecutionRequests:  rv1,
	}

	sszBid := &eth.BuilderBidElectra{
		Header:             hdr,
		BlobKzgCommitments: b.BlobsBundle.KzgCommitments,
		Value:              val.SSZBytes(),
		Pubkey:             secKey.PublicKey().Marshal(),
		ExecutionRequests:  requests,
	}
	d, err := signing.ComputeDomain(params.BeaconConfig().DomainApplicationBuilder,
		nil, /* fork version */
		nil /* genesis val root */)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the domain")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	rt, err := signing.ComputeSigningRoot(sszBid, d)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not compute the signing root")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sig := secKey.Sign(rt[:])
	hdrResp := &builderAPI.ExecHeaderResponseElectra{
		Version: "electra",
		Data: struct {
			Signature hexutil.Bytes                 `json:"signature"`
			Message   *builderAPI.BuilderBidElectra `json:"message"`
		}{
			Signature: sig.Marshal(),
			Message:   bid,
		},
	}
	err = writeBuilderResponse(w, req, version.Electra, hdrResp, &eth.SignedBuilderBidElectra{Message: sszBid, Signature: sig.Marshal()})
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not encode response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	p.currVersion = version.Electra
	p.currPayload = wObj
	p.blobBundle = b.BlobsBundle
}

func (p *Builder) handleBlindedBlock(w http.ResponseWriter, req *http.Request) {
	err := p.decodeBlindedBlockRequest(req)
	if err != nil {
		p.cfg.logger.WithError(err).WithField("version", version.String(p.currVersion)).Error("Could not decode blinded block")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if p.currPayload == nil {
		p.cfg.logger.Error("No payload is cached")
		http.Error(w, "payload not found", http.StatusInternalServerError)
		return
	}

	resp, err := ExecutionPayloadResponseFromData(p.currVersion, p.currPayload, p.blobBundle)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not convert the payload")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sszResponse, err := executionPayloadSSZResponse(p.currVersion, p.currPayload, p.blobBundle)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not encode full payload SSZ response")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err = writeBuilderResponse(w, req, p.currVersion, resp, sszResponse); err != nil {
		p.cfg.logger.WithError(err).Error("Could not encode full payload response")
	}
}

func (p *Builder) handleBlindedBlockPostFulu(w http.ResponseWriter, req *http.Request) {
	if err := decodeBlindedBlockFuluRequest(req); err != nil {
		p.cfg.logger.WithError(err).Error("Could not decode Fulu blinded block")
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (p *Builder) decodeBlindedBlockRequest(req *http.Request) error {
	if httputil.IsRequestSsz(req) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return err
		}
		switch p.currVersion {
		case version.Electra:
			return (&eth.SignedBlindedBeaconBlockElectra{}).UnmarshalSSZ(body)
		case version.Deneb:
			return (&eth.SignedBlindedBeaconBlockDeneb{}).UnmarshalSSZ(body)
		case version.Capella:
			return (&eth.SignedBlindedBeaconBlockCapella{}).UnmarshalSSZ(body)
		default:
			return (&eth.SignedBlindedBeaconBlockBellatrix{}).UnmarshalSSZ(body)
		}
	}
	switch p.currVersion {
	case version.Electra:
		block, err := decodeSingleJSON[structs.SignedBlindedBeaconBlockElectra](req.Body)
		if err != nil || block.Message == nil {
			return errors.New("invalid blinded block")
		}
		_, err = block.ToConsensus()
		return err
	case version.Deneb:
		block, err := decodeSingleJSON[structs.SignedBlindedBeaconBlockDeneb](req.Body)
		if err != nil || block.Message == nil {
			return errors.New("invalid blinded block")
		}
		_, err = block.ToConsensus()
		return err
	case version.Capella:
		block, err := decodeSingleJSON[structs.SignedBlindedBeaconBlockCapella](req.Body)
		if err != nil || block.Message == nil {
			return errors.New("invalid blinded block")
		}
		_, err = block.ToGeneric()
		return err
	default:
		block, err := decodeSingleJSON[structs.SignedBlindedBeaconBlockBellatrix](req.Body)
		if err != nil || block.Message == nil {
			return errors.New("invalid blinded block")
		}
		_, err = block.ToGeneric()
		return err
	}
}

func decodeBlindedBlockFuluRequest(req *http.Request) error {
	if httputil.IsRequestSsz(req) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return err
		}
		return (&eth.SignedBlindedBeaconBlockFulu{}).UnmarshalSSZ(body)
	}
	block, err := decodeSingleJSON[structs.SignedBlindedBeaconBlockFulu](req.Body)
	if err != nil || block.Message == nil {
		return errors.New("invalid Fulu blinded block")
	}
	_, err = block.ToConsensus()
	return err
}

func executionPayloadSSZResponse(v int, data interfaces.ExecutionData, bundle *v1.BlobsBundle) (ssz.Marshaler, error) {
	switch v {
	case version.Bellatrix:
		payload, ok := data.Proto().(*v1.ExecutionPayload)
		if !ok {
			return nil, errInvalidTypeConversion
		}
		return payload, nil
	case version.Capella:
		payload, ok := data.Proto().(*v1.ExecutionPayloadCapella)
		if !ok {
			return nil, errInvalidTypeConversion
		}
		return payload, nil
	case version.Deneb, version.Electra:
		payload, ok := data.Proto().(*v1.ExecutionPayloadDeneb)
		if !ok {
			return nil, errInvalidTypeConversion
		}
		return &v1.ExecutionPayloadDenebAndBlobsBundle{Payload: payload, BlobsBundle: bundle}, nil
	default:
		return nil, errInvalidTypeConversion
	}
}

var errInvalidTypeConversion = errors.New("unable to translate between api and foreign type")

// ExecutionPayloadResponseFromData converts an ExecutionData interface value to a payload response.
// This involves serializing the execution payload value so that the abstract payload envelope can be used.
func ExecutionPayloadResponseFromData(v int, ed interfaces.ExecutionData, bundle *v1.BlobsBundle) (*builderAPI.ExecutionPayloadResponse, error) {
	pb := ed.Proto()
	var data any
	var err error
	ver := version.String(v)
	switch pbStruct := pb.(type) {
	case *v1.ExecutionPayloadDeneb:
		payloadStruct, err := structs.ExecutionPayloadDenebFromConsensus(pbStruct)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert a Deneb ExecutionPayload to an API response")
		}
		data = &builderAPI.ExecutionPayloadDenebAndBlobsBundle{
			ExecutionPayload: payloadStruct,
			BlobsBundle:      builderAPI.FromBundleProto(bundle),
		}
	case *v1.ExecutionPayloadCapella:
		data, err = structs.ExecutionPayloadCapellaFromConsensus(pbStruct)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert a Capella ExecutionPayload to an API response")
		}
	case *v1.ExecutionPayload:
		data, err = structs.ExecutionPayloadFromConsensus(pbStruct)
		if err != nil {
			return nil, errors.Wrap(err, "failed to convert a Bellatrix ExecutionPayload to an API response")
		}
	default:
		return nil, errInvalidTypeConversion
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal execution payload version=%s", ver)
	}
	return &builderAPI.ExecutionPayloadResponse{
		Version: ver,
		Data:    encoded,
	}, nil
}

func (p *Builder) retrievePendingBlock() (*v1.ExecutionPayload, error) {
	result := &engine.ExecutableData{}
	if p.currId == nil {
		return nil, errors.New("no payload id is cached")
	}
	err := p.execClient.CallContext(context.Background(), result, GetPayloadMethod, *p.currId)
	if err != nil {
		return nil, err
	}
	payloadEnv, err := modifyExecutionPayload(*result, big.NewInt(0), nil, nil)
	if err != nil {
		return nil, err
	}
	marshalledOutput, err := payloadEnv.ExecutionPayload.MarshalJSON()
	if err != nil {
		return nil, err
	}
	bellatrixPayload := &v1.ExecutionPayload{}
	if err = json.Unmarshal(marshalledOutput, bellatrixPayload); err != nil {
		return nil, err
	}
	p.currId = nil
	return bellatrixPayload, nil
}

func (p *Builder) retrievePendingBlockCapella() (*v1.ExecutionPayloadCapellaWithValue, error) {
	result := &engine.ExecutionPayloadEnvelope{}
	if p.currId == nil {
		return nil, errors.New("no payload id is cached")
	}
	err := p.execClient.CallContext(context.Background(), result, GetPayloadMethodV2, *p.currId)
	if err != nil {
		return nil, err
	}
	payloadEnv, err := modifyExecutionPayload(*result.ExecutionPayload, result.BlockValue, nil, nil)
	if err != nil {
		return nil, err
	}
	marshalledOutput, err := payloadEnv.MarshalJSON()
	if err != nil {
		return nil, err
	}
	capellaPayload := &v1.ExecutionPayloadCapellaWithValue{}
	if err = json.Unmarshal(marshalledOutput, capellaPayload); err != nil {
		return nil, err
	}
	p.currId = nil
	return capellaPayload, nil
}

func (p *Builder) retrievePendingBlockDeneb() (*v1.ExecutionPayloadDenebWithValueAndBlobsBundle, error) {
	result := &engine.ExecutionPayloadEnvelope{}
	if p.currId == nil {
		return nil, errors.New("no payload id is cached")
	}
	err := p.execClient.CallContext(context.Background(), result, GetPayloadMethodV3, *p.currId)
	if err != nil {
		return nil, err
	}
	if p.prevBeaconRoot == nil {
		p.cfg.logger.Errorf("previous root is nil")
	}
	payloadEnv, err := modifyExecutionPayload(*result.ExecutionPayload, result.BlockValue, p.prevBeaconRoot, nil)
	if err != nil {
		return nil, err
	}
	payloadEnv.BlobsBundle = result.BlobsBundle
	marshalledOutput, err := payloadEnv.MarshalJSON()
	if err != nil {
		return nil, err
	}
	denebPayload := &v1.ExecutionPayloadDenebWithValueAndBlobsBundle{}
	if err = json.Unmarshal(marshalledOutput, denebPayload); err != nil {
		return nil, err
	}
	p.currId = nil
	return denebPayload, nil
}

func (p *Builder) retrievePendingBlockElectra() (*v1.ExecutionBundleElectra, error) {
	result := &engine.ExecutionPayloadEnvelope{}
	if p.currId == nil {
		return nil, errors.New("no payload id is cached")
	}
	err := p.execClient.CallContext(context.Background(), result, GetPayloadMethodV4, *p.currId)
	if err != nil {
		return nil, err
	}
	if p.prevBeaconRoot == nil {
		p.cfg.logger.Errorf("previous root is nil")
	}

	payloadEnv, err := modifyExecutionPayload(*result.ExecutionPayload, result.BlockValue, p.prevBeaconRoot, result.Requests)
	if err != nil {
		return nil, err
	}
	payloadEnv.BlobsBundle = result.BlobsBundle
	marshalledOutput, err := payloadEnv.MarshalJSON()
	if err != nil {
		return nil, err
	}
	electraPayload := &v1.ExecutionBundleElectra{}
	if err = json.Unmarshal(marshalledOutput, electraPayload); err != nil {
		return nil, err
	}
	p.currId = nil
	return electraPayload, nil
}

func (p *Builder) sendHttpRequest(req *http.Request, requestBytes []byte) (*http.Response, error) {
	proxyReq, err := http.NewRequest(req.Method, p.cfg.destinationUrl.String(), req.Body)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not create new request")
		return nil, err
	}

	// Set the modified request as the proxy request body.
	proxyReq.Body = io.NopCloser(bytes.NewBuffer(requestBytes))

	// Required proxy headers for forwarding JSON-RPC requests to the execution client.
	proxyReq.Header.Set("Host", req.Host)
	proxyReq.Header.Set("X-Forwarded-For", req.RemoteAddr)
	proxyReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	if p.cfg.secret != "" {
		client = network.NewHttpClientWithSecret(p.cfg.secret, "")
	}
	proxyRes, err := client.Do(proxyReq)
	if err != nil {
		p.cfg.logger.WithError(err).Error("Could not forward request to destination server")
		return nil, err
	}
	return proxyRes, nil
}

// Peek into the bytes of an HTTP request's body.
func parseRequestBytes(req *http.Request) ([]byte, error) {
	requestBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	if err = req.Body.Close(); err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewBuffer(requestBytes))
	return requestBytes, nil
}

// Checks whether the JSON-RPC request is for the Ethereum engine API.
func isEngineAPICall(reqBytes []byte) bool {
	jsonRequest, err := unmarshalRPCObject(reqBytes)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "cannot unmarshal array"):
			return false
		default:
			return false
		}
	}
	return strings.Contains(jsonRequest.Method, "engine_")
}

func unmarshalRPCObject(b []byte) (*jsonRPCObject, error) {
	r := &jsonRPCObject{}
	if err := json.Unmarshal(b, r); err != nil {
		return nil, err
	}
	return r, nil
}

func modifyExecutionPayload(execPayload engine.ExecutableData, fees *big.Int, prevBeaconRoot []byte, requests [][]byte) (*engine.ExecutionPayloadEnvelope, error) {
	modifiedBlock, err := executableDataToBlock(execPayload, prevBeaconRoot, requests)
	if err != nil {
		return &engine.ExecutionPayloadEnvelope{}, err
	}
	return engine.BlockToExecutableData(modifiedBlock, fees, nil /*blobs*/, requests /*requests*/), nil
}

// This modifies the provided payload to imprint the builder's extra data
func executableDataToBlock(params engine.ExecutableData, prevBeaconRoot []byte, requests [][]byte) (*gethTypes.Block, error) {
	txs, err := decodeTransactions(params.Transactions)
	if err != nil {
		return nil, err
	}
	// Only set withdrawalsRoot if it is non-nil. This allows CLs to use
	// ExecutableData before withdrawals are enabled by marshaling
	// Withdrawals as the json null value.
	var withdrawalsRoot *common.Hash
	if params.Withdrawals != nil {
		h := gethTypes.DeriveSha(gethTypes.Withdrawals(params.Withdrawals), trie.NewStackTrie(nil))
		withdrawalsRoot = &h
	}

	var requestsHash *common.Hash
	if requests != nil {
		h := gethTypes.CalcRequestsHash(requests)
		requestsHash = &h
	}

	header := &gethTypes.Header{
		ParentHash:      params.ParentHash,
		UncleHash:       gethTypes.EmptyUncleHash,
		Coinbase:        params.FeeRecipient,
		Root:            params.StateRoot,
		TxHash:          gethTypes.DeriveSha(gethTypes.Transactions(txs), trie.NewStackTrie(nil)),
		ReceiptHash:     params.ReceiptsRoot,
		Bloom:           gethTypes.BytesToBloom(params.LogsBloom),
		Difficulty:      common.Big0,
		Number:          new(big.Int).SetUint64(params.Number),
		GasLimit:        params.GasLimit,
		GasUsed:         params.GasUsed,
		Time:            params.Timestamp,
		BaseFee:         params.BaseFeePerGas,
		Extra:           []byte("prysm-builder"), // add in extra data
		MixDigest:       params.Random,
		WithdrawalsHash: withdrawalsRoot,
		BlobGasUsed:     params.BlobGasUsed,
		ExcessBlobGas:   params.ExcessBlobGas,
		RequestsHash:    requestsHash,
	}

	if prevBeaconRoot != nil {
		pRoot := common.Hash(prevBeaconRoot)
		header.ParentBeaconRoot = &pRoot
	}

	body := gethTypes.Body{
		Transactions: txs,
		Uncles:       nil,
		Withdrawals:  params.Withdrawals,
	}
	block := gethTypes.NewBlockWithHeader(header).WithBody(body)
	return block, nil
}

func decodeTransactions(enc [][]byte) ([]*gethTypes.Transaction, error) {
	var txs = make([]*gethTypes.Transaction, len(enc))
	for i, encTx := range enc {
		var tx gethTypes.Transaction
		if err := tx.UnmarshalBinary(encTx); err != nil {
			return nil, fmt.Errorf("invalid transaction %d: %w", i, err)
		}
		txs[i] = &tx
	}
	return txs, nil
}
