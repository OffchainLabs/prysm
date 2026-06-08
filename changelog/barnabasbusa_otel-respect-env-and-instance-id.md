### Changed

- Tracer now honors `OTEL_SERVICE_NAME` and `OTEL_RESOURCE_ATTRIBUTES`, and emits `service.instance.id` from `--tracing-process-name` (legacy `process_name` attribute retained for backward compatibility).
