# tools/dev — cross-platform dev runner

Single Go binary that replaces every host-side bash/make/AWS-CLI helper this
project used to ship with. It exists so the whole stack can be operated the
same way on macOS, Linux and Windows with only Docker and a Go toolchain
installed — no `make`, no Git Bash, no `awscli`, no WSL.

## Why

The case asks for `docker-compose up` to start everything. That part works
everywhere because it runs inside Linux containers. The friction has always
been the host scripts around it:

| Host helper           | Problem on Windows                       |
| --------------------- | ---------------------------------------- |
| `Makefile`            | `make` not installed by default          |
| `scripts/seed.sh`     | Bash + `uuidgen` + `aws` CLI required    |
| AWS CLI for seeding   | Extra install, version drift             |

This runner removes all of those by talking to Docker via `docker compose`
(or `docker-compose`) and to SQS via the AWS SDK for Go directly.

## Usage

From the project root:

```bash
go run ./tools/dev <command>
```

| Command         | What it does                                                |
| --------------- | ----------------------------------------------------------- |
| `up`            | `docker compose up -d` + waits for all health checks green  |
| `stop`          | `docker compose stop` — graceful SIGTERM, volumes preserved |
| `down`          | `docker compose down -v` — stop + remove volumes            |
| `restart`       | `down` then `up`                                            |
| `build`         | Build the processor and aggregator images                   |
| `logs [svc]`    | Stream logs (all services, or one)                          |
| `seed`          | Publish 14 valid + 5 invalid + 2 duplicate events           |
| `health`        | Hit `/health` on LocalStack, Processor and Aggregator       |
| `test`          | Run `go test ./...` inside each service                     |
| `clean`         | `down` + remove built binaries                              |
| `help`          | Show usage                                                  |

The Makefile in the repo root is now a thin wrapper around these commands —
`make up` is exactly `go run ./tools/dev up`. On Windows where `make` is
absent, just call the Go runner directly.

## Requirements

- Go 1.24+
- Docker (Docker Desktop, Colima, OrbStack or Rancher Desktop — anything that
  exposes either `docker compose` v2 or the legacy `docker-compose` binary).
