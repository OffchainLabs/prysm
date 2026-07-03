package client

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/core/helpers"
	fieldparams "github.com/OffchainLabs/prysm/v7/config/fieldparams"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
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

// submittedSyncMsgKey groups sync committee messages submitted for the same slot and block root.
type submittedSyncMsgKey struct {
	blockRoot [fieldparams.RootLength]byte
	slot      primitives.Slot
}

// submittedAttKey is defined as a concatenation of:
//   - AttestationData.BeaconBlockRoot
//   - AttestationData.Source.HashTreeRoot()
//   - AttestationData.Target.HashTreeRoot()
type submittedAttKey [96]byte

func (k submittedAttKey) FromAttData(data *ethpb.AttestationData) error {
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

// LogSubmissions logs info about all successful submissions the validator client made this slot.
func (v *validator) LogSubmissions(slot primitives.Slot) {
	v.logSubmittedAtts(slot)
	v.logSubmittedSyncCommitteeMessages(slot)
}

// saveSubmittedSyncMessage saves the submitted sync committee message along with the signer's
// validator index. The purpose of this is to display combined logs for all keys managed by the
// validator client.
func (v *validator) saveSubmittedSyncMessage(msg *ethpb.SyncCommitteeMessage) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

	if v.submittedSyncMessages == nil {
		v.submittedSyncMessages = make(map[submittedSyncMsgKey][]uint64)
	}
	key := submittedSyncMsgKey{slot: msg.Slot}
	copy(key.blockRoot[:], msg.BlockRoot)
	v.submittedSyncMessages[key] = append(v.submittedSyncMessages[key], uint64(msg.ValidatorIndex))
}

// logSubmittedAtts logs info about submitted attestations.
func (v *validator) logSubmittedAtts(slot primitives.Slot) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

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
			"slot":             slot,
			"committeeIndices": committees,
			"pubkeys":          pubkeys,
			"blockRoot":        fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.beaconBlockRoot)),
			"sourceEpoch":      attLog.data.source.Epoch,
			"sourceRoot":       fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.source.Root)),
			"targetEpoch":      attLog.data.target.Epoch,
			"targetRoot":       fmt.Sprintf("%#x", bytesutil.Trunc(attLog.data.target.Root)),
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

// logSubmittedSyncCommitteeMessages logs info about submitted sync committee messages.
func (v *validator) logSubmittedSyncCommitteeMessages(slot primitives.Slot) {
	v.submissionLogsLock.Lock()
	defer v.submissionLogsLock.Unlock()

	for key, indices := range v.submittedSyncMessages {
		slices.Sort(indices)
		log.WithFields(logrus.Fields{
			"slot":             slot,
			"dataSlot":         key.slot,
			"blockRoot":        fmt.Sprintf("%#x", bytesutil.Trunc(key.blockRoot[:])),
			"validatorIndices": helpers.PrettySlice(indices),
			"messages":         len(indices),
		}).Info("Submitted sync committee messages")
	}

	v.submittedSyncMessages = make(map[submittedSyncMsgKey][]uint64)
}
