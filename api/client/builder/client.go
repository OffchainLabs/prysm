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
	"strings"
	"text/template"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/api/client"
	"github.com/prysmaticlabs/prysm/v5/api/server/structs"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing"
	"github.com/prysmaticlabs/prysm/v5/monitoring/tracing/trace"
	v1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
	log "github.com/sirupsen/logrus"
)

const (
	getExecHeaderPath          = "/eth/v1/builder/header/{{.Slot}}/{{.ParentHash}}/{{.Pubkey}}"
	getStatus                  = "/eth/v1/builder/status"
	postBlindedBeaconBlockPath = "/eth/v1/builder/blinded_blocks"
	postRegisterValidatorPath  = "/eth/v1/builder/validators"
)

var errMalformedHostname = errors.New("hostname must include port, separated by one colon, like example.com:3500")
var errMalformedRequest = errors.New("required request data are missing")
var errNotBlinded = errors.New("submitted block is not blinded")

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

type requestLogger struct{}

func (*requestLogger) observe(r *http.Request) (e error) {
	b := bytes.NewBuffer(nil)
	if r.Body == nil {
		log.WithFields(log.Fields{
			"bodyBase64": "(nil value)",
			"url":        r.URL.String(),
		}).Info("builder http request")
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
	log.WithFields(log.Fields{
		"bodyBase64": string(body),
		"url":        r.URL.String(),
	}).Info("builder http request")

	return nil
}

var _ observer = &requestLogger{}

// BuilderClient provides a collection of helper methods for calling Builder API endpoints.
type BuilderClient interface {
	NodeURL() string
	GetHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubkey [48]byte) (SignedBid, error)
	RegisterValidator(ctx context.Context, svr []*ethpb.SignedValidatorRegistrationV1) error
	SubmitBlindedBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, *v1.BlobsBundle, error)
	Status(ctx context.Context) error
}

// Client provides a collection of helper methods for calling Builder API endpoints.
type Client struct {
	hc      *http.Client
	baseURL *url.URL
	obvs    []observer
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
		hc:      &http.Client{},
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
func (c *Client) do(ctx context.Context, method string, path string, body io.Reader, opts ...reqOption) (res []byte, err error) {
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
	if r.StatusCode != http.StatusOK {
		err = non200Err(r)
		return
	}
	res, err = io.ReadAll(io.LimitReader(r.Body, client.MaxBodySize))
	if err != nil {
		err = errors.Wrap(err, "error reading http response body from builder server")
		return
	}
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
func (c *Client) GetHeader(ctx context.Context, slot primitives.Slot, parentHash [32]byte, pubkey [48]byte) (SignedBid, error) {
	path, err := execHeaderPath(slot, parentHash, pubkey)
	if err != nil {
		return nil, err
	}
	hb, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	v := &VersionResponse{}
	if err := json.Unmarshal(hb, v); err != nil {
		return nil, errors.Wrapf(err, "error unmarshaling the builder GetHeader response, using slot=%d, parentHash=%#x, pubkey=%#x", slot, parentHash, pubkey)
	}
	switch strings.ToLower(v.Version) {
	case strings.ToLower(version.String(version.Deneb)):
		hr := &ExecHeaderResponseDeneb{}
		if err := json.Unmarshal(hb, hr); err != nil {
			return nil, errors.Wrapf(err, "error unmarshaling the builder GetHeader response, using slot=%d, parentHash=%#x, pubkey=%#x", slot, parentHash, pubkey)
		}
		p, err := hr.ToProto()
		if err != nil {
			return nil, errors.Wrapf(err, "could not extract proto message from header")
		}
		return WrappedSignedBuilderBidDeneb(p)
	case strings.ToLower(version.String(version.Capella)):
		hr := &ExecHeaderResponseCapella{}
		if err := json.Unmarshal(hb, hr); err != nil {
			return nil, errors.Wrapf(err, "error unmarshaling the builder GetHeader response, using slot=%d, parentHash=%#x, pubkey=%#x", slot, parentHash, pubkey)
		}
		p, err := hr.ToProto()
		if err != nil {
			return nil, errors.Wrapf(err, "could not extract proto message from header")
		}
		return WrappedSignedBuilderBidCapella(p)
	case strings.ToLower(version.String(version.Bellatrix)):
		hr := &ExecHeaderResponse{}
		if err := json.Unmarshal(hb, hr); err != nil {
			return nil, errors.Wrapf(err, "error unmarshaling the builder GetHeader response, using slot=%d, parentHash=%#x, pubkey=%#x", slot, parentHash, pubkey)
		}
		p, err := hr.ToProto()
		if err != nil {
			return nil, errors.Wrap(err, "could not extract proto message from header")
		}
		return WrappedSignedBuilderBid(p)
	default:
		return nil, fmt.Errorf("unsupported header version %s", strings.ToLower(v.Version))
	}
}

// RegisterValidator encodes the SignedValidatorRegistrationV1 message to json (including hex-encoding the byte
// fields with 0x prefixes) and posts to the builder validator registration endpoint.
func (c *Client) RegisterValidator(ctx context.Context, svr []*ethpb.SignedValidatorRegistrationV1) error {
	ctx, span := trace.StartSpan(ctx, "builder.client.RegisterValidator")
	defer span.End()
	span.SetAttributes(trace.Int64Attribute("num_reqs", int64(len(svr))))

	if len(svr) == 0 {
		err := errors.Wrap(errMalformedRequest, "empty validator registration list")
		tracing.AnnotateError(span, err)
		return err
	}
	vs := make([]*structs.SignedValidatorRegistration, len(svr))
	for i := 0; i < len(svr); i++ {
		vs[i] = structs.SignedValidatorRegistrationFromConsensus(svr[i])
	}
	body, err := json.Marshal(vs)
	if err != nil {
		err := errors.Wrap(err, "error encoding the SignedValidatorRegistration value body in RegisterValidator")
		tracing.AnnotateError(span, err)
		return err
	}

	_, err = c.do(ctx, http.MethodPost, postRegisterValidatorPath, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	log.WithField("num_registrations", len(svr)).Info("successfully registered validator(s) on builder")
	return nil
}

var errResponseVersionMismatch = errors.New("builder API response uses a different version than requested in " + api.VersionHeader + " header")

// SubmitBlindedBlock calls the builder API endpoint that binds the validator to the builder and submits the block.
// The response is the full execution payload used to create the blinded block.
func (c *Client) SubmitBlindedBlock(ctx context.Context, sb interfaces.ReadOnlySignedBeaconBlock) (interfaces.ExecutionData, *v1.BlobsBundle, error) {
	if !sb.IsBlinded() {
		return nil, nil, errNotBlinded
	}

	// massage the proto struct type data into the api response type.
	mj, err := structs.SignedBeaconBlockMessageJsoner(sb)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error generating blinded beacon block post request")
	}

	body, err := json.Marshal(mj)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error marshaling blinded block post request to json")
	}
	postOpts := func(r *http.Request) {
		r.Header.Add("Eth-Consensus-Version", version.String(sb.Version()))
		r.Header.Set("Content-Type", api.JsonMediaType)
		r.Header.Set("Accept", api.JsonMediaType)
	}
	// post the blinded block - the execution payload response should contain the unblinded payload, along with the
	// blobs bundle if it is post deneb.
	rb, err := c.do(ctx, http.MethodPost, postBlindedBeaconBlockPath, bytes.NewBuffer(body), postOpts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error posting the blinded block to the builder api")
	}
	// ExecutionPayloadResponse parses just the outer container and the Value key, enabling it to use the .Value
	// key to determine which underlying data type to use to finish the unmarshaling.
	ep := &ExecutionPayloadResponse{}
	if err := json.Unmarshal(rb, ep); err != nil {
		return nil, nil, errors.Wrap(err, "error unmarshaling the builder ExecutionPayloadResponse")
	}
	if strings.ToLower(ep.Version) != version.String(sb.Version()) {
		return nil, nil, errors.Wrapf(errResponseVersionMismatch, "req=%s, recv=%s", strings.ToLower(ep.Version), version.String(sb.Version()))
	}
	// This parses the rest of the response and returns the inner data field.
	pp, err := ep.ParsePayload()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to parse execution payload from builder with version=%s", ep.Version)
	}
	// Get the payload as a proto.Message so it can be wrapped as an execution payload interface.
	pb, err := pp.PayloadProto()
	if err != nil {
		return nil, nil, err
	}
	ed, err := blocks.NewWrappedExecutionData(pb)
	if err != nil {
		return nil, nil, err
	}
	bb, ok := pp.(BlobBundler)
	if ok {
		bbpb, err := bb.BundleProto()
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to extract blobs bundle from builder response with version=%s", ep.Version)
		}
		return ed, bbpb, nil
	}
	return ed, nil, nil
}

// Status asks the remote builder server for a health check. A response of 200 with an empty body is the success/healthy
// response, and an error response may have an error message. This method will return a nil value for error in the
// happy path, and an error with information about the server response body for a non-200 response.
func (c *Client) Status(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, getStatus, nil)
	return err
}

func non200Err(response *http.Response) error {
	bodyBytes, err := io.ReadAll(io.LimitReader(response.Body, client.MaxErrBodySize))
	var errMessage ErrorMessage
	var body string
	if err != nil {
		body = "(Unable to read response body.)"
	} else {
		body = "response body:\n" + string(bodyBytes)
	}
	msg := fmt.Sprintf("code=%d, url=%s, body=%s", response.StatusCode, response.Request.URL, body)
	switch response.StatusCode {
	case http.StatusNoContent:
		log.WithError(ErrNoContent).Debug(msg)
		return ErrNoContent
	case http.StatusBadRequest:
		log.WithError(ErrBadRequest).Debug(msg)
		if jsonErr := json.Unmarshal(bodyBytes, &errMessage); jsonErr != nil {
			return errors.Wrap(jsonErr, "unable to read response body")
		}
		return errors.Wrap(ErrBadRequest, errMessage.Message)
	case http.StatusNotFound:
		log.WithError(ErrNotFound).Debug(msg)
		if jsonErr := json.Unmarshal(bodyBytes, &errMessage); jsonErr != nil {
			return errors.Wrap(jsonErr, "unable to read response body")
		}
		return errors.Wrap(ErrNotFound, errMessage.Message)
	case http.StatusInternalServerError:
		log.WithError(ErrNotOK).Debug(msg)
		if jsonErr := json.Unmarshal(bodyBytes, &errMessage); jsonErr != nil {
			return errors.Wrap(jsonErr, "unable to read response body")
		}
		return errors.Wrap(ErrNotOK, errMessage.Message)
	default:
		log.WithError(ErrNotOK).Debug(msg)
		return errors.Wrap(ErrNotOK, fmt.Sprintf("unsupported error code: %d", response.StatusCode))
	}
}
