package queue

import "context"

type MessageProcessor interface {
	Name() string
	Process(ctx context.Context, body string) error
}