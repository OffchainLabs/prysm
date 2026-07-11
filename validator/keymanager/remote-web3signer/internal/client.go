package internal

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/bls"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing"
	"github.com/OffchainLabs/prysm/v7/monitoring/tracing/trace"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	ethApiNamespace = "/api/v1/eth2/sign/"
)

type SignRequestJson []byte

// SignatureResponse is the struct representing the signing request response in json format
type SignatureResponse struct {
	Signature hexutil.Bytes `json:"signature"`
}

// HttpSignerClient defines the interface for interacting with a remote web3signer.
type HttpSignerClient interface {
	Sign(ctx context.Context, pubKey string, request SignRequestJson) (bls.Signature, error)
	GetPublicKeys(ctx context.Context, url string) ([]string, error)
}

// ApiClient a wrapper object around web3signer APIs. Please refer to the docs from Consensys' web3signer project.
type ApiClient struct {
	BaseURL    *url.URL
	RestClient *http.Client
}

// NewApiClient method instantiates a new ApiClient object.
func NewApiClient(baseEndpoint string, timeout time.Duration, caCertPath, clientCertPath, clientKeyPath string) (*ApiClient, error) {
	u, err := url.ParseRequestURI(baseEndpoint)
	if err != nil {
		return nil, errors.Wrap(err, "invalid format, unable to parse url")
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("web3signer url must be in the format of http(s)://host:port url used: %v", baseEndpoint)
	}
	transport, err := newTransport(caCertPath, clientCertPath, clientKeyPath)
	if err != nil {
		return nil, err
	}
	return &ApiClient{
		BaseURL:    u,
		RestClient: &http.Client{Transport: otelhttp.NewTransport(transport), Timeout: timeout},
	}, nil
}

func newTransport(caCertPath, clientCertPath, clientKeyPath string) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if caCertPath == "" && clientCertPath == "" && clientKeyPath == "" {
		return transport, nil
	}
	if (clientCertPath == "") != (clientKeyPath == "") {
		return nil, errors.New("web3signer client certificate and key must be provided together")
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if caCertPath != "" {
		roots, err := x509.SystemCertPool()
		if err != nil {
			return nil, errors.Wrap(err, "could not load system CA certificates")
		}
		caCert, err := os.ReadFile(filepath.Clean(caCertPath)) // #nosec G304 -- path is supplied by the operator.
		if err != nil {
			return nil, errors.Wrap(err, "could not read web3signer CA certificate")
		}
		if !roots.AppendCertsFromPEM(caCert) {
			return nil, errors.New("could not parse web3signer CA certificate")
		}
		tlsConfig.RootCAs = roots
	}
	if clientCertPath != "" {
		certificate, err := tls.LoadX509KeyPair(filepath.Clean(clientCertPath), filepath.Clean(clientKeyPath))
		if err != nil {
			return nil, errors.Wrap(err, "could not load web3signer client certificate and key")
		}
		tlsConfig.Certificates = []tls.Certificate{certificate}
	}
	transport.TLSClientConfig = tlsConfig
	return transport, nil
}

// Sign is a wrapper method around the web3signer sign api.
func (client *ApiClient) Sign(ctx context.Context, pubKey string, request SignRequestJson) (bls.Signature, error) {
	requestPath := ethApiNamespace + pubKey
	resp, err := client.doRequest(ctx, "sign", http.MethodPost, client.BaseURL.String()+requestPath, bytes.NewBuffer(request))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("public key not found")
	}
	if resp.StatusCode == http.StatusPreconditionFailed {
		return nil, fmt.Errorf("signing operation failed due to slashing protection rules,  Signing Request URL: %v, Status: %v", client.BaseURL.String()+requestPath, resp.StatusCode)
	}
	contentType := resp.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/json") {
		var sigResp SignatureResponse
		if err := unmarshalResponse(resp.Body, &sigResp); err != nil {
			return nil, err
		}
		return bls.SignatureFromBytes(sigResp.Signature)
	} else {
		return unmarshalSignatureResponse(resp.Body)
	}
}

// GetPublicKeys is a wrapper method around the web3signer publickeys api (this may be removed in the future or moved to another location due to its usage).
func (client *ApiClient) GetPublicKeys(ctx context.Context, url string) ([]string, error) {
	resp, err := client.doRequest(ctx, "public_keys", http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	var publicKeys []string
	if err := unmarshalResponse(resp.Body, &publicKeys); err != nil {
		return nil, err
	}
	if len(publicKeys) == 0 {
		return publicKeys, nil
	}
	// early check if it's a hex and a public key
	// note: a full loop will be conducted in keymanager.go if the quick check passes
	b, err := hexutil.Decode(publicKeys[0])
	if err != nil {
		return nil, errors.Wrap(err, "unable to decode public key")
	}
	if len(b) != fieldparams.BLSPubkeyLength {
		return nil, fmt.Errorf("invalid public key length of %v bytes", len(b))
	}
	return publicKeys, nil
}

// ReloadSignerKeys is a wrapper method around the web3signer reload api.
func (client *ApiClient) ReloadSignerKeys(ctx context.Context) error {
	const requestPath = "/reload"
	if _, err := client.doRequest(ctx, "reload", http.MethodPost, client.BaseURL.String()+requestPath, nil); err != nil {
		return err
	}
	return nil
}

// GetServerStatus is a wrapper method around the web3signer upcheck api
func (client *ApiClient) GetServerStatus(ctx context.Context) (string, error) {
	const requestPath = "/upcheck"
	resp, err := client.doRequest(ctx, "upcheck", http.MethodGet, client.BaseURL.String()+requestPath, nil /* no body needed on get request */)
	if err != nil {
		return "", err
	}
	var status string
	if err := unmarshalResponse(resp.Body, &status); err != nil {
		return "", err
	}
	return status, nil
}

// doRequest is a utility method for requests.
func (client *ApiClient) doRequest(ctx context.Context, requestType, httpMethod, fullPath string, body io.Reader) (*http.Response, error) {
	ctx, span := trace.StartSpan(ctx, "remote_web3signer.Client.doRequest")
	defer span.End()
	span.SetAttributes(
		trace.StringAttribute("httpMethod", httpMethod),
		trace.StringAttribute("fullPath", fullPath),
		trace.BoolAttribute("hasBody", body != nil),
	)
	req, err := http.NewRequestWithContext(ctx, httpMethod, fullPath, body)
	if err != nil {
		return nil, errors.Wrap(err, "invalid format, failed to create new Post Request Object")
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.RestClient.Do(req)
	duration := time.Since(start)
	if err != nil {
		status := requestStatus(err)
		signRequestDurationSeconds.WithLabelValues(requestType, req.Method, status).Observe(duration.Seconds())
		log.WithFields(logrus.Fields{"url": fullPath, "requestType": requestType, "status": status}).WithError(err).Error("Web3Signer request failed")
		err = errors.Wrap(err, "failed to execute json request")
		tracing.AnnotateError(span, err)
		return resp, err
	} else {
		signRequestDurationSeconds.WithLabelValues(requestType, req.Method, strconv.Itoa(resp.StatusCode)).Observe(duration.Seconds())
	}
	if resp.StatusCode != http.StatusOK {
		log.WithFields(logrus.Fields{
			"url":         fullPath,
			"requestType": requestType,
			"status":      resp.StatusCode,
		}).Error("Web3signer request failed")
	}
	if resp.StatusCode == http.StatusInternalServerError {
		err = fmt.Errorf("internal Web3Signer server error, Signing Request URL: %v Status: %v", fullPath, resp.StatusCode)
		tracing.AnnotateError(span, err)
		return nil, err
	} else if resp.StatusCode == http.StatusBadRequest {
		err = fmt.Errorf("bad request format, Signing Request URL: %v Status: %v", fullPath, resp.StatusCode)
		tracing.AnnotateError(span, err)
		return nil, err
	}
	return resp, nil
}

func requestStatus(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "error"
}

// unmarshalResponse is a utility method for unmarshalling responses.
func unmarshalResponse(responseBody io.ReadCloser, unmarshalledResponseObject any) error {
	defer closeBody(responseBody)
	if err := json.NewDecoder(responseBody).Decode(&unmarshalledResponseObject); err != nil {
		body, err := io.ReadAll(responseBody)
		if err != nil {
			return errors.Wrap(err, "failed to read response body")
		}
		return errors.Wrap(err, fmt.Sprintf("invalid format, unable to read response body: %v", string(body)))
	}
	return nil
}

func unmarshalSignatureResponse(responseBody io.ReadCloser) (bls.Signature, error) {
	defer closeBody(responseBody)
	body, err := io.ReadAll(responseBody)
	if err != nil {
		return nil, err
	}
	sigBytes, err := hexutil.Decode(string(body))
	if err != nil {
		return nil, err
	}
	return bls.SignatureFromBytes(sigBytes)
}

// closeBody a utility method to wrap an error for closing
func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		log.WithError(err).Error("Could not close response body")
	}
}
