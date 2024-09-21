package pgxprof

import (
	"context"
	"encoding/json"
	"os"

	"github.com/jackc/pgx/v5"
)

var (
	_ pgx.QueryTracer = (*QueryTracer)(nil)
	_ pgx.BatchTracer = (*QueryTracer)(nil)
)

// QueryTracer represent a composite query tracer
type QueryTracer struct {
	// Querier is the pgx.Querier interface
	Options *QueryOptions
}

// TraceQueryStart implements pgx.QueryTracer.
func (q *QueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	query := &pgx.QueuedQuery{
		SQL:       data.SQL,
		Arguments: data.Args,
	}
	// trace the query
	// TODO: handle the trace
	_, err := q.TraceQuery(ctx, conn, query)
	if err != nil {
		return ctx
	}

	return ctx
}

// TraceQueryEnd implements pgx.QueryTracer.
func (q *QueryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	// no-op
}

// TraceBatchStart implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	// trace the batch
	for _, query := range data.Batch.QueuedQueries {
		// trace the query
		// TODO: handle the trace
		_, err := q.TraceQuery(ctx, conn, query)
		if err != nil {
			return ctx
		}
	}

	return ctx
}

// TraceBatchQuery implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchQuery(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchQueryData) {
	// no-op
}

// TraceBatchEnd implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceBatchEndData) {
	// no-op
}

// TraceQueryExplain queries the report of the query.
func (x *QueryTracer) TraceQuery(ctx context.Context, conn *pgx.Conn, query *pgx.QueuedQuery) (*TraceQueryData, error) {
	// parse the query options
	options := x.options(query.SQL)
	// if the options is nil, return the QueryReport
	if options == nil {
		return &TraceQueryData{}, nil
	}

	// prepare the statement
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// prepare the query
	query.SQL = options.String() + " " + query.SQL
	// analyze the query
	row := tx.QueryRow(ctx, query.SQL, query.Arguments...)

	data := []byte{}
	// scan the row
	if err := row.Scan(&data); err != nil {
		return nil, err
	}

	record := &TraceQueryData{}
	// unmarshal the data
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, err
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(record)

	return record, nil
}

func (x *QueryTracer) options(query string) *QueryOptions {
	// parse the query options
	options, err := ParseQueryOptions(query)
	if err != nil {
		// we should not cache the item
		return x.Options
	}

	if !options.Explain {
		return nil
	}

	return options
}

// TraceQueryData is the query explain.
type TraceQueryData struct {
	// Analyzes is the query analyzes.
	Analyzes []*QueryTrace
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (x *TraceQueryData) UnmarshalJSON(data []byte) error {
	x.Analyzes = []*QueryTrace{}
	return json.Unmarshal(data, &x.Analyzes)
}
