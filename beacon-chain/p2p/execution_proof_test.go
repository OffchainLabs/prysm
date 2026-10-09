package p2p

import (
	"testing"

	"github.com/OffchainLabs/prysm/v7/testing/require"
	"github.com/ethereum/go-ethereum/p2p/enode"
)

func TestFilterPeerForExecutionProof(t *testing.T) {
	node := func(t *testing.T, aware *ExecutionProofAware) *enode.Node {
		db, err := enode.OpenDB("")
		require.NoError(t, err)
		t.Cleanup(db.Close)
		_, k := createAddrAndPrivKey(t)
		localNode := enode.NewLocalNode(db, k)
		if aware != nil {
			localNode.Set(*aware)
		}
		return localNode.Node()
	}
	value := func(v uint8) *ExecutionProofAware {
		aware := ExecutionProofAware(v)
		return &aware
	}

	t.Run("aware", func(t *testing.T) {
		subnets, err := filterPeerForExecutionProof(node(t, value(1)))
		require.NoError(t, err)
		require.DeepEqual(t, map[uint64]bool{0: true}, subnets)
	})

	t.Run("not aware", func(t *testing.T) {
		subnets, err := filterPeerForExecutionProof(node(t, value(0)))
		require.NoError(t, err)
		require.Equal(t, 0, len(subnets))
	})

	t.Run("no entry", func(t *testing.T) {
		subnets, err := filterPeerForExecutionProof(node(t, nil))
		require.NoError(t, err)
		require.Equal(t, 0, len(subnets))
	})
}

func TestSubnetTopic(t *testing.T) {
	digest := [4]byte{0x01, 0x02, 0x03, 0x04}

	t.Run("topic with subnets", func(t *testing.T) {
		require.Equal(t, "/eth2/01020304/beacon_attestation_5", SubnetTopic(AttestationSubnetTopicFormat, digest, 5))
	})

	t.Run("topic without subnets", func(t *testing.T) {
		require.Equal(t, "/eth2/01020304/execution_proof", SubnetTopic(ExecutionProofTopicFormat, digest, 0))
	})
}
