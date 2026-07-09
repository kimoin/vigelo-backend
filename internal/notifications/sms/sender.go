package sms

import "context"

type Message struct {
	To      string
	Body    string
	Sender  string
}

type Sender interface {
	Send(ctx context.Context, msg Message) error
}
