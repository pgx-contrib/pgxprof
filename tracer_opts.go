package pgxprof

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// QueryOptions represents the options that can be specified in a SQL query.
type QueryOptions struct {
	// Explain is a boolean that specifies whether to explain the query.
	Explain bool
	// Analyze is a boolean that specifies whether to analyze the query.
	Analyze bool
}

// String returns the string representation of the query options.
func (x *QueryOptions) String() string {
	var command []string
	var options []string

	if x.Explain {
		command = append(command, "EXPLAIN")
	}

	// prepare the analyze statement
	if x.Analyze {
		options = append(options, "ANALYZE")
	}

	options = append(options, "FORMAT JSON")
	command = append(command, "("+strings.Join(options, ", ")+")")

	return strings.Join(command, " ")
}

var patterns = []*regexp.Regexp{
	regexp.MustCompile(`(@explain) (\d+)`),
	regexp.MustCompile(`(@analyze) (\d+[s|m|h|d])`),
}

// ParseQueryOptions parses query options from a SQL query.
func ParseQueryOptions(query string) (*QueryOptions, error) {
	var matches [][]string
	// prepare the matches
	for _, pattern := range patterns {
		// find the options
		item := pattern.FindAllStringSubmatch(query, 2)
		// if the item is empty
		if len(item) == 0 {
			return nil, fmt.Errorf("invalid query cache options")
		}
		// append the item to the matches
		matches = append(matches, item...)
	}

	options := &QueryOptions{}
	// iterate over the matches and set the options
	for _, item := range matches {
		// if the length of the item is not equal to 2, print MATCH and
		if len(item) < 3 {
			return nil, fmt.Errorf("invalid query cache options")
		}
		// set the options fields
		switch item[1] {
		case "@explain":
			value, err := strconv.ParseBool(item[2])
			switch {
			case err != nil:
				return nil, fmt.Errorf("invalid @explain query option: %w", err)
			default:
				options.Explain = value
			}
		case "@analyze":
			value, err := strconv.ParseBool(item[2])
			switch {
			case err != nil:
				return nil, fmt.Errorf("invalid @analyze query option: %w", err)
			default:
				options.Analyze = value
			}

		}
	}

	return options, nil
}

// QueryPlan is the query plan.
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

// QueryTrace is the query anlysis.
type QueryTrace struct {
	Plan          *QueryPlan `json:"Plan"`
	PlanningTime  float64    `json:"Planning Time"`
	ExecutionTime float64    `json:"Execution Time"`
}
