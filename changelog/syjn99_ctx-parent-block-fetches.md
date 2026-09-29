### Fixed

- Cancel background sidecar parent-block and payload-envelope requests when the sync service stops, and stop retrying canceled requests.
- Apply the response timeout to the complete parent-block and payload-envelope response reads for each attempt, preserving retries when an individual request times out.
