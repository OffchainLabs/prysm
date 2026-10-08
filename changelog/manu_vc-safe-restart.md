### Added

- Validator client: duty-aware shutdown. On SIGINT/SIGTERM, postpone the shutdown until the rewarded duties (attestation, sync committee message, block proposal) of the current slot are done, and either at least 3 seconds are left before the next slot or the next slot has no rewarded duty, so that a quick restart does not miss any rewarded duty. The shutdown is postponed by at most 8 seconds. A second interrupt stops immediately. Use `--disable-duty-aware-shutdown` to opt out.
