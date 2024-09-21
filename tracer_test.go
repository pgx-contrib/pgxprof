package pgxprof_test

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgx-contrib/pgxprof"
)

func ExampleQueryTracer() {
	config, err := pgxpool.ParseConfig(os.Getenv("PGX_DATABASE_URL"))
	if err != nil {
		panic(err)
	}

	config.ConnConfig.Tracer = &pgxprof.QueryTracer{
		// set the default query options, which can be overridden by the query
		// -- @explain true
		// -- @analyze true
		Options: &pgxprof.QueryOptions{
			Explain: true,
			Analyze: true,
		},
	}

	// create a new connection pool
	querier, err := pgxpool.NewWithConfig(context.TODO(), config)
	if err != nil {
		panic(err)
	}
	// close the connection when the function returns
	defer querier.Close()

	// fetch all the organizations
	rows, err := querier.Query(context.TODO(), "SELECT * FROM customer")
	if err != nil {
		panic(err)
	}
	// close the rows when the function returns
	defer rows.Close()

	// Customer struct must be defined
	type Customer struct {
		FirstName string `db:"first_name"`
		LastName  string `db:"last_name"`
	}

	for rows.Next() {
		customer, err := pgx.RowToStructByName[Customer](rows)
		if err != nil {
			panic(err)
		}

		fmt.Println(customer.FirstName)
	}
}
