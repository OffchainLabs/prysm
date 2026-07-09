### Fixed

- Treat an ENR port value of `0` as absent when building dial addresses, so peers advertising a zero port are no longer dialed at `/tcp/0`.
