package queue

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"processor/internal/infra/retry"
)

type SQSPublisher struct {
	client   *sqs.Client
	queueURL string
	logger   *slog.Logger
	retry    retry.Config
}

func NewSQSPublisher(
	client *sqs.Client,
	queueURL string,
	logger *slog.Logger,
) *SQSPublisher {
	return &SQSPublisher{
		client:   client,
		queueURL: queueURL,
		logger:   logger,
		retry:    retry.Default(),
	}
}

// Publish sends message to the configured SQS queue, retrying transient
// errors with exponential backoff before propagating the failure. This is
// the application-layer retry the case asks for; SQS's redrive policy
// still handles repeated end-to-end failures by moving the message to the
// DLQ after maxReceiveCount attempts on the receive side.
func (sp *SQSPublisher) Publish(ctx context.Context, message string) error {
	input := &sqs.SendMessageInput{
		QueueUrl:    aws.String(sp.queueURL),
		MessageBody: aws.String(message),
	}

	var messageID string
	err := retry.Do(ctx, sp.retry, func() error {
		result, err := sp.client.SendMessage(ctx, input)
		if err != nil {
			sp.logger.Warn("send message attempt failed", "error", err)
			return fmt.Errorf("send message failed: %w", err)
		}
		if result.MessageId != nil {
			messageID = *result.MessageId
		}
		return nil
	})
	if err != nil {
		return err
	}

	sp.logger.Debug("message published", "message_id", messageID)
	return nil
}
