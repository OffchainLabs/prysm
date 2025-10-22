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
We will introduce a new config flag `--log-config <file_path>` which overrides the default verbosity for the packages defined in the config file.

An example of a verbosity config file:
``` 
[log.conf]

- blockchain: debug
- p2p: info
- light-client: error
```

Any package not mentioned in the config file will take the verbosity of the provided `--verbosity` flag, or if not provided, the default `info` value.

This flag has nothing to do with visibility.


### Prerequisite
Since this uses the package prefix names as identifiers, it won't work unless every package defines their own prefix correctly and uses it.
So as a prerequisite we would need to make sure every package has that set up.