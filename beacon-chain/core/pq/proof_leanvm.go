package pq

import (
	"encoding/binary"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	leanpq "leanvm.org/go"
)

const leanVMMaxChildren = 16

// Proofs from the leanVM bindings, in process.
type leanVM struct {
	threads   int
	setup     sync.Once
	setupErr  error
	proveLock sync.Mutex
}

// An explicit thread count starts the prover at once, so a count the machine cannot run fails here and not on the first proof.
func NewLeanVMBackend(threads int) (ProofBackend, error) {
	l := &leanVM{threads: threads}
	if threads == 0 {
		return l, nil
	}
	return l, l.start()
}

// With the default count the prover starts on the first proof, so a node that never proves holds no prover threads.
func (l *leanVM) start() error {
	l.setup.Do(func() {
		l.setupErr = leanpq.Setup(l.threads)
		if l.setupErr == nil {
			log.WithFields(logrus.Fields{
				"threads":         l.threads,
				"protocolVersion": leanpq.ProtocolVersion(),
			}).Info("Started the leanVM prover")
		}
	})
	return l.setupErr
}

// The leaf index is not part of a leanVM claim.
func statementClaims(statements []Statement) []leanpq.Claim {
	var claims []leanpq.Claim
	for _, s := range statements {
		for _, pubkey := range s.Pubkeys {
			claims = append(claims, leanpq.Claim{PublicKey: [32]byte(pubkey[len(pubkey)-xmssPubkeyLength:]), Message: s.Message})
		}
	}
	return claims
}

func closeAggregates(aggregates []*leanpq.Aggregate) {
	for _, a := range aggregates {
		a.Close()
	}
}

func (l *leanVM) aggregate(children []*leanpq.Aggregate, signed []leanpq.XmssSignedMessage) (*leanpq.Aggregate, error) {
	if err := l.start(); err != nil {
		return nil, errors.Wrap(err, "could not start the leanVM prover")
	}
	// leanVM proves one job at a time, the lock keeps waiting time out of the logged duration.
	queued := time.Now()
	l.proveLock.Lock()
	defer l.proveLock.Unlock()
	start := time.Now()
	proverQueueWait.Observe(start.Sub(queued).Seconds())
	proven, err := leanpq.AggregateSignatures(children, signed, nil)
	if err != nil {
		return nil, errors.Wrap(err, "leanVM could not prove")
	}
	op, inputs := "prove", len(signed)
	if len(children) > 0 {
		op, inputs = "merge", len(children)
	}
	proverDuration.WithLabelValues(op).Observe(time.Since(start).Seconds())
	proverInputs.WithLabelValues(op).Observe(float64(inputs))
	log.WithFields(logrus.Fields{
		"children":   len(children),
		"signatures": len(signed),
		"duration":   time.Since(start),
	}).Debug("Proved with leanVM")
	return proven, nil
}

func (l *leanVM) Prove(statements []Statement, signatures [][][]byte) ([]byte, error) {
	if len(statements) != len(signatures) {
		return nil, errors.Errorf("%d statements but %d signature lists", len(statements), len(signatures))
	}
	var signed []leanpq.XmssSignedMessage
	for i, sigs := range signatures {
		st := statements[i]
		if len(sigs) != len(st.Pubkeys) {
			return nil, errors.Errorf("statement %d has %d pubkeys but %d signatures", i, len(st.Pubkeys), len(sigs))
		}
		for j, sig := range sigs {
			if len(sig) != XMSSSignatureLength {
				return nil, errors.Errorf("signature has length %d", len(sig))
			}
			// leanVM does not check the leaf, so it is checked here.
			if binary.LittleEndian.Uint32(sig) != st.LeafIndex {
				return nil, errors.Errorf("signature %d of statement %d is at another leaf", j, i)
			}
			pubkey := st.Pubkeys[j]
			signed = append(signed, leanpq.XmssSignedMessage{
				PublicKey: [32]byte(pubkey[len(pubkey)-xmssPubkeyLength:]),
				Message:   st.Message,
				Signature: leanpq.XmssSignature(sig),
			})
		}
	}
	if len(signed) == 0 {
		return nil, nil
	}
	proven, err := l.aggregate(nil, signed)
	if err != nil {
		return nil, err
	}
	defer proven.Close()
	return proven.Bytes()
}

func (l *leanVM) Merge(children []ProofChild) ([]byte, error) {
	var level []*leanpq.Aggregate
	defer func() { closeAggregates(level) }()
	for i, child := range children {
		claims := statementClaims(child.Statements)
		if (len(claims) == 0) != (len(child.Proof) == 0) {
			return nil, errors.Errorf("child %d has claims without a proof or a proof without claims", i)
		}
		if len(claims) == 0 {
			continue
		}
		a, err := leanpq.AggregateFromBytes(child.Proof, claims, nil)
		if err != nil {
			return nil, errors.Wrapf(err, "child %d is not a valid proof", i)
		}
		level = append(level, a)
	}
	if len(level) == 0 {
		return nil, nil
	}
	for len(level) > 1 {
		var next []*leanpq.Aggregate
		for chunk := range slices.Chunk(level, leanVMMaxChildren) {
			if len(chunk) == 1 {
				next = append(next, chunk[0])
				continue
			}
			merged, err := l.aggregate(chunk, nil)
			if err != nil {
				closeAggregates(next)
				return nil, err
			}
			closeAggregates(chunk)
			next = append(next, merged)
		}
		level = next
	}
	return level[0].Bytes()
}

func (*leanVM) Verify(statements []Statement, proof []byte) error {
	claims := statementClaims(statements)
	if len(claims) == 0 || len(proof) == 0 {
		if len(claims) == 0 && len(proof) == 0 {
			return nil
		}
		return errors.Wrap(ErrInvalidAttestationProof, "empty proof or empty claims")
	}
	start := time.Now()
	a, err := leanpq.AggregateFromBytes(proof, claims, nil)
	if err != nil {
		return errors.Wrap(ErrInvalidAttestationProof, err.Error())
	}
	a.Close()
	log.WithFields(logrus.Fields{
		"claims":    len(claims),
		"proofSize": len(proof),
		"duration":  time.Since(start),
	}).Debug("Verified with leanVM")
	return nil
}

func (*leanVM) Succinct() bool { return true }
