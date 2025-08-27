package client

import (
	"fmt"
	"strconv"

	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

var log = logrus.WithField("prefix", "client")

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
	v.attLogsLock.Lock()
	defer v.attLogsLock.Unlock()
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

// LogSubmittedAtts logs info about submitted attestations.
func (v *validator) LogSubmittedAtts(slot primitives.Slot) {
	v.attLogsLock.Lock()
	defer v.attLogsLock.Unlock()

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

// LogSubmittedSyncCommitteeMessages logs info about submitted sync committee messages.
func (v *validator) LogSubmittedSyncCommitteeMessages() {
	if count := v.syncCommitteeStats.totalMessagesSubmitted.Load(); count > 0 {
		log.WithField("messages", count).
			Debug("Submitted sync committee messages successfully to beacon node")

		// Reset the amount.
		v.syncCommitteeStats.totalMessagesSubmitted.Store(0)
	}
}
