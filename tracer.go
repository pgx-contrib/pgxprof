package pgxprof

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var (
	_ pgx.QueryTracer = (*QueryTracer)(nil)
	_ pgx.BatchTracer = (*QueryTracer)(nil)
)

// QueryTracer profiles pgx queries by wrapping them in
// EXPLAIN (ANALYZE, FORMAT JSON) and reporting the decoded plan.
//
// Profiling runs synchronously before the real query executes, roughly
// doubling per-query latency. QueryTracer is intended for development and
// debugging, not production hot paths.
type QueryTracer struct {
	// Options is the default set of EXPLAIN options applied to every
	// profilable statement. Per-query directives such as
	//
	//	-- @explain true
	//	-- @analyze true
	//
	// override these defaults for the statement in which they appear.
	// When Options is nil and no directives are present, the statement
	// is not profiled.
	Options *QueryOptions
	// Reporter receives the captured traces. When nil, a WriterReporter
	// that writes JSON to os.Stdout is used.
	Reporter Reporter
}

// TraceQueryStart implements pgx.QueryTracer.
func (q *QueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	trace, err := q.traceQuery(ctx, conn, data)
	if err != nil || trace == nil {
		return ctx
	}
	q.reporter().Report(ctx, trace)
	return ctx
}

// TraceQueryEnd implements pgx.QueryTracer.
func (q *QueryTracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
}

// TraceBatchStart implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	trace, err := q.traceBatch(ctx, conn, data)
	if err != nil || trace == nil {
		return ctx
	}
	q.reporter().ReportBatch(ctx, trace)
	return ctx
}

// TraceBatchQuery implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchQuery(_ context.Context, _ *pgx.Conn, _ pgx.TraceBatchQueryData) {
}

// TraceBatchEnd implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceBatchEndData) {
}

func (q *QueryTracer) reporter() Reporter {
	if q.Reporter != nil {
		return q.Reporter
	}
	return &WriterReporter{}
}

func (q *QueryTracer) traceQuery(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) (*TraceQueryData, error) {
	opts := q.resolveOptions(data.SQL)
	if opts == nil || !opts.Explain {
		return nil, nil
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	explained := opts.String() + " " + data.SQL

	var raw []byte
	if err := tx.QueryRow(ctx, explained, data.Args...).Scan(&raw); err != nil {
		return nil, err
	}

	traces, err := decodeTraces(raw)
	if err != nil {
		return nil, err
	}

	return &TraceQueryData{
		Query:  data.SQL,
		Args:   data.Args,
		Traces: traces,
	}, nil
}

type batchItem struct {
	explained string
	origSQL   string
	args      []any
}

func (q *QueryTracer) traceBatch(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchStartData) (*TraceBatchData, error) {
	var items []batchItem
	for _, queued := range data.Batch.QueuedQueries {
		opts := q.resolveOptions(queued.SQL)
		if opts == nil || !opts.Explain {
			continue
		}
		items = append(items, batchItem{
			explained: opts.String() + " " + queued.SQL,
			origSQL:   queued.SQL,
			args:      queued.Arguments,
		})
	}
	if len(items) == 0 {
		return nil, nil
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	out := &TraceBatchData{Queries: make([]*TraceQueryData, 0, len(items))}
	for _, item := range items {
		var raw []byte
		if err := tx.QueryRow(ctx, item.explained, item.args...).Scan(&raw); err != nil {
			return nil, err
		}
		traces, err := decodeTraces(raw)
		if err != nil {
			return nil, err
		}
		out.Queries = append(out.Queries, &TraceQueryData{
			Query:  item.origSQL,
			Args:   item.args,
			Traces: traces,
		})
	}
	return out, nil
}

// resolveOptions returns the effective QueryOptions for a statement.
// Inline directives override the tracer defaults. A statement that is not
// profilable (DDL, empty, etc.) returns nil.
func (q *QueryTracer) resolveOptions(sql string) *QueryOptions {
	if !isProfilable(sql) {
		return nil
	}
	parsed, err := ParseQueryOptions(sql)
	if err != nil {
		return nil
	}
	if parsed != nil {
		return parsed
	}
	return q.Options
}

var profilablePrefixes = []string{
	"SELECT",
	"INSERT",
	"UPDATE",
	"DELETE",
	"MERGE",
	"VALUES",
	"EXECUTE",
	"DECLARE",
	"CREATE TABLE AS",
	"CREATE MATERIALIZED VIEW",
}

func isProfilable(query string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(stripComments(query)))
	for _, prefix := range profilablePrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return true
		}
	}
	return false
}

func stripComments(query string) string {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader(query))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		fmt.Fprintln(&buf, line)
	}
	return buf.String()
}

func decodeTraces(raw []byte) ([]*QueryTrace, error) {
	var traces []*QueryTrace
	if err := json.Unmarshal(raw, &traces); err != nil {
		return nil, err
	}
	return traces, nil
}

// TraceBatchData contains the profiling output for a pgx batch.
type TraceBatchData struct {
	// Queries is the per-statement trace data.
	Queries []*TraceQueryData `json:"Queries"`
}

// TraceQueryData contains the profiling output for a single query.
type TraceQueryData struct {
	// Query is the original SQL as submitted by the caller, without the
	// EXPLAIN prefix.
	Query string `json:"Query"`
	// Args is the argument list bound to the query.
	Args []any `json:"Args"`
	// Traces is the decoded EXPLAIN output.
	Traces []*QueryTrace `json:"Traces"`
}
