# Stage 1: Builder
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w" \
    -o aggregator ./cmd/main.go

RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags="-s -w" \
    -o healthcheck ./cmd/healthcheck

# Stage 2: Runtime (distroless para imagem mínima)
FROM gcr.io/distroless/base-debian11:nonroot

COPY --from=builder /app/aggregator /aggregator
COPY --from=builder /app/healthcheck /healthcheck

EXPOSE 8080

ENTRYPOINT ["/aggregator"]
