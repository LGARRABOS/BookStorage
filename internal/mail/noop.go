package mail

import "context"

type noopSender struct{}

func (noopSender) Send(context.Context, Message) error { return nil }
