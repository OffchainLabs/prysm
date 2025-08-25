package beaconapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/OffchainLabs/prysm/v6/testing/endtoend/params"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

var (
	errEmptyPrysmData      = errors.New("Prysm data is empty")
	errEmptyLighthouseData = errors.New("Lighthouse data is empty")
)

const (
	msgWrongJSON          = "JSON response has wrong structure, expected %T, got %T"
	msgRequestFailed      = "%s request failed with response code %d with response body %s"
	msgUnknownNode        = "unknown node type %s"
	msgSSZUnmarshalFailed = "failed to unmarshal SSZ"
)

func doJSONGETRequest(template, requestPath string, beaconNodeIdx int, resp interface{}, bnType ...string) error {
	if len(bnType) == 0 {
		bnType = []string{"Prysm"}
	}

	var port int
	switch bnType[0] {
	case "Prysm":
		port = params.TestParams.Ports.PrysmBeaconNodeHTTPPort
	case "Lighthouse":
		port = params.TestParams.Ports.LighthouseBeaconNodeHTTPPort
	default:
		return fmt.Errorf(msgUnknownNode, bnType[0])
	}

	basePath := fmt.Sprintf(template, port+beaconNodeIdx)
	httpResp, err := http.Get(basePath + requestPath)
	if err != nil {
		return errors.Wrap(err, "request failed")
	}

	var body interface{}
	if httpResp.StatusCode != http.StatusOK {
		if httpResp.Header.Get("Content-Type") == api.JsonMediaType {
			if err = json.NewDecoder(httpResp.Body).Decode(&body); err != nil {
				return errors.Wrap(err, "failed to decode response body")
			}
		} else {
			defer closeBody(httpResp.Body)
			body, err = io.ReadAll(httpResp.Body)
			if err != nil {
				return errors.Wrap(err, "failed to read response body")
			}
		}
		return fmt.Errorf(msgRequestFailed, bnType[0], httpResp.StatusCode, body)
	}

	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return errors.Wrap(err, "failed to decode response body")
	}
	return nil
}

func doSSZGETRequest(template, requestPath string, beaconNodeIdx int, bnType ...string) ([]byte, error) {
	if len(bnType) == 0 {
		bnType = []string{"Prysm"}
	}

	var port int
	switch bnType[0] {
	case "Prysm":
		port = params.TestParams.Ports.PrysmBeaconNodeHTTPPort
	case "Lighthouse":
		port = params.TestParams.Ports.LighthouseBeaconNodeHTTPPort
	default:
		return nil, fmt.Errorf(msgUnknownNode, bnType[0])
	}

	basePath := fmt.Sprintf(template, port+beaconNodeIdx)

	req, err := http.NewRequest(http.MethodGet, basePath+requestPath, http.NoBody)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	if resp.StatusCode != http.StatusOK {
		var body interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, errors.Wrap(err, "failed to decode response body")
		}
		return nil, fmt.Errorf(msgRequestFailed, bnType[0], resp.StatusCode, body)
	}
	defer closeBody(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	return body, nil
}

func doJSONPOSTRequest(template, requestPath string, beaconNodeIdx int, postObj, resp interface{}, bnType ...string) error {
	if len(bnType) == 0 {
		bnType = []string{"Prysm"}
	}

	var port int
	switch bnType[0] {
	case "Prysm":
		port = params.TestParams.Ports.PrysmBeaconNodeHTTPPort
	case "Lighthouse":
		port = params.TestParams.Ports.LighthouseBeaconNodeHTTPPort
	default:
		return fmt.Errorf(msgUnknownNode, bnType[0])
	}

	basePath := fmt.Sprintf(template, port+beaconNodeIdx)
	b, err := json.Marshal(postObj)
	if err != nil {
		return errors.Wrap(err, "failed to marshal POST object")
	}
	httpResp, err := http.Post(
		basePath+requestPath,
		"application/json",
		bytes.NewBuffer(b),
	)
	if err != nil {
		return errors.Wrap(err, "request failed")
	}

	var body interface{}
	if httpResp.StatusCode != http.StatusOK {
		if httpResp.Header.Get("Content-Type") == api.JsonMediaType {
			if err = json.NewDecoder(httpResp.Body).Decode(&body); err != nil {
				return errors.Wrap(err, "failed to decode response body")
			}
		} else {
			defer closeBody(httpResp.Body)
			body, err = io.ReadAll(httpResp.Body)
			if err != nil {
				return errors.Wrap(err, "failed to read response body")
			}
		}
		return fmt.Errorf(msgRequestFailed, bnType[0], httpResp.StatusCode, body)
	}

	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return errors.Wrap(err, "failed to decode response body")
	}
	return nil
}

func doSSZPOSTRequest(template, requestPath string, beaconNodeIdx int, postObj interface{}, bnType ...string) ([]byte, error) {
	if len(bnType) == 0 {
		bnType = []string{"Prysm"}
	}

	var port int
	switch bnType[0] {
	case "Prysm":
		port = params.TestParams.Ports.PrysmBeaconNodeHTTPPort
	case "Lighthouse":
		port = params.TestParams.Ports.LighthouseBeaconNodeHTTPPort
	default:
		return nil, fmt.Errorf(msgUnknownNode, bnType[0])
	}

	basePath := fmt.Sprintf(template, port+beaconNodeIdx)
	b, err := json.Marshal(postObj)
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal POST object")
	}

	req, err := http.NewRequest(http.MethodPost, basePath+requestPath, bytes.NewBuffer(b))
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	if resp.StatusCode != http.StatusOK {
		var body interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, errors.Wrap(err, "failed to decode response body")
		}
		return nil, fmt.Errorf(msgRequestFailed, bnType[0], resp.StatusCode, body)
	}
	defer closeBody(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	return body, nil
}

func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		log.WithError(err).Error("Could not close response body")
	}
}
