package processor

import "context"

type Processor interface {
	Name() string
	Process(ctx context.Context, body string) error
}
