package beacon

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/api/client"
	"github.com/pkg/errors"
)

// GetExecutionPayload retrieves the SignedExecutionPayloadEnvelope for the given block id.
// Block identifier can be one of: "head" (canonical head in node's view), "genesis", "finalized",
// <slot>, <hex encoded blockRoot with 0x prefix>. Variables of type StateOrBlockId are exported by this package
// for the named identifiers.
// The return value contains the ssz-encoded bytes.
func (c *Client) GetExecutionPayload(ctx context.Context, blockId StateOrBlockId) ([]byte, error) {
	blockPath := RenderGetBlockPath(blockId)
	b, err := c.Get(ctx, blockPath, client.WithSSZEncoding())
	if err != nil {
		return nil, errors.Wrapf(err, "error requesting execuction payload by id = %s", blockId)
	}
	return b, nil
}
