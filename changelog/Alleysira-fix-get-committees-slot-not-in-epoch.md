### Fixed

- Made `/eth/v1/beacon/states/{state_id}/committees` endpoint return `400` when slot does not belong to the specified epoch, aligning with the Beacon API spec (#15355)
