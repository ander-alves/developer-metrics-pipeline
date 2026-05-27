package domain

import (
	"time"
)

type ProcessedEvent struct {
	EventID     string    `json:"event_id" dynamodbav:"event_id"`
	DeveloperID string    `json:"developer_id" dynamodbav:"developer_id"`
	MetricType  string    `json:"metric_type" dynamodbav:"metric_type"`
	Value       int       `json:"value" dynamodbav:"value"`
	Repository  string    `json:"repository" dynamodbav:"repository"`
	Timestamp   time.Time `json:"timestamp" dynamodbav:"timestamp"`
	ProcessedAt time.Time `json:"processed_at" dynamodbav:"processed_at"`
	ProcessorID string    `json:"processor_id" dynamodbav:"processor_id"`
	CreatedAt   time.Time `json:"created_at" dynamodbav:"created_at"`
}
