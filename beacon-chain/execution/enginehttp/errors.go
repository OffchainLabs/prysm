package enginehttp

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
)

// ErrNoContent is returned when the EL responds 204 No Content (e.g. the blob
// pool is syncing, or an all-or-nothing /blobs request missed). Callers treat
// it as "the EL could not serve this request", not a transport failure. Match
// it with errors.Is.
var ErrNoContent = errors.New("enginehttp: 204 no content")

// RFC 7807 problem `type` URIs from the spec (execution-apis#793), written as
// relative references rooted at /engine-api/errors/.
const (
	ProblemParseError        = "/engine-api/errors/parse-error"
	ProblemInvalidRequest    = "/engine-api/errors/invalid-request"
	ProblemSSZDecodeError    = "/engine-api/errors/ssz-decode-error"
	ProblemUnsupportedFork   = "/engine-api/errors/unsupported-fork"
	ProblemMethodNotFound    = "/engine-api/errors/method-not-found"
	ProblemUnknownPayload    = "/engine-api/errors/unknown-payload"
	ProblemInvalidForkchoice = "/engine-api/errors/invalid-forkchoice"
	ProblemReorgTooDeep      = "/engine-api/errors/reorg-too-deep"
	ProblemRequestTooLarge   = "/engine-api/errors/request-too-large"
	ProblemUnsupportedMedia  = "/engine-api/errors/unsupported-media-type"
	ProblemInvalidBody       = "/engine-api/errors/invalid-body"
	ProblemInvalidAttributes = "/engine-api/errors/invalid-attributes"
	ProblemInternal          = "/engine-api/errors/internal"
)

// Problem is the RFC 7807 application/problem+json error body. The spec uses
// only `type` and `detail`; other fields some ELs emit (title, status) are
// ignored.
type Problem struct {
	Type   string `json:"type"`
	Detail string `json:"detail,omitempty"`
}

// Error is a transport-level Engine API error: a non-2xx HTTP response.
type Error struct {
	// Status is the HTTP status code.
	Status int
	// Problem is the decoded problem+json body, best effort.
	Problem Problem
	// RawBody holds the response body when it was not decodable problem+json.
	RawBody string
}

func (e *Error) Error() string {
	switch {
	case e.Problem.Type != "" || e.Problem.Detail != "":
		msg := e.Problem.Type
		if e.Problem.Detail != "" {
			if msg != "" {
				msg += ": "
			}
			msg += e.Problem.Detail
		}
		return fmt.Sprintf("enginehttp: HTTP %d (%s)", e.Status, msg)
	case e.RawBody != "":
		return fmt.Sprintf("enginehttp: HTTP %d: %s", e.Status, e.RawBody)
	default:
		return fmt.Sprintf("enginehttp: HTTP %d", e.Status)
	}
}

// httpError builds an *Error from a non-2xx response, decoding the
// application/problem+json body when present and falling back to the raw body.
func httpError(status int, body []byte) error {
	e := &Error{Status: status}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		var p Problem
		if err := json.Unmarshal(trimmed, &p); err == nil {
			e.Problem = p
			return e
		}
	}
	e.RawBody = string(trimmed)
	return e
}
