package helpers

import (
	"bytes"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state/stateutil"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
)

const (
	legacyXMSSPubkeyDomain = 0x00
	legacyVRFRootDomain    = 0x01
)

var xmssDepositPrefix = make([]byte, stateutil.XMSSPubkeyPrefixLength)

// Spec: is_legacy_pubkey
func IsLegacyPubkey(pubkey []byte) bool {
	return len(pubkey) != fieldparams.BLSPubkeyLength || !bytes.Equal(pubkey[:stateutil.XMSSPubkeyPrefixLength], xmssDepositPrefix)
}

// Spec: compute_deposit_pubkey, returned in the padded form the state holds
func DepositPubkeyPQ(pubkey []byte) [fieldparams.BLSPubkeyLength]byte {
	if IsLegacyPubkey(pubkey) {
		derived := hash.Hash(append(bytes.Clone(pubkey), legacyXMSSPubkeyDomain))
		return stateutil.PadXMSSPubkey(derived[:])
	}
	return bytesutil.ToBytes48(pubkey)
}

// Spec: compute_legacy_vrf_root
func LegacyVRFRoot(pubkey []byte) [32]byte {
	return hash.Hash(append(bytes.Clone(pubkey), legacyVRFRootDomain))
}
