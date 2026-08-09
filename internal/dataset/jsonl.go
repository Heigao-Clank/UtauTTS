package dataset

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
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
		if record.SchemaVersion != 1 && record.SchemaVersion != SchemaVersion {
			return nil, fmt.Errorf("%s:%d: unsupported schema version %d", path, line, record.SchemaVersion)
		}
		if record.Label != 0 && record.Label != 1 {
			return nil, fmt.Errorf("%s:%d: invalid label %d", path, line, record.Label)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}
