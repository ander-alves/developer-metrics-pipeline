package queue

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"processor/internal/domain"
)

type SQSConsumer struct {
	client            *sqs.Client
	queueURL          string
	maxMessages       int32
	waitTimeSeconds   int32
	visibilityTimeout int32
	logger            *slog.Logger
}

func NewSQSConsumer(
	client *sqs.Client,
	queueURL string,
	logger *slog.Logger,
) *SQSConsumer {
	return &SQSConsumer{
		client:            client,
		queueURL:          queueURL,
		maxMessages:       10,
		waitTimeSeconds:   20,
		visibilityTimeout: 30,
		logger:            logger,
	}
}

func (sc *SQSConsumer) ReceiveMessages(
	ctx context.Context,
) ([]domain.Message, error) {
	input := &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(sc.queueURL),
		MaxNumberOfMessages:   sc.maxMessages,
		WaitTimeSeconds:       sc.waitTimeSeconds,
		AttributeNames:        []types.QueueAttributeName{types.QueueAttributeNameAll},
		MessageAttributeNames: []string{"All"},
	}

	result, err := sc.client.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("receive message failed: %w", err)
	}

	messages := make([]domain.Message, len(result.Messages))
	for i, msg := range result.Messages {
		messages[i] = domain.Message{
			ReceiptHandle: *msg.ReceiptHandle,
			Body:          *msg.Body,
			Attributes:    msg.Attributes,
		}
	}

	if len(messages) > 0 {
		sc.logger.Debug("received messages from queue", "count", len(messages))
	}

	return messages, nil
}

func (sc *SQSConsumer) DeleteMessage(
	ctx context.Context,
	receiptHandle string,
) error {
	_, err := sc.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(sc.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete message failed: %w", err)
	}
	return nil
}

func (sc *SQSConsumer) ChangeMessageVisibility(
	ctx context.Context,
	receiptHandle string,
	visibilityTimeoutSeconds int32,
) error {
	_, err := sc.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(sc.queueURL),
		ReceiptHandle:     aws.String(receiptHandle),
		VisibilityTimeout: visibilityTimeoutSeconds,
	})
	if err != nil {
		return fmt.Errorf("change visibility failed: %w", err)
	}
	return nil
}
