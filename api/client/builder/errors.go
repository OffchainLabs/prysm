package builder

import "github.com/pkg/errors"

// Every error returned for a response status the caller did not expect wraps this, more specific errors may wrap it too.
var ErrUnexpectedStatus = errors.New("did not receive expected response from API")

// ErrNotFound specifically means that a '404 - NOT FOUND' response was received from the API.
var ErrNotFound = errors.Wrap(ErrUnexpectedStatus, "recv 404 NotFound response from API")

// ErrBadRequest specifically means that a '400 - BAD REQUEST' response was received from the API.
var ErrBadRequest = errors.Wrap(ErrUnexpectedStatus, "recv 400 BadRequest response from API")

// ErrNoContent specifically means that a '204 - No Content' response was received from the API.
// Typically, a 204 is a success but in this case for the Header API means No header is available
var ErrNoContent = errors.New("recv 204 no content response from API, No header is available")

// ErrUnsupportedMediaType specifically means that a '415 - Unsupported Media Type' was received from the API.
var ErrUnsupportedMediaType = errors.Wrap(ErrUnexpectedStatus, "The media type in \"Content-Type\" header is unsupported, and the request has been rejected. This occurs when a HTTP request supplies a payload in a content-type that the server is not able to handle.")

// ErrNotAcceptable specifically means that a '406 - Not Acceptable' was received from the API.
var ErrNotAcceptable = errors.Wrap(ErrUnexpectedStatus, "The accept header value is not acceptable")
var ErrBadGateway = errors.Wrap(ErrUnexpectedStatus, "recv 502 BadGateway response from API")
