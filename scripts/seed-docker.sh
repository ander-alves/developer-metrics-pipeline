#!/bin/bash
# Seed script para rodar dentro da rede Docker (sem Go ou AWS CLI no host).
# Usa a imagem amazon/aws-cli — UUID gerado via kernel (/proc/sys/kernel/random/uuid).
set -e

ENDPOINT="http://localstack:4566"
REGION="us-east-1"
QUEUE_URL="$ENDPOINT/000000000000/raw-events"
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

uuid() { cat /proc/sys/kernel/random/uuid; }

send() {
  aws --endpoint-url "$ENDPOINT" --region "$REGION" \
    sqs send-message --queue-url "$QUEUE_URL" --message-body "$1" > /dev/null
}

echo "Seeding raw-events (14 válidos, 5 inválidos, 2 duplicados)..."

# --- Válidos ---
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"commits\",\"value\":5,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"commits\",\"value\":8,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"commits\",\"value\":3,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"pull_requests\",\"value\":2,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"pull_requests\",\"value\":1,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"review_time_minutes\",\"value\":45,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-001\",\"metric_type\":\"review_time_minutes\",\"value\":30,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-002\",\"metric_type\":\"commits\",\"value\":12,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-002\",\"metric_type\":\"commits\",\"value\":7,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-002\",\"metric_type\":\"pull_requests\",\"value\":3,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-002\",\"metric_type\":\"review_time_minutes\",\"value\":60,\"repository\":\"org/repo-1\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-003\",\"metric_type\":\"commits\",\"value\":4,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-003\",\"metric_type\":\"pull_requests\",\"value\":2,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-003\",\"metric_type\":\"review_time_minutes\",\"value\":25,\"repository\":\"org/repo-2\",\"timestamp\":\"$TIMESTAMP\"}"

# --- Inválidos (DLQ) ---
send "{\"event_id\":\"invalid-uuid\",\"developer_id\":\"dev-004\",\"metric_type\":\"commits\",\"value\":5,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"\",\"metric_type\":\"commits\",\"value\":5,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-004\",\"metric_type\":\"invalid_metric\",\"value\":5,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-004\",\"metric_type\":\"commits\",\"value\":-5,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$(uuid)\",\"developer_id\":\"dev-004\",\"metric_type\":\"review_time_minutes\",\"value\":2000,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"

# --- Duplicados (idempotência) ---
DUP_ID=$(uuid)
send "{\"event_id\":\"$DUP_ID\",\"developer_id\":\"dev-dup\",\"metric_type\":\"commits\",\"value\":10,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"
send "{\"event_id\":\"$DUP_ID\",\"developer_id\":\"dev-dup\",\"metric_type\":\"commits\",\"value\":10,\"repository\":\"org/repo\",\"timestamp\":\"$TIMESTAMP\"}"

echo "Seed concluído. Aguarde ~10s e consulte:"
echo "  curl http://localhost:8080/metrics/dev-001/summary"
