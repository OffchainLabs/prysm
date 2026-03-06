package enginev1

// SSZ-REST wire format types for EIP-8161.
// These types define the SSZ containers exchanged over the SSZ-REST Engine API
// transport per execution-apis PR #764. They use clean Go types (fixed arrays,
// slices with SSZ tags) for correct sszgen code generation.
//
// Conversion between these wire types and the proto types used internally by
// Prysm happens in beacon-chain/execution/sszrest_encoding.go.

// PayloadStatusV1SSZ is the SSZ wire format for PayloadStatus responses.
//
//	status:              uint8              (1 byte, fixed)
//	latest_valid_hash:   List[Hash32, 1]    (variable — 0 or 32 bytes)
//	validation_error:    List[uint8, 1024]  (variable — 0..1024 bytes)
type PayloadStatusV1SSZ struct {
	Status          uint8
	LatestValidHash [][]byte `ssz-size:"?,32" ssz-max:"1"`
	ValidationError []byte   `ssz-max:"1024"`
}

// ForkchoiceUpdatedResponseSSZ is the SSZ wire format for ForkchoiceUpdated responses.
//
//	payload_status:  PayloadStatusV1SSZ   (variable)
//	payload_id:      List[Bytes8, 1]      (variable — 0 or 8 bytes)
type ForkchoiceUpdatedResponseSSZ struct {
	PayloadStatus *PayloadStatusV1SSZ
	PayloadId     [][]byte `ssz-size:"?,8" ssz-max:"1"`
}

// PayloadAttributesV3SSZ is the SSZ wire format for PayloadAttributesV3.
//
//	timestamp:                 uint64                   (8 bytes, fixed)
//	prev_randao:               Bytes32                  (32 bytes, fixed)
//	suggested_fee_recipient:   Bytes20                  (20 bytes, fixed)
//	withdrawals:               List[Withdrawal, 16]     (variable)
//	parent_beacon_block_root:  Root                     (32 bytes, fixed)
type PayloadAttributesV3SSZ struct {
	Timestamp             uint64
	PrevRandao            [32]byte              `ssz-size:"32"`
	SuggestedFeeRecipient [20]byte              `ssz-size:"20"`
	Withdrawals           []*WithdrawalSSZ      `ssz-max:"16"`
	ParentBeaconBlockRoot [32]byte              `ssz-size:"32"`
}

// WithdrawalSSZ is the SSZ wire format for Withdrawal (44 bytes fixed).
type WithdrawalSSZ struct {
	Index          uint64
	ValidatorIndex uint64
	Address        [20]byte `ssz-size:"20"`
	Amount         uint64
}

// ForkchoiceUpdatedV3RequestSSZ is the SSZ wire format for ForkchoiceUpdated v3 requests.
//
//	forkchoice_state:    ForkchoiceState                    (96 bytes, fixed)
//	payload_attributes:  List[PayloadAttributesV3SSZ, 1]    (variable — empty or 1 element)
type ForkchoiceUpdatedV3RequestSSZ struct {
	ForkchoiceState   *ForkchoiceState
	PayloadAttributes []*PayloadAttributesV3SSZ `ssz-max:"1"`
}

// GetBlobsRequestSSZ is the SSZ wire format for GetBlobs requests.
//
//	versioned_hashes: List[Hash32, 4096]
type GetBlobsRequestSSZ struct {
	VersionedHashes [][]byte `ssz-size:"?,32" ssz-max:"4096"`
}

// ExchangeCapabilitiesSSZ is the SSZ wire format for ExchangeCapabilities
// requests and responses.
//
//	capabilities: List[List[uint8, 64], 128]
type ExchangeCapabilitiesSSZ struct {
	Capabilities [][]byte `ssz-max:"128,64"`
}

// ClientVersionV1SSZ is the SSZ wire format for a single client version entry.
//
//	code:    List[uint8, 8]   (variable)
//	name:    List[uint8, 64]  (variable)
//	version: List[uint8, 64]  (variable)
//	commit:  Bytes4           (4 bytes, fixed)
type ClientVersionV1SSZ struct {
	Code    []byte  `ssz-max:"8"`
	Name    []byte  `ssz-max:"64"`
	Version []byte  `ssz-max:"64"`
	Commit  [4]byte `ssz-size:"4"`
}

// ClientVersionResponseSSZ is the SSZ wire format for GetClientVersion responses.
//
//	versions: List[ClientVersionV1SSZ, 16]
type ClientVersionResponseSSZ struct {
	Versions []*ClientVersionV1SSZ `ssz-max:"16"`
}
