package state_native

import (
	"fmt"

	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/runtime/version"
)

// LatestExecutionPayloadHeader of the beacon state.
func (b *BeaconState) LatestExecutionPayloadHeader() (interfaces.ExecutionData, error) {
	if b.version < version.Bellatrix {
		return nil, errNotSupported("LatestExecutionPayloadHeader", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.version >= version.Deneb {
		return blocks.WrappedExecutionPayloadHeaderDeneb(b.latestExecutionPayloadHeaderDeneb.Copy())
	}

	if b.version >= version.Capella {
		return blocks.WrappedExecutionPayloadHeaderCapella(b.latestExecutionPayloadHeaderCapella.Copy())
	}

	if b.version >= version.Bellatrix {
		return blocks.WrappedExecutionPayloadHeader(b.latestExecutionPayloadHeader.Copy())
	}

	return nil, fmt.Errorf("unsupported version (%s) for latest execution payload header", version.String(b.version))
}
