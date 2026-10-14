package hdiff

import (
	"encoding/binary"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const checkpointDecoupledLength = 40 // fixed SSZ size: slot(8) + root(32)

// Every FG field is an override, the bitlists change on nearly every block anyway.
func diffDecoupledFields(diff *stateDiff, target state.ReadOnlyBeaconState) error {
	var err error
	if diff.height, err = target.Height(); err != nil {
		return err
	}
	if diff.entry, err = target.Entry(); err != nil {
		return err
	}
	if diff.targetParticipation, err = target.TargetParticipation(); err != nil {
		return err
	}
	if diff.progress, err = target.Progress(); err != nil {
		return err
	}
	if diff.justifiedHeight, err = target.JustifiedHeight(); err != nil {
		return err
	}
	if diff.justified, err = target.Justified(); err != nil {
		return err
	}
	if diff.finalizedHeight, err = target.FinalizedHeight(); err != nil {
		return err
	}
	if diff.finalized, err = target.Finalized(); err != nil {
		return err
	}
	diff.pendingFinality, err = target.PendingFinality()
	return err
}

func serializedDecoupledFieldsSize(s *stateDiff) int {
	size := 3*8 + 3*checkpointDecoupledLength
	size += 8 + len(s.targetParticipation)
	size += 8 + len(s.progress)
	size += 8
	for _, p := range s.pendingFinality {
		size += 8 + p.SizeSSZ()
	}
	return size
}

func serializeDecoupledFields(ret []byte, s *stateDiff) []byte {
	var err error
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.height))
	if ret, err = s.entry.MarshalSSZTo(ret); err != nil {
		logrus.WithError(err).Error("Failed to marshal entry")
		return nil
	}
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.targetParticipation)))
	ret = append(ret, s.targetParticipation...)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.progress)))
	ret = append(ret, s.progress...)
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.justifiedHeight))
	if ret, err = s.justified.MarshalSSZTo(ret); err != nil {
		logrus.WithError(err).Error("Failed to marshal justified")
		return nil
	}
	ret = binary.LittleEndian.AppendUint64(ret, uint64(s.finalizedHeight))
	if ret, err = s.finalized.MarshalSSZTo(ret); err != nil {
		logrus.WithError(err).Error("Failed to marshal finalized")
		return nil
	}
	ret = binary.LittleEndian.AppendUint64(ret, uint64(len(s.pendingFinality)))
	for _, p := range s.pendingFinality {
		ret = binary.LittleEndian.AppendUint64(ret, uint64(p.SizeSSZ()))
		if ret, err = p.MarshalSSZTo(ret); err != nil {
			logrus.WithError(err).Error("Failed to marshal pending finality")
			return nil
		}
	}
	return ret
}

func (ret *stateDiff) readDecoupledFields(data *[]byte) error {
	var err error
	if ret.height, err = readHeight(data, "height"); err != nil {
		return err
	}
	if ret.entry, err = readCheckpointDecoupled(data, "entry"); err != nil {
		return err
	}
	if ret.targetParticipation, err = readBytes(data, "targetParticipation"); err != nil {
		return err
	}
	if ret.progress, err = readBytes(data, "progress"); err != nil {
		return err
	}
	if ret.justifiedHeight, err = readHeight(data, "justifiedHeight"); err != nil {
		return err
	}
	if ret.justified, err = readCheckpointDecoupled(data, "justified"); err != nil {
		return err
	}
	if ret.finalizedHeight, err = readHeight(data, "finalizedHeight"); err != nil {
		return err
	}
	if ret.finalized, err = readCheckpointDecoupled(data, "finalized"); err != nil {
		return err
	}
	if len(*data) < 8 {
		return errors.Wrap(errDataSmall, "pendingFinality count")
	}
	count := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	*data = (*data)[8:]
	if count < 0 || count > len(*data)/8 {
		return errors.Wrap(errDataSmall, "pendingFinality data")
	}
	ret.pendingFinality = make([]*ethpb.PendingFinality, count)
	for i := range count {
		b, err := readBytes(data, "pendingFinality")
		if err != nil {
			return err
		}
		ret.pendingFinality[i] = &ethpb.PendingFinality{}
		if err := ret.pendingFinality[i].UnmarshalSSZ(b); err != nil {
			return errors.Wrap(err, "failed to unmarshal pending finality")
		}
	}
	return nil
}

func readHeight(data *[]byte, field string) (primitives.Height, error) {
	if len(*data) < 8 {
		return 0, errors.Wrap(errDataSmall, field)
	}
	h := primitives.Height(binary.LittleEndian.Uint64((*data)[:8]))
	*data = (*data)[8:]
	return h, nil
}

func readCheckpointDecoupled(data *[]byte, field string) (*ethpb.CheckpointDecoupled, error) {
	if len(*data) < checkpointDecoupledLength {
		return nil, errors.Wrap(errDataSmall, field)
	}
	c := &ethpb.CheckpointDecoupled{}
	if err := c.UnmarshalSSZ((*data)[:checkpointDecoupledLength]); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal %s", field)
	}
	*data = (*data)[checkpointDecoupledLength:]
	return c, nil
}

func readBytes(data *[]byte, field string) ([]byte, error) {
	if len(*data) < 8 {
		return nil, errors.Wrap(errDataSmall, field+" length")
	}
	n := int(binary.LittleEndian.Uint64((*data)[:8])) // lint:ignore uintcast
	*data = (*data)[8:]
	if n < 0 || n > len(*data) {
		return nil, errors.Wrap(errDataSmall, field)
	}
	out := make([]byte, n)
	copy(out, (*data)[:n])
	*data = (*data)[n:]
	return out, nil
}

func applyDecoupledFields(source state.BeaconState, diff *stateDiff) error {
	if err := source.SetHeight(diff.height); err != nil {
		return err
	}
	if err := source.SetEntry(diff.entry); err != nil {
		return err
	}
	if err := source.SetTargetParticipation(diff.targetParticipation); err != nil {
		return err
	}
	if err := source.SetProgress(diff.progress); err != nil {
		return err
	}
	if err := source.SetJustifiedHeight(diff.justifiedHeight); err != nil {
		return err
	}
	if err := source.SetJustified(diff.justified); err != nil {
		return err
	}
	if err := source.SetFinalizedHeight(diff.finalizedHeight); err != nil {
		return err
	}
	if err := source.SetFinalized(diff.finalized); err != nil {
		return err
	}
	return source.SetPendingFinality(diff.pendingFinality)
}
