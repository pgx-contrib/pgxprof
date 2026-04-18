package pgxprof

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
)

// Reporter receives profiling data produced by QueryTracer. Implementations
// must be safe for concurrent use.
type Reporter interface {
	// Report delivers the trace for a single query.
	Report(ctx context.Context, trace *TraceQueryData)
	// ReportBatch delivers the traces for a pgx batch.
	ReportBatch(ctx context.Context, trace *TraceBatchData)
}

// WriterReporter writes indented JSON traces to Writer. If Writer is nil,
// os.Stdout is used.
type WriterReporter struct {
	Writer io.Writer
}

// Report implements Reporter.
func (r *WriterReporter) Report(_ context.Context, trace *TraceQueryData) {
	r.encode(trace)
}

// ReportBatch implements Reporter.
func (r *WriterReporter) ReportBatch(_ context.Context, trace *TraceBatchData) {
	r.encode(trace)
}

func (r *WriterReporter) encode(v any) {
	w := r.Writer
	if w == nil {
		w = os.Stdout
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// LoggerReporter emits traces through an slog.Logger. If Logger is nil,
// slog.Default() is used.
type LoggerReporter struct {
	Logger *slog.Logger
}

// Report implements Reporter.
func (r *LoggerReporter) Report(ctx context.Context, trace *TraceQueryData) {
	r.logger().LogAttrs(ctx, slog.LevelInfo, "pgxprof.query",
		slog.String("query", trace.Query),
		slog.Any("traces", trace.Traces),
	)
}

// ReportBatch implements Reporter.
func (r *LoggerReporter) ReportBatch(ctx context.Context, trace *TraceBatchData) {
	for _, q := range trace.Queries {
		r.Report(ctx, q)
	}
}

func (r *LoggerReporter) logger() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
