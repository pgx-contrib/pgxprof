package pgxprof_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/pgx-contrib/pgxprof"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type captureReporter struct {
	mu      sync.Mutex
	queries []*pgxprof.TraceQueryData
	batches []*pgxprof.TraceBatchData
}

func (c *captureReporter) Report(_ context.Context, t *pgxprof.TraceQueryData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.queries = append(c.queries, t)
}

func (c *captureReporter) ReportBatch(_ context.Context, t *pgxprof.TraceBatchData) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.batches = append(c.batches, t)
}

// ---------------------------------------------------------------------------
// Unit specs
// ---------------------------------------------------------------------------

var _ = Describe("QueryOptions", func() {
	Describe("String()", func() {
		It("returns empty string when nil", func() {
			var opts *pgxprof.QueryOptions
			Expect(opts.String()).To(Equal(""))
		})

		It("returns empty string when Explain is false", func() {
			Expect((&pgxprof.QueryOptions{}).String()).To(Equal(""))
		})

		It("emits EXPLAIN (FORMAT JSON) when only Explain is set", func() {
			Expect((&pgxprof.QueryOptions{Explain: true}).String()).
				To(Equal("EXPLAIN (FORMAT JSON)"))
		})

		It("emits EXPLAIN (ANALYZE, FORMAT JSON) when both are set", func() {
			Expect((&pgxprof.QueryOptions{Explain: true, Analyze: true}).String()).
				To(Equal("EXPLAIN (ANALYZE, FORMAT JSON)"))
		})

		It("suppresses Analyze when Explain is false", func() {
			Expect((&pgxprof.QueryOptions{Analyze: true}).String()).To(Equal(""))
		})
	})
})

var _ = Describe("ParseQueryOptions", func() {
	It("returns nil, nil when no directives are present", func() {
		opts, err := pgxprof.ParseQueryOptions("SELECT 1")
		Expect(err).To(BeNil())
		Expect(opts).To(BeNil())
	})

	It("parses @explain true", func() {
		opts, err := pgxprof.ParseQueryOptions("-- @explain true\nSELECT 1")
		Expect(err).To(BeNil())
		Expect(opts).NotTo(BeNil())
		Expect(opts.Explain).To(BeTrue())
		Expect(opts.Analyze).To(BeFalse())
	})

	It("parses @explain false", func() {
		opts, err := pgxprof.ParseQueryOptions("-- @explain false\nSELECT 1")
		Expect(err).To(BeNil())
		Expect(opts).NotTo(BeNil())
		Expect(opts.Explain).To(BeFalse())
	})

	It("parses combined @explain and @analyze", func() {
		opts, err := pgxprof.ParseQueryOptions("-- @explain true\n-- @analyze true\nSELECT 1")
		Expect(err).To(BeNil())
		Expect(opts.Explain).To(BeTrue())
		Expect(opts.Analyze).To(BeTrue())
	})

	It("parses @analyze on its own", func() {
		opts, err := pgxprof.ParseQueryOptions("-- @analyze true\nSELECT 1")
		Expect(err).To(BeNil())
		Expect(opts.Analyze).To(BeTrue())
		Expect(opts.Explain).To(BeFalse())
	})

	It("tolerates extra whitespace between directive and value", func() {
		opts, err := pgxprof.ParseQueryOptions("-- @explain    true\nSELECT 1")
		Expect(err).To(BeNil())
		Expect(opts.Explain).To(BeTrue())
	})

	It("returns error for invalid @explain value", func() {
		_, err := pgxprof.ParseQueryOptions("-- @explain maybe\nSELECT 1")
		Expect(err).NotTo(BeNil())
	})

	It("returns error for invalid @analyze value", func() {
		_, err := pgxprof.ParseQueryOptions("-- @analyze 5s\nSELECT 1")
		Expect(err).NotTo(BeNil())
	})
})

var _ = Describe("WriterReporter", func() {
	var buf *bytes.Buffer

	BeforeEach(func() {
		buf = &bytes.Buffer{}
	})

	It("writes indented JSON for a single query", func() {
		r := &pgxprof.WriterReporter{Writer: buf}
		r.Report(context.Background(), &pgxprof.TraceQueryData{
			Query:  "SELECT 1",
			Traces: []*pgxprof.QueryTrace{{ExecutionTime: 0.5}},
		})

		var decoded pgxprof.TraceQueryData
		Expect(json.Unmarshal(buf.Bytes(), &decoded)).To(Succeed())
		Expect(decoded.Query).To(Equal("SELECT 1"))
		Expect(decoded.Traces).To(HaveLen(1))
		Expect(decoded.Traces[0].ExecutionTime).To(Equal(0.5))
		Expect(strings.Contains(buf.String(), "  ")).To(BeTrue())
	})

	It("writes indented JSON for a batch", func() {
		r := &pgxprof.WriterReporter{Writer: buf}
		r.ReportBatch(context.Background(), &pgxprof.TraceBatchData{
			Queries: []*pgxprof.TraceQueryData{
				{Query: "SELECT 1"},
				{Query: "SELECT 2"},
			},
		})

		var decoded pgxprof.TraceBatchData
		Expect(json.Unmarshal(buf.Bytes(), &decoded)).To(Succeed())
		Expect(decoded.Queries).To(HaveLen(2))
	})
})

var _ = Describe("LoggerReporter", func() {
	It("emits a log record per query", func() {
		buf := &bytes.Buffer{}
		r := &pgxprof.LoggerReporter{
			Logger: slog.New(slog.NewJSONHandler(buf, nil)),
		}
		r.Report(context.Background(), &pgxprof.TraceQueryData{Query: "SELECT 7"})
		Expect(buf.String()).To(ContainSubstring(`"query":"SELECT 7"`))
	})

	It("emits one log record per query in a batch", func() {
		buf := &bytes.Buffer{}
		r := &pgxprof.LoggerReporter{
			Logger: slog.New(slog.NewJSONHandler(buf, nil)),
		}
		r.ReportBatch(context.Background(), &pgxprof.TraceBatchData{
			Queries: []*pgxprof.TraceQueryData{
				{Query: "SELECT 1"},
				{Query: "SELECT 2"},
			},
		})
		Expect(strings.Count(buf.String(), "\n")).To(Equal(2))
	})
})

// ---------------------------------------------------------------------------
// Integration specs (require PGX_DATABASE_URL)
// ---------------------------------------------------------------------------

var _ = Describe("Integration", Ordered, func() {
	var (
		ctx  context.Context
		pool *pgxpool.Pool
		rep  *captureReporter
	)

	newPool := func(tracer *pgxprof.QueryTracer) *pgxpool.Pool {
		dsn := os.Getenv("PGX_DATABASE_URL")
		if dsn == "" {
			Skip("PGX_DATABASE_URL not set")
		}
		cfg, err := pgxpool.ParseConfig(dsn)
		Expect(err).To(Succeed())
		cfg.ConnConfig.Tracer = tracer
		p, err := pgxpool.NewWithConfig(context.Background(), cfg)
		Expect(err).To(Succeed())
		return p
	}

	BeforeEach(func() {
		ctx = context.Background()
		rep = &captureReporter{}
	})

	AfterEach(func() {
		if pool != nil {
			pool.Close()
			pool = nil
		}
	})

	Describe("Query", func() {
		It("captures EXPLAIN with a populated ExecutionTime when defaults enable Analyze", func() {
			pool = newPool(&pgxprof.QueryTracer{
				Options:  &pgxprof.QueryOptions{Explain: true, Analyze: true},
				Reporter: rep,
			})

			var n int
			Expect(pool.QueryRow(ctx, "SELECT 1").Scan(&n)).To(Succeed())
			Expect(n).To(Equal(1))

			Expect(rep.queries).To(HaveLen(1))
			trace := rep.queries[0]
			Expect(trace.Query).To(Equal("SELECT 1"))
			Expect(trace.Traces).NotTo(BeEmpty())
			Expect(trace.Traces[0].ExecutionTime).To(BeNumerically(">", 0))
		})

		It("skips profiling when no defaults are set and no directives are present", func() {
			pool = newPool(&pgxprof.QueryTracer{Reporter: rep})

			var n int
			Expect(pool.QueryRow(ctx, "SELECT 2").Scan(&n)).To(Succeed())
			Expect(rep.queries).To(BeEmpty())
		})

		It("honours per-query directives without defaults", func() {
			pool = newPool(&pgxprof.QueryTracer{Reporter: rep})

			var n int
			query := "-- @explain true\n-- @analyze true\nSELECT 3"
			Expect(pool.QueryRow(ctx, query).Scan(&n)).To(Succeed())
			Expect(rep.queries).To(HaveLen(1))
			Expect(rep.queries[0].Traces[0].ExecutionTime).To(BeNumerically(">", 0))
		})
	})

	Describe("SendBatch", func() {
		It("produces one trace per profilable statement", func() {
			pool = newPool(&pgxprof.QueryTracer{
				Options:  &pgxprof.QueryOptions{Explain: true, Analyze: true},
				Reporter: rep,
			})

			batch := &pgx.Batch{}
			batch.Queue("SELECT 1")
			batch.Queue("SELECT 2")
			br := pool.SendBatch(ctx, batch)
			for range 2 {
				var n int
				Expect(br.QueryRow().Scan(&n)).To(Succeed())
			}
			Expect(br.Close()).To(Succeed())

			Expect(rep.batches).To(HaveLen(1))
			Expect(rep.batches[0].Queries).To(HaveLen(2))
			for _, q := range rep.batches[0].Queries {
				Expect(q.Traces).NotTo(BeEmpty())
				Expect(q.Traces[0].ExecutionTime).To(BeNumerically(">", 0))
			}
		})
	})
})
