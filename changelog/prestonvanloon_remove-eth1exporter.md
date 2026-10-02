### Removed

- Removed the `tools/eth1exporter` address balance Prometheus exporter. It was unrelated to beacon chain eth1data handling, had no build, CI, or deployment wiring outside a manual Bazel image target, and defaulted to the retired Holesky endpoint.
