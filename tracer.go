package pgxprof

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
func (q *QueryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, query pgx.TraceQueryStartData) context.Context {
	// trace the query
	trace, err := q.TraceQuery(ctx, conn, query)
	if err != nil {
		return ctx
	}

	// TODO: handle the trace
	if trace != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(trace)
	}

	return ctx
}

// TraceQuery traces the query.
func (x *QueryTracer) TraceQuery(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) (*TraceQueryData, error) {
	// parse the query options
	options := x.options(data.SQL)
	// if the options is nil, return the QueryReport
	if options == nil {
		return nil, nil
	}

	// prepare the statement
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// prepare the query
	query := options.String() + " " + data.SQL
	// analyze the query
	row := tx.QueryRow(ctx, query, data.Args...)

	output := []byte{}
	// scan the row
	if err := row.Scan(&output); err != nil {
		return nil, err
	}

	trace := &TraceQueryData{
		Query: data.SQL,
		Args:  data.Args,
	}
	// unmarshal the data
	if err := json.Unmarshal(output, &trace); err != nil {
		return nil, err
	}

	return trace, nil
}

// TraceQueryEnd implements pgx.QueryTracer.
func (q *QueryTracer) TraceQueryEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	// no-op
}

// TraceBatchStart implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchStart(ctx context.Context, conn *pgx.Conn, batch pgx.TraceBatchStartData) context.Context {
	// trace the query
	trace, err := q.TraceBatch(ctx, conn, batch)
	if err != nil {
		return ctx
	}

	// TODO: handle the trace
	if trace != nil {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		encoder.Encode(trace)
	}

	return ctx
}

// TraceBatch traces the query.
func (x *QueryTracer) TraceBatch(ctx context.Context, conn *pgx.Conn, data pgx.TraceBatchStartData) (*TraceBatchData, error) {
	batch := &pgx.Batch{}
	batchOptions := []*QueryOptions{}

	for _, item := range data.Batch.QueuedQueries {
		// parse the query options
		options := x.options(item.SQL)
		// if the options is nil, return the QueryReport
		if options == nil {
			continue
		}
		batchOptions = append(batchOptions, options)
		// prepare the query
		query := options.String() + " " + item.SQL
		// append the query to the batch
		batch.Queue(query, item.Arguments...)
	}

	if batch.Len() == 0 {
		return nil, nil
	}

	// prepare the statement
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	stack := &TraceBatchData{}
	// prepare the batch
	for _, item := range batch.QueuedQueries {
		rows, err := tx.Query(ctx, item.SQL, item.Arguments...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		index := 0
		// iterate the rows
		for rows.Next() {
			if err := rows.Err(); err != nil {
				return nil, err
			}

			output := []byte{}
			// scan the row
			if err := rows.Scan(&output); err != nil {
				return nil, err
			}

			trace := &TraceQueryData{
				Query: batch.QueuedQueries[index].SQL,
				Args:  batch.QueuedQueries[index].Arguments,
			}
			// prepare the trace query
			trace.Query = strings.TrimPrefix(trace.Query, batchOptions[index].String())
			trace.Query = strings.TrimPrefix(trace.Query, " ")
			// unmarshal the data
			if err := json.Unmarshal(output, &trace); err != nil {
				return nil, err
			}

			stack.Queries = append(stack.Queries, trace)
		}
	}

	return stack, nil
}

// TraceBatchQuery implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchQuery(_ context.Context, _ *pgx.Conn, _ pgx.TraceBatchQueryData) {
	// no-op
}

// TraceBatchEnd implements pgx.BatchTracer.
func (q *QueryTracer) TraceBatchEnd(_ context.Context, _ *pgx.Conn, _ pgx.TraceBatchEndData) {
	// no-op
}

func (x *QueryTracer) prepare(query string) string {
	buffer := &bytes.Buffer{}
	// prepare the scanner
	scanner := bufio.NewScanner(bytes.NewBufferString(query))

	for scanner.Scan() {
		text := scanner.Text()
		// if the text is not a comment, append the text
		if prefix := strings.TrimSpace(text); !strings.HasPrefix(prefix, "--") {
			fmt.Fprintln(buffer, text)
		}
	}

	query = buffer.String()
	query = strings.ToUpper(query)
	query = strings.TrimSpace(query)

	return query
}

func (x *QueryTracer) options(query string) *QueryOptions {
	prefix := []string{
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

	operation := x.prepare(query)
	// check the operation
	for _, name := range prefix {
		// if the query is the operation, return
		if strings.HasPrefix(operation, name) {
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
	}

	return nil
}

// TraceBatchData is the query explain.
type TraceBatchData struct {
	// Queries is the query analyzes.
	Queries []*TraceQueryData `Queries:"Queries"`
}

// TraceQueryData is the query explain.
type TraceQueryData struct {
	// Query is the query
	Query string `json:"Query"`
	// Args is the query arguments
	Args []any `json:"Args"`
	// Traces is the query analyzes.
	Traces []*QueryTrace `json:"Traces"`
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (x *TraceQueryData) UnmarshalJSON(data []byte) error {
	x.Traces = []*QueryTrace{}
	return json.Unmarshal(data, &x.Traces)
}
