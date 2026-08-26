### Added

- New `beacon-chain/p2p/peerscoring` package: a self-contained peer scoring and grey-listing service replacing the bad-responses, peer-status and gossip scorers. Grey-listing (any-scorer verdict, composite score of `-math.MaxInt`) is decoupled from scoring; bad responses carry a granular source enum and reason; the libp2p gossip score, behaviour penalty and topic snapshots are mirrored for the debug RPC.
- New `peerscoring.GossipRejectionsStore`: records every gossip message our topic validators reject (peer, topic, agent, reason, time) to debug which peers send invalid messages on which topics — pure observability, feeding neither scoring nor grey-listing. Per peer it serves agent → rejections and topic → rejections views; only the most recent 100 rejections are retained per peer, and peers pruned from the peer store are dropped.

### Changed

- Peer scoring, grey-listing and peer chain-status tracking now live in `p2p/peerscoring`, wired directly into the p2p service; `peers.Status` no longer exposes scoring or badness APIs.
- The block provider scorer is now the self-contained `p2p/blockprovider.Selector` (it is a sync peer-selection mechanism, not scoring), owned by the p2p service and reached via `BlockProviderSelector()`; it keeps its own state instead of writing into `peerdata.PeerData`, drops fully-decayed idle entries, and is pruned alongside the peer store.
- The `p2p_peer_count{state="Bad"}` metric now counts the full grey-list verdict instead of only bad-responses badness.
- Peer scoring state is evicted when the peer store prunes a peer (grey-listed peers are retained), and only the most recent bad-response strikes are kept per peer (default 25, configurable), bounding peer scoring memory.
- The gossip score mirror is reconciled against each libp2p inspector report: peers libp2p no longer tracks have their gossip state cleared (entries with no other scoring state are dropped), so a gossip grey-listing now expires with libp2p's score retention window instead of persisting until restart.
- Grey-list verdicts now carry the reason (which aspect fired and why) instead of a generic "peer is grey-listed" error, improving disconnect and refusal logs.
- Composite peer scores are now scale-normalized: each aspect (bad responses, peer status, gossip) contributes in `[-1, 1]` before weighting, so the raw libp2p gossip score no longer dominates prune ordering by sheer magnitude.
- Excess-inbound-peer pruning now applies the subnet-coverage filter before trimming to the prune quota (previously protected peers silently shrank the quota, leaving the node over its limits), preserves worst-score-first ordering through a stable subnet-count sort, and also triggers when only the inbound limit (not the total limit) is exceeded. `peers.Status.PeersToPrune` is now `PruneCandidates`, returning the full ordered candidate list and the prune count.
- Trusted peers are protected from libp2p connection-manager trimming for as long as they remain trusted.

### Removed

- Removed the deprecated `--disable-peer-scorer` flag; peer scoring is always enabled.
- Removed `BadResponses`, chain-status and gossip mirror fields from `peerdata.PeerData` along with the legacy bad-responses, peer-status and gossip scorers.
- Removed the `peers/scorers` package and the `ProcessedBlocks`/`BlockProviderUpdated` fields from `peerdata.PeerData`; the peer store now holds only network bookkeeping.
