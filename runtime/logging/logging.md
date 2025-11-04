## Advanced Log Control in Prysm

### Goals
The goal of this work is to give more control over the logging system to the user. Right now the user can only control the global verbose level of the logs, which has some limitations. Mainly, the user does not have control over:
1. Which packages are logging to the terminal. (Per-Package Visibility)
2. What is the verbosity level for each package. (Per-Package Verbosity)

We try to tackle these limitations, by providing a simple, optional mechanism to the user.

### Per-Package Visibility
We will offer two new config flags:

1. `--log-include-only <comma separated list of package names>`: by providing this flag, the logs will only consist of the packages that the user has mentioned. All other logs will be ignored.
2. `--log-exclude <comma separated list of package names>`: by providing this flag, the logs will consist of all logs, except the logs from packages mentioned by the user.

These flags are mutually exclusive.
These flags have nothing to do with verbosity.

### Per-Package Verbosity
We currently have a flag `--verbosity <trace|debug|info|warn|error>` which sets the default verbosity for all logs.
We will introduce a new config flag `--log-vmodule value` which overrides the default verbosity for the packages mentioned here.

An example of the flag use:
`--log-vmodule beacon-chain/db/kv=5,beacon-chain/p2p=4`

Any package not mentioned will take the verbosity of the provided `--verbosity` flag, or if not provided, the default `info` value.

This flag has nothing to do with visibility.


### Prerequisite
We need to define a `doc.go` file for every package that we care about<sup>1</sup>.
The file should include a declaration of the `log` variable with a field `package` set to 
the full path of the package inside prysm.

For example package `kv` will have this `doc.go` file:

```go
// Package kv handles the key value db of prysm
//We can add package descriptions here, which is encouraged.
//
package kv

import "github.com/sirupsen/logrus"

var log = logrus.WithField("package", "beacon-chain/db/kv")
```
This way we can filter logs based on the paths provided by the user.

**1:**

We can have a script that generates the `doc.go` file for every package.
We also can enforce this rule on the GitHub actions checks.
But we might not need EVERY package to have this. 
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

The point is, that we can exclude some of these paths from the rule.

**Open Question:** how to handle packages that do not emit a log (yet)? should we define the log variable as `_`?

### Improvements
We can also offer additional features here:

- We can let the users know that they can skip writing `beacon-chain` or `validator` in their path. We can just try for those paths as well, since most logs come from these packages.
- We can allow pattern matchings, so users can have more freedom of expressing what packages they want to mention.