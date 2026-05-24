.PHONY: help up down restart build logs logs-processor logs-aggregator logs-localstack seed health test clean

# Convenience wrappers around the cross-platform Go runner.
# Everything here just delegates to "go run ./tools/dev", so behaviour is
# identical whether you use make, run the runner directly, or invoke it from
# PowerShell on Windows.
DEV := go run ./tools/dev

help:
	@echo "Developer Metrics Pipeline"
	@echo ""
	@echo "Make targets (delegate to the Go runner at tools/dev):"
	@echo "  make up                - Start the full stack and wait until healthy"
	@echo "  make down              - Stop and remove containers + volumes"
	@echo "  make restart           - down + up"
	@echo "  make build             - Build the service images"
	@echo "  make logs              - Tail logs of all services"
	@echo "  make logs-processor    - Tail logs of the processor service"
	@echo "  make logs-aggregator   - Tail logs of the aggregator service"
	@echo "  make logs-localstack   - Tail logs of LocalStack"
	@echo "  make seed              - Publish sample events into raw-events"
	@echo "  make health            - Check health endpoints"
	@echo "  make test              - Run go test ./... in each service"
	@echo "  make clean             - down + delete built Go binaries"
	@echo ""
	@echo "Cross-platform equivalent (works on Windows without make/bash):"
	@echo "  go run ./tools/dev <command>"

up:
	$(DEV) up

down:
	$(DEV) down

restart:
	$(DEV) restart

build:
	$(DEV) build

logs:
	$(DEV) logs

logs-processor:
	$(DEV) logs processor

logs-aggregator:
	$(DEV) logs aggregator

logs-localstack:
	$(DEV) logs localstack

seed:
	$(DEV) seed

health:
	$(DEV) health

test:
	$(DEV) test

clean:
	$(DEV) clean