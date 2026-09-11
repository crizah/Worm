package emitter

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/crizah/Worm/schema"
)

type Emitter interface {
	Emitt() []string
	buildTable(t *schema.Table) string
	buildColumns(c *schema.Column) string
}

func writeFile(d string, f string, stmts []string) error {
	// add this into Emitter, this is out rn only for testing purpouses
	dir := "./"
	if d != "" {
		dir = d
	}

	fileName := "sqlite-migration-test.sql"
	if f != "" {
		fileName = f
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	fullPath := filepath.Join(dir, fileName)

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}

	defer file.Close()
	for _, line := range stmts {
		if _, err := file.WriteString(line + "\n\n"); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
	}

	return nil
}

func sortTables(tables []*schema.Table) []*schema.Table {
	// build adj map, i.e for every table, what tables depend on it
	// build incoming arr, i.e for every table, how many tables depend on this guy
	// sort arr, add to queue all that have 0 and remove dependency one by one, if the dependency for any guy becoumes 0
	// add to ans
	// cyclic guys will be all thats left, add them as is

	// TODO: since multiple fks in the same table can ref to the same refTable, handle that, but i think it should be fine
	var ans []*schema.Table
	adjMap := make(map[*schema.Table][]*schema.Table)
	incoming := make(map[*schema.Table]int) // a map is way better i dont think a vector will work

	for _, t := range tables {
		incoming[t] = 0
	}

	for _, t := range tables {
		for _, fk := range t.FKs {
			incoming[t]++
			adjMap[fk.RefTable] = append(adjMap[fk.RefTable], t)
		}
	}

	var q []*schema.Table
	for _, t := range tables {
		if incoming[t] == 0 {
			q = append(q, t)
		}
	}

	for !(len(q) == 0) {
		t := q[0] // front
		q = q[1:] // pop
		ans = append(ans, t)

		// for all the guys that this giuy references
		for _, n := range adjMap[t] {
			incoming[n]--

			if incoming[n] == 0 {
				// no more outgoing, add to queue
				q = append(q, n)
			}
		}

	}

	// only guys left will be cyclic guys, append them as is
	if len(ans) != len(tables) {
		for _, t := range tables {
			if incoming[t] > 0 {
				ans = append(ans, t)
			}
		}

	}

	return ans

}
