package filesystem

import "context"

// RunUpMigrations only exists to satisfy the interface.
func (*Store) RunUpMigrations(_ context.Context) error {
	return nil
}

// RunDownMigrations only exists to satisfy the interface.
func (*Store) RunDownMigrations(_ context.Context) error {
	return nil
}
