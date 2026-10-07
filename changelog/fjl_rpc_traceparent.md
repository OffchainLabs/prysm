### Changed

- Added support for OpenTelemetry distributed tracing. The engine API client now sends a
  traceparent header, which can be used to correlate traces with the execution layer
  client.
