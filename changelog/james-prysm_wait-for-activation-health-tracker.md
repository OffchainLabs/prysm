### Changed

- Wait-for-activation retries now wait for the next beacon node health probe to report healthy, replacing the linear backoff sleep of up to 60 seconds.

### Fixed

- `WaitForActivation` no longer recurses on every retry and epoch, which kept its tracing span open and grew the stack for the whole wait.
