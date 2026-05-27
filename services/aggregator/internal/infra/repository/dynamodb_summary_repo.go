package repository

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"aggregator/internal/domain"
	"aggregator/internal/infra/retry"
)

type DynamoDBSummaryRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    *slog.Logger
	retry     retry.Config
}

func NewDynamoDBSummaryRepository(
	client *dynamodb.Client,
	tableName string,
	logger *slog.Logger,
) *DynamoDBSummaryRepository {
	return &DynamoDBSummaryRepository{
		client:    client,
		tableName: tableName,
		logger:    logger,
		retry:     retry.Default(),
	}
}

func (repo *DynamoDBSummaryRepository) GetOrCreate(
	ctx context.Context,
	developerID string,
) (*domain.DeveloperSummary, error) {
	result, err := repo.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &repo.tableName,
		Key: map[string]types.AttributeValue{
			"developer_id": &types.AttributeValueMemberS{Value: developerID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get item error: %w", err)
	}

	if result.Item != nil {
		var summary domain.DeveloperSummary
		if err := attributevalue.UnmarshalMap(result.Item, &summary); err != nil {
			return nil, fmt.Errorf("unmarshal error: %w", err)
		}
		return &summary, nil
	}

	now := time.Now().UTC()
	summary := &domain.DeveloperSummary{
		DeveloperID:  developerID,
		UpdatedAt:    now,
		LastActivity: now,
	}

	repo.logger.Debug("created new summary", "developer_id", developerID)
	return summary, nil
}

func (repo *DynamoDBSummaryRepository) Update(
	ctx context.Context,
	summary *domain.DeveloperSummary,
) error {
	item, err := attributevalue.MarshalMap(summary)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	err = retry.Do(ctx, repo.retry, func() error {
		_, putErr := repo.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: &repo.tableName,
			Item:      item,
		})
		if putErr != nil {
			repo.logger.Warn("put summary attempt failed",
				"developer_id", summary.DeveloperID,
				"error", putErr,
			)
			return fmt.Errorf("put item error: %w", putErr)
		}
		return nil
	})
	if err != nil {
		return err
	}

	repo.logger.Debug("summary updated",
		"developer_id", summary.DeveloperID,
		"events_processed", summary.EventsProcessed,
	)
	return nil
}