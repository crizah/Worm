package parser

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/crizah/Worm/schema"
)

// parses the sql file and returns a schema object
type Parser struct {
	schemaDir *Dir
}

type Dir struct {
	dirPath   string
	filePaths *[]string // sorted
}

func New(dirPath string) (*Parser, error) {
	info, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("Cant access directory at path: %s, err: %w", dirPath, err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("Path %s is not a directory", dirPath)
	}

	// validate the files are .sql
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("Failed to read directory: %w", err)
	}

	var schemaFiles []string
	for _, file := range entries {
		// check for .sql extensions and ignore directories inside
		if !file.IsDir() && filepath.Ext(file.Name()) == ".sql" {
			schemaFiles = append(schemaFiles, filepath.Join(dirPath, file.Name()))

		}
	}

	// sort the filenames based on the ordring
	sort.Strings(schemaFiles)

	return &Parser{
		schemaDir: &Dir{
			dirPath:   dirPath,
			filePaths: &schemaFiles,
		},
	}, nil

}

func (p *Parser) Parse() (*schema.Schema, error) {
	// initialise a schema obj and have every goroutine write to it
	var wg sync.WaitGroup
	var mu sync.Mutex
	s := &schema.Schema{
		DbName: "", // get this from somewhere else, not sure where lol
		Tables: make([]schema.Table, 2),
	}
	for _, path := range *p.schemaDir.filePaths {
		// open, if one fails, all fail
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", p.schemaDir.dirPath, err)
		}
		data, err := io.ReadAll(f)
		f.Close() // dont defer
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		// parse the actual data
		go func(data []byte, s *schema.Schema) {
			wg.Add(1)
			// seperate the data into segments based on semicolons

		}(data, s)

	}

}
