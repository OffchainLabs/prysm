### Fixed

- Fixed some database slices that were used outside of a read transaction. See [bbolt README](https://github.com/etcd-io/bbolt/blob/7b38172caf8cde993d187be4b8738fbe9266fde8/README.md?plain=1#L852) for more on this caveat.
