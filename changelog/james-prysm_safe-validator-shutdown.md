## Added

- Added `max-health-checks` flag that sets the maximum times the validator tries to check the health of the beacon node before timing out. 0 or a negative number is indefinite.

## Fixed

- Validator client shuts down cleanly on error instead of fatal error. 

## Changed

- If beacon node disconnects can now time out on retries 