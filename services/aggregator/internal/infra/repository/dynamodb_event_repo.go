package repository

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"aggregator/internal/domain"
	"aggregator/internal/infra/retry"
)

type DynamoDBEventRepository struct {
	client    *dynamodb.Client
	tableName string
	logger    *slog.Logger
	retry     retry.Config
}

func NewDynamoDBEventRepository(
	client *dynamodb.Client,
	tableName string,
	logger *slog.Logger,
) *DynamoDBEventRepository {
	return &DynamoDBEventRepository{
		client:    client,
		tableName: tableName,
		logger:    logger,
		retry:     retry.Default(),
	}
}

func (repo *DynamoDBEventRepository) Create(
	ctx context.Context,
	event *domain.ProcessedEvent,
) error {
	item, err := attributevalue.MarshalMap(event)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	err = retry.Do(ctx, repo.retry, func() error {
		_, putErr := repo.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName: &repo.tableName,
			Item:      item,
		})
		if putErr != nil {
			repo.logger.Warn("put event attempt failed",
				"event_id", event.EventID,
				"error", putErr,
			)
			return fmt.Errorf("put item error: %w", putErr)
		}
		return nil
	})
	if err != nil {
		return err
	}

	repo.logger.Debug("event saved", "event_id", event.EventID)
	return nil
}

func (repo *DynamoDBEventRepository) GetByID(
	ctx context.Context,
	eventID string,
) (*domain.ProcessedEvent, error) {
	result, err := repo.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &repo.tableName,
		Key: map[string]types.AttributeValue{
			"event_id": &types.AttributeValueMemberS{Value: eventID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("get item error: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("event not found")
	}

	var event domain.ProcessedEvent
	if err := attributevalue.UnmarshalMap(result.Item, &event); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return &event, nil
}

func (repo *DynamoDBEventRepository) GetByDeveloperID(
	ctx context.Context,
	developerID string,
) ([]*domain.ProcessedEvent, error) {
	result, err := repo.client.Scan(ctx, &dynamodb.ScanInput{
		TableName:        &repo.tableName,
		FilterExpression: aws.String("developer_id = :dev_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":dev_id": &types.AttributeValueMemberS{Value: developerID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}

	var events []*domain.ProcessedEvent
	if err := attributevalue.UnmarshalListOfMaps(result.Items, &events); err != nil {
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}

	return events, nil
}