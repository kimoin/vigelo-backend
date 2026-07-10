package push

import "context"

// Message is a single push delivery attempt to one registered device endpoint.
type Message struct {
	Platform    string
	Token       string
	Title       string
	Body        string
	Environment string
	AlertID     string
	DeviceID    string
	AlertType   string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}
