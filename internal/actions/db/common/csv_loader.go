package common

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

type CSVLoadOptions struct {
	FilePath         string
	Columns          []string
	Delimiter        string
	HeaderInFirstRow bool
}

type CSVData struct {
	Columns []string
	Rows    [][]string
}

func ParseDelimiter(input string) (rune, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return ',', nil
	}
	runes := []rune(trimmed)
	if len(runes) != 1 {
		return 0, fmt.Errorf("delimiter must be a single character")
	}
	return runes[0], nil
}

func LoadCSVFile(options CSVLoadOptions) (CSVData, error) {
	filePath := strings.TrimSpace(options.FilePath)
	if filePath == "" {
		return CSVData{}, fmt.Errorf("file path is required")
	}

	delimiter, err := ParseDelimiter(options.Delimiter)
	if err != nil {
		return CSVData{}, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return CSVData{}, fmt.Errorf("opening csv file %q: %w", filePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = delimiter
	records, err := reader.ReadAll()
	if err != nil {
		return CSVData{}, fmt.Errorf("reading csv file %q: %w", filePath, err)
	}
	if len(records) == 0 {
		return CSVData{}, fmt.Errorf("csv file %q is empty", filePath)
	}

	columns := normalizeColumnNames(options.Columns)
	rows := records
	if options.HeaderInFirstRow {
		headerColumns := normalizeColumnNames(records[0])
		rows = records[1:]
		if len(columns) == 0 {
			columns = headerColumns
		}
	}

	if len(columns) == 0 {
		return CSVData{}, fmt.Errorf("columns are required when csv header is disabled")
	}

	for rowIndex, row := range rows {
		if len(row) != len(columns) {
			return CSVData{}, fmt.Errorf("row %d has %d values, expected %d", rowIndex+1, len(row), len(columns))
		}
	}

	return CSVData{Columns: columns, Rows: rows}, nil
}

func normalizeColumnNames(columns []string) []string {
	normalized := make([]string, 0, len(columns))
	for _, column := range columns {
		trimmed := strings.TrimSpace(column)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	return normalized
}
