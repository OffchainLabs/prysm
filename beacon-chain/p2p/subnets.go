package p2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/go-bitfield"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/cache"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/peerdas"
	"github.com/OffchainLabs/prysm/v7/cmd/beacon-chain/flags"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/consensus-types/wrapper"
	"github.com/OffchainLabs/prysm/v7/crypto/hash"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	pb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/p2p/enr"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
)

var (
	attestationSubnetCount = params.BeaconConfig().AttestationSubnetCount
	syncCommsSubnetCount   = params.BeaconConfig().SyncCommitteeSubnetCount

	attSubnetEnrKey         = params.BeaconNetworkConfig().AttSubnetKey
	syncCommsSubnetEnrKey   = params.BeaconNetworkConfig().SyncCommsSubnetKey
	custodyGroupCountEnrKey = params.BeaconNetworkConfig().CustodyGroupCountKey
)

// The value used with the subnet, in order
// to create an appropriate key to retrieve
// the relevant lock. This is used to differentiate
// sync subnets from others. This is deliberately
// chosen as more than 64 (attestation subnet count).
const syncLockerVal = 100

// The value used with the blob sidecar subnet, in order
// to create an appropriate key to retrieve
// the relevant lock. This is used to differentiate
// blob subnets from others. This is deliberately
// chosen more than sync and attestation subnet combined.
const blobSubnetLockerVal = 110

// The value used with the data column sidecar subnet, in order
// to create an appropriate key to retrieve
// the relevant lock. This is used to differentiate
// data column subnets from others. This is deliberately
// chosen more than sync, attestation and blob subnet (6) combined.
const dataColumnSubnetVal = 150

const errSavingSequenceNumber = "saving sequence number after updating subnets: %w"

// DialPeers dials multiple peers concurrently up to `maxConcurrentDials` at a time.
// In case of a dial failure, it logs the error but continues dialing other peers.
func (s *Service) DialPeers(ctx context.Context, maxConcurrentDials int, nodes []*enode.Node) uint {
	var mut sync.Mutex

	counter := uint(0)
	for start := 0; start < len(nodes); start += maxConcurrentDials {
		if ctx.Err() != nil {
			return counter
		}

		var wg sync.WaitGroup
		stop := min(start+maxConcurrentDials, len(nodes))
		for _, node := range nodes[start:stop] {
			log := log.WithField("nodeID", node.ID())
			info, _, err := convertToAddrInfo(node)
			if err != nil {
				log.WithError(err).Debug("Could not convert node to addr info")
				continue
			}

			if info == nil {
				log.Debug("Nil addr info")
				continue
			}

			wg.Go(func() {
				fmt.Println("connecting to address", info.String())
				if err := s.connectWithPeer(ctx, *info); err != nil {
					fmt.Println("connection failure", err)
					log.WithError(err).WithField("info", info.String()).Debug("Could not connect with peer")
					return
				}

				mut.Lock()
				defer mut.Unlock()
				counter++
			})
		}

		wg.Wait()
	}

	return counter
}

// lower threshold to broadcast object compared to searching
// for a subnet. So that even in the event of poor peer
// connectivity, we can still broadcast an attestation.
func (s *Service) hasPeerWithTopic(topic string) bool {
	// In the event peer threshold is lower, we will choose the lower
	// threshold.
	minPeers := min(1, flags.Get().MinimumPeersPerSubnet)
	peersWithSubnet := s.pubsub.ListPeers(topic)
	peersWithSubnetCount := len(peersWithSubnet)

	enoughPeers := peersWithSubnetCount >= minPeers

	return enoughPeers
}

// Updates the service's discv5 listener record's attestation subnet
// with a new value for a bitfield of subnets tracked. It also updates
// the node's metadata by increasing the sequence number and the
// subnets tracked by the node.
func (s *Service) updateSubnetRecordWithMetadata(bitV bitfield.Bitvector64) error {
	entry := enr.WithEntry(attSubnetEnrKey, &bitV)
	s.dv5Listener.LocalNode().Set(entry)
	s.metaData = wrapper.WrappedMetadataV0(&pb.MetaDataV0{
		SeqNumber: s.metaData.SequenceNumber() + 1,
		Attnets:   bitV,
	})

	if err := s.saveSequenceNumberIfNeeded(); err != nil {
		return fmt.Errorf(errSavingSequenceNumber, err)
	}
	return nil
}

// Updates the service's discv5 listener record's attestation subnet
// with a new value for a bitfield of subnets tracked. It also record's
// the sync committee subnet in the enr. It also updates the node's
// metadata by increasing the sequence number and the subnets tracked by the node.
func (s *Service) updateSubnetRecordWithMetadataV2(
	bitVAtt bitfield.Bitvector64,
	bitVSync bitfield.Bitvector4,
	custodyGroupCount uint64,
) error {
	entry := enr.WithEntry(attSubnetEnrKey, &bitVAtt)
	subEntry := enr.WithEntry(syncCommsSubnetEnrKey, &bitVSync)

	localNode := s.dv5Listener.LocalNode()
	localNode.Set(entry)
	localNode.Set(subEntry)

	if params.FuluEnabled() {
		custodyGroupCountEntry := enr.WithEntry(custodyGroupCountEnrKey, custodyGroupCount)
		localNode.Set(custodyGroupCountEntry)
	}

	s.metaData = wrapper.WrappedMetadataV1(&pb.MetaDataV1{
		SeqNumber: s.metaData.SequenceNumber() + 1,
		Attnets:   bitVAtt,
		Syncnets:  bitVSync,
	})

	if err := s.saveSequenceNumberIfNeeded(); err != nil {
		return fmt.Errorf(errSavingSequenceNumber, err)
	}
	return nil
}

// updateSubnetRecordWithMetadataV3 updates:
// - attestation subnet tracked,
// - sync subnets tracked, and
// - custody subnet count
// both in the node's record and in the node's metadata.
func (s *Service) updateSubnetRecordWithMetadataV3(
	bitVAtt bitfield.Bitvector64,
	bitVSync bitfield.Bitvector4,
	custodyGroupCount uint64,
) error {
	attSubnetsEntry := enr.WithEntry(attSubnetEnrKey, &bitVAtt)
	syncSubnetsEntry := enr.WithEntry(syncCommsSubnetEnrKey, &bitVSync)
	custodyGroupCountEntry := enr.WithEntry(custodyGroupCountEnrKey, custodyGroupCount)

	localNode := s.dv5Listener.LocalNode()
	localNode.Set(attSubnetsEntry)
	localNode.Set(syncSubnetsEntry)
	localNode.Set(custodyGroupCountEntry)

	s.metaData = wrapper.WrappedMetadataV2(&pb.MetaDataV2{
		SeqNumber:         s.metaData.SequenceNumber() + 1,
		Attnets:           bitVAtt,
		Syncnets:          bitVSync,
		CustodyGroupCount: custodyGroupCount,
	})

	if err := s.saveSequenceNumberIfNeeded(); err != nil {
		return fmt.Errorf(errSavingSequenceNumber, err)
	}
	return nil
}

// saveSequenceNumberIfNeeded saves the sequence number in DB if either of the following conditions is met:
// - the static peer ID flag is set
// - the fulu epoch is set
func (s *Service) saveSequenceNumberIfNeeded() error {
	// Short-circuit if we don't need to save the sequence number.
	if !(s.cfg.StaticPeerID || params.FuluEnabled()) {
		return nil
	}

	return s.cfg.DB.SaveMetadataSeqNum(s.ctx, s.metaData.SequenceNumber())
}

func initializePersistentSubnets(id enode.ID, epoch primitives.Epoch) error {
	_, ok, expTime := cache.SubnetIDs.GetPersistentSubnets()
	if ok && expTime.After(time.Now()) {
		return nil
	}
	subs, err := computeSubscribedSubnets(id, epoch)
	if err != nil {
		return err
	}
	newExpTime := computeSubscriptionExpirationTime(id, epoch)
	cache.SubnetIDs.AddPersistentCommittee(subs, newExpTime)
	return nil
}

// Spec pseudocode definition:
//
// def compute_subscribed_subnets(node_id: NodeID, epoch: Epoch) -> Sequence[SubnetID]:
//
//	return [compute_subscribed_subnet(node_id, epoch, index) for index in range(SUBNETS_PER_NODE)]
func computeSubscribedSubnets(nodeID enode.ID, epoch primitives.Epoch) ([]uint64, error) {
	cfg := params.BeaconConfig()

	if flags.Get().SubscribeToAllSubnets {
		subnets := make([]uint64, 0, cfg.AttestationSubnetCount)
		for i := range cfg.AttestationSubnetCount {
			subnets = append(subnets, i)
		}
		return subnets, nil
	}

	subnets := make([]uint64, 0, cfg.SubnetsPerNode)
	for i := range cfg.SubnetsPerNode {
		sub, err := computeSubscribedSubnet(nodeID, epoch, i)
		if err != nil {
			return nil, errors.Wrap(err, "compute subscribed subnet")
		}
		subnets = append(subnets, sub)
	}

	return subnets, nil
}

//	Spec pseudocode definition:
//
// def compute_subscribed_subnet(node_id: NodeID, epoch: Epoch, index: int) -> SubnetID:
//
//	node_id_prefix = node_id >> (NODE_ID_BITS - ATTESTATION_SUBNET_PREFIX_BITS)
//	node_offset = node_id % EPOCHS_PER_SUBNET_SUBSCRIPTION
//	permutation_seed = hash(uint_to_bytes(uint64((epoch + node_offset) // EPOCHS_PER_SUBNET_SUBSCRIPTION)))
//	permutated_prefix = compute_shuffled_index(
//	    node_id_prefix,
//	    1 << ATTESTATION_SUBNET_PREFIX_BITS,
//	    permutation_seed,
//	)
//	return SubnetID((permutated_prefix + index) % ATTESTATION_SUBNET_COUNT)
func computeSubscribedSubnet(nodeID enode.ID, epoch primitives.Epoch, index uint64) (uint64, error) {
	nodeOffset, nodeIdPrefix := computeOffsetAndPrefix(nodeID)
	seedInput := (nodeOffset + uint64(epoch)) / params.BeaconConfig().EpochsPerSubnetSubscription
	permSeed := hash.Hash(bytesutil.Bytes8(seedInput))
	permutatedPrefix, err := helpers.ComputeShuffledIndex(primitives.ValidatorIndex(nodeIdPrefix), 1<<params.BeaconConfig().AttestationSubnetPrefixBits, permSeed, true)
	if err != nil {
		return 0, err
	}
	subnet := (uint64(permutatedPrefix) + index) % params.BeaconConfig().AttestationSubnetCount
	return subnet, nil
}

func computeSubscriptionExpirationTime(nodeID enode.ID, epoch primitives.Epoch) time.Duration {
	nodeOffset, _ := computeOffsetAndPrefix(nodeID)
	pastEpochs := (nodeOffset + uint64(epoch)) % params.BeaconConfig().EpochsPerSubnetSubscription
	remEpochs := params.BeaconConfig().EpochsPerSubnetSubscription - pastEpochs
	epochDuration := time.Duration(params.BeaconConfig().SlotsPerEpoch.Mul(params.BeaconConfig().SecondsPerSlot))
	epochTime := time.Duration(remEpochs) * epochDuration
	return epochTime * time.Second
}

func computeOffsetAndPrefix(nodeID enode.ID) (uint64, uint64) {
	num := uint256.NewInt(0).SetBytes(nodeID.Bytes())
	remBits := params.BeaconConfig().NodeIdBits - params.BeaconConfig().AttestationSubnetPrefixBits
	// Number of bits left will be representable by a uint64 value.
	nodeIdPrefix := num.Rsh(num, uint(remBits)).Uint64()
	// Reinitialize big int.
	num = uint256.NewInt(0).SetBytes(nodeID.Bytes())
	nodeOffset := num.Mod(num, uint256.NewInt(params.BeaconConfig().EpochsPerSubnetSubscription)).Uint64()
	return nodeOffset, nodeIdPrefix
}

// Initializes a bitvector of attestation subnets beacon nodes is subscribed to
// and creates a new ENR entry with its default value.
func initializeAttSubnets(node *enode.LocalNode) *enode.LocalNode {
	bitV := bitfield.NewBitvector64()
	entry := enr.WithEntry(attSubnetEnrKey, bitV.Bytes())
	node.Set(entry)
	return node
}

// Initializes a bitvector of sync committees subnets beacon nodes is subscribed to
// and creates a new ENR entry with its default value.
func initializeSyncCommSubnets(node *enode.LocalNode) *enode.LocalNode {
	bitV := bitfield.Bitvector4{byte(0x00)}
	entry := enr.WithEntry(syncCommsSubnetEnrKey, bitV.Bytes())
	node.Set(entry)
	return node
}

// Reads the attestation subnets entry from a node's ENR and determines
// the committee indices of the attestation subnets the node is subscribed to.
func attestationSubnets(record *enr.Record) (map[uint64]bool, error) {
	bitV, err := attBitvector(record)
	if err != nil {
		return nil, errors.Wrap(err, "att bit vector")
	}

	// lint:ignore uintcast -- subnet count can be safely cast to int.
	if len(bitV) != byteCount(int(attestationSubnetCount)) {
		return nil, errors.Errorf("invalid bitvector provided, it has a size of %d", len(bitV))
	}

	indices := make(map[uint64]bool, attestationSubnetCount)
	for i := range attestationSubnetCount {
		if bitV.BitAt(i) {
			indices[i] = true
		}
	}

	return indices, nil
}

// Reads the sync subnets entry from a node's ENR and determines
// the committee indices of the sync subnets the node is subscribed to.
func syncSubnets(record *enr.Record) (map[uint64]bool, error) {
	bitV, err := syncBitvector(record)
	if err != nil {
		return nil, errors.Wrap(err, "sync bit vector")
	}

	// lint:ignore uintcast -- subnet count can be safely cast to int.
	if len(bitV) != byteCount(int(syncCommsSubnetCount)) {
		return nil, errors.Errorf("invalid bitvector provided, it has a size of %d", len(bitV))
	}

	indices := make(map[uint64]bool, syncCommsSubnetCount)
	for i := range syncCommsSubnetCount {
		if bitV.BitAt(i) {
			indices[i] = true
		}
	}
	return indices, nil
}

// Retrieve the data columns subnets from a node's ENR and node ID.
func dataColumnSubnets(nodeID enode.ID, record *enr.Record) (map[uint64]bool, error) {
	// Retrieve the custody count from the ENR.
	custodyGroupCount, err := peerdas.CustodyGroupCountFromRecord(record)
	if err != nil {
		return nil, errors.Wrap(err, "custody group count from record")
	}

	// Retrieve the peer info.
	peerInfo, _, err := peerdas.Info(nodeID, custodyGroupCount)
	if err != nil {
		return nil, errors.Wrap(err, "peer info")
	}

	// Get custody columns subnets from the columns.
	return peerInfo.DataColumnsSubnets, nil
}

// Parses the attestation subnets ENR entry in a node and extracts its value
// as a bitvector for further manipulation.
func attBitvector(record *enr.Record) (bitfield.Bitvector64, error) {
	bitV := bitfield.NewBitvector64()
	entry := enr.WithEntry(attSubnetEnrKey, &bitV)
	err := record.Load(entry)
	if err != nil {
		return nil, err
	}
	return bitV, nil
}

// Parses the attestation subnets ENR entry in a node and extracts its value
// as a bitvector for further manipulation.
func syncBitvector(record *enr.Record) (bitfield.Bitvector4, error) {
	bitV := bitfield.Bitvector4{byte(0x00)}
	entry := enr.WithEntry(syncCommsSubnetEnrKey, &bitV)
	err := record.Load(entry)
	if err != nil {
		return nil, err
	}
	return bitV, nil
}

// The subnet locker is a map which keeps track of all
// mutexes stored per subnet. This locker is reused
// between both the attestation, sync blob and data column subnets.
// Sync subnets are stored by (subnet+syncLockerVal).
// Blob subnets are stored by (subnet+blobSubnetLockerVal).
// Data column subnets are stored by (subnet+dataColumnSubnetVal).
// This is to prevent conflicts while allowing subnets
// to use a single locker.
func (s *Service) subnetLocker(i uint64) *sync.RWMutex {
	s.subnetsLockLock.Lock()
	defer s.subnetsLockLock.Unlock()

	l, ok := s.subnetsLock[i]
	if !ok {
		l = &sync.RWMutex{}
		s.subnetsLock[i] = l
	}
	return l
}

// Determines the number of bytes that are used
// to represent the provided number of bits.
func byteCount(bitCount int) int {
	numOfBytes := bitCount / 8
	if bitCount%8 != 0 {
		numOfBytes++
	}
	return numOfBytes
}
