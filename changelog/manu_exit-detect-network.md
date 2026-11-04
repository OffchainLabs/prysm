### Fixed

- `validator accounts voluntary-exit` and `prysmctl validator exit`: Detect the network from the beacon node to print the right beaconcha.in tracking URLs (e.g. `hoodi.beaconcha.in` on Hoodi), instead of relying on the `--<network>` flag. No URL is printed on networks not tracked by beaconcha.in.
- `validator accounts voluntary-exit` and `prysmctl validator exit`: Return an error instead of panicking when the beacon node genesis cannot be retrieved.
