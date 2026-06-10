package enginev1

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed. It preserves legacy roots until generated
// progressive implementations replace these methods.
func (e *ExecutionRequests) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayload) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadCapella) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadDeneb) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadGloas) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadHeader) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadHeaderCapella) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}

// HashTreeRootProgressive is a temporary shim used while fastssz progressive
// root generation is developed.
func (e *ExecutionPayloadHeaderDeneb) HashTreeRootProgressive() ([32]byte, error) {
	return e.HashTreeRoot()
}
