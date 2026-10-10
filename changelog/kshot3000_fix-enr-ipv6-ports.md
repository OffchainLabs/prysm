### Fixed

- Fixed peer dialing for IPv6 ENRs: Prysm now reads the `tcp6`/`udp6`/`quic6` ENR entries (falling back to the IPv4 entries) when building a peer's dial addresses, so IPv6-only peers are dialed and dual-stack peers are dialed on their IPv6 port.
