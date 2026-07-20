package db

type Database interface {
	migrate() error
	setVersion(version string, description string) error
}
