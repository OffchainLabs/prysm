package api

import "net/http"

const (
	// Headers
	EthConsensusVersionHeader        = "Eth-Consensus-Version"
	EthExecutionPayloadBlindedHeader = "Eth-Execution-Payload-Blinded"
	EthExecutionPayloadValueHeader   = "Eth-Execution-Payload-Value"
	EthConsensusBlockValueHeader     = "Eth-Consensus-Block-Value"
	AcceptEncodingHeader             = "Accept-Encoding"
	AcceptHeader                     = "Accept"
	AccessControlAllowOriginHeader   = "Access-Control-Allow-Origin"
	AuthorizationHeader              = "Authorization"
	CacheControlHeader               = "Cache-Control"
	ConnectionHeader                 = "Connection"
	ContentEncodingHeader            = "Content-Encoding"
	ContentLengthHeader              = "Content-Length"
	ContentTypeHeader                = "Content-Type"
	HostHeader                       = "Host"
	OriginHeader                     = "Origin"
	UserAgentHeader                  = "User-Agent"

	// Header values
	JsonMediaType        = "application/json"
	OctetStreamMediaType = "application/octet-stream"
	PlainMediaType       = "text/plain"
	EventStreamMediaType = "text/event-stream"
	KeepAlive            = "keep-alive"
	GzipEncoding         = "gzip"
	BasicAuthorization   = "Basic"
	BearerAuthorization  = "Bearer"
	NoCache              = "no-cache"
)

// SetSSEHeaders sets the headers needed for a server-sent event response.
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set(ContentTypeHeader, EventStreamMediaType)
	w.Header().Set(ConnectionHeader, KeepAlive)
}
