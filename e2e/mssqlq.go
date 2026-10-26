//go:build ignore

// mssqlq runs T-SQL on a SQL Server for the e2e scripts (lib.sh e2e_mssql), as mysql / psql do for the other engines: the DSN is
// the argument, the SQL is stdin, split into batches at the lines that are only GO (a create view or a create function is a batch's
// first statement); one connection for all of them, so a set or a use holds for the rest. A result set is printed, one row to a
// line, the columns separated by a tab and a NULL as NULL.
//
//	go run e2e/mssqlq.go 'sqlserver://sa:…@host:51433?database=sqlmux&TrustServerCertificate=true' <<<'select 1'
package main

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	_ "github.com/microsoft/go-mssqldb"
)

var goLine = regexp.MustCompile(`(?im)^\s*go\s*$`)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: mssqlq DSN < sql")
		os.Exit(2)
	}
	in, _ := io.ReadAll(os.Stdin)
	db, err := sql.Open("sqlserver", os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer db.Close()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer conn.Close()
	for _, batch := range goLine.Split(string(in), -1) {
		if strings.TrimSpace(batch) == "" {
			continue
		}
		rows, err := conn.QueryContext(ctx, batch)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for {
			cols, _ := rows.Columns()
			for rows.Next() {
				vals := make([]any, len(cols))
				ptrs := make([]any, len(cols))
				for i := range vals {
					ptrs[i] = &vals[i]
				}
				if err := rows.Scan(ptrs...); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				out := make([]string, len(cols))
				for i, v := range vals {
					switch x := v.(type) {
					case nil:
						out[i] = "NULL"
					case []byte:
						out[i] = string(x)
					default:
						out[i] = fmt.Sprint(x)
					}
				}
				fmt.Println(strings.Join(out, "\t"))
			}
			if !rows.NextResultSet() {
				break
			}
		}
		if err := rows.Err(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		rows.Close()
	}
}
