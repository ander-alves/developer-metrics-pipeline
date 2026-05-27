// @title        Developer Metrics API
// @version      1.0
// @description  Aggregated developer productivity metrics — commits, pull requests, review time.
// @host         localhost:8080
// @BasePath     /

package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "aggregator/docs"
	"aggregator/internal/infra/api"
	aggregatorConfig "aggregator/internal/infra/config"
	"aggregator/internal/infra/logging"
	"aggregator/internal/infra/queue"
	"aggregator/internal/infra/repository"
	"aggregator/internal/usecase"
)

func main() {
	logger := logging.New("aggregator")
	logger.Info("iniciando serviço aggregator")

	cfg, err := aggregatorConfig.LoadConfig()
	if err != nil {
		logger.Error("falha ao carregar configuração", "error", err)
		os.Exit(1)
	}

	logger.Info("configuração carregada",
		"events_table", cfg.DynamoDBEventsTable,
		"summary_table", cfg.DynamoDBSummaryTable,
		"processed_events_queue", cfg.SQSProcessedEventsQueue,
	)

	awsConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("não foi possível carregar configuração do AWS SDK", "error", err)
		os.Exit(1)
	}

	if cfg.AWSEndpointURL != "" {
		logger.Info("usando endpoint AWS sobrescrito", "endpoint", cfg.AWSEndpointURL)
	}

	sqsClient := sqs.NewFromConfig(awsConfig, func(o *sqs.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
		}
	})
	dynamoClient := dynamodb.NewFromConfig(awsConfig, func(o *dynamodb.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
		}
	})

	eventRepo := repository.NewDynamoDBEventRepository(dynamoClient, cfg.DynamoDBEventsTable, logger)
	summaryRepo := repository.NewDynamoDBSummaryRepository(dynamoClient, cfg.DynamoDBSummaryTable, logger)
	consumer := queue.NewSQSConsumer(sqsClient, cfg.SQSProcessedEventsQueue, logger)
	aggregateUseCase := usecase.NewAggregateMetricsUseCase(eventRepo, summaryRepo, logger)
	handlers := api.NewAPIHandlers(eventRepo, summaryRepo, logger)

	router := chi.NewRouter()
	handlers.RegisterRoutes(router)
	router.Get("/swagger/*", httpSwagger.WrapHandler)

	httpServer := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("servidor HTTP escutando", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("erro no servidor", "error", err)
			os.Exit(1)
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("aggregator iniciado, consumindo eventos processados")

	var consumerWg sync.WaitGroup
	consumerWg.Add(1)

	go func() {
		defer consumerWg.Done()
		pollTicker := time.NewTicker(5 * time.Second)
		defer pollTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("consumer loop shutting down")
				return
			case <-pollTicker.C:
				messages, err := consumer.ReceiveMessages(ctx)
				if err != nil {
					logger.Error("erro ao receber mensagens", "error", err)
					continue
				}
				if len(messages) == 0 {
					continue
				}
				logger.Info("mensagens recebidas", "count", len(messages))

				for _, msg := range messages {
					if err := aggregateUseCase.AggregateEvent(ctx, msg.Body); err != nil {
						logger.Warn("agregação falhou, será reenviada via SQS", "error", err)
						continue
					}
					if err := consumer.DeleteMessage(ctx, msg.ReceiptHandle); err != nil {
						logger.Error("erro ao deletar mensagem", "error", err)
					} else {
						logger.Debug("mensagem deletada")
					}
				}
			case <-sigChan:
				logger.Info("sinal de desligamento recebido")
				cancel()
			}
		}
	}()

	<-ctx.Done()

	logger.Info("aguardando consumidor finalizar")
	consumerWg.Wait()

	logger.Info("encerrando servidor HTTP")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("erro ao encerrar servidor", "error", err)
	}

	logger.Info("aggregator parado")
}
