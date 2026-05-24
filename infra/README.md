# Infrastructure

Recursos AWS (emulados via LocalStack) usados pelo pipeline.

```
infra/
├── localstack/
│   └── init-aws.sh                         # Cria filas + roda migrations no boot
└── dynamodb/
    ├── tables/
    │   ├── events.json
    │   └── developer_summary.json
    └── migrations/
        ├── 001_create_tables.sh
        └── 002_add_gsi.sh
```

Tudo é criado automaticamente quando o LocalStack sobe — o `init-aws.sh` é
montado em `/etc/localstack/init/ready.d/` e executado quando o container
fica saudável.

> No Windows o bit de execução do script pode não ser preservado pelo git;
> nesse caso o runner Go (`go run ./tools/dev up`) cria os mesmos recursos
> via HTTP, de forma idempotente.

## SQS

| Fila | Papel | DLQ | maxReceiveCount |
|---|---|---|---|
| `raw-events` | Eventos brutos consumidos pelo Processor | `raw-events-dlq` | 3 |
| `processed-events` | Eventos enriquecidos consumidos pelo Aggregator | `processed-events-dlq` | 3 |
| `raw-events-dlq` | Retém eventos inválidos | — | — |
| `processed-events-dlq` | Retém falhas de agregação | — | — |

Long polling (`WaitTimeSeconds=20`) e visibility timeout de 30s configurados
nos consumers.

## DynamoDB

| Tabela | PK | Conteúdo |
|---|---|---|
| `events` | `event_id` (S) | Um item por evento processado — base da idempotência |
| `developer_summary` | `developer_id` (S) | Totais agregados por desenvolvedor (commits, PRs, review time, contagem) |

Billing: `PAY_PER_REQUEST` (on-demand) em ambas.
