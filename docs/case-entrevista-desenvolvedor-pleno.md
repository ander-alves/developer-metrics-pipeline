# Case Técnico — Desenvolvedor Pleno (Golang)
## Time de AI Coding Tools

---

## Instruções para o Candidato

Você deve implementar o sistema descrito abaixo e entregar:

1. **Repositório Git** com o código-fonte (GitHub público ou privado com acesso concedido)
2. **Vídeo de até 10 minutos** explicando:
   - Suas decisões de arquitetura
   - Como rodar o projeto
   - Demonstração do sistema funcionando (via Docker)
   - O que você faria diferente com mais tempo

**Prazo: 5 dias corridos** a partir do recebimento.

---

## O Desafio: Developer Metrics Pipeline

### Contexto

Você faz parte de um time que coleta métricas de produtividade de desenvolvedores (commits, pull requests, tempo de review). Os eventos chegam em uma fila, são processados por um serviço, e o resultado é publicado em uma segunda fila para outro serviço consumir e agregar.

Seu desafio é construir **2 serviços em Go** que se comunicam via filas SQS (LocalStack):

```
[SQS: raw-events] → [Serviço 1: Processor] → [SQS: processed-events] → [Serviço 2: Aggregator] → [DynamoDB]
                                                                                                        ↓
                                                                                               [API REST de consulta]
```

**Tudo deve rodar com um único comando: `docker-compose up`**

---

### Arquitetura

| Componente | Descrição |
|------------|-----------|
| **LocalStack** | Container com SQS + DynamoDB |
| **Processor** (container 1) | Consome da fila `raw-events`, valida, enriquece e publica na fila `processed-events` |
| **Aggregator** (container 2) | Consome da fila `processed-events`, agrega métricas e persiste no DynamoDB. Expõe a API REST |

### Filas SQS (LocalStack)

| Fila | Papel |
|------|-------|
| `raw-events` | Recebe eventos brutos de métricas |
| `raw-events-dlq` | Dead Letter Queue da fila raw-events |
| `processed-events` | Recebe eventos validados e enriquecidos pelo Processor |
| `processed-events-dlq` | Dead Letter Queue da fila processed-events |

---

### Estrutura dos Eventos

#### Mensagem na fila `raw-events` (entrada)

```json
{
  "event_id": "uuid-v4",
  "developer_id": "dev-123",
  "metric_type": "commits | pull_requests | review_time_minutes",
  "value": 15,
  "repository": "org/repo-name",
  "timestamp": "2026-04-15T10:30:00Z"
}
```

#### Mensagem na fila `processed-events` (saída do Processor → entrada do Aggregator)

```json
{
  "event_id": "uuid-v4",
  "developer_id": "dev-123",
  "metric_type": "commits",
  "value": 15,
  "repository": "org/repo-name",
  "timestamp": "2026-04-15T10:30:00Z",
  "processed_at": "2026-04-15T10:30:05Z",
  "processor_id": "processor-instance-1"
}
```


---

### Requisitos Funcionais

#### Serviço 1: Processor

- Consumir mensagens da fila `raw-events` (SQS via LocalStack)
- Processar de forma concorrente (worker pool com número configurável de workers)
- Validar os eventos (regras abaixo)
- Eventos válidos: enriquecer com `processed_at` e `processor_id`, publicar na fila `processed-events`
- Eventos inválidos: rejeitar (mensagem vai para `raw-events-dlq` após 3 tentativas)
- Implementar retry com backoff exponencial

**Validações:**
- `event_id` — obrigatório, formato UUID válido
- `developer_id` — obrigatório, não pode ser vazio
- `metric_type` — deve ser um dos valores: `commits`, `pull_requests`, `review_time_minutes`
- `value` — deve ser >= 0; para `review_time_minutes` o máximo é 1440 (24h)
- `timestamp` — obrigatório, não pode ser uma data futura

#### Serviço 2: Aggregator

- Consumir mensagens da fila `processed-events` (SQS via LocalStack)
- Garantir idempotência: se o mesmo `event_id` chegar duas vezes, não duplicar
- Persistir no DynamoDB (LocalStack):
  - Tabela `events`: registro individual de cada evento processado
  - Tabela `developer_summary`: métricas agregadas por desenvolvedor
- Atualizar os totais do desenvolvedor a cada evento recebido (aggregation incremental)
- Expor API REST para consulta

**API REST (exposta pelo Aggregator):**

- `GET /metrics/:developer_id` — retorna todos os eventos de um desenvolvedor
- `GET /metrics/:developer_id/summary` — retorna resumo agregado:
  ```json
  {
    "developer_id": "dev-123",
    "total_commits": 142,
    "total_pull_requests": 38,
    "avg_review_time_minutes": 45.2,
    "events_processed": 195,
    "last_activity": "2026-04-15T10:30:00Z"
  }
  ```

- `GET /health` — health check (verifica conexão com SQS e DynamoDB)

---

### Requisitos Não-Funcionais

| Requisito | Expectativa |
|-----------|-------------|
| **Docker** | `docker-compose up` sobe tudo (LocalStack, Processor, Aggregator) |
| **LocalStack** | SQS e DynamoDB — filas e tabelas criadas automaticamente no startup |
| **Configuração** | Variáveis de ambiente (endpoints, nomes de filas, workers, etc.) |
| **Logs** | Estruturados (JSON), com correlation por event_id |
| **Graceful Shutdown** | Ambos os serviços devem encerrar de forma limpa (drenar workers, fechar conexões) |
| **Testes** | Testes unitários para validação e lógica de processamento |
| **README** | Instruções claras de como rodar, testar e usar a API |

---

### Estrutura sugerida (não obrigatória)

Esperamos ver **Clean Architecture** — separação clara entre domínio, casos de uso e infraestrutura:

```
/
├── services/
│   ├── processor/
│   │   ├── cmd/
│   │   │   └── main.go
│   │   ├── internal/
│   │   │   ├── domain/          # entities, value objects, regras de negócio
│   │   │   ├── usecase/         # orquestração: validar, enriquecer, publicar
│   │   │   └── infra/
│   │   │       ├── queue/       # adapter SQS (consumer + publisher)
│   │   │       ├── worker/      # worker pool
│   │   │       └── config/
│   │   ├── Dockerfile
│   │   └── go.mod
│   └── aggregator/
│       ├── cmd/
│       │   └── main.go
│       ├── internal/
│       │   ├── domain/          # entities, regras de agregação
│       │   ├── usecase/         # orquestração: consumir, agregar, persistir
│       │   └── infra/
│       │       ├── queue/       # adapter SQS (consumer)
│       │       ├── repository/  # adapter DynamoDB
│       │       ├── api/         # adapter HTTP (handlers)
│       │       └── config/
│       ├── Dockerfile
│       └── go.mod
├── infra/
│   └── localstack/
│       └── init-aws.sh       # cria filas e tabelas no startup
├── scripts/
│   └── seed.sh               # publica mensagens de teste na fila raw-events
├── docker-compose.yml
└── README.md
```

---

### Inicialização do LocalStack

O arquivo `init-aws.sh` deve criar automaticamente:

```bash
# Filas SQS
aws --endpoint-url=http://localhost:4566 sqs create-queue --queue-name raw-events-dlq
aws --endpoint-url=http://localhost:4566 sqs create-queue --queue-name raw-events \
  --attributes '{"RedrivePolicy": "{\"deadLetterTargetArn\":\"arn:aws:sqs:us-east-1:000000000000:raw-events-dlq\",\"maxReceiveCount\":\"3\"}"}'

aws --endpoint-url=http://localhost:4566 sqs create-queue --queue-name processed-events-dlq
aws --endpoint-url=http://localhost:4566 sqs create-queue --queue-name processed-events \
  --attributes '{"RedrivePolicy": "{\"deadLetterTargetArn\":\"arn:aws:sqs:us-east-1:000000000000:processed-events-dlq\",\"maxReceiveCount\":\"3\"}"}'

# Tabelas DynamoDB
aws --endpoint-url=http://localhost:4566 dynamodb create-table \
  --table-name events \
  --attribute-definitions AttributeName=event_id,AttributeType=S \
  --key-schema AttributeName=event_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST

aws --endpoint-url=http://localhost:4566 dynamodb create-table \
  --table-name developer_summary \
  --attribute-definitions AttributeName=developer_id,AttributeType=S \
  --key-schema AttributeName=developer_id,KeyType=HASH \
  --billing-mode PAY_PER_REQUEST
```

---

### Seed de Dados

Inclua um script (`scripts/seed.sh`) que publique pelo menos **20 mensagens** na fila `raw-events` via AWS CLI apontando para o LocalStack, incluindo:
- Mensagens válidas (vários developers e metric types)
- 2-3 mensagens inválidas (para mostrar validação e DLQ funcionando)
- 1-2 mensagens duplicadas (para mostrar idempotência no Aggregator)

---

## Critérios de Avaliação

### Essenciais (vai/não vai)
- [ ] `docker-compose up` funciona sem intervenção manual
- [ ] Dois containers de serviço rodando (Processor + Aggregator)
- [ ] Comunicação entre serviços exclusivamente via fila SQS
- [ ] Processor valida e publica na segunda fila
- [ ] Aggregator consome, persiste no DynamoDB e expõe API
- [ ] DLQ funcionando para mensagens com falha
- [ ] Código compila e testes passam

### Qualidade de Código (peso alto)
| Critério | O que avaliamos |
|----------|-----------------|
| Clean Architecture | Separação clara de camadas (domain, usecase, infra/adapter) em cada serviço |
| Go idiomático | Interfaces, error handling, nomenclatura, packages |
| Concorrência | Worker pool, context, graceful shutdown |
| Tratamento de erros | Erros tipados, sem panic, retry com lógica clara |
| Testabilidade | Interfaces que permitem mock, testes unitários reais |
| Comunicação entre serviços | Contrato claro entre Processor e Aggregator via mensagens |

### Diferenciais (destacam o candidato)
- [ ] Dockerfile multi-stage com imagem final mínima (scratch/distroless)
- [ ] Tracing distribuído entre os dois serviços (OpenTelemetry)
- [ ] Makefile com comandos úteis (build, test, lint, run)
- [ ] Documentação da API (Swagger/OpenAPI)

---

## Sobre o Vídeo

O vídeo é tão importante quanto o código. Queremos entender:

1. **Como você pensa** — por que dois serviços? como eles conversam? por que essa separação?
2. **Como você comunica** — consegue explicar decisões técnicas de forma clara?
3. **Autocrítica** — o que você faria diferente com mais tempo? o que ficou ruim?
4. **Demonstração real** — mostre o fluxo completo: mensagem entrando na fila → Processor → segunda fila → Aggregator → DynamoDB → API respondendo

**Dica:** não precisa ser um vídeo editado ou produzido. Pode ser gravação de tela com narração simples. Valorizamos clareza, não produção.

---

## O que NÃO avaliamos

- Framework HTTP específico (pode usar chi, gin, echo, net/http puro)
- Perfeição visual — não precisa de frontend
- Quantidade de código — preferimos menos código bem escrito do que muito código medíocre
- Versão específica do Go (use 1.21+)

---

## Dúvidas?

Se tiver dúvidas sobre o escopo, tome uma decisão e documente no README. Parte da avaliação é ver como você lida com ambiguidade.

Boa sorte!
