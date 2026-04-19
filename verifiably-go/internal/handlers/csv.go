package handlers

import (
	"encoding/csv"
	"errors"
	"io"
)

// parseCSVRows reads a CSV file and returns a slice of field-name→value maps
// (one per data row, excluding the header) plus the header itself. The first
// non-empty row is treated as the header.
func parseCSVRows(r io.Reader) ([]map[string]string, []string, error) {
	reader := csv.NewReader(r)
	reader.FieldsPerRecord = -1 // tolerate trailing blank columns
	reader.TrimLeadingSpace = true

	var header []string
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if allEmpty(rec) {
			continue
		}
		header = rec
		break
	}
	if len(header) == 0 {
		return nil, nil, errors.New("empty CSV")
	}

	var rows []map[string]string
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		if allEmpty(rec) {
			continue
		}
		row := make(map[string]string, len(header))
		for i, name := range header {
			if i < len(rec) {
				row[name] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, header, nil
}

func allEmpty(rec []string) bool {
	for _, v := range rec {
		if v != "" {
			return false
		}
	}
	return true
}
