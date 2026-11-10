### Fixed

- Log streaming no longer blocks the logger. Previously, a client of the validator client's `/v2/validator/health/logs/validator/stream` endpoint or of the beacon node's gRPC `Health/StreamBeaconLogs` that stopped reading made every log call wait forever. The process froze: duties stopped, gRPC calls timed out and SIGTERM was ignored. Now, a log line is dropped for a client whose buffer is full.
