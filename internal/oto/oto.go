package oto

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

type Entry struct {
	Filename     string
	Alias        string
	Offset       float64
	Fixed        float64
	Blank        float64
	Preutterance float64
	Overlap      float64
	OtoPath      string
	Line         int
}

type Diagnostic struct {
	Line    int
	Message string
}

type Ini struct {
	Path        string
	Encoding    string
	Entries     map[string][]Entry
	Diagnostics []Diagnostic
}

// ReadIni reads an oto.ini encoded as UTF-8 or Shift_JIS. Malformed lines are
// retained as diagnostics so one bad entry does not make an entire bank fail.
func ReadIni(otoPath string) (*Ini, error) {
	data, err := os.ReadFile(otoPath)
	if err != nil {
		return nil, err
	}

	text, encoding, err := Decode(data)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", otoPath, err)
	}

	absPath, err := filepath.Abs(otoPath)
	if err != nil {
		return nil, err
	}

	result := &Ini{
		Path:     absPath,
		Encoding: encoding,
		Entries:  map[string][]Entry{},
	}
	baseDir := filepath.Dir(absPath)
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		entry, err := parseLine(line, baseDir)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Line: lineNumber, Message: err.Error()})
			continue
		}
		entry.OtoPath = absPath
		entry.Line = lineNumber
		result.Entries[entry.Alias] = append(result.Entries[entry.Alias], entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Decode decodes UTF-8 or Shift_JIS metadata used by UTAU voicebanks.
func Decode(data []byte) (string, string, error) {
	data = []byte(strings.TrimPrefix(string(data), "\ufeff"))
	if utf8.Valid(data) {
		return string(data), "UTF-8", nil
	}
	decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), data)
	if err != nil {
		return "", "", err
	}
	return string(decoded), "Shift_JIS", nil
}

func parseLine(line, baseDir string) (Entry, error) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return Entry{}, fmt.Errorf("missing '='")
	}

	filename := strings.TrimSpace(parts[0])
	if filename == "" {
		return Entry{}, fmt.Errorf("empty filename")
	}
	fields := strings.Split(parts[1], ",")
	if len(fields) < 6 {
		return Entry{}, fmt.Errorf("expected alias and 5 parameters, got %d fields", len(fields))
	}

	alias := strings.TrimSpace(fields[0])
	if alias == "" {
		alias = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	values := make([]float64, 5)
	for i := range values {
		value := strings.TrimSpace(fields[i+1])
		if value == "" {
			values[i] = 0
			continue
		}
		parsed, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return Entry{}, fmt.Errorf("invalid parameter %d %q", i+1, value)
		}
		values[i] = parsed
	}

	fullPath := filename
	if !filepath.IsAbs(filename) {
		fullPath = filepath.Join(baseDir, filepath.FromSlash(strings.ReplaceAll(filename, "\\", "/")))
	}
	fullPath = filepath.Clean(fullPath)

	return Entry{
		Filename:     fullPath,
		Alias:        alias,
		Offset:       values[0],
		Fixed:        values[1],
		Blank:        values[2],
		Preutterance: values[3],
		Overlap:      values[4],
	}, nil
}
