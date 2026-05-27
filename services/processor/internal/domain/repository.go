package domain

import "context"

type Publisher interface {
	Publish(ctx context.Context, message string) error
}

type Consumer interface {
	ReceiveMessages(ctx context.Context) ([]Message, error)
	DeleteMessage(ctx context.Context, receiptHandle string) error
	ChangeMessageVisibility(ctx context.Context, receiptHandle string, visibilityTimeoutSeconds int32) error
}

type Message struct {
	ReceiptHandle string
	Body          string
	Attributes    map[string]string
}
