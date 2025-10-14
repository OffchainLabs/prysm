package beacon_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/api/apiutil"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/network/httputil"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type reqOption func(*http.Request)

type RestHandler interface {
	Get(ctx context.Context, endpoint string, resp interface{}) error
	GetSSZ(ctx context.Context, endpoint string) ([]byte, http.Header, error)
	Post(ctx context.Context, endpoint string, headers map[string]string, data *bytes.Buffer, resp interface{}) error
	PostSSZ(ctx context.Context, endpoint string, headers map[string]string, data *bytes.Buffer) ([]byte, http.Header, error)
	HttpClient() *http.Client
	Host() string
	SetHost(host string)
}

type BeaconApiRestHandler struct {
	client       http.Client
	host         string
	reqOverrides []reqOption
}

// NewBeaconApiRestHandler returns a RestHandler
func NewBeaconApiRestHandler(client http.Client, host string) RestHandler {
	brh := &BeaconApiRestHandler{
		client: client,
		host:   host,
	}
	brh.appendAcceptOverride()
	return brh
}

// appendAcceptOverride enables the Accept header to be customized at runtime via an environment variable.
// This is specified as an env var because it is a niche option that prysm may use for performance testing or debugging
// bug which users are unlikely to need. Using an env var keeps the set of user-facing flags cleaner.
func (c *BeaconApiRestHandler) appendAcceptOverride() {
	if accept := os.Getenv(params.EnvNameOverrideAccept); accept != "" {
		c.reqOverrides = append(c.reqOverrides, func(req *http.Request) {
			req.Header.Set("Accept", accept)
		})
	}
}

// HttpClient returns the underlying HTTP client of the handler
func (c *BeaconApiRestHandler) HttpClient() *http.Client {
	return &c.client
}

// Host returns the underlying HTTP host
func (c *BeaconApiRestHandler) Host() string {
	return c.host
}

// Get sends a GET request and decodes the response body as a JSON object into the passed in object.
// If an HTTP error is returned, the body is decoded as a DefaultJsonError JSON object and returned as the first return value.
func (c *BeaconApiRestHandler) Get(ctx context.Context, endpoint string, resp interface{}) error {
	url := c.host + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return errors.Wrapf(err, "failed to create request for endpoint %s", url)
	}
	req.Header.Set("User-Agent", version.BuildData())
	httpResp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to perform request for endpoint %s", url)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			return
		}
	}()

	return decodeResp(httpResp, resp)
}

func (c *BeaconApiRestHandler) GetSSZ(ctx context.Context, endpoint string) ([]byte, http.Header, error) {
	url := c.host + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create request for endpoint %s", url)
	}

	primaryAcceptType := fmt.Sprintf("%s;q=%s", api.OctetStreamMediaType, "0.95")
	secondaryAcceptType := fmt.Sprintf("%s;q=%s", api.JsonMediaType, "0.9")
	acceptHeaderString := fmt.Sprintf("%s,%s", primaryAcceptType, secondaryAcceptType)
	req.Header.Set("Accept", acceptHeaderString)

	for _, o := range c.reqOverrides {
		o(req)
	}

	req.Header.Set("User-Agent", version.BuildData())
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to perform request for endpoint %s", url)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			return
		}
	}()
	accept := req.Header.Get("Accept")
	contentType := httpResp.Header.Get("Content-Type")
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read response body for %s", httpResp.Request.URL)
	}

	if !apiutil.PrimaryAcceptMatches(accept, contentType) {
		log.WithFields(logrus.Fields{
			"Accept":       accept,
			"Content-Type": contentType,
		}).Debug("Server responded with non primary accept type")
	}

	// non-2XX codes are a failure
	if !strings.HasPrefix(httpResp.Status, "2") {
		decoder := json.NewDecoder(bytes.NewBuffer(body))
		errorJson := &httputil.DefaultJsonError{}
		if err = decoder.Decode(errorJson); err != nil {
			return nil, nil, fmt.Errorf("HTTP request for %s unsuccessful (%d: %s)", httpResp.Request.URL, httpResp.StatusCode, string(body))
		}
		return nil, nil, errorJson
	}

	return body, httpResp.Header, nil
}

// Post sends a POST request and decodes the response body as a JSON object into the passed in object.
// If an HTTP error is returned, the body is decoded as a DefaultJsonError JSON object and returned as the first return value.
func (c *BeaconApiRestHandler) Post(
	ctx context.Context,
	apiEndpoint string,
	headers map[string]string,
	data *bytes.Buffer,
	resp interface{},
) error {
	if data == nil {
		return errors.New("data is nil")
	}

	url := c.host + apiEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, data)
	if err != nil {
		return errors.Wrapf(err, "failed to create request for endpoint %s", url)
	}

	for headerKey, headerValue := range headers {
		req.Header.Set(headerKey, headerValue)
	}
	req.Header.Set("Content-Type", api.JsonMediaType)
	req.Header.Set("User-Agent", version.BuildData())
	httpResp, err := c.client.Do(req)
	if err != nil {
		return errors.Wrapf(err, "failed to perform request for endpoint %s", url)
	}
	defer func() {
		if err = httpResp.Body.Close(); err != nil {
			return
		}
	}()

	return decodeResp(httpResp, resp)
}

// PostSSZ sends a POST request and prefers an SSZ (application/octet-stream) response body.
func (c *BeaconApiRestHandler) PostSSZ(
	ctx context.Context,
	apiEndpoint string,
	headers map[string]string,
	data *bytes.Buffer,
) ([]byte, http.Header, error) {
	if data == nil {
		return nil, nil, errors.New("data is nil")
	}
	url := c.host + apiEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, data)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to create request for endpoint %s", url)
	}

	// Accept header: prefer octet-stream (SSZ), fall back to JSON
	primaryAcceptType := fmt.Sprintf("%s;q=%s", api.OctetStreamMediaType, "0.95")
	secondaryAcceptType := fmt.Sprintf("%s;q=%s", api.JsonMediaType, "0.9")
	acceptHeaderString := fmt.Sprintf("%s,%s", primaryAcceptType, secondaryAcceptType)
	req.Header.Set("Accept", acceptHeaderString)

	// User-supplied headers
	for headerKey, headerValue := range headers {
		req.Header.Set(headerKey, headerValue)
	}

	for _, o := range c.reqOverrides {
		o(req)
	}
	req.Header.Set("Content-Type", api.OctetStreamMediaType)
	req.Header.Set("User-Agent", version.BuildData())
	httpResp, err := c.client.Do(req)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to perform request for endpoint %s", url)
	}
	defer func() {
		if err := httpResp.Body.Close(); err != nil {
			return
		}
	}()

	accept := req.Header.Get("Accept")
	contentType := httpResp.Header.Get("Content-Type")
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read response body for %s", httpResp.Request.URL)
	}

	if !apiutil.PrimaryAcceptMatches(accept, contentType) {
		log.WithFields(logrus.Fields{
			"Accept":       accept,
			"Content-Type": contentType,
		}).Debug("Server responded with non primary accept type")
	}

	// non-2XX codes are a failure
	if !strings.HasPrefix(httpResp.Status, "2") {
		decoder := json.NewDecoder(bytes.NewBuffer(body))
		errorJson := &httputil.DefaultJsonError{}
		if err = decoder.Decode(errorJson); err != nil {
			return nil, nil, fmt.Errorf("HTTP request for %s unsuccessful (%d: %s)", httpResp.Request.URL, httpResp.StatusCode, string(body))
		}
		return nil, nil, errorJson
	}

	return body, httpResp.Header, nil
}

func decodeResp(httpResp *http.Response, resp interface{}) error {
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return errors.Wrapf(err, "failed to read response body for %s", httpResp.Request.URL)
	}

	if !strings.Contains(httpResp.Header.Get("Content-Type"), api.JsonMediaType) {
		// 2XX codes are a success
		if strings.HasPrefix(httpResp.Status, "2") {
			return nil
		}
		return &httputil.DefaultJsonError{Code: httpResp.StatusCode, Message: string(body)}
	}

	decoder := json.NewDecoder(bytes.NewBuffer(body))
	// non-2XX codes are a failure
	if !strings.HasPrefix(httpResp.Status, "2") {
		errorJson := &httputil.DefaultJsonError{}
		if err = decoder.Decode(errorJson); err != nil {
			return errors.Wrapf(err, "failed to decode response body into error json for %s", httpResp.Request.URL)
		}
		return errorJson
	}
	// resp is nil for requests that do not return anything.
	if resp != nil {
		if err = decoder.Decode(resp); err != nil {
			return errors.Wrapf(err, "failed to decode response body into json for %s", httpResp.Request.URL)
		}
	}

	return nil
}

func (c *BeaconApiRestHandler) SetHost(host string) {
	c.host = host
}
