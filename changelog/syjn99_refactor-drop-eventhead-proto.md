### Ignored

- Replace the `EventHead` protobuf message with a plain Go struct carried on the state feed, removing the proto→JSON conversion indirection for the `head` event. No wire-format change to the `head` SSE output.
