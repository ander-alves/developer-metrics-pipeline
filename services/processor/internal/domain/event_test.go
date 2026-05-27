package domain

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestValidateRawEvent(t *testing.T) {
	v := &DefaultValidator{}

	tests := []struct {
		name    string
		event   *RawEvent
		wantErr bool
	}{
		{
			name: "valid event - commits",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       5,
				Repository:  "org/repo",
				Timestamp:   time.Now().Add(-1 * time.Hour),
			},
			wantErr: false,
		},
		{
			name: "valid event - pull_requests",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-456",
				MetricType:  "pull_requests",
				Value:       2,
				Repository:  "org/repo",
				Timestamp:   time.Now().Add(-2 * time.Hour),
			},
			wantErr: false,
		},
		{
			name: "valid event - review_time_minutes",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-789",
				MetricType:  "review_time_minutes",
				Value:       60,
				Repository:  "org/repo",
				Timestamp:   time.Now().Add(-30 * time.Minute),
			},
			wantErr: false,
		},
		{
			name: "invalid UUID",
			event: &RawEvent{
				EventID:     "invalid-uuid",
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       5,
				Timestamp:   time.Now(),
			},
			wantErr: true,
		},
		{
			name: "empty event_id",
			event: &RawEvent{
				EventID:     "",
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       5,
				Timestamp:   time.Now(),
			},
			wantErr: true,
		},
		{
			name: "empty developer_id",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "",
				MetricType:  "commits",
				Value:       5,
				Timestamp:   time.Now(),
			},
			wantErr: true,
		},
		{
			name: "invalid metric_type",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "invalid_metric",
				Value:       5,
				Timestamp:   time.Now(),
			},
			wantErr: true,
		},
		{
			name: "negative value",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       -5,
				Timestamp:   time.Now(),
			},
			wantErr: true,
		},
		{
			name: "review_time exceeds 1440",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "review_time_minutes",
				Value:       1500,
				Timestamp:   time.Now().Add(-1 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "future timestamp",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       5,
				Timestamp:   time.Now().Add(1 * time.Hour), // well beyond the 5min skew tolerance
			},
			wantErr: true,
		},
		{
			name: "timestamp within skew tolerance is accepted",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       5,
				Repository:  "org/repo",
				Timestamp:   time.Now().Add(2 * time.Minute), // 2min ahead — within the 5min tolerance
			},
			wantErr: false,
		},
		{
			name: "zero timestamp",
			event: &RawEvent{
				EventID:     uuid.New().String(),
				DeveloperID: "dev-123",
				MetricType:  "commits",
				Value:       5,
				Timestamp:   time.Time{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.ValidateRawEvent(tt.event)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRawEvent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewValidatedEvent(t *testing.T) {
	v := &DefaultValidator{}

	raw := &RawEvent{
		EventID:     uuid.New().String(),
		DeveloperID: "dev-123",
		MetricType:  "commits",
		Value:       10,
		Repository:  "org/repo",
		Timestamp:   time.Now().Add(-1 * time.Hour),
	}

	processed, err := NewValidatedEvent(raw, v)
	if err != nil {
		t.Fatalf("NewValidatedEvent() error = %v", err)
	}

	if processed.EventID != raw.EventID {
		t.Errorf("EventID mismatch: got %s, want %s", processed.EventID, raw.EventID)
	}
	if processed.ProcessorID == "" {
		t.Error("ProcessorID should not be empty")
	}
	if processed.ProcessedAt.IsZero() {
		t.Error("ProcessedAt should not be zero")
	}
}
