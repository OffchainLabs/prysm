### Removed

- Removed a tracing span on signature requests. These requests usually took less than 5 nanoseconds and are generally not worth tracing.
