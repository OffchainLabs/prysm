### Changed

- Updated tracing exporter from jaeger to otelhttp. This should not be a breaking change. Jaeger supports otel format, however you may need to update your URL as the default otel-collector port is 4318. See the [OpenTelemtry Protocol Exporter docs](https://opentelemetry.io/docs/specs/otel/protocol/exporter/) for more details.
