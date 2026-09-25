### Ignored

- E2E only: the metrics evaluator's head-slot check now tolerates slots skipped network-wide (no node has a block for them) instead of failing a synced node whose head trails the wall clock by more than one slot, and polls for up to one slot before failing. Fixes the `metrics_check_epoch_N` failures in the mainnet multi-client postsubmit run.
