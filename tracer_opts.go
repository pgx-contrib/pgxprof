package pgxprof

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// QueryOptions controls the EXPLAIN command applied to a profiled query.
type QueryOptions struct {
	// Explain wraps the query in EXPLAIN. When false, the query is not profiled.
	Explain bool
	// Analyze adds the ANALYZE clause, which executes the query to collect
	// runtime statistics. ANALYZE is ignored when Explain is false.
	Analyze bool
}

// String returns the EXPLAIN prefix for the given options, or "" when
// Explain is false.
func (x *QueryOptions) String() string {
	if x == nil || !x.Explain {
		return ""
	}

	opts := make([]string, 0, 2)
	if x.Analyze {
		opts = append(opts, "ANALYZE")
	}
	opts = append(opts, "FORMAT JSON")

	return "EXPLAIN (" + strings.Join(opts, ", ") + ")"
}

var (
	explainPattern = regexp.MustCompile(`@explain\s+(\S+)`)
	analyzePattern = regexp.MustCompile(`@analyze\s+(\S+)`)
)

// ParseQueryOptions extracts `@explain`/`@analyze` directives from the query
// text (typically in a leading SQL comment). It returns (nil, nil) when no
// directives are present, and an error only when a directive has a malformed
// value.
func ParseQueryOptions(query string) (*QueryOptions, error) {
	var opts *QueryOptions

	if m := explainPattern.FindStringSubmatch(query); m != nil {
		v, err := strconv.ParseBool(m[1])
		if err != nil {
			return nil, fmt.Errorf("invalid @explain value %q: %w", m[1], err)
		}
		opts = &QueryOptions{Explain: v}
	}

	if m := analyzePattern.FindStringSubmatch(query); m != nil {
		v, err := strconv.ParseBool(m[1])
		if err != nil {
			return nil, fmt.Errorf("invalid @analyze value %q: %w", m[1], err)
		}
		if opts == nil {
			opts = &QueryOptions{}
		}
		opts.Analyze = v
	}

	return opts, nil
}

// QueryPlan is a node in the PostgreSQL EXPLAIN output tree.
type QueryPlan struct {
	Alias             string       `json:"Alias"`
	RelationName      string       `json:"Relation Name"`
	NodeType          string       `json:"Node Type"`
	Plans             []*QueryPlan `json:"Plans"`
	ActualStartupTime float64      `json:"Actual Startup Time"`
	ActualLoops       int          `json:"Actual Loops"`
	ActualTotalTime   float64      `json:"Actual Total Time"`
	PlanRows          int          `json:"Plan Rows"`
	PlanWidth         int          `json:"Plan Width"`
	ActualRows        int          `json:"Actual Rows"`
	StartupCost       float64      `json:"Startup Cost"`
	TotalCost         float64      `json:"Total Cost"`
	AsyncCapable      bool         `json:"Async Capable"`
	ParallelAware     bool         `json:"Parallel Aware"`
}

// QueryTrace is a single EXPLAIN result for a query. PostgreSQL returns an
// array of these; most queries produce one entry.
type QueryTrace struct {
	Plan          *QueryPlan `json:"Plan"`
	PlanningTime  float64    `json:"Planning Time"`
	ExecutionTime float64    `json:"Execution Time"`
}
