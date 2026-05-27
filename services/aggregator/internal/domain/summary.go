package domain

import "time"

// AvgReviewTimeMinutes is the mean over review_time_minutes events only;
// ReviewTimeEventsCount is the denominator so commits/PR events don't dilute it.
type DeveloperSummary struct {
	DeveloperID            string    `json:"developer_id" dynamodbav:"developer_id"`
	TotalCommits           int       `json:"total_commits" dynamodbav:"total_commits"`
	TotalPullRequests      int       `json:"total_pull_requests" dynamodbav:"total_pull_requests"`
	TotalReviewTimeMinutes float64   `json:"total_review_time_minutes" dynamodbav:"total_review_time_minutes"`
	ReviewTimeEventsCount  int       `json:"review_time_events_count" dynamodbav:"review_time_events_count"`
	AvgReviewTimeMinutes   float64   `json:"avg_review_time_minutes" dynamodbav:"avg_review_time_minutes"`
	EventsProcessed        int       `json:"events_processed" dynamodbav:"events_processed"`
	LastActivity           time.Time `json:"last_activity" dynamodbav:"last_activity"`
	UpdatedAt              time.Time `json:"updated_at" dynamodbav:"updated_at"`
}

func (ds *DeveloperSummary) UpdateWithEvent(event *ProcessedEvent) {
	ds.EventsProcessed++
	ds.LastActivity = event.Timestamp

	switch event.MetricType {
	case "commits":
		ds.TotalCommits += event.Value
	case "pull_requests":
		ds.TotalPullRequests += event.Value
	case "review_time_minutes":
		ds.TotalReviewTimeMinutes += float64(event.Value)
		ds.ReviewTimeEventsCount++
		ds.AvgReviewTimeMinutes = ds.TotalReviewTimeMinutes / float64(ds.ReviewTimeEventsCount)
	}

	ds.UpdatedAt = time.Now().UTC()
}
