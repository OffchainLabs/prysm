## Advanced Log Control in Prysm

> [!WARNING]  
> This is a work in progress. The design is not finalized, nor the implementation is complete.
> This warning will be removed when the design and implementation are finalized.

### Goals

The goal of this work is to give more control over the logging system to the user. Right now the user can only control
the global verbose level of the logs, which has some limitations. Mainly, the user does not have control over:

1. Which packages are logging to the terminal. (Per-Package Visibility)
2. What is the verbosity level for each package. (Per-Package Verbosity)

We try to tackle these limitations, by providing a simple, optional mechanism to the user.

### Per-Package Visibility

We will offer two new config flags:

1. `--log-only <comma separated path of packages>`: by providing this flag, the logs will only consist of
   the packages that the user has mentioned. All other logs will be ignored.
2. `--log-exclude <comma separated list of package paths>`: by providing this flag, the logs will consist of all enabled
   logs, except the logs from packages mentioned by the user.

These flags follow the inheritance model, meaning that if a package is included/excluded, all its sub-packages are also
included/excluded. Here is an example:

`--log-only beacon-chain/db,beacon-chain/p2p  --log-exclude beacon-chain/db/kv,beacon-chain/p2p/peers`

In case of conflicting rules (same package mentioned in both flags), the `--log-only` flag takes precedence.

These flags have nothing to do with verbosity.

### Per-Package Verbosity

We currently have a flag `--verbosity <trace|debug|info|warn|error>` which sets the default verbosity for all logs.
We will introduce a new config flag `--log-vmodule value` which overrides the default verbosity for the packages
mentioned here.

An example of the flag use:
`--log-vmodule beacon-chain/db/kv=trace,beacon-chain/p2p=debug`

Any package not mentioned will take the verbosity of the provided `--verbosity` flag, or if not provided, the default
`info` value.

This flag has nothing to do with visibility.

### Prerequisite

We need to define a `log.go` file for every package that uses logging.
The file should include a declaration of the `log` variable with a field `package` set to
the full path of the package inside prysm.

For example package `kv` will have this `log.go` file:

```go
package kv

import "github.com/sirupsen/logrus"

var log = logrus.WithField("package", "beacon-chain/db/kv")
```

This way we can filter logs based on the paths provided by the user.

### Implementation

We can have a script that generates the `log.go` file for every package that uses logging.
After generating the files, we can have a CI check that ensures that every package that uses logging has this file.
If not, devs should fix it manually by adding this file, or by running the script again.

We do not need EVERY package to have this file.
For example `testing/` or `tools/` are probably not needed.
Here is the list of number of packages per top level folder in the project:

- api – 14
- async – 3
- beacon-chain – 112
- cache – 2
- cmd – 30
- config – 6
- consensus-types – 13
- container – 7
- contracts – 2
- crypto – 11
- encoding – 6
- genesis – 2
- io – 4
- math – 1
- monitoring – 7
- network – 3
- proto – 17
- runtime – 11
- testing – 84
- time – 4
- tools – 50
- validator – 37

We can exclude some of these paths from the rule.

### Improvements

We can also offer additional features here:

- We can let the users know that they can skip writing `beacon-chain` or `validator` in their path. We can just try for
  those paths as well, since most logs come from these packages.
- We could warn the user in case of conflicting rules in the visibility flags.
