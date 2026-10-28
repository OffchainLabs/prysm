package p2p_test

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	"github.com/OffchainLabs/prysm/v7/testing/require"
)

// FuzzExtractGossipDigest checks the digest parser against the fixed layout of a gossip topic:
// "/eth2/", 8 hex characters, then "/".
func FuzzExtractGossipDigest(f *testing.F) {
	f.Add("/eth2/b5303f2a/beacon_block/ssz_snappy")
	f.Add("/eth2/B5303F2A/beacon_block/ssz_snappy")
	// Digest longer than 4 bytes.
	f.Add("/eth2/b5303f2a00/beacon_block/ssz_snappy")
	// Wrong prefix.
	f.Add("/eth3/b5303f2a/beacon_block/ssz_snappy")

	f.Fuzz(func(t *testing.T, topic string) {
		digest, err := p2p.ExtractGossipDigest(topic)

		const prefix = "/eth2/"
		valid := len(topic) > len(prefix)+8 && strings.HasPrefix(topic, prefix) && topic[len(prefix)+8] == '/'
		var want []byte
		if valid {
			var hexErr error
			want, hexErr = hex.DecodeString(topic[len(prefix) : len(prefix)+8])
			valid = hexErr == nil
		}
		if !valid {
			require.NotNil(t, err, "accepted %q", topic)
			return
		}
		require.NoError(t, err)
		require.Equal(t, hex.EncodeToString(want), hex.EncodeToString(digest[:]))
	})
}
