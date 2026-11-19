### Fixed

- Never attempt a proposer reorg when `PROPOSER_REORG_CUTOFF_BPS` cannot produce a cutoff within the slot; an out-of-range cutoff previously made `GetProposerHead` reorg regardless of proposal time. A warning is logged when such a config is loaded.
