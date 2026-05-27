package config

import (
	"fmt"
	"os"
)

type Config struct {
	AWSEndpointURL          string
	AWSRegion               string
	SQSProcessedEventsQueue string
	DynamoDBEndpoint        string
	DynamoDBEventsTable     string
	DynamoDBSummaryTable    string
	HTTPPort                string
	LogLevel                string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		AWSEndpointURL:          getEnv("AWS_ENDPOINT_URL", "http://localhost:4566"),
		AWSRegion:               getEnv("AWS_REGION", "us-east-1"),
		SQSProcessedEventsQueue: getEnvRequired("SQS_PROCESSED_EVENTS_QUEUE"),
		DynamoDBEndpoint:        getEnv("DYNAMODB_ENDPOINT", "http://localhost:4566"),
		DynamoDBEventsTable:     getEnv("DYNAMODB_EVENTS_TABLE", "events"),
		DynamoDBSummaryTable:    getEnv("DYNAMODB_SUMMARY_TABLE", "developer_summary"),
		HTTPPort:                getEnv("HTTP_PORT", "8080"),
		LogLevel:                getEnv("LOG_LEVEL", "info"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.SQSProcessedEventsQueue == "" {
		return fmt.Errorf("SQS_PROCESSED_EVENTS_QUEUE is required")
	}
	if c.DynamoDBEventsTable == "" {
		return fmt.Errorf("DYNAMODB_EVENTS_TABLE is required")
	}
	if c.DynamoDBSummaryTable == "" {
		return fmt.Errorf("DYNAMODB_SUMMARY_TABLE is required")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvRequired(key string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return ""
}
