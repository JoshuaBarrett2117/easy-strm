package main

import "database/sql"

func getDBInstance() *sql.DB {
	return db
}
