package api

import "net/http"

const (
	VersionHeader                 = "Eth-Consensus-Version"
	ExecutionPayloadBlindedHeader = "Eth-Execution-Payload-Blinded"
	ExecutionPayloadValueHeader   = "Eth-Execution-Payload-Value"
	ConsensusBlockValueHeader     = "Eth-Consensus-Block-Value"
	JsonMediaType                 = "application/json"
	OctetStreamMediaType          = "application/octet-stream"
	EventStreamMediaType          = "text/event-stream"
	KeepAlive                     = "keep-alive"
)

// SetSSEHeaders sets the headers needed for a server-sent event response.
func SetSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", EventStreamMediaType)
	w.Header().Set("Connection", KeepAlive)
}
