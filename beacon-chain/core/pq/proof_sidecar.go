package pq

import (
	"bytes"
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/pq/leanvmsidecar"
	"github.com/pkg/errors"
)

const (
	sidecarLogInvRate    = 2
	sidecarMaxChildren   = 16
	sidecarProveTimeout  = 300 * time.Second
	sidecarVerifyTimeout = 30 * time.Second
	sidecarClaimLength   = 64
)

type sidecarConn struct {
	mu      sync.Mutex
	socket  string
	timeout time.Duration
	client  *leanvmsidecar.Client
}

// A failed call leaves the stream out of step, so the connection is dropped and dialed again next time.
func (c *sidecarConn) call(f func(*leanvmsidecar.Client) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client == nil {
		client, err := leanvmsidecar.Dial(c.socket)
		if err != nil {
			return errors.Wrap(err, "could not reach the leanVM sidecar")
		}
		c.client = client
	}
	if err := c.client.SetDeadline(time.Now().Add(c.timeout)); err != nil {
		return err
	}
	err := f(c.client)
	var rejection *leanvmsidecar.Rejection
	if err != nil && !errors.As(err, &rejection) {
		_ = c.client.Close()
		c.client = nil
	}
	return err
}

// Proofs from the leanVM sidecar process, the backend Lighthouse runs on a PQ network.
type sidecar struct {
	prove  sidecarConn
	verify sidecarConn
}

func NewSidecarBackend(socket string) (ProofBackend, error) {
	s := &sidecar{
		prove:  sidecarConn{socket: socket, timeout: sidecarProveTimeout},
		verify: sidecarConn{socket: socket, timeout: sidecarVerifyTimeout},
	}
	if err := s.verify.call(func(*leanvmsidecar.Client) error { return nil }); err != nil {
		return nil, err
	}
	return s, nil
}

// A leanVM claim is a pubkey and a message, sorted and without duplicates. The leaf index is not part of it.
func statementClaims(statements []Statement) [][]byte {
	var claims [][]byte
	for _, s := range statements {
		for _, pubkey := range s.Pubkeys {
			claim := make([]byte, 0, sidecarClaimLength)
			claim = append(claim, pubkey[len(pubkey)-xmssPubkeyLength:]...)
			claims = append(claims, append(claim, s.Message[:]...))
		}
	}
	slices.SortFunc(claims, bytes.Compare)
	return slices.CompactFunc(claims, bytes.Equal)
}

func encodeClaims(claims [][]byte) []byte {
	out := binary.LittleEndian.AppendUint64(make([]byte, 0, 17+len(claims)*sidecarClaimLength), uint64(len(claims)))
	for _, claim := range claims {
		out = append(out, claim...)
	}
	// No leanSPHINCS claims and no DA commitment.
	return append(out, make([]byte, 9)...)
}

func (s *sidecar) aggregate(children []leanvmsidecar.Proven, signatures [][]byte, expected []byte) ([]byte, error) {
	var proven leanvmsidecar.Proven
	err := s.prove.call(func(c *leanvmsidecar.Client) error {
		var err error
		proven, err = c.Aggregate(children, signatures, nil, nil, nil, sidecarLogInvRate)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "sidecar could not prove")
	}
	if !bytes.Equal(proven.Claims, expected) {
		return nil, errors.New("sidecar proved other claims than asked")
	}
	return proven.Proof, nil
}

func (s *sidecar) Prove(statements []Statement, signatures [][][]byte) ([]byte, error) {
	if len(statements) != len(signatures) {
		return nil, errors.Errorf("%d statements but %d signature lists", len(statements), len(signatures))
	}
	claims := statementClaims(statements)
	if len(claims) == 0 {
		return nil, nil
	}
	var raw [][]byte
	for i, sigs := range signatures {
		st := statements[i]
		if len(sigs) != len(st.Pubkeys) {
			return nil, errors.Errorf("statement %d has %d pubkeys but %d signatures", i, len(st.Pubkeys), len(sigs))
		}
		for j, sig := range sigs {
			if len(sig) != XMSSSignatureLength {
				return nil, errors.Errorf("signature has length %d", len(sig))
			}
			// The sidecar does not check the leaf, so it is checked here.
			if binary.LittleEndian.Uint32(sig) != st.LeafIndex {
				return nil, errors.Errorf("signature %d of statement %d is at another leaf", j, i)
			}
			pubkey := st.Pubkeys[j]
			item := make([]byte, 0, sidecarClaimLength+XMSSSignatureLength)
			item = append(item, pubkey[len(pubkey)-xmssPubkeyLength:]...)
			item = append(item, st.Message[:]...)
			raw = append(raw, append(item, sig...))
		}
	}
	return s.aggregate(nil, raw, encodeClaims(claims))
}

func (s *sidecar) Merge(children []ProofChild) ([]byte, error) {
	type node struct {
		claims [][]byte
		proof  []byte
	}
	var level []node
	for i, child := range children {
		claims := statementClaims(child.Statements)
		if (len(claims) == 0) != (len(child.Proof) == 0) {
			return nil, errors.Errorf("child %d has claims without a proof or a proof without claims", i)
		}
		if len(claims) != 0 {
			level = append(level, node{claims: claims, proof: child.Proof})
		}
	}
	for len(level) > 1 {
		var next []node
		for chunk := range slices.Chunk(level, sidecarMaxChildren) {
			if len(chunk) == 1 {
				next = append(next, chunk[0])
				continue
			}
			var union [][]byte
			proven := make([]leanvmsidecar.Proven, len(chunk))
			for i, n := range chunk {
				union = append(union, n.claims...)
				proven[i] = leanvmsidecar.Proven{Claims: encodeClaims(n.claims), Proof: n.proof}
			}
			slices.SortFunc(union, bytes.Compare)
			union = slices.CompactFunc(union, bytes.Equal)
			proof, err := s.aggregate(proven, nil, encodeClaims(union))
			if err != nil {
				return nil, err
			}
			next = append(next, node{claims: union, proof: proof})
		}
		level = next
	}
	if len(level) == 0 {
		return nil, nil
	}
	return level[0].proof, nil
}

func (s *sidecar) Verify(statements []Statement, proof []byte) error {
	claims := statementClaims(statements)
	if len(claims) == 0 || len(proof) == 0 {
		if len(claims) == 0 && len(proof) == 0 {
			return nil
		}
		return errors.Wrap(ErrInvalidAttestationProof, "empty proof or empty claims")
	}
	var ok bool
	err := s.verify.call(func(c *leanvmsidecar.Client) error {
		var err error
		ok, err = c.Verify(encodeClaims(claims), proof)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "sidecar could not verify")
	}
	if !ok {
		return ErrInvalidAttestationProof
	}
	return nil
}
