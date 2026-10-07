package eth

// SigningData is the container hashed to derive a signing root.
type SigningData struct {
	ObjectRoot []byte `ssz-size:"32"`
	Domain     []byte `ssz-size:"32"`
}

// ForkData is the container hashed to derive a fork data root.
type ForkData struct {
	CurrentVersion        []byte `ssz-size:"4"`
	GenesisValidatorsRoot []byte `ssz-size:"32"`
}

// DepositMessage is the subset of deposit data that is signed.
type DepositMessage struct {
	PublicKey             []byte `spec-name:"pubkey" ssz-size:"48"`
	WithdrawalCredentials []byte `ssz-size:"32"`
	Amount                uint64
}

// PowBlock is the Bellatrix fork choice view of a PoW block.
type PowBlock struct {
	BlockHash       []byte `ssz-size:"32"`
	ParentHash      []byte `ssz-size:"32"`
	TotalDifficulty []byte `ssz-size:"32"`
}
