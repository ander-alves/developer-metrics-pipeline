#!/bin/bash
set -e

echo "=========================================="
echo "Inicializando LocalStack"
echo "=========================================="

# Cores
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Obter diretório raiz do projeto
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/../.." && pwd )"
INFRA_DIR="$PROJECT_ROOT/infra"

echo ""
echo "Projeto root: $PROJECT_ROOT"
echo "Infra dir: $INFRA_DIR"

# Aguardar LocalStack estar pronto
echo ""
echo "Aguardando LocalStack..."
for i in {1..30}; do
  if awslocal sqs list-queues > /dev/null 2>&1; then
    echo -e "${GREEN}LocalStack pronto${NC}"
    break
  fi
  echo "  Tentativa $i..."
  sleep 1
done

# ============ SQS ============
echo ""
echo "========== SQS =========="
echo "Criando Dead Letter Queues..."

awslocal sqs create-queue \
  --queue-name raw-events-dlq \
  --region us-east-1 \
  2>/dev/null || echo "raw-events-dlq já existe"

awslocal sqs create-queue \
  --queue-name processed-events-dlq \
  --region us-east-1 \
  2>/dev/null || echo "processed-events-dlq já existe"

echo ""
echo "Criando filas SQS..."

  2>/dev/null || echo "raw-events já existe"
awslocal sqs create-queue --queue-name raw-events --region us-east-1 2>/dev/null || echo "raw-events já existe"

awslocal sqs create-queue --queue-name processed-events --region us-east-1 2>/dev/null || echo "processed-events já existe"

# Attach DLQ redrive policies using ARNs resolved via awslocal. This uses
# AWS CLI's --query/--output text to avoid depending on jq and fragile
# nested quoting.
set_redrive() {
  local queue_name="$1"
  local dlq_name="$2"
  local queue_url
  local dlq_url
  queue_url=$(awslocal sqs get-queue-url --queue-name "$queue_name" --region us-east-1 --output text 2>/dev/null || true)
  dlq_url=$(awslocal sqs get-queue-url --queue-name "$dlq_name" --region us-east-1 --output text 2>/dev/null || true)
  if [ -z "$queue_url" ] || [ -z "$dlq_url" ]; then
    echo "could not resolve URLs for $queue_name or $dlq_name — skipping redrive policy"
    return
  fi
  dlq_arn=$(awslocal sqs get-queue-attributes --queue-url "$dlq_url" --attribute-names QueueArn --region us-east-1 --query 'Attributes.QueueArn' --output text 2>/dev/null || true)
  if [ -z "$dlq_arn" ]; then
    echo "could not resolve ARN for $dlq_name — skipping redrive policy"
    return
  fi
  redrive_json=$(printf '{"deadLetterTargetArn":"%s","maxReceiveCount":"3"}' "$dlq_arn")
  awslocal sqs set-queue-attributes --queue-url "$queue_url" --attributes "RedrivePolicy=$redrive_json" --region us-east-1 2>/dev/null || echo "failed to set redrive policy for $queue_name"
}

set_redrive raw-events raw-events-dlq
set_redrive processed-events processed-events-dlq
  2>/dev/null || echo "processed-events já existe"

echo -e "${GREEN}SQS configurado${NC}"

# ============ DynamoDB ============
echo ""
echo "========== DynamoDB =========="
echo "Executando migrations..."

# Migration 001: Criar tabelas
echo ""
echo "Migration 001: Create Tables"
if [ -f "$INFRA_DIR/dynamodb/migrations/001_create_tables.sh" ]; then
  bash "$INFRA_DIR/dynamodb/migrations/001_create_tables.sh"
else
  echo "Arquivo não encontrado: $INFRA_DIR/dynamodb/migrations/001_create_tables.sh"
  exit 1
fi

echo -e "${GREEN}DynamoDB configurado${NC}"

# ============ RESUMO ============
echo ""
echo "=========================================="
echo -e "${GREEN}LocalStack inicializado com sucesso!${NC}"
echo "=========================================="
echo ""

echo "Filas SQS:"
awslocal sqs list-queues --region us-east-1 | jq -r '.QueueUrls[]' | sed 's/^/  - /'

echo ""
echo "Tabelas DynamoDB:"
awslocal dynamodb list-tables --region us-east-1 | jq -r '.TableNames[]' | sed 's/^/  - /'

echo ""
echo "Endpoints:"
echo "  - LocalStack: http://localhost:4566"
echo "  - Processor: http://localhost:8081"
echo "  - Aggregator: http://localhost:8080"
echo ""

echo "Próximos passos:"
echo "  1. make seed (popular fila)"
echo "  2. make logs (ver processamento)"
echo "  3. curl http://localhost:8080/metrics/dev-001/summary"
echo ""
