package builder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"text/template"

	"github.com/OffchainLabs/prysm/v7/api"
	"github.com/OffchainLabs/prysm/v7/api/client"
	"github.com/OffchainLabs/prysm/v7/api/server/structs"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/io/logs"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	v1 "github.com/OffchainLabs/prysm/v7/proto/engine/v1"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	getExecHeaderPath            = "/eth/v1/builder/header/{{.Slot}}/{{.ParentHash}}/{{.Pubkey}}"
	getStatus                    = "/eth/v1/builder/status"
	postBlindedBeaconBlockPath   = "/eth/v1/builder/blinded_blocks"
	postBlindedBeaconBlockV2Path = "/eth/v2/builder/blinded_blocks"
	postRegisterValidatorPath    = "/eth/v1/builder/validators"
)

var (
	vrExample             = &ethpb.SignedValidatorRegistrationV1{}
	vrSize                = vrExample.SizeSSZ()
	errMalformedHostname  = errors.New("hostname must include port, separated by one colon, like example.com:3500")
	errMalformedRequest   = errors.New("required request data are missing")
	errNotBlinded         = errors.New("submitted block is not blinded")
	errVersionUnsupported = errors.New("version is not supported")
)

// ClientOpt is a functional option for the Client type (http.Client wrapper)
type ClientOpt func(*Client)

type observer interface {
	observe(r *http.Request) error
}

func WithObserver(m observer) ClientOpt {
	return func(c *Client) {
		c.obvs = append(c.obvs, m)
	}
}

func WithSSZ() ClientOpt {
	return func(c *Client) {
		c.sszEnabled = true
	}
}

type requestLogger struct{}

func (*requestLogger) observe(r *http.Request) (e error) {
	b := bytes.NewBuffer(nil)
	if r.Body == nil {
		log.WithFields(logrus.Fields{
			"bodyBase64": "(nil value)",
			"url":        r.URL.String(),
		}).Info("Builder http request")
		return nil
	}
	t := io.TeeReader(r.Body, b)
	defer func() {
		if r.Body != nil {
			e = r.Body.Close()
		}
	}()
	body, err := io.ReadAll(t)
	if err != nil {
		return err
	}
	r.Body = io.NopCloser(b)
	log.WithFields(logrus.Fields{
		"bodyBase64": string(body),
		"url":        r.URL.String(),
	}).Info("Builder http request")

	return nil
}

var _ observer = &requestLogger{}

// BuilderClient provides a collection of helper methods for calling Builder API endpoints.
type BuilderClient interface {
	NodeURL() string
	GetHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubkey [48]byte) (SignedBid, error)
	RegisterValidator(ctx context.Context, svr []*ethpb.SignedValidatorRegistrationV1) error
	SubmitBlindedBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, v1.BlobsBundler, error)
	SubmitBlindedBlockPostFulu(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) error
	GetExecutionPayloadBid(ctx context.Context, slot primitives.Slot, parentHash, parentRoot [32]byte, proposerPubkey [48]byte, auth *ethpb.SignedRequestAuthV1) (*ethpb.SignedExecutionPayloadBid, error)
	SubmitSignedBeaconBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) error
	SubmitBuilderPreferences(ctx context.Context, validatorPubkey [48]byte, req *ethpb.BuilderPreferencesRequestV1) error
	Status(ctx context.Context) error
}

// Client provides a collection of helper methods for calling Builder API endpoints.
type Client struct {
	hc          *http.Client
	baseURL     *url.URL
	obvs        []observer
	sszEnabled  bool
	sszRejected atomic.Bool
}

// useSSZ reports whether requests should be SSZ-encoded.
func (c *Client) useSSZ() bool {
	return c.sszEnabled && !c.sszRejected.Load()
}

// markSSZRejected records that the remote builder does not support SSZ so subsequent
// requests use JSON. It logs once, on the first transition.
func (c *Client) markSSZRejected() {
	if c.sszRejected.CompareAndSwap(false, true) {
		log.WithField("url", logs.MaskCredentialsLogging(c.NodeURL())).Warn("Remote builder rejected SSZ request; falling back to JSON encoding for subsequent requests")
	}
}

// httpError carries the response status code alongside a package sentinel, so callers branch
// on the status (spec-defined) rather than on the error body, which may not be JSON.
type httpError struct {
	status  int
	wrapped error
}

func (e *httpError) Error() string { return e.wrapped.Error() }
func (e *httpError) Unwrap() error { return e.wrapped }

// isSSZRejection reports whether err is an SSZ content-negotiation rejection by the builder:
// 415 Unsupported Media Type (request body) or 406 Not Acceptable (Accept header).
func isSSZRejection(err error) bool {
	var e *httpError
	return errors.As(err, &e) && (e.status == http.StatusUnsupportedMediaType || e.status == http.StatusNotAcceptable)
}

// sszFallback runs do in the client's current SSZ mode; on an SSZ rejection it latches SSZ
// off and retries once with JSON. Error-only methods use sszFallbackErr.
func sszFallback[T any](c *Client, do func(ssz bool) (T, error)) (T, error) {
	ssz := c.useSSZ()
	res, err := do(ssz)
	if err != nil && ssz && isSSZRejection(err) {
		c.markSSZRejected()
		res, err = do(false)
	}
	return res, err
}

// sszFallbackErr is sszFallback for methods that return only an error.
func (c *Client) sszFallbackErr(do func(ssz bool) error) error {
	_, err := sszFallback(c, func(ssz bool) (struct{}, error) {
		return struct{}{}, do(ssz)
	})
	return err
}

// NewClient constructs a new client with the provided options (ex WithTimeout).
// `host` is the base host + port used to construct request urls. This value can be
// a URL string, or NewClient will assume an http endpoint if just `host:port` is used.
func NewClient(host string, opts ...ClientOpt) (*Client, error) {
	u, err := urlForHost(host)
	if err != nil {
		return nil, err
	}
	c := &Client{
		hc:      &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)},
		baseURL: u,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

func urlForHost(h string) (*url.URL, error) {
	// try to parse as url (being permissive)
	if u, err := url.Parse(h); err == nil && u.Host != "" {
		return u, nil
	}
	// try to parse as host:port
	host, port, err := net.SplitHostPort(h)
	if err != nil {
		return nil, errMalformedHostname
	}
	return &url.URL{Host: net.JoinHostPort(host, port), Scheme: "http"}, nil
}

// NodeURL returns a human-readable string representation of the beacon node base url.
func (c *Client) NodeURL() string {
	return c.baseURL.String()
}

type reqOption func(*http.Request)

// do is a generic, opinionated request function to reduce boilerplate amongst the methods in this package api/client/builder.
// It validates that the HTTP response status matches the expectedStatus parameter.
func (c *Client) do(ctx context.Context, method string, path string, body io.Reader, expectedStatus int, opts ...reqOption) (res []byte, header http.Header, err error) {
	res, _, header, err = c.doWithStatus(ctx, method, path, body, []int{expectedStatus}, opts...)
	return
}

// doWithStatus is do for endpoints with more than one acceptable response status; it returns the matched status.
func (c *Client) doWithStatus(ctx context.Context, method string, path string, body io.Reader, expectedStatuses []int, opts ...reqOption) (res []byte, status int, header http.Header, err error) {
	ctx, span := trace.StartSpan(ctx, "builder.client.do")
	defer func() {
		tracing.AnnotateError(span, err)
		span.End()
	}()

	u := c.baseURL.ResolveReference(&url.URL{Path: path})

	span.SetAttributes(trace.StringAttribute("url", u.String()),
		trace.StringAttribute("method", method))

	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return
	}
	req.Header.Add("User-Agent", version.BuildData())
	for _, o := range opts {
		o(req)
	}
	for _, o := range c.obvs {
		if err = o.observe(req); err != nil {
			return
		}
	}
	r, err := c.hc.Do(req)
	if err != nil {
		return
	}
	defer func() {
		closeErr := r.Body.Close()
		if closeErr != nil {
			log.WithError(closeErr).Error("Failed to close response body")
		}
	}()
	if !slices.Contains(expectedStatuses, r.StatusCode) {
		err = unexpectedStatusErr(r, expectedStatuses)
		return
	}
	status = r.StatusCode
	res, err = io.ReadAll(io.LimitReader(r.Body, client.MaxBodySize))
	if err != nil {
		err = errors.Wrap(err, "error reading http response body from builder server")
		return
	}
	header = r.Header
	return
}

var execHeaderTemplate = template.Must(template.New("").Parse(getExecHeaderPath))

func execHeaderPath(slot primitives.Slot, parentHash [32]byte, pubkey [48]byte) (string, error) {
	v := struct {
		Slot       primitives.Slot
		ParentHash string
		Pubkey     string
	}{
		Slot:       slot,
		ParentHash: fmt.Sprintf("%#x", parentHash),
		Pubkey:     fmt.Sprintf("%#x", pubkey),
	}
	b := bytes.NewBuffer(nil)
	err := execHeaderTemplate.Execute(b, v)
	if err != nil {
		return "", errors.Wrapf(err, "error rendering exec header template with slot=%d, parentHash=%#x, pubkey=%#x", slot, parentHash, pubkey)
	}
	return b.String(), nil
}

// GetHeader is used by a proposing validator to request an execution payload header from the Builder node.
// If the builder rejects the SSZ request, it retries once using JSON.
func (c *Client) GetHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubkey [48]byte) (SignedBid, error) {
	return sszFallback(c, func(ssz bool) (SignedBid, error) {
		return c.getHeader(ctx, slot, parentHash, pubkey, ssz)
	})
}

func (c *Client) getHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubkey [48]byte, ssz bool) (SignedBid, error) {
	path, err := execHeaderPath(slot, parentHash, pubkey)
	if err != nil {
		return nil, err
	}
	accept := api.JsonMediaType
	if ssz {
		accept = api.OctetStreamMediaType
	}
	getOpts := func(r *http.Request) {
		r.Header.Set("Accept", accept)
	}
	data, header, err := c.do(ctx, http.MethodGet, path, nil, http.StatusOK, getOpts)
	if err != nil {
		return nil, errors.Wrap(err, "error getting header from builder server")
	}

	bid, err := c.parseHeaderResponse(data, header, slot, ssz)
	if err != nil {
		return nil, errors.Wrapf(
			err,
			"error rendering exec header template with slot=%d, parentHash=%#x, pubkey=%#x",
			slot,
			parentHash,
			pubkey,
		)
	}
	return bid, nil
}

func (c *Client) parseHeaderResponse(data []byte, header http.Header, slot primitives.Slot, ssz bool) (SignedBid, error) {
	var versionHeader string
	if ssz || header.Get(api.VersionHeader) != "" {
		versionHeader = header.Get(api.VersionHeader)
	} else {
		// If we don't have a version header, attempt to parse JSON for version
		v := &VersionResponse{}
		if err := json.Unmarshal(data, v); err != nil {
			return nil, errors.Wrap(
				err,
				"error unmarshaling builder GetHeader response",
			)
		}
		versionHeader = strings.ToLower(v.Version)
	}

	ver, err := version.FromString(versionHeader)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("unsupported header version %s", versionHeader))
	}

	if ver >= version.Electra {
		return c.parseHeaderElectra(data, slot, ssz)
	}
	if ver >= version.Deneb {
		return c.parseHeaderDeneb(data, ssz)
	}
	if ver >= version.Capella {
		return c.parseHeaderCapella(data, ssz)
	}
	if ver >= version.Bellatrix {
		return c.parseHeaderBellatrix(data, ssz)
	}

	return nil, fmt.Errorf("unsupported header version %s", versionHeader)
}

func (c *Client) parseHeaderElectra(data []byte, slot primitives.Slot, ssz bool) (SignedBid, error) {
	if ssz {
		sb := &ethpb.SignedBuilderBidElectra{}
		if err := sb.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal SignedBuilderBidElectra SSZ")
		}
		return WrappedSignedBuilderBidElectra(sb)
	}
	hr := &ExecHeaderResponseElectra{}
	if err := json.Unmarshal(data, hr); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal ExecHeaderResponseElectra JSON")
	}
	p, err := hr.ToProto(slot)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert ExecHeaderResponseElectra to proto")
	}
	return WrappedSignedBuilderBidElectra(p)
}

func (c *Client) parseHeaderDeneb(data []byte, ssz bool) (SignedBid, error) {
	if ssz {
		sb := &ethpb.SignedBuilderBidDeneb{}
		if err := sb.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal SignedBuilderBidDeneb SSZ")
		}
		return WrappedSignedBuilderBidDeneb(sb)
	}
	hr := &ExecHeaderResponseDeneb{}
	if err := json.Unmarshal(data, hr); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal ExecHeaderResponseDeneb JSON")
	}
	p, err := hr.ToProto()
	if err != nil {
		return nil, errors.Wrap(err, "could not convert ExecHeaderResponseDeneb to proto")
	}
	return WrappedSignedBuilderBidDeneb(p)
}

func (c *Client) parseHeaderCapella(data []byte, ssz bool) (SignedBid, error) {
	if ssz {
		sb := &ethpb.SignedBuilderBidCapella{}
		if err := sb.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal SignedBuilderBidCapella SSZ")
		}
		return WrappedSignedBuilderBidCapella(sb)
	}
	hr := &ExecHeaderResponseCapella{}
	if err := json.Unmarshal(data, hr); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal ExecHeaderResponseCapella JSON")
	}
	p, err := hr.ToProto()
	if err != nil {
		return nil, errors.Wrap(err, "could not convert ExecHeaderResponseCapella to proto")
	}
	return WrappedSignedBuilderBidCapella(p)
}

func (c *Client) parseHeaderBellatrix(data []byte, ssz bool) (SignedBid, error) {
	if ssz {
		sb := &ethpb.SignedBuilderBid{}
		if err := sb.UnmarshalSSZ(data); err != nil {
			return nil, errors.Wrap(err, "could not unmarshal SignedBuilderBid SSZ")
		}
		return WrappedSignedBuilderBid(sb)
	}
	hr := &ExecHeaderResponse{}
	if err := json.Unmarshal(data, hr); err != nil {
		return nil, errors.Wrap(err, "could not unmarshal ExecHeaderResponse JSON")
	}
	p, err := hr.ToProto()
	if err != nil {
		return nil, errors.Wrap(err, "could not convert ExecHeaderResponse to proto")
	}
	return WrappedSignedBuilderBid(p)
}

// RegisterValidator encodes the registrations (SSZ, or hex-encoded JSON) and posts them to the
// builder. If the builder rejects SSZ, it retries once using JSON.
func (c *Client) RegisterValidator(ctx context.Context, svr []*ethpb.SignedValidatorRegistrationV1) error {
	ctx, span := trace.StartSpan(ctx, "builder.client.RegisterValidator")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("num_reqs", int64(len(svr))))

	if len(svr) == 0 {
		err := errors.Wrap(errMalformedRequest, "empty validator registration list")
		tracing.AnnotateError(span, err)
		return err
	}

	err := c.sszFallbackErr(func(ssz bool) error {
		return c.registerValidator(ctx, svr, ssz)
	})
	if err != nil {
		tracing.AnnotateError(span, err)
		return err
	}
	log.WithField("registrationCount", len(svr)).Debug("Successfully registered validator(s) on builder")
	return nil
}

func (c *Client) registerValidator(ctx context.Context, svr []*ethpb.SignedValidatorRegistrationV1, ssz bool) error {
	var (
		body     []byte
		err      error
		postOpts reqOption
	)
	if ssz {
		postOpts = func(r *http.Request) {
			r.Header.Set("Content-Type", api.OctetStreamMediaType)
			r.Header.Set("Accept", api.OctetStreamMediaType)
		}
		body, err = sszValidatorRegisterRequest(svr)
		if err != nil {
			return errors.Wrap(err, "error ssz encoding the SignedValidatorRegistration value body in RegisterValidator")
		}
	} else {
		postOpts = func(r *http.Request) {
			r.Header.Set("Content-Type", api.JsonMediaType)
			r.Header.Set("Accept", api.JsonMediaType)
		}
		body, err = jsonValidatorRegisterRequest(svr)
		if err != nil {
			return errors.Wrap(err, "error json encoding the SignedValidatorRegistration value body in RegisterValidator")
		}
	}

	if _, _, err = c.do(ctx, http.MethodPost, postRegisterValidatorPath, bytes.NewBuffer(body), http.StatusOK, postOpts); err != nil {
		return errors.Wrap(err, "do")
	}
	return nil
}

func jsonValidatorRegisterRequest(svr []*ethpb.SignedValidatorRegistrationV1) ([]byte, error) {
	vs := make([]*structs.SignedValidatorRegistration, len(svr))
	for i := range svr {
		vs[i] = structs.SignedValidatorRegistrationFromConsensus(svr[i])
	}
	body, err := json.Marshal(vs)
	if err != nil {
		return nil, err
	}
	return body, nil
}

func sszValidatorRegisterRequest(svr []*ethpb.SignedValidatorRegistrationV1) ([]byte, error) {
	if uint64(len(svr)) > params.BeaconConfig().ValidatorRegistryLimit {
		return nil, errors.Wrap(errMalformedRequest, "validator registry limit exceeded")
	}
	ssz := make([]byte, vrSize*len(svr))
	for i, vr := range svr {
		sszrep, err := vr.MarshalSSZ()
		if err != nil {
			return nil, errors.Wrap(err, "failed to marshal validator registry ssz")
		}
		copy(ssz[i*vrSize:(i+1)*vrSize], sszrep)
	}
	return ssz, nil
}

var errResponseVersionMismatch = errors.New("builder API response uses a different version than requested in " + api.VersionHeader + " header")

func getVersionsBlockToPayload(blockVersion int) (int, error) {
	if blockVersion >= version.Fulu {
		return version.Fulu, nil
	}
	if blockVersion >= version.Deneb {
		return version.Deneb, nil
	}
	if blockVersion == version.Capella {
		return version.Capella, nil
	}
	if blockVersion == version.Bellatrix {
		return version.Bellatrix, nil
	}
	return 0, errors.Wrapf(errVersionUnsupported, "block version %d", blockVersion)
}

// SubmitBlindedBlock submits the blinded block and returns the unblinded execution payload.
// If the builder rejects SSZ, it retries once using JSON.
func (c *Client) SubmitBlindedBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, v1.BlobsBundler, error) {
	type payload struct {
		ed    interfaces.ExecutionData
		blobs v1.BlobsBundler
	}
	p, err := sszFallback(c, func(ssz bool) (payload, error) {
		ed, blobs, err := c.submitBlindedBlock(ctx, sb, ssz)
		return payload{ed, blobs}, err
	})
	return p.ed, p.blobs, err
}

func (c *Client) submitBlindedBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock, ssz bool) (interfaces.ExecutionData, v1.BlobsBundler, error) {
	body, postOpts, err := c.buildBlindedBlockRequest(sb, ssz)
	if err != nil {
		return nil, nil, err
	}

	// post the blinded block - the execution payload response should contain the unblinded payload, along with the
	// blobs bundle if it is post deneb.
	data, header, err := c.do(ctx, http.MethodPost, postBlindedBeaconBlockPath, bytes.NewBuffer(body), http.StatusOK, postOpts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error posting the blinded block to the builder api")
	}

	ver, err := c.checkBlockVersion(data, header, ssz)
	if err != nil {
		return nil, nil, err
	}

	expectedPayloadVer, err := getVersionsBlockToPayload(sb.Version())
	if err != nil {
		return nil, nil, err
	}
	gotPayloadVer, err := getVersionsBlockToPayload(ver)
	if err != nil {
		return nil, nil, err
	}
	if expectedPayloadVer != gotPayloadVer {
		return nil, nil, errors.Wrapf(errResponseVersionMismatch, "expected payload version %d, got %d", expectedPayloadVer, gotPayloadVer)
	}

	ed, blobs, err := c.parseBlindedBlockResponse(data, ver, ssz)
	if err != nil {
		return nil, nil, err
	}

	return ed, blobs, nil
}

// SubmitBlindedBlockPostFulu submits the blinded block post-Fulu, where relays return only a status
// code. If the builder rejects SSZ, it retries once using JSON.
func (c *Client) SubmitBlindedBlockPostFulu(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) error {
	return c.sszFallbackErr(func(ssz bool) error {
		return c.submitBlindedBlockPostFulu(ctx, sb, ssz)
	})
}

func (c *Client) submitBlindedBlockPostFulu(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock, ssz bool) error {
	body, postOpts, err := c.buildBlindedBlockRequest(sb, ssz)
	if err != nil {
		return err
	}

	// Post the blinded block - the response should only contain a status code (no payload)
	_, _, err = c.do(ctx, http.MethodPost, postBlindedBeaconBlockV2Path, bytes.NewBuffer(body), http.StatusAccepted, postOpts)
	if err != nil {
		return errors.Wrap(err, "error posting the blinded block to the builder api post-Fulu")
	}

	// Success is indicated by no error (status 202)
	return nil
}

func (c *Client) checkBlockVersion(respBytes []byte, header http.Header, ssz bool) (int, error) {
	var versionHeader string
	if ssz {
		versionHeader = strings.ToLower(header.Get(api.VersionHeader))
	} else {
		// fallback to JSON-based version extraction
		v := &VersionResponse{}
		if err := json.Unmarshal(respBytes, v); err != nil {
			return 0, errors.Wrapf(err, "error unmarshaling JSON version fallback")
		}
		versionHeader = strings.ToLower(v.Version)
	}

	ver, err := version.FromString(versionHeader)
	if err != nil {
		return 0, errors.Wrapf(err, "unsupported header version %s", versionHeader)
	}

	return ver, nil
}

// Helper: build request body for SubmitBlindedBlock
func (c *Client) buildBlindedBlockRequest(sb interfaces.ReadOnlySignedBeaconBlock, ssz bool) ([]byte, reqOption, error) {
	if !sb.IsBlinded() {
		return nil, nil, errNotBlinded
	}

	if ssz {
		body, err := sb.MarshalSSZ()
		if err != nil {
			return nil, nil, errors.Wrap(err, "could not marshal SSZ for blinded block")
		}
		opt := func(r *http.Request) {
			r.Header.Set(api.VersionHeader, version.String(sb.Version()))
			r.Header.Set("Content-Type", api.OctetStreamMediaType)
			r.Header.Set("Accept", api.OctetStreamMediaType)
		}
		return body, opt, nil
	}

	mj, err := structs.SignedBeaconBlockMessageJsoner(sb)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error generating blinded beacon block post request")
	}
	body, err := json.Marshal(mj)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error marshaling blinded block to JSON")
	}
	opt := func(r *http.Request) {
		r.Header.Set(api.VersionHeader, version.String(sb.Version()))
		r.Header.Set("Content-Type", api.JsonMediaType)
		r.Header.Set("Accept", api.JsonMediaType)
	}
	return body, opt, nil
}

// Helper: parse the response returned by SubmitBlindedBlock
func (c *Client) parseBlindedBlockResponse(
	respBytes []byte,
	forkVersion int,
	ssz bool,
) (interfaces.ExecutionData, v1.BlobsBundler, error) {
	if ssz {
		return c.parseBlindedBlockResponseSSZ(respBytes, forkVersion)
	}
	return c.parseBlindedBlockResponseJSON(respBytes, forkVersion)
}

func (c *Client) parseBlindedBlockResponseSSZ(
	respBytes []byte,
	forkVersion int,
) (interfaces.ExecutionData, v1.BlobsBundler, error) {
	if forkVersion >= version.Fulu {
		payloadAndBlobs := &v1.ExecutionPayloadDenebAndBlobsBundleV2{}
		if err := payloadAndBlobs.UnmarshalSSZ(respBytes); err != nil {
			return nil, nil, errors.Wrap(err, "unable to unmarshal ExecutionPayloadDenebAndBlobsBundleV2 SSZ")
		}
		ed, err := blocks.NewWrappedExecutionData(payloadAndBlobs.Payload)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "unable to wrap execution data for %s", version.String(forkVersion))
		}
		return ed, payloadAndBlobs.BlobsBundle, nil
	} else if forkVersion >= version.Deneb {
		payloadAndBlobs := &v1.ExecutionPayloadDenebAndBlobsBundle{}
		if err := payloadAndBlobs.UnmarshalSSZ(respBytes); err != nil {
			return nil, nil, errors.Wrap(err, "unable to unmarshal ExecutionPayloadDenebAndBlobsBundle SSZ")
		}
		ed, err := blocks.NewWrappedExecutionData(payloadAndBlobs.Payload)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "unable to wrap execution data for %s", version.String(forkVersion))
		}
		return ed, payloadAndBlobs.BlobsBundle, nil
	} else if forkVersion >= version.Capella {
		payload := &v1.ExecutionPayloadCapella{}
		if err := payload.UnmarshalSSZ(respBytes); err != nil {
			return nil, nil, errors.Wrap(err, "unable to unmarshal ExecutionPayloadCapella SSZ")
		}
		ed, err := blocks.NewWrappedExecutionData(payload)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "unable to wrap execution data for %s", version.String(forkVersion))
		}
		return ed, nil, nil
	} else if forkVersion >= version.Bellatrix {
		payload := &v1.ExecutionPayload{}
		if err := payload.UnmarshalSSZ(respBytes); err != nil {
			return nil, nil, errors.Wrap(err, "unable to unmarshal ExecutionPayload SSZ")
		}
		ed, err := blocks.NewWrappedExecutionData(payload)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "unable to wrap execution data for %s", version.String(forkVersion))
		}
		return ed, nil, nil
	} else {
		return nil, nil, fmt.Errorf("unsupported header version %s", version.String(forkVersion))
	}
}

func (c *Client) parseBlindedBlockResponseJSON(
	respBytes []byte,
	forkVersion int,
) (interfaces.ExecutionData, *v1.BlobsBundle, error) {
	ep := &ExecutionPayloadResponse{}
	if err := json.Unmarshal(respBytes, ep); err != nil {
		return nil, nil, errors.Wrap(err, "error unmarshaling ExecutionPayloadResponse")
	}
	pp, err := ep.ParsePayload()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse payload with version=%s", ep.Version)
	}
	pb, err := pp.PayloadProto()
	if err != nil {
		return nil, nil, err
	}
	ed, err := blocks.NewWrappedExecutionData(pb)
	if err != nil {
		return nil, nil, err
	}

	// Check if it contains blobs
	bb, ok := pp.(BlobBundler)
	if ok {
		bbpb, err := bb.BundleProto()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to extract blobs bundle from version=%s", ep.Version)
		}
		return ed, bbpb, nil
	}
	return ed, nil, nil
}

// Status asks the remote builder server for a health check. A response of 200 with an empty body is the success/healthy
// response, and an error response may have an error message. This method will return a nil value for error in the
// happy path, and an error with information about the server response body for a non-200 response.
func (c *Client) Status(ctx context.Context) error {
	getOpts := func(r *http.Request) {
		r.Header.Set("Accept", api.JsonMediaType)
	}
	_, _, err := c.do(ctx, http.MethodGet, getStatus, nil, http.StatusOK, getOpts)
	return err
}

// errorMessageOrBody extracts the message from the spec's JSON ErrorMessage body, falling
// back to the raw text when a builder returns a non-JSON error (e.g. Commit Boost).
func errorMessageOrBody(bodyBytes []byte) string {
	var errMessage ErrorMessage
	if err := json.Unmarshal(bodyBytes, &errMessage); err == nil && errMessage.Message != "" {
		return errMessage.Message
	}
	return strings.TrimSpace(string(bodyBytes))
}

func unexpectedStatusErr(response *http.Response, expected []int) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(response.Body, client.MaxErrBodySize))
	var body string
	if err != nil {
		body = "(Unable to read response body.)"
	} else {
		body = "response body:\n" + string(bodyBytes)
	}
	msg := fmt.Sprintf("expected=%v, got=%d, url=%s, body=%s", expected, response.StatusCode, response.Request.URL, body)

	var sentinel error
	switch response.StatusCode {
	case http.StatusNoContent:
		log.WithError(ErrNoContent).Debug(msg)
		return ErrNoContent
	case http.StatusUnsupportedMediaType:
		sentinel = ErrUnsupportedMediaType
	case http.StatusNotAcceptable:
		sentinel = ErrNotAcceptable
	case http.StatusBadRequest:
		sentinel = ErrBadRequest
	case http.StatusNotFound:
		sentinel = ErrNotFound
	case http.StatusInternalServerError:
		sentinel = ErrNotOK
	case http.StatusBadGateway:
		sentinel = ErrBadGateway
	default:
		log.WithError(ErrNotOK).Debug(msg)
		return errors.Wrap(ErrNotOK, fmt.Sprintf("unsupported error code: %d", response.StatusCode))
	}
	log.WithError(sentinel).Debug(msg)
	return &httpError{status: response.StatusCode, wrapped: errors.Wrap(sentinel, errorMessageOrBody(bodyBytes))}
}
