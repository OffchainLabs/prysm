### Fixed

- Read the IPv6-specific `tcp6` and `quic6` ENR keys when building dial addresses for a peer whose preferred endpoint address is IPv6, so IPv6-only peers advertising only `tcp6`/`quic6` are dialed, and dual-stack peers whose IPv6 ports differ from the IPv4 ones are dialed on the correct port.
