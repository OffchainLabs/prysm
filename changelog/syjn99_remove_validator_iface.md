### Removed

- Removed the 39-method `iface.Validator` interface and its hand-written `testutil.FakeValidator` double. The runner, service and health monitor now take the concrete `*validator`, and `validator/rpc` declares the 7-method `ValidatorService` interface it actually consumes.
