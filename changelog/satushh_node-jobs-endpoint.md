### Added

- Added `GET /prysm/v1/node/jobs` and `GET /prysm/v1/node/jobs/{job_id}` endpoints reporting the status, phase, progress, and error state of long-running node operations, starting with the `backfill` job.
- The `backfill` service is the only job producer for now; the database pruner, startup database migrations, and initial sync are planned to register as jobs in follow-up work.
