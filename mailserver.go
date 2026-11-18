package main

type (
	QueueProcessor interface {
		Consume()
	}
)
