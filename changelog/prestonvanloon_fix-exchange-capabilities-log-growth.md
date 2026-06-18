### Fixed

- Fixed `engine_exchangeCapabilities` requesting an ever-growing list of engine methods. Fork-specific endpoints were appended to a shared slice on every execution client reconnect, causing the "Connected execution client does not support some requested engine methods" warning to grow unbounded with duplicate entries.
