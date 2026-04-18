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

	// install the tracer; per-query directives override these defaults
	// -- @explain true
	// -- @analyze true
	config.ConnConfig.Tracer = &pgxprof.QueryTracer{
		Options: &pgxprof.QueryOptions{
			Explain: true,
			Analyze: true,
		},
	}

	// create a new connection pool
	pool, err := pgxpool.NewWithConfig(context.TODO(), config)
	if err != nil {
		panic(err)
	}
	// close the pool when the function returns
	defer pool.Close()

	// fetch all the customers
	rows, err := pool.Query(context.TODO(), "SELECT first_name, last_name FROM customer")
	if err != nil {
		panic(err)
	}
	// close the rows when the function returns
	defer rows.Close()

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
