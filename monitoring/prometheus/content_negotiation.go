package prometheus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/OffchainLabs/prysm/v6/api"
	"github.com/golang/gddo/httputil"
)

// generatedResponse is a container for response output.
type generatedResponse struct {
	// Err is protocol error, if any.
	Err string `json:"error"`

	// Data is response output, if any.
	Data interface{} `json:"data"`
}

// negotiateContentType parses "Accept:" header and returns preferred content type string.
func negotiateContentType(r *http.Request) string {
	contentTypes := []string{
		api.PlainMediaType,
		api.JsonMediaType,
	}
	return httputil.NegotiateContentType(r, contentTypes, api.PlainMediaType)
}

// writeResponse is content-type aware response writer.
func writeResponse(w http.ResponseWriter, r *http.Request, response generatedResponse) error {
	switch negotiateContentType(r) {
	case api.PlainMediaType:
		buf, ok := response.Data.(bytes.Buffer)
		if !ok {
			return fmt.Errorf("unexpected data: %v", response.Data)
		}
		if _, err := w.Write(buf.Bytes()); err != nil {
			return fmt.Errorf("could not write response body: %w", err)
		}
	case api.JsonMediaType:
		w.Header().Set(api.ContentTypeHeader, api.JsonMediaType)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			return err
		}
	}
	return nil
}
