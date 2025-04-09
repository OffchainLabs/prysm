package beaconapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/api"
	"github.com/prysmaticlabs/prysm/v5/testing/endtoend/params"
	log "github.com/sirupsen/logrus"
)

var (
	errEmptyPrysmData      = errors.New("Prysm data is empty")
	errEmptyLighthouseData = errors.New("Lighthouse data is empty")
)

const (
	msgWrongJson          = "JSON response has wrong structure, expected %T, got %T"
	msgRequestFailed      = "%s request failed with response code %d with response body %s"
	msgUnknownNode        = "unknown node type %s"
	msgSSZUnmarshalFailed = "failed to unmarshal SSZ"
)

func doJSONGetRequest(template, requestPath string, beaconNodeIdx int, resp interface{}, bnType ...string) error {
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
	httpResp, err := http.Get(
		basePath + requestPath,
	)
	if err != nil {
		return err
	}

	var body interface{}
	if httpResp.StatusCode != http.StatusOK {
		if httpResp.Header.Get("Content-Type") == api.JsonMediaType {
			if err = json.NewDecoder(httpResp.Body).Decode(&body); err != nil {
				return err
			}
		} else {
			defer closeBody(httpResp.Body)
			body, err = io.ReadAll(httpResp.Body)
			if err != nil {
				return err
			}
		}
		return fmt.Errorf(msgRequestFailed, bnType[0], httpResp.StatusCode, body)
	}
	return json.NewDecoder(httpResp.Body).Decode(&resp)
}

func doSSZGetRequest(template, requestPath string, beaconNodeIdx int, bnType ...string) ([]byte, error) {
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
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		var body interface{}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf(msgRequestFailed, bnType[0], resp.StatusCode, body)
	}
	defer closeBody(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func doJSONPostRequest(template, requestPath string, beaconNodeIdx int, postObj, resp interface{}, bnType ...string) error {
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
		return err
	}
	httpResp, err := http.Post(
		basePath+requestPath,
		"application/json",
		bytes.NewBuffer(b),
	)
	if err != nil {
		return err
	}

	var body interface{}
	if httpResp.StatusCode != http.StatusOK {
		if httpResp.Header.Get("Content-Type") == api.JsonMediaType {
			if err = json.NewDecoder(httpResp.Body).Decode(&body); err != nil {
				return err
			}
		} else {
			defer closeBody(httpResp.Body)
			body, err = io.ReadAll(httpResp.Body)
			if err != nil {
				return err
			}
		}
		return fmt.Errorf(msgRequestFailed, bnType[0], httpResp.StatusCode, body)
	}
	return json.NewDecoder(httpResp.Body).Decode(&resp)
}

func closeBody(body io.Closer) {
	if err := body.Close(); err != nil {
		log.WithError(err).Error("could not close response body")
	}
}
