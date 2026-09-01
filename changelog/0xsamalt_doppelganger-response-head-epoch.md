### Improved

- Doppelganger detection: `CheckDoppelGanger` returns the beacon node's head epoch in `DoppelGangerResponse`, allowing the validator client to clear reloaded keys without a separate `ChainHead` RPC.
