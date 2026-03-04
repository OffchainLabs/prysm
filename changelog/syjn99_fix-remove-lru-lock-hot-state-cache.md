### Ignored

- Remove redundant enclosing `sync.RWMutex` from `hotStateCache` in stategen, as the underlying `lru.Cache` already provides internal thread-safe locking.
