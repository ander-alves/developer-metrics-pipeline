package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"processor/internal/domain"
	processorConfig "processor/internal/infra/config"
	"processor/internal/infra/logging"
	"processor/internal/infra/queue"
	"processor/internal/infra/worker"
	"processor/internal/usecase"
)

func main() {
	logger := logging.New("processor")
	logger.Info("iniciando serviço processor")

	cfg, err := processorConfig.LoadConfig()
	if err != nil {
		logger.Error("falha ao carregar configuração", "error", err)
		os.Exit(1)
	}

	logger.Info("configuração carregada",
		"workers", cfg.WorkerCount,
		"region", cfg.AWSRegion,
		"raw_events_queue", cfg.SQSRawEventsQueue,
		"processed_events_queue", cfg.SQSProcessedEvents,
	)

	awsConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.AWSRegion),
	)
	if err != nil {
		logger.Error("não foi possível carregar configuração do AWS SDK", "error", err)
		os.Exit(1)
	}

	// EndpointResolverV2 in current SDK versions only honors the per-service
	// Options.BaseEndpoint — neither AWS_ENDPOINT_URL env nor a BaseEndpoint
	// set on the shared aws.Config reaches it reliably. We set it per-client
	// below when a custom endpoint (LocalStack) is configured.
	if cfg.AWSEndpointURL != "" {
		logger.Info("usando endpoint AWS sobrescrito", "endpoint", cfg.AWSEndpointURL)
	}

	sqsClient := sqs.NewFromConfig(awsConfig, func(o *sqs.Options) {
		if cfg.AWSEndpointURL != "" {
			o.BaseEndpoint = aws.String(cfg.AWSEndpointURL)
		}
	})

	consumer := queue.NewSQSConsumer(sqsClient, cfg.SQSRawEventsQueue, logger)
	publisher := queue.NewSQSPublisher(sqsClient, cfg.SQSProcessedEvents, logger)
	validator := &domain.DefaultValidator{}
	processor := usecase.NewEventProcessor(validator, publisher, logger)

	handler := func(ctx context.Context, body string) error {
		result := processor.ProcessEvent(ctx, usecase.ProcessEventInput{SQSBody: body})
		return result.Error
	}

	pool := worker.NewWorkerPool(cfg.WorkerCount, handler, logger)

	go startHealthServer(logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := pool.Start(ctx); err != nil {
		logger.Error("falha ao iniciar pool de workers", "error", err)
		os.Exit(1)
	}

	logger.Info("pool de workers iniciado, consumindo mensagens da fila raw-events")

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
					job := worker.Job{
						ID:   msg.ReceiptHandle,
						Body: msg.Body,
						Done: make(chan error, 1),
					}

					if err := pool.Submit(ctx, job); err != nil {
						logger.Error("erro ao submeter job", "error", err)
						continue
					}

					go func(m domain.Message, j worker.Job) {
						select {
						case jobErr := <-j.Done:
							if jobErr != nil {
								// Não deletar — SQS reentregará até maxReceiveCount=3,
								// depois disso a mensagem vai para a DLQ.
								logger.Warn("job falhou, será reenviado via SQS", "error", jobErr)
							} else {
								if err := consumer.DeleteMessage(ctx, m.ReceiptHandle); err != nil {
									logger.Error("erro ao deletar mensagem", "error", err)
								} else {
									logger.Debug("mensagem deletada")
								}
							}
						case <-ctx.Done():
							logger.Warn("contexto cancelado antes do término do job")
						}
					}(msg, job)
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

	logger.Info("esvaziando pool de workers", "timeout_seconds", 30)
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer drainCancel()

	if err := pool.Drain(drainCtx); err != nil {
		logger.Error("erro ao esvaziar pool", "error", err)
	}

	logger.Info("processor parado")
}

func startHealthServer(logger *slog.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"service": "processor",
			"time":    time.Now().UTC(),
		})
	})

	logger.Info("servidor de health check escutando", "addr", ":8081")
	server := &http.Server{Addr: ":8081", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("erro no servidor de health", "error", err)
	}
}
