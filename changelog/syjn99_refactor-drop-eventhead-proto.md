### Ignored

- Replace the `EventHead` protobuf message with a plain Go struct carried on the state feed, removing the proto→JSON conversion indirection for the `head` event. No wire-format change to the `head` SSE output.
- Remove the unused `EventBlock` protobuf message.
- Replace the `EventChainReorg` protobuf message with a plain Go struct carried on the state feed. No wire-format change to the `chain_reorg` SSE output.
- Replace the `EventFinalizedCheckpoint` protobuf message with a plain Go struct carried on the state feed. No wire-format change to the `finalized_checkpoint` SSE output.
