package parser

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Line struct {
	ID         string `json:"id"`
	Filename   string `json:"file"`
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Definition string `json:"def"`
}

// Pointers distinguish omitted collections from requested empty arrays.
type Extraction struct {
	Functions *[]Function `json:"functions,omitempty"`
	Lines     *[]Line     `json:"lines,omitempty"`
}

// ExtractDirectory extracts only the selected collections from regular C files.
func ExtractDirectory(directory string, functions, lines bool) (*Extraction, error) {
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s: expected a directory", directory)
	}
	result := &Extraction{}
	if functions {
		result.Functions = &[]Function{}
	}
	if lines {
		result.Lines = &[]Line{}
	}
	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || (filepath.Ext(path) != ".c" && filepath.Ext(path) != ".h") {
			return nil
		}
		if functions {
			// Preserve the existing directory analyzer's handling of individual file failures.
			extracted, err := analyzeCFile(path)
			if err == nil {
				*result.Functions = append(*result.Functions, extracted...)
			}
		}
		if lines {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for number := 1; len(content) > 0; number++ {
				end := bytes.IndexByte(content, '\n')
				var line []byte
				if end < 0 {
					line, content = content, nil
				} else {
					line, content = content[:end], content[end+1:]
					line = bytes.TrimSuffix(line, []byte{'\r'})
				}
				*result.Lines = append(*result.Lines, Line{
					ID: fmt.Sprintf("%s:%d", path, number), Filename: path,
					Start: number, End: number, Definition: string(line),
				})
			}
		}
		return nil
	})
	if lines {
		sort.SliceStable(*result.Lines, func(i, j int) bool {
			return (*result.Lines)[i].Filename < (*result.Lines)[j].Filename
		})
	}
	return result, err
}
