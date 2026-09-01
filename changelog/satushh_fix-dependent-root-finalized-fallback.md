### Fixed

- Fall back to `finalizedDependentRoot` when the fork-choice head has been pruned past the finalized checkpoint, so the ePBS `head_v2` payload-update event is no longer dropped with `Could not notify event feed of head_v2 payload update` across a finalization boundary. The fallback is scoped to the finalized epoch, the only epoch whose dependent root equals `finalizedDependentRoot`; for any other epoch the pruned-node error still surfaces instead of returning a wrong root.
