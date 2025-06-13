## Added

- Added `max-health-checks` flag that sets the maximum times the validator tries to check the health of the beacon node before timing out. -1 is indefinite.

## Fixed

- Validator client shutsdown cleanly on error instead of fatal error. 

## Changed

- If beacon node disconnects we can retry before failing. 