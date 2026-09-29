### Changed

- Derive the default builder `auth_data` from the builder URL's hostname instead of the full URL bytes, per builder-specs#168.
- Reject builder URLs without a hostname (`https://:8080`) or with a non-ASCII one. Internationalized hostnames must be punycode-encoded.
