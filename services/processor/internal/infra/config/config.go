package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	AWSEndpointURL       string
	AWSRegion            string
	SQSRawEventsQueue    string
	SQSProcessedEvents   string
	WorkerCount          int
	LogLevel             string
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		AWSEndpointURL:     getEnv("AWS_ENDPOINT_URL", "http://localhost:4566"),
		AWSRegion:          getEnv("AWS_REGION", "us-east-1"),
		SQSRawEventsQueue:  getEnvRequired("SQS_RAW_EVENTS_QUEUE"),
		SQSProcessedEvents: getEnvRequired("SQS_PROCESSED_EVENTS_QUEUE"),
		WorkerCount:        getEnvInt("WORKER_COUNT", 4),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) validate() error {
	if c.SQSRawEventsQueue == "" {
		return fmt.Errorf("SQS_RAW_EVENTS_QUEUE is required")
	}
	if c.SQSProcessedEvents == "" {
		return fmt.Errorf("SQS_PROCESSED_EVENTS_QUEUE is required")
	}
	if c.WorkerCount <= 0 {
		return fmt.Errorf("WORKER_COUNT must be > 0")
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

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}
