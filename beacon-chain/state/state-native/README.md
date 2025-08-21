## Adding a new field to the state

Note: Whenever only the name of a file is provided, it's assumed to be in the `/beacon-chain/state/state-native` package.

- Add a `BeaconState[Version]FieldCount` configuration item to `/config/params/config.go` and set it in `/config/params/mainnet_config.go`.
- Add the field to the `BeaconState` struct in `beacon_state.go`. Update the marshaling structs in the same file too.
- Add the field's metadata to `/beacon-chain/state/state-native/types/types.go`.
- Add a getter and a setter for the field, either to existing `getter_XXX.go`/`setter_XXX.go` files or create new ones if the field doesn't fit anywhere.
Add the new getter and setter to `/beacon-chain/state/interfaces.go`.
- Update state hashing in `hasher.go`.
- Update `ToProtoUnsafe()` and `ToProto()` functions and add a new `ProtobufBeaconState[Version]` function, all in `getters_state.go`.
- If the field is a multi-value slice, update `multi_value_slices.go`.
- Update `spec_parameters.go`.
- Update `state_trie.go`:
  - Add a `[version]Fields` variable that contains all fields of the new state version.
  - Add a `[version]SharedFieldRefCount` constant that represents the number of fields whose references are shared between states. Multi-value slice references are not shared in this way so don't include them.
  - Add the following functions: `InitializeFromProto[Version]()`, `InitializeFromProtoUnsafe[Version]()`.
  - Update the following functions: `Copy()`, `initializeMerkleLayers()`, `RecordStateMetrics()` (applies only to multi-value slice fields), `rootSelector()`,
`finalizerCleanup()` (applies only to multi-value slice fields).
- If the field is a slice, add it to the field map in `types.go`. This only applies to large slices that need to be rehashed only in part. In particular, this mostly applies for arrays of objects, and not for arrays of basic SSZ types as these are not hashed by taking the root of each element. 
- If the field is a slice, update the `fieldConverters()` function in `/beacon-chain/state/fieldtrie/field_trie_helpers.go`. The exact implementation will vary
depending on a few factors (is the field similar to an existing one, is it a multi-value slice etc). This applies only for the slices as mentioned in the previous comment. 
