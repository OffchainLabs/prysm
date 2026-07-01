package event

import "fmt"

// SubscriptionError reports that the beacon node rejected the events
// subscription with a non-200 status (e.g. HTTP 400 when a topic is unknown).
type SubscriptionError struct {
	StatusCode int
	Body       string
}

func (e *SubscriptionError) Error() string {
	return fmt.Sprintf("received status code %d subscribing to beacon node events: %q", e.StatusCode, e.Body)
}
