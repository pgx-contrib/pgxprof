# pgxprof

[![CI](https://github.com/pgx-contrib/pgxprof/actions/workflows/ci.yml/badge.svg)](https://github.com/pgx-contrib/pgxprof/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/pgx-contrib/pgxprof)](https://github.com/pgx-contrib/pgxprof/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/pgx-contrib/pgxprof.svg)](https://pkg.go.dev/github.com/pgx-contrib/pgxprof)
[![License](https://img.shields.io/github/license/pgx-contrib/pgxprof)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/pgx-contrib/pgxprof)](go.mod)
[![pgx Version](https://img.shields.io/badge/pgx-v5-blue)](https://github.com/jackc/pgx)

Query profiler for [pgx v5](https://github.com/jackc/pgx).

## Features

- Transparent `EXPLAIN (ANALYZE, FORMAT JSON)` wrapping for `Query`, `QueryRow`, and `SendBatch`
- Inline SQL annotation directives control profiling per query
- Decoded `QueryPlan` / `QueryTrace` Go structs ready for further processing
- Pluggable `Reporter` interface with built-in JSON writer and `log/slog` sinks
- Works with any `pgx`-compatible connection: `*pgx.Conn`, `*pgxpool.Pool`, or `pgx.Tx`

## Installation

```bash
go get github.com/pgx-contrib/pgxprof
```

## Usage

### Basic pool setup

```go
config, err := pgxpool.ParseConfig(os.Getenv("PGX_DATABASE_URL"))
if err != nil {
    panic(err)
}

config.ConnConfig.Tracer = &pgxprof.QueryTracer{
    // Default options applied when a query has no inline annotations.
    Options: &pgxprof.QueryOptions{
        Explain: true,
        Analyze: true,
    },
    // Defaults to a WriterReporter that prints JSON to stdout.
    Reporter: &pgxprof.LoggerReporter{Logger: slog.Default()},
}

pool, err := pgxpool.NewWithConfig(ctx, config)
```

### Per-query annotations

Embed profiler directives in SQL comments. Annotations override the default `Options` for that statement:

```sql
-- @explain true
-- @analyze true
SELECT id, name FROM users WHERE active = true
```

```go
rows, err := pool.Query(ctx, `
    -- @explain true
    -- @analyze true
    SELECT id, name FROM users WHERE active = $1`, true)
```

### Custom reporter

Implement the `Reporter` interface to ship traces anywhere:

```go
type Reporter interface {
    Report(ctx context.Context, trace *TraceQueryData)
    ReportBatch(ctx context.Context, trace *TraceBatchData)
}
```

## Annotation directives

| Directive | Value | Description |
|---|---|---|
| `@explain` | `true` / `false` | Wrap the query in `EXPLAIN`. Required for profiling. |
| `@analyze` | `true` / `false` | Add the `ANALYZE` clause. Ignored when `@explain` is `false`. |

## Performance caveat

`EXPLAIN ANALYZE` **executes the query** inside a transaction that `pgxprof`
rolls back, so profiling runs every matching statement twice: once for the
caller and once for the plan. This roughly doubles per-query latency and
database load.

Use `pgxprof` in development and staging environments, or gate it behind
per-query directives so only the queries you're investigating pay the cost.

## Development

### DevContainer

Open in VS Code with the Dev Containers extension. The environment provides Go,
PostgreSQL 18, and Nix automatically.

```
PGX_DATABASE_URL=postgres://vscode@postgres:5432/pgxprof?sslmode=disable
```

### Nix

```bash
nix develop          # enter shell with Go
go tool ginkgo run -r
```

### Run tests

```bash
# Unit tests only (no database required)
go tool ginkgo run -r

# With integration tests
export PGX_DATABASE_URL="postgres://localhost/pgxprof?sslmode=disable"
go tool ginkgo run -r
```

## License

[MIT](LICENSE)
