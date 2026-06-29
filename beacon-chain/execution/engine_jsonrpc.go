package execution

import "time"

var (
	supportedEngineEndpoints = []string{
		NewPayloadMethod,
		NewPayloadMethodV2,
		NewPayloadMethodV3,
		ForkchoiceUpdatedMethod,
		ForkchoiceUpdatedMethodV2,
		ForkchoiceUpdatedMethodV3,
		GetPayloadMethod,
		GetPayloadMethodV2,
		GetPayloadMethodV3,
		GetPayloadBodiesByHashV1,
		GetPayloadBodiesByRangeV1,
		GetBlobsV1,
	}

	electraEngineEndpoints = []string{
		NewPayloadMethodV4,
		GetPayloadMethodV4,
	}

	fuluEngineEndpoints = []string{
		GetPayloadMethodV5,
		GetBlobsV2,
		GetBlobsV3,
		HasBlobs,
	}

	gloasEngineEndpoints = []string{
		NewPayloadMethodV5,
		GetPayloadMethodV6,
		ForkchoiceUpdatedMethodV4,
		GetPayloadBodiesByHashV2,
		GetPayloadBodiesByRangeV2,
	}
)

const (
	// NewPayloadMethod v1 request string for JSON-RPC.
	NewPayloadMethod = "engine_newPayloadV1"
	// NewPayloadMethodV2 v2 request string for JSON-RPC.
	NewPayloadMethodV2 = "engine_newPayloadV2"
	NewPayloadMethodV3 = "engine_newPayloadV3"
	// NewPayloadMethodV4 is the engine_newPayloadVX method added at Electra.
	NewPayloadMethodV4 = "engine_newPayloadV4"
	// NewPayloadMethodV5 is the engine_newPayloadVX method added at Gloas.
	NewPayloadMethodV5 = "engine_newPayloadV5"
	// ForkchoiceUpdatedMethod v1 request string for JSON-RPC.
	ForkchoiceUpdatedMethod = "engine_forkchoiceUpdatedV1"
	// ForkchoiceUpdatedMethodV2 v2 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV2 = "engine_forkchoiceUpdatedV2"
	// ForkchoiceUpdatedMethodV3 v3 request string for JSON-RPC.
	ForkchoiceUpdatedMethodV3 = "engine_forkchoiceUpdatedV3"
	// GetPayloadMethod v1 request string for JSON-RPC.
	GetPayloadMethod = "engine_getPayloadV1"
	// GetPayloadMethodV2 v2 request string for JSON-RPC.
	GetPayloadMethodV2 = "engine_getPayloadV2"
	// GetPayloadMethodV3 is the get payload method added for deneb
	GetPayloadMethodV3 = "engine_getPayloadV3"
	// GetPayloadMethodV4 is the get payload method added for electra
	GetPayloadMethodV4 = "engine_getPayloadV4"
	// GetPayloadMethodV5 is the get payload method added for fulu
	GetPayloadMethodV5 = "engine_getPayloadV5"
	// GetPayloadMethodV6 is the get payload method added for gloas/amsterdam.
	GetPayloadMethodV6 = "engine_getPayloadV6"
	// ForkchoiceUpdatedMethodV4 is the forkchoice updated method added for gloas/amsterdam.
	ForkchoiceUpdatedMethodV4 = "engine_forkchoiceUpdatedV4"
	// BlockByHashMethod request string for JSON-RPC.
	BlockByHashMethod = "eth_getBlockByHash"
	// BlockByNumberMethod request string for JSON-RPC.
	BlockByNumberMethod = "eth_getBlockByNumber"
	// GetPayloadBodiesByHashV1 is the engine_getPayloadBodiesByHashX JSON-RPC method for pre-Electra payloads.
	GetPayloadBodiesByHashV1 = "engine_getPayloadBodiesByHashV1"
	// GetPayloadBodiesByRangeV1 is the engine_getPayloadBodiesByRangeX JSON-RPC method for pre-Electra payloads.
	GetPayloadBodiesByRangeV1 = "engine_getPayloadBodiesByRangeV1"
	// GetPayloadBodiesByHashV2 is the engine_getPayloadBodiesByHashV2 JSON-RPC method for amsterdam payloads.
	GetPayloadBodiesByHashV2 = "engine_getPayloadBodiesByHashV2"
	// GetPayloadBodiesByRangeV2 is the engine_getPayloadBodiesByRangeV2 JSON-RPC method for amsterdam payloads.
	GetPayloadBodiesByRangeV2 = "engine_getPayloadBodiesByRangeV2"
	// ExchangeCapabilities request string for JSON-RPC.
	ExchangeCapabilities = "engine_exchangeCapabilities"
	// GetBlobsV1 request string for JSON-RPC.
	GetBlobsV1 = "engine_getBlobsV1"
	// GetBlobsV2 request string for JSON-RPC.
	GetBlobsV2 = "engine_getBlobsV2"
	// GetBlobsV3 request string for JSON-RPC.
	GetBlobsV3 = "engine_getBlobsV3"
	// HasBlobs request string for JSON-RPC.
	HasBlobs = "engine_hasBlobs"
	// GetClientVersionV1 is the JSON-RPC method that identifies the execution client.
	GetClientVersionV1 = "engine_getClientVersionV1"
	// Defines the seconds before timing out engine endpoints with non-block execution semantics.
	defaultEngineTimeout = time.Second
)
