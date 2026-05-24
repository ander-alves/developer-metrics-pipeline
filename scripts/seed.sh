#!/bin/bash

set -e

echo "Populando fila raw-events com dados de teste..."

AWS_ENDPOINT="http://localhost:4566"
REGION="us-east-1"
QUEUE_URL="$AWS_ENDPOINT/000000000000/raw-events"

# Verificar se AWS CLI está disponível
if ! command -v aws &> /dev/null; then
    echo "AWS CLI não encontrada."
    echo ""
    echo "Opções:"
    echo "  1. Instalar AWS CLI:        brew install awscli  (macOS)"
    echo "                              pip install awscli   (Linux/Windows)"
    echo "  2. Usar o runner Go (cross-platform, sem dependências extras):"
    echo "                              go run ./tools/dev seed"
    exit 1
fi

# Timestamp atual
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ)

# Array de eventos válidos
declare -a VALID_EVENTS

# Dev 001 - Commits
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"commits","value":5,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"commits","value":8,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"commits","value":3,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

# Dev 001 - Pull Requests
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"pull_requests","value":2,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"pull_requests","value":1,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

# Dev 001 - Review Time
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"review_time_minutes","value":45,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-001","metric_type":"review_time_minutes","value":30,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

# Dev 002 - Commits
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-002","metric_type":"commits","value":12,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-002","metric_type":"commits","value":7,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

# Dev 002 - Pull Requests
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-002","metric_type":"pull_requests","value":3,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')

# Dev 002 - Review Time
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-002","metric_type":"review_time_minutes","value":60,"repository":"org/repo-1","timestamp":"'$TIMESTAMP'"}')

# Dev 003 - Commits
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-003","metric_type":"commits","value":4,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

# Dev 003 - Pull Requests
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-003","metric_type":"pull_requests","value":2,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

# Dev 003 - Review Time
VALID_EVENTS+=('{"event_id":"'$(uuidgen)'","developer_id":"dev-003","metric_type":"review_time_minutes","value":25,"repository":"org/repo-2","timestamp":"'$TIMESTAMP'"}')

echo ""
echo "Enviando eventos válidos (${#VALID_EVENTS[@]} eventos)..."

for event in "${VALID_EVENTS[@]}"; do
    aws sqs send-message \
        --endpoint-url "$AWS_ENDPOINT" \
        --region "$REGION" \
        --queue-url "$QUEUE_URL" \
        --message-body "$event" > /dev/null
    echo "Evento enviado"
done

# Eventos inválidos para testar DLQ
echo ""
echo "Enviando eventos inválidos (para testar DLQ e validação)..."

# UUID inválido
INVALID_1='{"event_id":"invalid-uuid","developer_id":"dev-004","metric_type":"commits","value":5,"repository":"org/repo","timestamp":"'$TIMESTAMP'"}'
echo "  [1] UUID inválido: $INVALID_1"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$INVALID_1" > /dev/null
echo "Enviado"

# developer_id vazio
INVALID_2='{"event_id":"'$(uuidgen)'","developer_id":"","metric_type":"commits","value":5,"repository":"org/repo","timestamp":"'$TIMESTAMP'"}'
echo "  [2] developer_id vazio"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$INVALID_2" > /dev/null
echo "Enviado"

# metric_type inválido
INVALID_3='{"event_id":"'$(uuidgen)'","developer_id":"dev-004","metric_type":"invalid_metric","value":5,"repository":"org/repo","timestamp":"'$TIMESTAMP'"}'
echo "  [3] metric_type inválido"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$INVALID_3" > /dev/null
echo "Enviado"

# value negativo
INVALID_4='{"event_id":"'$(uuidgen)'","developer_id":"dev-004","metric_type":"commits","value":-5,"repository":"org/repo","timestamp":"'$TIMESTAMP'"}'
echo "  [4] value negativo"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$INVALID_4" > /dev/null
echo "Enviado"

# review_time > 1440
INVALID_5='{"event_id":"'$(uuidgen)'","developer_id":"dev-004","metric_type":"review_time_minutes","value":2000,"repository":"org/repo","timestamp":"'$TIMESTAMP'"}'
echo "  [5] review_time > 1440 (24h)"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$INVALID_5" > /dev/null
echo "Enviado"

# Eventos duplicados para testar idempotência
echo ""
echo "Enviando eventos duplicados (para testar idempotência)..."

DUPLICATE_ID=$(uuidgen)
DUPLICATE='{"event_id":"'$DUPLICATE_ID'","developer_id":"dev-dup","metric_type":"commits","value":10,"repository":"org/repo","timestamp":"'$TIMESTAMP'"}'

echo "  [1] Primeiro envio"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$DUPLICATE" > /dev/null
echo "Enviado"

echo "  [2] Segundo envio (mesmo event_id)"
aws sqs send-message \
    --endpoint-url "$AWS_ENDPOINT" \
    --region "$REGION" \
    --queue-url "$QUEUE_URL" \
    --message-body "$DUPLICATE" > /dev/null
echo "   Enviado"

echo ""
echo "=========================================="
echo "Seed concluído!"
echo "=========================================="
echo ""
echo "Resumo:"
echo "${#VALID_EVENTS[@]} eventos válidos"
echo "5 eventos inválidos (DLQ)"
echo "2 eventos duplicados"
echo ""
echo "Próximos passos:"
echo "  1. Aguarde ~5-10s para processamento"
echo "  2. Consulte resultados:"
echo "     curl http://localhost:8080/metrics/dev-001/summary | jq ."
echo ""
