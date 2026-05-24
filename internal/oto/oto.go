package oto

import (
    "bufio"
    "os"
    "path/filepath"
    "strconv"
    "strings"

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
}

type Ini struct {
    Entries map[string][]Entry
}

func ReadIni(otoPath string) (*Ini, error) {
    file, err := os.Open(otoPath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    decoder := transform.NewReader(file, japanese.ShiftJIS.NewDecoder())
    scanner := bufio.NewScanner(decoder)
    scanner.Buffer(make([]byte, 1024), 1024*1024)

    oto := &Ini{Entries: map[string][]Entry{}}
    baseDir := filepath.Dir(otoPath)

    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        entry, ok := parseLine(line, baseDir)
        if !ok {
            continue
        }
        oto.Entries[entry.Alias] = append(oto.Entries[entry.Alias], entry)
    }
    if err := scanner.Err(); err != nil {
        return nil, err
    }
    return oto, nil
}

func parseLine(line, baseDir string) (Entry, bool) {
    parts := strings.SplitN(line, "=", 2)
    if len(parts) != 2 {
        return Entry{}, false
    }

    filename := strings.TrimSpace(parts[0])
    fields := strings.Split(parts[1], ",")
    if len(fields) < 6 {
        return Entry{}, false
    }

    alias := strings.TrimSpace(fields[0])
    offset, ok := parseFloat(fields[1])
    if !ok {
        return Entry{}, false
    }
    fixed, ok := parseFloat(fields[2])
    if !ok {
        return Entry{}, false
    }
    blank, ok := parseFloat(fields[3])
    if !ok {
        return Entry{}, false
    }
    preutterance, ok := parseFloat(fields[4])
    if !ok {
        return Entry{}, false
    }
    overlap, ok := parseFloat(fields[5])
    if !ok {
        return Entry{}, false
    }

    fullPath := filename
    if !filepath.IsAbs(filename) {
        fullPath = filepath.Join(baseDir, filename)
    }

    return Entry{
        Filename:     fullPath,
        Alias:        alias,
        Offset:       offset,
        Fixed:        fixed,
        Blank:        blank,
        Preutterance: preutterance,
        Overlap:      overlap,
    }, true
}

func parseFloat(value string) (float64, bool) {
    value = strings.TrimSpace(value)
    if value == "" {
        return 0, false
    }
    num, err := strconv.ParseFloat(value, 64)
    if err != nil {
        return 0, false
    }
    return num, true
}
