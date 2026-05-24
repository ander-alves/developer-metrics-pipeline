#!/bin/bash
set -e

echo "Migration: 001 - Create Tables"
echo "=================================="

# Cores para output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

ENDPOINT_URL="${AWS_ENDPOINT_URL:-http://localhost:4566}"
REGION="${AWS_REGION:-us-east-1}"

# Função para criar tabela
create_table() {
    local table_name=$1
    local config_file=$2

    echo ""
    echo "Criando tabela: $table_name"

    if awslocal dynamodb describe-table --table-name "$table_name" &>/dev/null 2>&1; then
        echo -e "${GREEN}Tabela $table_name já existe${NC}"
    else
        echo "Criando tabela $table_name..."

        # Lê o arquivo JSON e cria a tabela.
        # Ignora ResourceInUseException — pode acontecer se o Go runner criou
        # a tabela concorrentemente (race condition no boot).
        output=$(awslocal dynamodb create-table \
            --cli-input-json file://"$config_file" \
            --region "$REGION" \
            2>&1) || true

        if echo "$output" | grep -q "ResourceInUseException"; then
            echo -e "${GREEN}Tabela $table_name já existe (criada pelo runner)${NC}"
        elif echo "$output" | grep -q '"TableDescription"'; then
            echo -e "${GREEN}Tabela $table_name criada com sucesso${NC}"
        else
            echo -e "${RED}❌ Erro ao criar tabela $table_name: $output${NC}"
            exit 1
        fi

        # Aguardar tabela ficar ativa
        echo "Aguardando tabela $table_name ficar ativa..."
        for i in {1..30}; do
            status=$(awslocal dynamodb describe-table --table-name "$table_name" --region "$REGION" | grep -o '"TableStatus": "[^"]*"' | cut -d'"' -f4)
            if [ "$status" == "ACTIVE" ]; then
                echo -e "${GREEN}Tabela $table_name está ativa${NC}"
                break
            fi
            echo "  Tentativa $i... Status: $status"
            sleep 1
        done
    fi
}

# Obter diretório do script
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TABLES_DIR="$(dirname "$SCRIPT_DIR")/tables"

echo "Tabelas configuradas em: $TABLES_DIR"

# Criar tabelas
create_table "events" "$TABLES_DIR/events.json"
create_table "developer_summary" "$TABLES_DIR/developer_summary.json"

echo ""
echo "=================================="
echo -e "${GREEN}Migration 001 concluída com sucesso!${NC}"
echo ""
echo "Tabelas criadas:"
awslocal dynamodb list-tables --region "$REGION" | grep -o '"[^"]*"' | grep -E 'events|developer_summary'
echo ""
