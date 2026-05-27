package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"processor/internal/domain"
)

// fakePublisher records every Publish call so tests can assert on it.
type fakePublisher struct {
	published []string
	err       error
}

func (f *fakePublisher) Publish(_ context.Context, msg string) error {
	if f.err != nil {
		return f.err
	}
	f.published = append(f.published, msg)
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func validBody(t *testing.T) string {
	t.Helper()
	raw := domain.RawEvent{
		EventID:     uuid.New().String(),
		DeveloperID: "dev-test",
		MetricType:  "commits",
		Value:       5,
		Repository:  "org/repo",
		Timestamp:   time.Now().Add(-1 * time.Hour),
	}
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// TestProcessEvent_ValidEvent_PublishesEnrichedEvent verifies that a valid
// raw event is validated, enriched with processed_at/processor_id, and
// published to the processed-events queue exactly once.
func TestProcessEvent_ValidEvent_PublishesEnrichedEvent(t *testing.T) {
	pub := &fakePublisher{}
	p := NewEventProcessor(&domain.DefaultValidator{}, pub, discardLogger())

	out := p.ProcessEvent(context.Background(), ProcessEventInput{SQSBody: validBody(t)})

	if !out.Success {
		t.Fatalf("expected success, got error: %v", out.Error)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(pub.published))
	}

	var processed domain.ProcessedEvent
	if err := json.Unmarshal([]byte(pub.published[0]), &processed); err != nil {
		t.Fatalf("published message is not valid JSON: %v", err)
	}
	if processed.ProcessedAt.IsZero() {
		t.Error("ProcessedAt should be set after enrichment")
	}
	if processed.ProcessorID == "" {
		t.Error("ProcessorID should be set after enrichment")
	}
}

// TestProcessEvent_InvalidEvent_DoesNotPublish verifies that a validation
// failure stops processing before the publish step — the message is left in
// the queue to be redelivered up to maxReceiveCount times before going to DLQ.
func TestProcessEvent_InvalidEvent_DoesNotPublish(t *testing.T) {
	pub := &fakePublisher{}
	p := NewEventProcessor(&domain.DefaultValidator{}, pub, discardLogger())

	invalidBody := `{"event_id":"not-a-uuid","developer_id":"dev-x","metric_type":"commits","value":1,"timestamp":"2020-01-01T00:00:00Z"}`

	out := p.ProcessEvent(context.Background(), ProcessEventInput{SQSBody: invalidBody})

	if out.Success {
		t.Fatal("expected failure for invalid event, got success")
	}
	if out.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if len(pub.published) != 0 {
		t.Errorf("invalid event should not be published, got %d publishes", len(pub.published))
	}
}

// TestProcessEvent_PublishFailure_ReturnsError verifies that a transient
// publisher error (e.g., SQS throttle) surfaces as an error so the SQS
// visibility timeout expires and the message is redelivered.
func TestProcessEvent_PublishFailure_ReturnsError(t *testing.T) {
	pub := &fakePublisher{err: errors.New("sqs: request throttled")}
	p := NewEventProcessor(&domain.DefaultValidator{}, pub, discardLogger())

	out := p.ProcessEvent(context.Background(), ProcessEventInput{SQSBody: validBody(t)})

	if out.Success {
		t.Fatal("expected failure when publisher errors, got success")
	}
	if !errors.Is(out.Error, pub.err) && out.Error == nil {
		t.Errorf("expected publisher error to propagate, got: %v", out.Error)
	}
}
