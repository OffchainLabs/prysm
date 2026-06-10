package eth

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed. It preserves legacy roots until generated
// progressive implementations replace these methods.
func (a *Attestation) HashTreeRootProgressive() ([32]byte, error) {
	return a.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (a *AttestationElectra) HashTreeRootProgressive() ([32]byte, error) {
	return a.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (i *IndexedAttestation) HashTreeRootProgressive() ([32]byte, error) {
	return i.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (i *IndexedAttestationElectra) HashTreeRootProgressive() ([32]byte, error) {
	return i.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (a *AttesterSlashing) HashTreeRootProgressive() ([32]byte, error) {
	return a.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (a *AttesterSlashingElectra) HashTreeRootProgressive() ([32]byte, error) {
	return a.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (a *AggregateAttestationAndProof) HashTreeRootProgressive() ([32]byte, error) {
	return a.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (a *AggregateAttestationAndProofElectra) HashTreeRootProgressive() ([32]byte, error) {
	return a.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (s *SignedAggregateAttestationAndProof) HashTreeRootProgressive() ([32]byte, error) {
	return s.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (s *SignedAggregateAttestationAndProofElectra) HashTreeRootProgressive() ([32]byte, error) {
	return s.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadBid) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (s *SignedExecutionPayloadBid) HashTreeRootProgressive() ([32]byte, error) {
	return s.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (b *BeaconBlockGloas) HashTreeRootProgressive() ([32]byte, error) {
	return b.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (b *BeaconBlockBodyGloas) HashTreeRootProgressive() ([32]byte, error) {
	return b.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (s *SignedBeaconBlockGloas) HashTreeRootProgressive() ([32]byte, error) {
	return s.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (d *DataColumnSidecar) HashTreeRootProgressive() ([32]byte, error) {
	return d.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (d *DataColumnSidecarGloas) HashTreeRootProgressive() ([32]byte, error) {
	return d.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadEnvelope) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (s *SignedExecutionPayloadEnvelope) HashTreeRootProgressive() ([32]byte, error) {
	return s.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (b *BlindedExecutionPayloadEnvelope) HashTreeRootProgressive() ([32]byte, error) {
	return b.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (s *SignedBlindedExecutionPayloadEnvelope) HashTreeRootProgressive() ([32]byte, error) {
	return s.HashTreeRoot()
}
