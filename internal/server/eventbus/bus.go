package eventbus

import "context"

// Bus is a thin abstraction over the internal event distribution mechanism.
type Bus interface {
	Publish(ctx context.Context, topic string, payload any) error
	Subscribe(topic string, ch chan<- any) (unsubscribe func(), err error)
}
