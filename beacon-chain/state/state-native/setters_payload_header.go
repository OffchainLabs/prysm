package state_native

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/prysmaticlabs/prysm/v5/beacon-chain/state/state-native/types"
	consensusblocks "github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	enginev1 "github.com/prysmaticlabs/prysm/v5/proto/engine/v1"
	_ "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// SetLatestExecutionPayloadHeader for the beacon state.
func (b *BeaconState) SetLatestExecutionPayloadHeader(val interfaces.ExecutionData) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.version < version.Bellatrix || b.version >= version.EPBS {
		return errNotSupported("SetLatestExecutionPayloadHeader", b.version)
	}

	switch header := val.Proto().(type) {
	case *enginev1.ExecutionPayload:
		if b.version != version.Bellatrix {
			return fmt.Errorf("wrong state version (%s) for bellatrix execution payload", version.String(b.version))
		}
		latest, err := consensusblocks.PayloadToHeader(val)
		if err != nil {
			return errors.Wrap(err, "could not convert payload to header")
		}
		b.latestExecutionPayloadHeader = latest
		b.markFieldAsDirty(types.LatestExecutionPayloadHeader)
		return nil
	case *enginev1.ExecutionPayloadCapella:
		if b.version != version.Capella {
			return fmt.Errorf("wrong state version (%s) for capella execution payload", version.String(b.version))
		}
		latest, err := consensusblocks.PayloadToHeaderCapella(val)
		if err != nil {
			return errors.Wrap(err, "could not convert payload to header")
		}
		b.latestExecutionPayloadHeaderCapella = latest
		b.markFieldAsDirty(types.LatestExecutionPayloadHeaderCapella)
		return nil
	case *enginev1.ExecutionPayloadDeneb:
		if b.version != version.Deneb && b.version != version.Electra {
			return fmt.Errorf("wrong state version (%s) for deneb execution payload", version.String(b.version))
		}
		latest, err := consensusblocks.PayloadToHeaderDeneb(val)
		if err != nil {
			return errors.Wrap(err, "could not convert payload to header")
		}
		b.latestExecutionPayloadHeaderDeneb = latest
		b.markFieldAsDirty(types.LatestExecutionPayloadHeaderDeneb)
		return nil
	case *enginev1.ExecutionPayloadHeader:
		if b.version != version.Bellatrix {
			return fmt.Errorf("wrong state version (%s) for bellatrix execution payload header", version.String(b.version))
		}
		b.latestExecutionPayloadHeader = header
		b.markFieldAsDirty(types.LatestExecutionPayloadHeader)
		return nil
	case *enginev1.ExecutionPayloadHeaderCapella:
		if b.version != version.Capella {
			return fmt.Errorf("wrong state version (%s) for capella execution payload header", version.String(b.version))
		}
		b.latestExecutionPayloadHeaderCapella = header
		b.markFieldAsDirty(types.LatestExecutionPayloadHeaderCapella)
		return nil
	case *enginev1.ExecutionPayloadHeaderDeneb:
		if b.version != version.Deneb && b.version != version.Electra {
			return fmt.Errorf("wrong state version (%s) for deneb execution payload header", version.String(b.version))
		}
		b.latestExecutionPayloadHeaderDeneb = header
		b.markFieldAsDirty(types.LatestExecutionPayloadHeaderDeneb)
		return nil
	default:
		return errors.New("value must be an execution payload header")
	}
}
