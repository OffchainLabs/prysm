//go:build !cgo || !((linux && (amd64 || arm64)) || (darwin && arm64))

// github.com/nalepae/go-ere only ships the ere verifier library for these platforms, and linking it needs cgo.

package proofengine

func newZkVMVerifier(zkVMKind, []byte) (zkVMVerifier, error) {
	return nil, errNotSupported
}
