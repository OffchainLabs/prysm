## Added

- Added `max-health-checks` flag that sets the maximum times the validator tries to check the health of the beacon node before timing out. 0 or a negative number is indefinite. (the default is 0)

## Fixed

- Validator client shuts down cleanly on error instead of fatal error. 

## Changed

- Previously, we optimistically believed the beacon node was healthy and tried to get chain start, but now we do a health check at the start.