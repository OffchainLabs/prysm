### Changed

- Validator client now retries only the failed next-epoch duties independently instead of re-pulling every duty, making duty updates resilient to transient beacon node errors.

### Fixed

- A next-epoch duty fetch failure no longer disrupts the validator's current-epoch duties.
