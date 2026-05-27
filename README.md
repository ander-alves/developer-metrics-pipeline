# Developer Metrics Pipeline

Pipeline event-driven em Go que valida, processa e agrega métricas de
produtividade de desenvolvedores (commits, pull requests, tempo de review).
Implementação do case técnico — ver
[`docs/case-entrevista-desenvolvedor-pleno.md`](./docs/case-entrevista-desenvolvedor-pleno.md).

## Arquitetura

```
raw-events ─►  Processor  ─►  processed-events  ─►  Aggregator  ─►  DynamoDB
   (SQS)       (valida +        (SQS)              (deduplica +         ↓
              enriquece)                            agrega)         REST API
     ↓                              ↓
 raw-events-dlq           processed-events-dlq
```

- **Processor** (porta 8081) — consome `raw-events`, valida 7 regras (UUID,
  developer_id, metric_type, value, review_time ≤ 1440, timestamp não-futuro),
  enriquece com `processed_at` e `processor_id`, publica em `processed-events`.
  Pool de workers (configurável via `WORKER_COUNT`, default 4).
- **Aggregator** (porta 8080) — consome `processed-events`, descarta
  duplicatas pelo `event_id` (idempotência), persiste no DynamoDB e expõe a
  API REST.
- **LocalStack** — emula SQS e DynamoDB. Filas e tabelas são criadas no
  boot pelo `init-aws.sh`.

## Quick start

Pré-requisitos: **Docker** (Desktop, Colima, OrbStack ou Rancher) e **Go 1.24+**.

```bash
go run ./tools/dev up      # sobe LocalStack + processor + aggregator e espera ficar healthy
go run ./tools/dev seed    # publica 21 eventos de teste (14 válidos, 5 inválidos, 2 duplicados)
go run ./tools/dev health  # confere /health de todos os serviços

curl http://localhost:8080/metrics/dev-001/summary
```

Resposta esperada:

```json
{
  "developer_id": "dev-001",
  "total_commits": 16,
  "total_pull_requests": 3,
  "total_review_time_minutes": 75,
  "review_time_events_count": 2,
  "avg_review_time_minutes": 37.5,
  "events_processed": 7,
  "last_activity": "...",
  "updated_at": "..."
}
```

### Caminhos alternativos

| Cenário | Comandos |
|---|---|
| **Só Docker** (sem Go no host) | `docker-compose up -d` → `docker-compose run seed` |
| Tudo (Windows/macOS/Linux) | `go run ./tools/dev up` → `go run ./tools/dev seed` |
| macOS/Linux com make | `make up` → `make seed` |

**Só Docker** — o serviço `seed` usa a imagem `amazon/aws-cli` e a fila interna:
```bash
docker-compose up -d
docker-compose run seed
curl http://localhost:8080/metrics/dev-001/summary
```

`make` é só um wrapper para o runner Go — não é necessário no Windows. Veja
[`tools/dev/README.md`](./tools/dev/README.md) para todos os subcomandos.

### Graceful shutdown

- Os serviços implementam shutdown gracioso: recebem `SIGTERM`/`SIGINT`,
  cancelam loops de polling e esperam o worker pool/drain antes de sair.
- Para garantir que o `processor` tenha tempo suficiente para drenar, o
  `docker-compose.yml` define `stop_grace_period: 35s` no serviço `processor`
  (e `stop_grace_period: 10s` no `aggregator`). Isso faz o Docker aguardar
  antes de forçar um kill quando você executa `docker compose down`.
- Se preferir preservar volumes e parar rapidamente, use `go run ./tools/dev stop`
  ou `docker compose stop` em vez de `down`.

Exemplos:

```bash
# parar preservando volumes e dando chance ao drain
go run ./tools/dev stop

# forçar remoção (envia SIGTERM e depois aguarda stop_grace_period)
docker compose down -v
```

## API

| Método | Rota | Descrição |
|---|---|---|
| GET | `/health` (8080, 8081) | Liveness probe |
| GET | `/metrics/{developer_id}` | Todos os eventos do desenvolvedor |
| GET | `/metrics/{developer_id}/summary` | Sumário agregado |
| GET | `/swagger/` (8080) | Swagger UI interativo |

## Estrutura

```
services/
├── processor/                          # Validação + enriquecimento
│   ├── cmd/main.go
│   └── internal/
│       ├── domain/                     # Entidades, regras de validação
│       ├── usecase/                    # ProcessEvent
│       └── infra/
│           ├── queue/                  # SQS consumer + publisher (com retry)
│           ├── worker/                 # Worker pool concorrente
│           ├── logging/                # slog JSON
│           ├── retry/                  # Backoff exponencial
│           └── config/                 # Env vars
└── aggregator/                         # Idempotência + agregação + API
    ├── cmd/main.go
    └── internal/
        ├── domain/                     # ProcessedEvent, DeveloperSummary
        ├── usecase/                    # AggregateMetrics (+ testes)
        └── infra/
            ├── queue/                  # SQS consumer
            ├── repository/             # DynamoDB (com retry)
            ├── api/                    # Handlers chi
            ├── logging/                # slog JSON
            ├── retry/                  # Backoff exponencial
            └── config/

infra/
├── localstack/init-aws.sh              # Bootstrap das filas e tabelas
└── dynamodb/
    ├── tables/                         # Schemas das tabelas (HASH simples, sem GSI)
    └── migrations/001_create_tables.sh # Cria eventos + developer_summary

tools/dev/                              # Runner Go cross-platform
                                        # (substitui make/bash/awscli)
```

## Decisões técnicas

- **Clean Architecture** com três camadas (`domain` / `usecase` / `infra`)
  em ambos os serviços — adapters atrás de interfaces para mockar.
- **Comunicação só via SQS**, nunca chamada direta — desacopla escalas,
  habilita DLQ nativa e retry sem código adicional.
- **Idempotência** pelo `event_id` como PK na tabela `events`: a checagem
  acontece *antes* do update do summary.
- **DLQ** com `maxReceiveCount=3` nas duas filas principais.
- **Worker pool** com long-polling (20s) reduz overhead.
- **Graceful shutdown** via `context.Context` + `WaitGroup`: SIGTERM drena
  a mensagem em voo e fecha o HTTP server.
- **Backoff exponencial** explícito em `internal/infra/retry/` é aplicado ao
  publish no SQS e ao write no DynamoDB. Roda *antes* do SQS reentregar,
  evitando consumir tentativas do `maxReceiveCount` em erros transitórios.
- **Logs estruturados JSON** via `log/slog` da stdlib — cada linha tem
  `service`, `event_id`, `developer_id`, level e msg, facilitando correlation.
- **Imagem distroless** (`gcr.io/distroless/base-debian11:nonroot`) — sem
  shell, ~20MB, roda como não-root. Builder em `golang:1.24-alpine`.
- **Runner Go cross-platform** (`tools/dev/`) — `docker-compose up` é
  mantido (exigência do case), mas o runner é o caminho recomendado
  porque também funciona no Windows sem WSL.

## Testes

```bash
go run ./tools/dev test
```

| Pacote | Testes | Cobre |
|---|---|---|
| `processor/internal/domain` | 13 testes | 5 regras de validação (12 sub-testes, inclui tolerância de skew) + enriquecimento ProcessedEvent |
| `processor/internal/usecase` | 3 testes | Evento válido publicado, inválido rejeitado, falha no publisher |
| `processor/internal/infra/retry` | 5 testes | Backoff, ctx cancel, attempts |
| `aggregator/internal/usecase` | 6 testes | Happy path, idempotência, falha persistência, JSON inválido, múltiplos metric types, avg correto por evento de review |
| `aggregator/internal/infra/retry` | 5 testes | Idem retry |

## Variáveis de ambiente

Todas configuradas inline em [`docker-compose.yml`](./docker-compose.yml).
Os defaults apontam pro LocalStack local — basta sobrescrever no compose
pra mirar em AWS real.

