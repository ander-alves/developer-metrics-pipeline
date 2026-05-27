package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"aggregator/internal/domain"
)

// ---------------------------------------------------------------------------
// In-memory test doubles for the repository interfaces.
//
// We intentionally avoid mocking libraries: the interfaces are small enough
// that a hand-rolled fake captures intent more clearly and the test stays
// readable.
// ---------------------------------------------------------------------------

type fakeEventRepo struct {
	stored      map[string]*domain.ProcessedEvent
	createCalls int
	createErr   error
}

func newFakeEventRepo() *fakeEventRepo {
	return &fakeEventRepo{stored: map[string]*domain.ProcessedEvent{}}
}

func (f *fakeEventRepo) Create(_ context.Context, e *domain.ProcessedEvent) error {
	f.createCalls++
	if f.createErr != nil {
		return f.createErr
	}
	f.stored[e.EventID] = e
	return nil
}

func (f *fakeEventRepo) GetByID(_ context.Context, id string) (*domain.ProcessedEvent, error) {
	if e, ok := f.stored[id]; ok {
		return e, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeEventRepo) GetByDeveloperID(_ context.Context, dev string) ([]*domain.ProcessedEvent, error) {
	var out []*domain.ProcessedEvent
	for _, e := range f.stored {
		if e.DeveloperID == dev {
			out = append(out, e)
		}
	}
	return out, nil
}

type fakeSummaryRepo struct {
	summaries   map[string]*domain.DeveloperSummary
	updateCalls int
}

func newFakeSummaryRepo() *fakeSummaryRepo {
	return &fakeSummaryRepo{summaries: map[string]*domain.DeveloperSummary{}}
}

func (f *fakeSummaryRepo) GetOrCreate(_ context.Context, dev string) (*domain.DeveloperSummary, error) {
	if s, ok := f.summaries[dev]; ok {
		return s, nil
	}
	s := &domain.DeveloperSummary{DeveloperID: dev}
	f.summaries[dev] = s
	return s, nil
}

func (f *fakeSummaryRepo) Update(_ context.Context, s *domain.DeveloperSummary) error {
	f.updateCalls++
	f.summaries[s.DeveloperID] = s
	return nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func eventJSON(t *testing.T, e domain.ProcessedEvent) string {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestAggregateEvent_HappyPath_Commits(t *testing.T) {
	events := newFakeEventRepo()
	summaries := newFakeSummaryRepo()
	uc := NewAggregateMetricsUseCase(events, summaries, discardLogger())

	body := eventJSON(t, domain.ProcessedEvent{
		EventID:     "11111111-1111-4111-8111-111111111111",
		DeveloperID: "dev-001",
		MetricType:  "commits",
		Value:       5,
		Repository:  "org/repo",
		Timestamp:   time.Now().UTC(),
		ProcessedAt: time.Now().UTC(),
		ProcessorID: "processor-test",
	})

	if err := uc.AggregateEvent(context.Background(), body); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := events.createCalls; got != 1 {
		t.Errorf("eventRepo.Create call count: want 1, got %d", got)
	}
	if got := summaries.updateCalls; got != 1 {
		t.Errorf("summaryRepo.Update call count: want 1, got %d", got)
	}
	got := summaries.summaries["dev-001"]
	if got.TotalCommits != 5 {
		t.Errorf("TotalCommits: want 5, got %d", got.TotalCommits)
	}
	if got.EventsProcessed != 1 {
		t.Errorf("EventsProcessed: want 1, got %d", got.EventsProcessed)
	}
}

func TestAggregateEvent_Idempotent_SameEventID(t *testing.T) {
	events := newFakeEventRepo()
	summaries := newFakeSummaryRepo()
	uc := NewAggregateMetricsUseCase(events, summaries, discardLogger())

	body := eventJSON(t, domain.ProcessedEvent{
		EventID:     "22222222-2222-4222-8222-222222222222",
		DeveloperID: "dev-002",
		MetricType:  "commits",
		Value:       3,
		Timestamp:   time.Now().UTC(),
	})

	// Process the same event twice. The second call must short-circuit
	// before Create and before Update — that's the idempotency guarantee
	// the case asks for.
	if err := uc.AggregateEvent(context.Background(), body); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := uc.AggregateEvent(context.Background(), body); err != nil {
		t.Fatalf("second call: %v", err)
	}

	if events.createCalls != 1 {
		t.Errorf("Create should be called exactly once for duplicates, got %d", events.createCalls)
	}
	if summaries.updateCalls != 1 {
		t.Errorf("Update should be called exactly once for duplicates, got %d", summaries.updateCalls)
	}
	if summaries.summaries["dev-002"].TotalCommits != 3 {
		t.Errorf("commits should not double-count: want 3, got %d", summaries.summaries["dev-002"].TotalCommits)
	}
}

func TestAggregateEvent_AccumulatesMultipleMetricTypes(t *testing.T) {
	events := newFakeEventRepo()
	summaries := newFakeSummaryRepo()
	uc := NewAggregateMetricsUseCase(events, summaries, discardLogger())

	for _, e := range []domain.ProcessedEvent{
		{EventID: uniq("a"), DeveloperID: "dev-003", MetricType: "commits", Value: 7, Timestamp: time.Now().UTC()},
		{EventID: uniq("b"), DeveloperID: "dev-003", MetricType: "pull_requests", Value: 2, Timestamp: time.Now().UTC()},
		{EventID: uniq("c"), DeveloperID: "dev-003", MetricType: "review_time_minutes", Value: 30, Timestamp: time.Now().UTC()},
	} {
		if err := uc.AggregateEvent(context.Background(), eventJSON(t, e)); err != nil {
			t.Fatalf("aggregate %s: %v", e.MetricType, err)
		}
	}

	got := summaries.summaries["dev-003"]
	if got.TotalCommits != 7 {
		t.Errorf("TotalCommits: want 7, got %d", got.TotalCommits)
	}
	if got.TotalPullRequests != 2 {
		t.Errorf("TotalPullRequests: want 2, got %d", got.TotalPullRequests)
	}
	if got.TotalReviewTimeMinutes != 30 {
		t.Errorf("TotalReviewTimeMinutes: want 30, got %v", got.TotalReviewTimeMinutes)
	}
	if got.EventsProcessed != 3 {
		t.Errorf("EventsProcessed: want 3, got %d", got.EventsProcessed)
	}
	// AvgReviewTimeMinutes must divide by review events only (1), not all 3.
	if got.AvgReviewTimeMinutes != 30.0 {
		t.Errorf("AvgReviewTimeMinutes: want 30.0, got %v", got.AvgReviewTimeMinutes)
	}
}

func TestAggregateEvent_AvgReviewTime_MultipleReviewEvents(t *testing.T) {
	events := newFakeEventRepo()
	summaries := newFakeSummaryRepo()
	uc := NewAggregateMetricsUseCase(events, summaries, discardLogger())

	for _, e := range []domain.ProcessedEvent{
		{EventID: uniq("r1"), DeveloperID: "dev-avg", MetricType: "commits", Value: 5, Timestamp: time.Now().UTC()},
		{EventID: uniq("r2"), DeveloperID: "dev-avg", MetricType: "review_time_minutes", Value: 20, Timestamp: time.Now().UTC()},
		{EventID: uniq("r3"), DeveloperID: "dev-avg", MetricType: "review_time_minutes", Value: 40, Timestamp: time.Now().UTC()},
	} {
		if err := uc.AggregateEvent(context.Background(), eventJSON(t, e)); err != nil {
			t.Fatalf("aggregate %s: %v", e.MetricType, err)
		}
	}

	got := summaries.summaries["dev-avg"]
	// 3 total events processed, but avg must use only the 2 review events as denominator.
	if got.EventsProcessed != 3 {
		t.Errorf("EventsProcessed: want 3, got %d", got.EventsProcessed)
	}
	if got.ReviewTimeEventsCount != 2 {
		t.Errorf("ReviewTimeEventsCount: want 2, got %d", got.ReviewTimeEventsCount)
	}
	if want := 30.0; got.AvgReviewTimeMinutes != want {
		t.Errorf("AvgReviewTimeMinutes: want %.1f, got %.1f", want, got.AvgReviewTimeMinutes)
	}
}

func TestAggregateEvent_InvalidJSON_ReturnsError(t *testing.T) {
	uc := NewAggregateMetricsUseCase(newFakeEventRepo(), newFakeSummaryRepo(), discardLogger())
	err := uc.AggregateEvent(context.Background(), "{not-json")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestAggregateEvent_PersistenceFailure_Propagates(t *testing.T) {
	events := newFakeEventRepo()
	events.createErr = errors.New("dynamodb throttled")
	summaries := newFakeSummaryRepo()
	uc := NewAggregateMetricsUseCase(events, summaries, discardLogger())

	body := eventJSON(t, domain.ProcessedEvent{
		EventID:     "33333333-3333-4333-8333-333333333333",
		DeveloperID: "dev-004",
		MetricType:  "commits",
		Value:       1,
		Timestamp:   time.Now().UTC(),
	})

	err := uc.AggregateEvent(context.Background(), body)
	if err == nil {
		t.Fatal("expected persistence failure to surface as an error")
	}
	if summaries.updateCalls != 0 {
		t.Errorf("summary should not be updated when event persistence fails (got %d updates)", summaries.updateCalls)
	}
}

// uniq returns a deterministic UUID-like ID derived from a label. Good
// enough for tests that just need distinct IDs.
func uniq(label string) string {
	return label + "-cafe-4000-8000-000000000001"
}