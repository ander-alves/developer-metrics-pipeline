package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type Job struct {
	ID   string
	Body string
	Done chan error
}

type WorkerPool struct {
	jobs        chan Job
	workerCount int
	wg          sync.WaitGroup
	logger      *slog.Logger
	handler     func(ctx context.Context, body string) error
	timeout     time.Duration
}

func NewWorkerPool(
	workerCount int,
	handler func(ctx context.Context, body string) error,
	logger *slog.Logger,
) *WorkerPool {
	return &WorkerPool{
		jobs:        make(chan Job, workerCount*2),
		workerCount: workerCount,
		handler:     handler,
		timeout:     30 * time.Second,
		logger:      logger,
	}
}

func (wp *WorkerPool) Start(ctx context.Context) error {
	if wp.workerCount <= 0 {
		return fmt.Errorf("worker count must be > 0")
	}

	wp.logger.Info("iniciando pool de workers", "workers", wp.workerCount)

	for i := 0; i < wp.workerCount; i++ {
		wp.wg.Add(1)
		go wp.worker(ctx, i)
	}

	return nil
}

func (wp *WorkerPool) worker(ctx context.Context, id int) {
	defer wp.wg.Done()

	wp.logger.Info("worker iniciado", "worker_id", id)

	for {
		select {
		case <-ctx.Done():
			wp.logger.Info("worker encerrando", "worker_id", id)
			return
		case job, ok := <-wp.jobs:
			if !ok {
				wp.logger.Info("canal de jobs fechado", "worker_id", id)
				return
			}

			jobCtx, cancel := context.WithTimeout(ctx, wp.timeout)
			err := wp.handler(jobCtx, job.Body)
			cancel()

			select {
			case job.Done <- err:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (wp *WorkerPool) Submit(ctx context.Context, job Job) error {
	select {
	case wp.jobs <- job:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (wp *WorkerPool) Drain(ctx context.Context) error {
	close(wp.jobs)

	done := make(chan error, 1)
	go func() {
		wp.wg.Wait()
		done <- nil
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}
