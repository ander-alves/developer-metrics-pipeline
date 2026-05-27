package domain

import (
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
)

type MetricType string

const (
	MetricTypeCommits          MetricType = "commits"
	MetricTypePullRequests     MetricType = "pull_requests"
	MetricTypeReviewTimeMinutes MetricType = "review_time_minutes"
)

type RawEvent struct {
	EventID     string    `json:"event_id"`
	DeveloperID string    `json:"developer_id"`
	MetricType  string    `json:"metric_type"`
	Value       int       `json:"value"`
	Repository  string    `json:"repository"`
	Timestamp   time.Time `json:"timestamp"`
}

type ProcessedEvent struct {
	EventID     string    `json:"event_id"`
	DeveloperID string    `json:"developer_id"`
	MetricType  string    `json:"metric_type"`
	Value       int       `json:"value"`
	Repository  string    `json:"repository"`
	Timestamp   time.Time `json:"timestamp"`
	ProcessedAt time.Time `json:"processed_at"`
	ProcessorID string    `json:"processor_id"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error on field %s: %s", e.Field, e.Message)
}

type Validator interface {
	ValidateRawEvent(event *RawEvent) error
}

type DefaultValidator struct{}

func (v *DefaultValidator) ValidateRawEvent(event *RawEvent) error {
	if event.EventID == "" {
		return ValidationError{Field: "event_id", Message: "required"}
	}
	if _, err := uuid.Parse(event.EventID); err != nil {
		return ValidationError{Field: "event_id", Message: "invalid UUID format"}
	}

	if event.DeveloperID == "" {
		return ValidationError{Field: "developer_id", Message: "required"}
	}

	switch MetricType(event.MetricType) {
	case MetricTypeCommits, MetricTypePullRequests, MetricTypeReviewTimeMinutes:
	default:
		return ValidationError{
			Field:   "metric_type",
			Message: fmt.Sprintf("invalid value: %s", event.MetricType),
		}
	}

	if event.Value < 0 {
		return ValidationError{Field: "value", Message: "must be >= 0"}
	}

	if MetricType(event.MetricType) == MetricTypeReviewTimeMinutes && event.Value > 1440 {
		return ValidationError{
			Field:   "value",
			Message: "review_time_minutes cannot exceed 1440 (24h)",
		}
	}

	if event.Timestamp.IsZero() {
		return ValidationError{Field: "timestamp", Message: "required"}
	}
	// 5min skew tolerance: Docker Desktop on Windows runs containers in a Linux
	// VM whose clock can lag the host by minutes after sleep/resume.
	if event.Timestamp.After(time.Now().UTC().Add(5 * time.Minute)) {
		return ValidationError{Field: "timestamp", Message: "cannot be in the future"}
	}

	return nil
}

func NewValidatedEvent(raw *RawEvent, validator Validator) (*ProcessedEvent, error) {
	if err := validator.ValidateRawEvent(raw); err != nil {
		return nil, err
	}

	return &ProcessedEvent{
		EventID:     raw.EventID,
		DeveloperID: raw.DeveloperID,
		MetricType:  raw.MetricType,
		Value:       raw.Value,
		Repository:  raw.Repository,
		Timestamp:   raw.Timestamp,
		ProcessedAt: time.Now().UTC(),
		ProcessorID: GetProcessorID(),
	}, nil
}

func GetProcessorID() string {
	hostname := os.Getenv("HOSTNAME")
	if hostname == "" {
		hostname = fmt.Sprintf("processor-%s", uuid.New().String()[:8])
	}
	return hostname
}
