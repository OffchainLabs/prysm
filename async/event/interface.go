package event

// SubscriberSender is an abstract representation of an *event.Feed
// to use in describing types that accept or return an *event.Feed.
type SubscriberSender interface {
	Subscribe(channel interface{}) Subscription
	Send(value interface{}) (nsent int)
}
