package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/crizah/Worm/schema"
)

// flat, serializable stand-in for schema.Type (which is an interface, so it
// cant round trip through sqlite/json on its own)
type typeDTO struct {
	kind   string
	name   string
	values []string
}

func toTypeDTO(t schema.Type) typeDTO {
	switch v := t.(type) {
	case schema.BoolType:
		return typeDTO{kind: "bool"}
	case schema.IntegerType:
		return typeDTO{kind: "integer"}
	case schema.UUIDType:
		return typeDTO{kind: "uuid"}
	case schema.TimeType:
		return typeDTO{kind: "time"}
	case schema.TextType:
		return typeDTO{kind: "text"}
	case schema.JSONType:
		return typeDTO{kind: "json"}
	case schema.EnumType:
		return typeDTO{kind: "enum", name: v.Name, values: v.Values}
	default:
		return typeDTO{kind: "text"}
	}
}

func getPaginationIndex(t *schema.Table) (*schema.Index, error) {
	// gives us the appropriate index to do pagination with
	// priority: pk -> unique (not null)
	// if neither exist, fail. TODO: have this faliure in the inspecter phase itself\
	for _, index := range t.Indexes {
		if index.IsPK {
			return index, nil
		}
	}

	// we didnt find a pk, check for non nullable unique contraints
	for _, index := range t.Indexes {
		if index.IsUnique {
			// check if all the columns assocoated with it are not nullable
			flag := false
			for _, col := range index.Columns {
				if col.IsNullable {
					flag = true
					break
				}
			}
			if !flag {
				// this index is valid
				return index, nil
			}
		}
	}
	// TODO: save table index and colmap in the stateDb
	// cant have any index
	return nil, fmt.Errorf("Didnt find any valid index") // TODO: again, have this failure exist on inspecter phase itself
}

func persistPlanTables(ctx context.Context, stateDb *sql.DB, tables []*schema.Table) error {
	for i, t := range tables {
		index, err := getPaginationIndex(t)
		if err != nil {
			return err
		}
		var cols []string
		for _, c := range index.Columns {
			cols = append(cols, c.Name)
		}
		_, err = stateDb.ExecContext(ctx,
			`INSERT INTO capture_plan_tables (table_name, ordinal, index_name, index_columns, index_is_unique) VALUES (?, ?, ?, ?, ?)`,
			t.Name, i, index.Name, strings.Join(cols, ", "), index.IsUnique,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func persistPlanColumns(ctx context.Context, stateDb *sql.DB, tables []*schema.Table) error {
	for _, t := range tables {
		for _, c := range t.Columns {
			dto := toTypeDTO(c.Type)
			_, err := stateDb.ExecContext(ctx,
				`INSERT INTO capture_plan_columns (table_name, column_name, type_kind, enum_name, enum_values) VALUES (?, ?, ?, ?, ?)`,
				t.Name, c.Name, dto.kind, dto.name, strings.Join(dto.values, ","),
			)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func fromTypeDTO(d typeDTO) schema.Type {
	switch d.kind {
	case "bool":
		return schema.BoolType{}
	case "integer":
		return schema.IntegerType{}
	case "uuid":
		return schema.UUIDType{}
	case "time":
		return schema.TimeType{}
	case "text":
		return schema.TextType{}
	case "json":
		return schema.JSONType{}
	case "enum":
		return schema.EnumType{Name: d.name, Values: d.values}
	default:
		return schema.TextType{}
	}
}

func getShit(ctx context.Context, stateDb *sql.DB) (schema.Stuff, error) {
	s := schema.Stuff{
		IndexMap: make(map[string]*schema.IndexRed),
		ColMap:   make(map[string]schema.Type),
	}

	rows, err := stateDb.QueryContext(ctx, `SELECT table_name, index_name, index_columns FROM capture_plan_tables ORDER BY ordinal`)
	if err != nil {
		return schema.Stuff{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var tableName, indexName, indexColumns string
		if err := rows.Scan(&tableName, &indexName, &indexColumns); err != nil {
			return schema.Stuff{}, err
		}
		s.Tables = append(s.Tables, tableName)
		s.IndexMap[tableName] = &schema.IndexRed{Name: indexName, ColumnNames: strings.Split(indexColumns, ", ")}
	}
	if err := rows.Err(); err != nil {
		return schema.Stuff{}, err
	}

	colRows, err := stateDb.QueryContext(ctx, `SELECT table_name, column_name, type_kind, enum_name, enum_values FROM capture_plan_columns`)
	if err != nil {
		return schema.Stuff{}, err
	}
	defer colRows.Close()

	for colRows.Next() {
		var tableName, columnName, kind, enumName, enumValues string
		if err := colRows.Scan(&tableName, &columnName, &kind, &enumName, &enumValues); err != nil {
			return schema.Stuff{}, err
		}
		var values []string
		if enumValues != "" {
			values = strings.Split(enumValues, ",")
		}
		s.ColMap[tableName+columnName] = fromTypeDTO(typeDTO{kind: kind, name: enumName, values: values})
	}
	if err := colRows.Err(); err != nil {
		return schema.Stuff{}, err
	}

	return s, nil
}

func persistStage(ctx context.Context, stateDb *sql.DB, stage int) error {
	if _, err := stateDb.ExecContext(ctx,
		`INSERT OR REPLACE INTO capture_stage (id, stage) VALUES (1, ?)`, stage); err != nil {
		return err
	}
	return nil
}
