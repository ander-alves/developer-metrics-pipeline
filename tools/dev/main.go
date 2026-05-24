// Package main provides a cross-platform development runner for the
// Developer Metrics Pipeline.
//
// It replaces every host-side bash/make/AWS-CLI helper this project used to
// require, so the stack can be operated identically on macOS, Linux and
// Windows with only a Go toolchain and Docker installed.
//
// Design choices:
//
//   - Zero external Go dependencies — the AWS SDK has been intentionally
//     dropped because its endpoint resolver in newer versions makes pointing
//     SQS at LocalStack from the host fragile. LocalStack accepts both the
//     SQS query protocol and the DynamoDB JSON protocol without signed
//     requests, so we talk to it directly over HTTP using net/http.
//
//   - All commands shell out to "docker compose" (v2) or "docker-compose"
//     (v1) — whichever the host has — so we do not depend on a specific
//     Docker distribution (Desktop, Colima, OrbStack, Rancher all work).
//
//   - The "up" command also creates SQS queues and DynamoDB tables via the
//     HTTP API. This is idempotent and exists so the project bootstraps
//     correctly on Windows clones where init-aws.sh may not have its
//     executable bit preserved by git.
//
// Usage: go run ./tools/dev <command>
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const (
	localstackEndpoint  = "http://localhost:4566"
	rawEventsQueue      = localstackEndpoint + "/000000000000/raw-events"
	processorHealthURL  = "http://localhost:8081/health"
	aggregatorHealthURL = "http://localhost:8080/health"
	localstackHealthURL = localstackEndpoint + "/_localstack/health"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cmd, args := os.Args[1], os.Args[2:]

	var err error
	switch cmd {
	case "up":
		err = up(ctx)
	case "stop":
		err = stop(ctx)
	case "down":
		err = down(ctx)
	case "restart":
		if err = down(ctx); err == nil {
			err = up(ctx)
		}
	case "build":
		err = build(ctx)
	case "logs":
		err = logs(ctx, args)
	case "seed":
		err = seed(ctx)
	case "health":
		err = healthCheck(ctx)
	case "test":
		err = testServices(ctx)
	case "clean":
		err = clean(ctx)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println(`Developer Metrics Pipeline — cross-platform dev runner

Usage:
  go run ./tools/dev <command> [args]

Commands:
  up          Start LocalStack + processor + aggregator and wait until healthy.
              Also ensures SQS queues and DynamoDB tables exist (idempotent).
  stop        Send SIGTERM to all containers and wait for graceful shutdown.
              Data volumes are preserved — use "up" to restart.
  down        Stop everything and remove containers, networks and volumes.
  restart     Equivalent to: down + up.
  build       Build the processor and aggregator Docker images.
  logs [svc]  Stream container logs (optionally for a single service).
  seed        Publish sample events into the raw-events SQS queue.
  health      Check health endpoints of all services.
  test        Run "go test ./..." inside each service.
  clean       Tear down stack and remove built Go binaries.
  help        Show this message.

The runner shells out to "docker compose" (v2) or "docker-compose" (v1),
whichever is available, and talks to LocalStack over plain HTTP so it works
on macOS, Linux and Windows with no extra host tools (no make, no bash,
no AWS CLI).`)
}

// -----------------------------------------------------------------------------
// docker compose helpers
// -----------------------------------------------------------------------------

func composeCmd() ([]string, error) {
	if _, err := exec.LookPath("docker"); err == nil {
		if err := exec.Command("docker", "compose", "version").Run(); err == nil {
			return []string{"docker", "compose"}, nil
		}
	}
	if _, err := exec.LookPath("docker-compose"); err == nil {
		return []string{"docker-compose"}, nil
	}
	return nil, errors.New(`neither "docker compose" nor "docker-compose" was found in PATH; install Docker Desktop, Colima, OrbStack or Rancher Desktop`)
}

func runCompose(ctx context.Context, args ...string) error {
	base, err := composeCmd()
	if err != nil {
		return err
	}
	full := append(append([]string{}, base[1:]...), args...)
	cmd := exec.CommandContext(ctx, base[0], full...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// -----------------------------------------------------------------------------
// commands
// -----------------------------------------------------------------------------

func up(ctx context.Context) error {
	fmt.Println("→ Starting services with docker compose...")
	if err := runCompose(ctx, "up", "-d"); err != nil {
		return fmt.Errorf("compose up failed: %w", err)
	}

	fmt.Println("→ Waiting for LocalStack to become healthy...")
	if err := waitForURL(ctx, localstackHealthURL, 60*time.Second); err != nil {
		return err
	}

	fmt.Println("→ Ensuring SQS queues and DynamoDB tables exist...")
	if err := ensureResources(ctx); err != nil {
		return fmt.Errorf("bootstrap resources: %w", err)
	}

	fmt.Println("→ Waiting for processor and aggregator to become healthy...")
	if err := waitForURL(ctx, processorHealthURL, 60*time.Second); err != nil {
		return err
	}
	if err := waitForURL(ctx, aggregatorHealthURL, 60*time.Second); err != nil {
		return err
	}

	fmt.Println("✓ All services healthy.")
	fmt.Println()
	fmt.Println("  LocalStack:  http://localhost:4566")
	fmt.Println("  Processor:   http://localhost:8081/health")
	fmt.Println("  Aggregator:  http://localhost:8080/health")
	fmt.Println()
	fmt.Println(`Next: "go run ./tools/dev seed" to publish sample events.`)
	return nil
}

func stop(ctx context.Context) error {
	fmt.Println("→ Stopping services (data preserved)...")
	return runCompose(ctx, "stop")
}

func down(ctx context.Context) error {
	fmt.Println("→ Stopping services and removing volumes...")
	return runCompose(ctx, "down", "-v")
}

func build(ctx context.Context) error {
	fmt.Println("→ Building images...")
	return runCompose(ctx, "build")
}

func logs(ctx context.Context, args []string) error {
	composeArgs := append([]string{"logs", "-f"}, args...)
	return runCompose(ctx, composeArgs...)
}

func clean(ctx context.Context) error {
	if err := down(ctx); err != nil {
		return err
	}
	for _, p := range []string{
		"services/processor/processor",
		"services/aggregator/aggregator",
	} {
		_ = os.Remove(p)
	}
	fmt.Println("✓ Clean done.")
	return nil
}

func testServices(ctx context.Context) error {
	for _, svc := range []string{"services/processor", "services/aggregator"} {
		fmt.Printf("→ go test ./... in %s\n", svc)
		cmd := exec.CommandContext(ctx, "go", "test", "-v", "./...")
		cmd.Dir = svc
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("tests failed for %s: %w", svc, err)
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// health checks
// -----------------------------------------------------------------------------

func healthCheck(ctx context.Context) error {
	checks := []struct {
		name string
		url  string
	}{
		{"LocalStack", localstackHealthURL},
		{"Processor", processorHealthURL},
		{"Aggregator", aggregatorHealthURL},
	}
	var failed bool
	client := &http.Client{Timeout: 3 * time.Second}
	for _, c := range checks {
		status, body, err := httpGet(ctx, client, c.url)
		switch {
		case err != nil:
			fmt.Printf("✗ %-11s %s — %v\n", c.name, c.url, err)
			failed = true
		case status >= 200 && status < 300:
			fmt.Printf("✓ %-11s %s — %s\n", c.name, c.url, summarize(body))
		default:
			fmt.Printf("✗ %-11s %s — HTTP %d\n", c.name, c.url, status)
			failed = true
		}
	}
	if failed {
		return errors.New("one or more health checks failed")
	}
	return nil
}

func waitForURL(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		status, _, err := httpGet(ctx, client, url)
		if err == nil && status >= 200 && status < 300 {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("%s did not respond within %s", url, timeout)
}

func httpGet(ctx context.Context, client *http.Client, url string) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, body, nil
}

func summarize(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 80 {
		return s[:77] + "..."
	}
	return s
}

// -----------------------------------------------------------------------------
// resource bootstrap (SQS queues + DynamoDB tables) via raw HTTP
// -----------------------------------------------------------------------------

// ensureResources creates the SQS queues and DynamoDB tables required by the
// pipeline. It is idempotent — existing resources are left alone. The Go
// runner does this even though init-aws.sh exists, because on Windows clones
// the script's executable bit may not survive and LocalStack will silently
// skip it.
func ensureResources(ctx context.Context) error {
	queues := []struct {
		name        string
		dlq         string
		maxReceive  int
	}{
		{name: "raw-events-dlq"},
		{name: "processed-events-dlq"},
		{name: "raw-events", dlq: "raw-events-dlq", maxReceive: 3},
		{name: "processed-events", dlq: "processed-events-dlq", maxReceive: 3},
	}
	for _, q := range queues {
		if err := createQueue(ctx, q.name, q.dlq, q.maxReceive); err != nil {
			return err
		}
	}

	tableSpecs := []map[string]interface{}{
		{
			"TableName": "events",
			"AttributeDefinitions": []map[string]string{
				{"AttributeName": "event_id", "AttributeType": "S"},
			},
			"KeySchema": []map[string]string{
				{"AttributeName": "event_id", "KeyType": "HASH"},
			},
			"BillingMode": "PAY_PER_REQUEST",
		},
		{
			"TableName": "developer_summary",
			"AttributeDefinitions": []map[string]string{
				{"AttributeName": "developer_id", "AttributeType": "S"},
			},
			"KeySchema": []map[string]string{
				{"AttributeName": "developer_id", "KeyType": "HASH"},
			},
			"BillingMode": "PAY_PER_REQUEST",
		},
	}
	for _, spec := range tableSpecs {
		if err := createTable(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}

func createQueue(ctx context.Context, name, dlq string, maxReceive int) error {
	form := url.Values{}
	form.Set("Action", "CreateQueue")
	form.Set("Version", "2012-11-05")
	form.Set("QueueName", name)
	if dlq != "" {
		policy := fmt.Sprintf(
			`{"deadLetterTargetArn":"arn:aws:sqs:us-east-1:000000000000:%s","maxReceiveCount":"%d"}`,
			dlq, maxReceive,
		)
		form.Set("Attribute.1.Name", "RedrivePolicy")
		form.Set("Attribute.1.Value", policy)
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, localstackEndpoint+"/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/sqs/aws4_request, SignedHeaders=host, Signature=test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("create queue %s: %w", name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("  ✓ queue %s\n", name)
		return nil
	}
	// LocalStack returns 400 with QueueAlreadyExists or similar when the queue
	// already exists with identical attributes — treat that as success.
	if strings.Contains(string(body), "QueueAlreadyExists") || strings.Contains(string(body), "AlreadyExists") {
		fmt.Printf("  ✓ queue %s (already exists)\n", name)
		return nil
	}
	return fmt.Errorf("create queue %s: HTTP %d — %s", name, resp.StatusCode, summarize(body))
}

func createTable(ctx context.Context, spec map[string]interface{}) error {
	name, _ := spec["TableName"].(string)
	body, _ := json.Marshal(spec)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, localstackEndpoint+"/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "DynamoDB_20120810.CreateTable")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/dynamodb/aws4_request, SignedHeaders=host, Signature=test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("create table %s: %w", name, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Printf("  ✓ table %s\n", name)
		return nil
	}
	if strings.Contains(string(respBody), "ResourceInUseException") {
		fmt.Printf("  ✓ table %s (already exists)\n", name)
		return nil
	}
	return fmt.Errorf("create table %s: HTTP %d — %s", name, resp.StatusCode, summarize(respBody))
}

// -----------------------------------------------------------------------------
// seed — publishes events into raw-events via the SQS query protocol
// -----------------------------------------------------------------------------

type rawEvent struct {
	EventID     string `json:"event_id"`
	DeveloperID string `json:"developer_id"`
	MetricType  string `json:"metric_type"`
	Value       int    `json:"value"`
	Repository  string `json:"repository"`
	Timestamp   string `json:"timestamp"`
}

func seed(ctx context.Context) error {
	now := time.Now().UTC().Format(time.RFC3339)

	valid := []rawEvent{
		{newUUID(), "dev-001", "commits", 5, "org/repo-1", now},
		{newUUID(), "dev-001", "commits", 8, "org/repo-1", now},
		{newUUID(), "dev-001", "commits", 3, "org/repo-2", now},
		{newUUID(), "dev-001", "pull_requests", 2, "org/repo-1", now},
		{newUUID(), "dev-001", "pull_requests", 1, "org/repo-2", now},
		{newUUID(), "dev-001", "review_time_minutes", 45, "org/repo-1", now},
		{newUUID(), "dev-001", "review_time_minutes", 30, "org/repo-2", now},
		{newUUID(), "dev-002", "commits", 12, "org/repo-1", now},
		{newUUID(), "dev-002", "commits", 7, "org/repo-2", now},
		{newUUID(), "dev-002", "pull_requests", 3, "org/repo-1", now},
		{newUUID(), "dev-002", "review_time_minutes", 60, "org/repo-1", now},
		{newUUID(), "dev-003", "commits", 4, "org/repo-2", now},
		{newUUID(), "dev-003", "pull_requests", 2, "org/repo-2", now},
		{newUUID(), "dev-003", "review_time_minutes", 25, "org/repo-2", now},
	}
	invalid := []rawEvent{
		{"invalid-uuid", "dev-004", "commits", 5, "org/repo", now},
		{newUUID(), "", "commits", 5, "org/repo", now},
		{newUUID(), "dev-004", "invalid_metric", 5, "org/repo", now},
		{newUUID(), "dev-004", "commits", -5, "org/repo", now},
		{newUUID(), "dev-004", "review_time_minutes", 2000, "org/repo", now},
	}
	dupID := newUUID()
	duplicates := []rawEvent{
		{dupID, "dev-dup", "commits", 10, "org/repo", now},
		{dupID, "dev-dup", "commits", 10, "org/repo", now},
	}

	fmt.Printf("→ Publishing %d valid events...\n", len(valid))
	if err := publishAll(ctx, valid); err != nil {
		return err
	}
	fmt.Printf("→ Publishing %d invalid events (DLQ test)...\n", len(invalid))
	if err := publishAll(ctx, invalid); err != nil {
		return err
	}
	fmt.Printf("→ Publishing %d duplicate events (idempotency test)...\n", len(duplicates))
	if err := publishAll(ctx, duplicates); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("✓ Seed complete.")
	fmt.Println("  Wait ~5-10s for processing, then query:")
	fmt.Println("    curl http://localhost:8080/metrics/dev-001/summary")
	return nil
}

func publishAll(ctx context.Context, events []rawEvent) error {
	for _, e := range events {
		body, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if err := sendMessage(ctx, rawEventsQueue, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func sendMessage(ctx context.Context, queueURL, body string) error {
	form := url.Values{}
	form.Set("Action", "SendMessage")
	form.Set("Version", "2012-11-05")
	form.Set("QueueUrl", queueURL)
	form.Set("MessageBody", body)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, localstackEndpoint+"/", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential=test/20240101/us-east-1/sqs/aws4_request, SignedHeaders=host, Signature=test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("send message: HTTP %d — %s", resp.StatusCode, summarize(respBody))
}

// newUUID returns an RFC 4122 v4 UUID using crypto/rand. We implement it
// inline so the tool depends only on the standard library.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail on supported platforms; if it ever
		// does we fall back to a timestamp-based pseudo-id rather than panic
		// so seeding still produces something usable.
		return fmt.Sprintf("seed-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
