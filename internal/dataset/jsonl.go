package dataset

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadConnections(path string) ([]ConnectionRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var records []ConnectionRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for line := 1; scanner.Scan(); line++ {
		var record ConnectionRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if record.SchemaVersion != 1 && record.SchemaVersion != 2 && record.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("%s:%d: unsupported schema version %d", path, line, record.SchemaVersion)
		}
		if record.Label != 0 && record.Label != 1 {
			return nil, fmt.Errorf("%s:%d: invalid label %d", path, line, record.Label)
		}
		if record.Weight <= 0 {
			record.Weight = 1
		}
		if record.LabelSource == "" {
			record.LabelSource = "weak_recording_continuity"
		}
		if record.RecordID == "" {
			record.RecordID = fmt.Sprintf("%s|%s:%d|%s:%d", record.GroupID, record.Previous.Source, record.Previous.Line, record.Current.Source, record.Current.Line)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func SaveConnections(path string, records []ConnectionRecord) (result error) {
	if directory := filepath.Dir(path); directory != "." {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); result == nil {
			result = closeErr
		}
	}()
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			return err
		}
	}
	return writer.Flush()
}
