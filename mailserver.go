package main

import "context"

type (
	QueueProcessor interface {
		Consume(ctx context.Context)
	}
)
