package execution

import (
	"fmt"
	"strings"

	gethRPC "github.com/ethereum/go-ethereum/rpc"
	"github.com/pkg/errors"
)

var (
	// ErrParse corresponds to JSON-RPC code -32700.
	ErrParse = errors.New("invalid JSON was received by the server")
	// ErrInvalidRequest corresponds to JSON-RPC code -32600.
	ErrInvalidRequest = errors.New("JSON sent is not valid request object")
	// ErrMethodNotFound corresponds to JSON-RPC code -32601.
	ErrMethodNotFound = errors.New("method not found")
	// ErrInvalidParams corresponds to JSON-RPC code -32602.
	ErrInvalidParams = errors.New("invalid method parameter(s)")
	// ErrInternal corresponds to JSON-RPC code -32603.
	ErrInternal = errors.New("internal JSON-RPC error")
	// ErrServer corresponds to JSON-RPC code -32000.
	ErrServer = errors.New("client error while processing request")
	// ErrUnknownPayload corresponds to JSON-RPC code -38001.
	ErrUnknownPayload = errors.New("payload does not exist or is not available")
	// ErrInvalidForkchoiceState corresponds to JSON-RPC code -38002.
	ErrInvalidForkchoiceState = errors.New("invalid forkchoice state")
	// ErrInvalidPayloadAttributes corresponds to JSON-RPC code -38003.
	ErrInvalidPayloadAttributes = errors.New("payload attributes are invalid / inconsistent")
	// ErrRequestTooLarge when the request is too large
	ErrRequestTooLarge = errors.New("request too large")
)

// Handles errors received from the RPC server according to the specification.
func handleRPCError(err error) error {
	if err == nil {
		return nil
	}
	if isTimeout(err) {
		return ErrHTTPTimeout
	}
	var e gethRPC.Error
	ok := errors.As(err, &e)
	if !ok {
		if strings.Contains(err.Error(), "401 Unauthorized") {
			log.Error("HTTP authentication to your execution client is not working. Please ensure " +
				"you are setting a correct value for the --jwt-secret flag in Prysm, or use an IPC connection if on " +
				"the same machine. Please see our documentation for more information on authenticating connections " +
				"here https://docs.prylabs.network/docs/execution-node/authentication")
			return fmt.Errorf("could not authenticate connection to execution client: %w", err)
		}
		return errors.Wrapf(err, "got an unexpected error in JSON-RPC response")
	}
	switch e.ErrorCode() {
	case -32700:
		errParseCount.Inc()
		return ErrParse
	case -32600:
		errInvalidRequestCount.Inc()
		return ErrInvalidRequest
	case -32601:
		errMethodNotFoundCount.Inc()
		return ErrMethodNotFound
	case -32602:
		errInvalidParamsCount.Inc()
		return ErrInvalidParams
	case -32603:
		errInternalCount.Inc()
		return ErrInternal
	case -38001:
		errUnknownPayloadCount.Inc()
		return ErrUnknownPayload
	case -38002:
		errInvalidForkchoiceStateCount.Inc()
		return ErrInvalidForkchoiceState
	case -38003:
		errInvalidPayloadAttributesCount.Inc()
		return ErrInvalidPayloadAttributes
	case -38004:
		errRequestTooLargeCount.Inc()
		return ErrRequestTooLarge
	case -32000:
		errServerErrorCount.Inc()
		// Only -32000 status codes are data errors in the RPC specification.
		var errWithData gethRPC.DataError
		ok := errors.As(err, &errWithData)
		if !ok {
			return errors.Wrapf(err, "got an unexpected error in JSON-RPC response")
		}
		return errors.Wrapf(ErrServer, "%v", errWithData.Error())
	default:
		return err
	}
}

// ErrHTTPTimeout returns true if the error is a http.Client timeout error.
var ErrHTTPTimeout = errors.New("timeout from http.Client")

type httpTimeoutError interface {
	Error() string
	Timeout() bool
}

func isTimeout(e error) bool {
	var t httpTimeoutError
	ok := errors.As(e, &t)
	return ok && t.Timeout()
}
