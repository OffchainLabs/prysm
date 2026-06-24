package backfill

import "github.com/pkg/errors"

var errUnrecoverable = errors.New("service in unrecoverable state")

func isRetryable(err error) bool {
	return !errors.Is(err, errUnrecoverable)
}
