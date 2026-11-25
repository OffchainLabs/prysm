## Fixed

- Fix incorrect `sync_eth2_synced` status when `beacon_head_slot` metric is missing. The comparison now only occurs when both `beacon_head_slot` and `beacon_clock_time_slot` metrics are successfully retrieved.

