package client

import (
	"fmt"
	"slices"
	"strconv"

	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/container/slice"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type submittedAttData struct {
	beaconBlockRoot []byte
	source          *ethpb.Checkpoint
	target          *ethpb.Checkpoint
}

type submittedAtt struct {
	data       submittedAttData
	pubkeys    [][]byte
	committees []primitives.CommitteeIndex
}

// slotRootKey groups submissions made for the same slot and block root.
type slotRootKey struct {
	blockRoot [fieldparams.RootLength]byte
	slot      primitives.Slot
}

// submittedSyncContribution accumulates the aggregator indices and the best observed
// participation bits per subcommittee for contributions sharing the same slot and block root.
type submittedSyncContribution struct {
	aggregatorIndices []uint64
	subcommitteeBits  map[uint64]uint64
}

// submittedPayloadAttKey groups payload attestations submitted for the same slot, block root,
// and payload status.
type submittedPayloadAttKey struct {
	blockRoot         [fieldparams.RootLength]byte
	slot              primitives.Slot
	payloadPresent    bool
	blobDataAvailable bool
}

// submittedAttKey is defined as a concatenation of:
//   - AttestationData.BeaconBlockRoot
//   - AttestationData.Source.HashTreeRoot()
//   - AttestationData.Target.HashTreeRoot()
type submittedAttKey [96]byte

func (k *submittedAttKey) FromAttData(data *ethpb.AttestationData) error {
	sourceRoot, err := data.Source.HashTreeRoot()
	if err != nil {
		return err
	}
	targetRoot, err := data.Target.HashTreeRoot()
	if err != nil {
		return err
	}
	copy(k[0:], data.BeaconBlockRoot)
	copy(k[32:], sourceRoot[:])
	copy(k[64:], targetRoot[:])
	return nil
}

// saveSubmittedAtt saves the submitted attestation data along with the attester's pubkey.
// The purpose of this is to display combined attesting logs for all keys managed by the validator client.
func (v *validator) saveSubmittedAtt(att ethpb.Att, pubkey []byte, isAggregate bool) error {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()
	data := att.GetData()
	key := submittedAttKey{}
	if err := key.FromAttData(data); err != nil {
		return errors.Wrapf(err, "could not create submitted attestation key")
	}
	d := submittedAttData{
		beaconBlockRoot: data.BeaconBlockRoot,
		source:          data.Source,
		target:          data.Target,
	}

	var submittedAtts map[submittedAttKey]*submittedAtt
	if isAggregate {
		submittedAtts = v.submittedAggregates
	} else {
		submittedAtts = v.submittedAtts
		v.setAttestedSlot(bytesutil.ToBytes48(pubkey), data.Slot)
	}

	if submittedAtts[key] == nil {
		submittedAtts[key] = &submittedAtt{
			d,
			[][]byte{},
			[]primitives.CommitteeIndex{},
		}
	}
	submittedAtts[key] = &submittedAtt{
		d,
		append(submittedAtts[key].pubkeys, pubkey),
		append(submittedAtts[key].committees, att.GetCommitteeIndex()),
	}

	return nil
}

func (v *validator) setAttestedSlot(pubkey [fieldparams.BLSPubkeyLength]byte, slot primitives.Slot) {
	epoch := slots.ToEpoch(slot)

	v.attestedSlotsLock.Lock()
	defer v.attestedSlotsLock.Unlock()

	for attestedEpoch := range v.attestedSlotsByKeyByEpoch {
		if attestedEpoch+1 < epoch {
			delete(v.attestedSlotsByKeyByEpoch, attestedEpoch)
		}
	}

	if v.attestedSlotsByKeyByEpoch[epoch] == nil {
		v.attestedSlotsByKeyByEpoch[epoch] = make(map[[fieldparams.BLSPubkeyLength]byte]primitives.Slot)
	}

	v.attestedSlotsByKeyByEpoch[epoch][pubkey] = slot
}

func (v *validator) attestedSlot(epoch primitives.Epoch, pubkey [fieldparams.BLSPubkeyLength]byte) (primitives.Slot, bool) {
	v.attestedSlotsLock.RLock()
	defer v.attestedSlotsLock.RUnlock()

	// Safe even if epoch is not in the map.
	slot, ok := v.attestedSlotsByKeyByEpoch[epoch][pubkey]
	return slot, ok
}

// saveSubmittedSyncMessage saves the submitted sync committee message along with the signer's
// validator index. The purpose of this is to display combined logs for all keys managed by the
// validator client.
func (v *validator) saveSubmittedSyncMessage(msg *ethpb.SyncCommitteeMessage) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

	if v.submittedSyncMessages == nil {
		v.submittedSyncMessages = make(map[slotRootKey][]uint64)
	}
	key := slotRootKey{slot: msg.Slot}
	copy(key.blockRoot[:], msg.BlockRoot)
	v.submittedSyncMessages[key] = append(v.submittedSyncMessages[key], uint64(msg.ValidatorIndex))
}

// saveSubmittedSyncContribution saves the submitted sync committee contribution along with the
// aggregator's validator index. The purpose of this is to display combined logs for all keys
// managed by the validator client.
func (v *validator) saveSubmittedSyncContribution(contributionAndProof *ethpb.ContributionAndProof) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

	if v.submittedSyncContributions == nil {
		v.submittedSyncContributions = make(map[slotRootKey]*submittedSyncContribution)
	}
	contribution := contributionAndProof.Contribution
	key := slotRootKey{slot: contribution.Slot}
	copy(key.blockRoot[:], contribution.BlockRoot)
	entry := v.submittedSyncContributions[key]
	if entry == nil {
		entry = &submittedSyncContribution{subcommitteeBits: make(map[uint64]uint64)}
		v.submittedSyncContributions[key] = entry
	}
	entry.aggregatorIndices = append(entry.aggregatorIndices, uint64(contributionAndProof.AggregatorIndex))
	if bits := contribution.AggregationBits.Count(); bits > entry.subcommitteeBits[contribution.SubcommitteeIndex] {
		entry.subcommitteeBits[contribution.SubcommitteeIndex] = bits
	}
}

// saveSubmittedPayloadAtt saves the submitted payload attestation data along with the PTC
// member's validator index. The purpose of this is to display combined logs for all keys managed
// by the validator client.
func (v *validator) saveSubmittedPayloadAtt(data *ethpb.PayloadAttestationData, idx primitives.ValidatorIndex) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

	if v.submittedPayloadAtts == nil {
		v.submittedPayloadAtts = make(map[submittedPayloadAttKey][]uint64)
	}
	key := submittedPayloadAttKey{
		slot:              data.Slot,
		payloadPresent:    data.PayloadPresent,
		blobDataAvailable: data.BlobDataAvailable,
	}
	copy(key.blockRoot[:], data.BeaconBlockRoot)
	v.submittedPayloadAtts[key] = append(v.submittedPayloadAtts[key], uint64(idx))
}

// LogSubmissions logs info about all successful submissions the validator client made this slot.
// Note: This method holds the lock for the entire duration of logging.
func (v *validator) LogSubmissions(slot primitives.Slot) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

	v.logSubmittedAtts(slot)
	v.logSubmittedPayloadAttestations(slot)
	v.logSubmittedSyncCommitteeMessages(slot)
	v.logSubmittedSyncCommitteeContributions(slot)
}

// logSubmittedAtts logs info about submitted attestations.
func (v *validator) logSubmittedAtts(slot primitives.Slot) {
	sinceSlotStartTime, err := v.sinceSlotStartTime(slot)
	if err != nil {
		log.WithError(err).WithField("slot", slot).Error("Failed to compute time since slot start")
	}

	for _, attLog := range v.submittedAtts {
		pubkeys := make([]string, len(attLog.pubkeys))
		for i, p := range attLog.pubkeys {
			pubkeys[i] = fmt.Sprintf("%#x", bytesutil.Trunc(p))
		}
		committees := make([]string, len(attLog.committees))
		for i, c := range attLog.committees {
			committees[i] = strconv.FormatUint(uint64(c), 10)
		}
		log.WithFields(logrus.Fields{
			"slot":               slot,
			"sinceSlotStartTime": sinceSlotStartTime,
			"committeeIndices":   committees,
			"pubkeys":            pubkeys,
			"blockRoot":          fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.beaconBlockRoot)),
			"sourceEpoch":        attLog.data.source.Epoch,
			"sourceRoot":         fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.source.Root)),
			"targetEpoch":        attLog.data.target.Epoch,
			"targetRoot":         fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.target.Root)),
		}).Info("Submitted new attestations")
	}
	for _, attLog := range v.submittedAggregates {
		pubkeys := make([]string, len(attLog.pubkeys))
		for i, p := range attLog.pubkeys {
			pubkeys[i] = fmt.Sprintf("%#x", bytesutil.Trunc(p))
		}
		committees := make([]string, len(attLog.committees))
		for i, c := range attLog.committees {
			committees[i] = strconv.FormatUint(uint64(c), 10)
		}
		log.WithFields(logrus.Fields{
			"slot":             slot,
			"committeeIndices": committees,
			"pubkeys":          pubkeys,
			"blockRoot":        fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.beaconBlockRoot)),
			"sourceEpoch":      attLog.data.source.Epoch,
			"sourceRoot":       fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.source.Root)),
			"targetEpoch":      attLog.data.target.Epoch,
			"targetRoot":       fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.target.Root)),
		}).Info("Submitted new aggregate attestations")
	}

	v.submittedAtts = make(map[submittedAttKey]*submittedAtt)
	v.submittedAggregates = make(map[submittedAttKey]*submittedAtt)
}

// logSubmittedPayloadAttestations logs info about submitted payload attestations.
func (v *validator) logSubmittedPayloadAttestations(slot primitives.Slot) {
	for key, indices := range v.submittedPayloadAtts {
		slices.Sort(indices)
		log.WithFields(logrus.Fields{
			"slot":              slot,
			"dataSlot":          key.slot,
			"blockRoot":         fmt.Sprintf("%#x", bytesutil.Trunc(key.blockRoot[:])),
			"payloadPresent":    key.payloadPresent,
			"blobDataAvailable": key.blobDataAvailable,
			"validatorIndices":  slice.PrettySlice(indices),
			"attestations":      len(indices),
		}).Info("Submitted payload attestations")
	}

	v.submittedPayloadAtts = make(map[submittedPayloadAttKey][]uint64)
}

// logSubmittedSyncCommitteeMessages logs info about submitted sync committee messages.
func (v *validator) logSubmittedSyncCommitteeMessages(slot primitives.Slot) {
	for key, indices := range v.submittedSyncMessages {
		slices.Sort(indices)
		log.WithFields(logrus.Fields{
			"slot":             slot,
			"dataSlot":         key.slot,
			"blockRoot":        fmt.Sprintf("%#x", bytesutil.Trunc(key.blockRoot[:])),
			"validatorIndices": slice.PrettySlice(indices),
			"messages":         len(indices),
		}).Info("Submitted sync committee messages")
	}

	v.submittedSyncMessages = make(map[slotRootKey][]uint64)
}

// logSubmittedSyncCommitteeContributions logs info about submitted sync committee contributions.
func (v *validator) logSubmittedSyncCommitteeContributions(slot primitives.Slot) {
	for key, entry := range v.submittedSyncContributions {
		slices.Sort(entry.aggregatorIndices)
		subcommittees := make([]uint64, 0, len(entry.subcommitteeBits))
		var totalBits uint64
		for idx, bits := range entry.subcommitteeBits {
			subcommittees = append(subcommittees, idx)
			totalBits += bits
		}
		slices.Sort(subcommittees)
		log.WithFields(logrus.Fields{
			"slot":              slot,
			"dataSlot":          key.slot,
			"blockRoot":         fmt.Sprintf("%#x", bytesutil.Trunc(key.blockRoot[:])),
			"subcommittees":     slice.PrettySlice(subcommittees),
			"totalBits":         totalBits,
			"aggregatorIndices": slice.PrettySlice(entry.aggregatorIndices),
			"contributions":     len(entry.aggregatorIndices),
		}).Info("Submitted sync committee contributions and proofs")
	}

	v.submittedSyncContributions = make(map[slotRootKey]*submittedSyncContribution)
}
