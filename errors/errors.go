package errors

type Error string

const (
	MIGRATION_ERROR  Error = "Error Migrating"
	CONNECTION_ERROR Error = "Error establishing connection"
)
