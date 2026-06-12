### Ignored

- Use `sync.WaitGroup.Go` (Go 1.25) in place of the `wg.Add(1)` / `go func() { defer wg.Done(); ... }()` spawn-then-wait boilerplate.
