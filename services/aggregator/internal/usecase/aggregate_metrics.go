package usecase

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"aggregator/internal/domain"
)

type AggregateMetricsUseCase struct {
	eventRepo   domain.EventRepository
	summaryRepo domain.SummaryRepository
	logger      *slog.Logger
}

func NewAggregateMetricsUseCase(
	eventRepo domain.EventRepository,
	summaryRepo domain.SummaryRepository,
	logger *slog.Logger,
) *AggregateMetricsUseCase {
	return &AggregateMetricsUseCase{
		eventRepo:   eventRepo,
		summaryRepo: summaryRepo,
		logger:      logger,
	}
}

func (uc *AggregateMetricsUseCase) AggregateEvent(
	ctx context.Context,
	eventJSON string,
) error {
	var event domain.ProcessedEvent
	if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
		return fmt.Errorf("erro ao parsear evento: %w", err)
	}

	uc.logger.Debug("agregando evento",
		"event_id", event.EventID,
		"developer_id", event.DeveloperID,
	)

	// Idempotency guard: ack the SQS message without aggregating again.
	existing, err := uc.eventRepo.GetByID(ctx, event.EventID)
	if err == nil && existing != nil {
		uc.logger.Info("evento já processado (idempotente)",
			"event_id", event.EventID,
			"developer_id", event.DeveloperID,
		)
		return nil
	}

	event.CreatedAt = now()
	if err := uc.eventRepo.Create(ctx, &event); err != nil {
		return fmt.Errorf("erro ao salvar evento: %w", err)
	}

	summary, err := uc.summaryRepo.GetOrCreate(ctx, event.DeveloperID)
	if err != nil {
		return fmt.Errorf("erro ao carregar resumo: %w", err)
	}

	summary.UpdateWithEvent(&event)

	if err := uc.summaryRepo.Update(ctx, summary); err != nil {
		return fmt.Errorf("erro ao atualizar resumo: %w", err)
	}

	uc.logger.Info("agregação concluída",
		"event_id", event.EventID,
		"developer_id", event.DeveloperID,
		"events_processed", summary.EventsProcessed,
	)

	return nil
}

func now() time.Time {
	return time.Now().UTC()
}
