package schema

import (
	"fmt"
	"strings"
)

type Dialect int // creating enums like this

const (
	SqliteDialect   Dialect = iota // gets 0
	PostgresDialect                // will get 1
)

// postgres conn strings are a url or a
// key=value dsn, anything else we treat as a sqlite file path
func ResolveDialect(connStr string) Dialect {
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") || strings.Contains(connStr, "host=") {
		return PostgresDialect
	}
	return SqliteDialect
}
func ResolveEnum(str string) (Dialect, error) {
	switch str {
	case "POSTGRESQL":
		return PostgresDialect, nil
	case "SQLITE":
		return SqliteDialect, nil
	default:
		return -1, fmt.Errorf("Unknown dialect %s", str)
	}
}

func DriverName(d Dialect) string {
	if d == PostgresDialect {
		return "postgres"
	}
	return "sqlite3"
}
