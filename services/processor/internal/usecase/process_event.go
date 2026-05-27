package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"processor/internal/domain"
)

type ProcessEventInput struct {
	RawEvent *domain.RawEvent
	SQSBody  string
}

type ProcessEventOutput struct {
	Success        bool
	ProcessedEvent *domain.ProcessedEvent
	Error          error
}

type EventProcessor struct {
	validator domain.Validator
	publisher domain.Publisher
	logger    *slog.Logger
}

func NewEventProcessor(
	validator domain.Validator,
	publisher domain.Publisher,
	logger *slog.Logger,
) *EventProcessor {
	return &EventProcessor{
		validator: validator,
		publisher: publisher,
		logger:    logger,
	}
}

func (p *EventProcessor) ProcessEvent(
	ctx context.Context,
	input ProcessEventInput,
) ProcessEventOutput {
	var result ProcessEventOutput

	var rawEvent domain.RawEvent
	if err := json.Unmarshal([]byte(input.SQSBody), &rawEvent); err != nil {
		result.Error = fmt.Errorf("invalid JSON: %w", err)
		p.logger.Error("parsing event", "error", result.Error)
		return result
	}

	processedEvent, err := domain.NewValidatedEvent(&rawEvent, p.validator)
	if err != nil {
		result.Error = fmt.Errorf("validation failed: %w", err)
		p.logger.Error("validation failed",
			"event_id", rawEvent.EventID,
			"developer_id", rawEvent.DeveloperID,
			"error", result.Error,
		)
		return result
	}

	eventJSON, _ := json.Marshal(processedEvent)
	if err := p.publisher.Publish(ctx, string(eventJSON)); err != nil {
		result.Error = fmt.Errorf("failed to publish: %w", err)
		p.logger.Error("publish failed",
			"event_id", processedEvent.EventID,
			"error", result.Error,
		)
		return result
	}

	result.Success = true
	result.ProcessedEvent = processedEvent
	p.logger.Info("event processed",
		"event_id", processedEvent.EventID,
		"developer_id", processedEvent.DeveloperID,
		"metric_type", processedEvent.MetricType,
	)

	return result
}
