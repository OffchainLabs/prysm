package p2p

import (
	"context"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/p2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (c *client) connectToPeers(ctx context.Context, peerMultiaddrs ...string) error {
	peers, err := p2p.PeersFromStringAddrs(peerMultiaddrs)
	if err != nil {
		return err
	}
	addrInfos, err := peer.AddrInfosFromP2pAddrs(peers...)
	if err != nil {
		return err
	}
	for _, info := range addrInfos {
		if info.ID == c.host.ID() {
			continue
		}
		if err := c.host.Connect(ctx, info); err != nil {
			return err
		}
	}
	return nil
}
