package pq

import (
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
)

const (
	vrfTreeDepth   = 31
	vrfProofLength = 32 * (vrfTreeDepth + 1)
)

var ErrInvalidVRFProof = errors.New("invalid VRF proof")

// Spec: is_valid_vrf_proof
func VerifyVRFProof(st state.ReadOnlyBeaconState, proposer primitives.ValidatorIndex, slot primitives.Slot, proof []byte) error {
	if len(proof) != vrfProofLength {
		return errors.Wrapf(ErrInvalidVRFProof, "length %d", len(proof))
	}
	root, err := st.VrfRootAtIndex(proposer)
	if err != nil {
		return err
	}
	node := [32]byte(proof[:32])
	index := uint64(slot)
	var buf [64]byte
	for i := 1; i <= vrfTreeDepth; i++ {
		sibling := proof[32*i : 32*(i+1)]
		if index&1 == 1 {
			copy(buf[:32], sibling)
			copy(buf[32:], node[:])
		} else {
			copy(buf[:32], node[:])
			copy(buf[32:], sibling)
		}
		node = hash.Hash(buf[:])
		index >>= 1
	}
	if node != root {
		return ErrInvalidVRFProof
	}
	return nil
}

// Spec: process_randao, the mix only
func MixInVRFProof(st state.BeaconState, proof []byte) error {
	if len(proof) != vrfProofLength {
		return errors.Wrapf(ErrInvalidVRFProof, "length %d", len(proof))
	}
	index := uint64(slots.ToEpoch(st.Slot()) % params.BeaconConfig().EpochsPerHistoricalVector)
	prev, err := st.RandaoMixAtIndex(index)
	if err != nil {
		return err
	}
	domain := params.BeaconConfig().DomainRandao
	buf := make([]byte, 0, 68)
	buf = append(buf, prev...)
	buf = append(buf, proof[:32]...)
	buf = append(buf, domain[:]...)
	return st.UpdateRandaoMixesAtIndex(index, hash.Hash(buf))
}
