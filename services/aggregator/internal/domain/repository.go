package domain

import "context"

type EventRepository interface {
	Create(ctx context.Context, event *ProcessedEvent) error
	GetByID(ctx context.Context, eventID string) (*ProcessedEvent, error)
	GetByDeveloperID(ctx context.Context, developerID string) ([]*ProcessedEvent, error)
}

type SummaryRepository interface {
	GetOrCreate(ctx context.Context, developerID string) (*DeveloperSummary, error)
	Update(ctx context.Context, summary *DeveloperSummary) error
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
