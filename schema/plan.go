package schema

// this is only herre because shared type. this codebase is a mess
type IndexRed struct {
	Name        string
	ColumnNames []string
}
type Stuff struct {
	Tables   []string             // table name sorted order
	IndexMap map[string]*IndexRed // indexname to index
	ColMap   map[string]Type      // table_name+column_name -> columnType
}
