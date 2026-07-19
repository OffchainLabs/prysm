### Ignored

- Fix flaky test `TestStatusRPCRequest_FinalizedBlockSkippedSlots` by guarding `wg.Done` with `sync.Once`, since the stream handler may be invoked more than once.
