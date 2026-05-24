package main

import (
    "bufio"
    "flag"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "sort"
    "strconv"
    "strings"

    "golang.org/x/text/encoding/japanese"
    "golang.org/x/text/transform"
)

type OtoEntry struct {
    Filename     string
    Alias        string
    Offset       float64
    Fixed        float64
    Blank        float64
    Preutterance float64
    Overlap      float64
}

type OtoIni struct {
    Entries map[string][]OtoEntry
}

func main() {
    var (
        otoPath  string
        listOnly bool
        alias    string
        limit    int
    )

    flag.StringVar(&otoPath, "oto", "", "path to oto.ini")
    flag.BoolVar(&listOnly, "list", false, "list aliases")
    flag.StringVar(&alias, "alias", "", "filter by alias")
    flag.IntVar(&limit, "limit", 20, "max entries to show")
    flag.Parse()

    if otoPath == "" {
        log.Fatal("-oto is required")
    }

    oto, err := ReadOtoIni(otoPath)
    if err != nil {
        log.Fatal(err)
    }

    if listOnly {
        aliases := make([]string, 0, len(oto.Entries))
        for key := range oto.Entries {
            aliases = append(aliases, key)
        }
        sort.Strings(aliases)
        for _, key := range aliases {
            fmt.Println(key)
        }
        return
    }

    if alias != "" {
        entries := oto.Entries[alias]
        if len(entries) == 0 {
            log.Fatalf("alias not found: %s", alias)
        }
        if limit > 0 && len(entries) > limit {
            entries = entries[:limit]
        }
        for _, entry := range entries {
            fmt.Printf("%s=%s,%.3f,%.3f,%.3f,%.3f,%.3f\n",
                entry.Filename,
                entry.Alias,
                entry.Offset,
                entry.Fixed,
                entry.Blank,
                entry.Preutterance,
                entry.Overlap,
            )
        }
        return
    }

    fmt.Printf("entries=%d\n", countEntries(oto))
    fmt.Printf("aliases=%d\n", len(oto.Entries))
}

func countEntries(oto *OtoIni) int {
    total := 0
    for _, entries := range oto.Entries {
        total += len(entries)
    }
    return total
}

func ReadOtoIni(otoPath string) (*OtoIni, error) {
    file, err := os.Open(otoPath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    decoder := transform.NewReader(file, japanese.ShiftJIS.NewDecoder())
    scanner := bufio.NewScanner(decoder)
    scanner.Buffer(make([]byte, 1024), 1024*1024)

    oto := &OtoIni{Entries: map[string][]OtoEntry{}}
    baseDir := filepath.Dir(otoPath)

    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        entry, ok := parseOtoLine(line, baseDir)
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

func parseOtoLine(line, baseDir string) (OtoEntry, bool) {
    parts := strings.SplitN(line, "=", 2)
    if len(parts) != 2 {
        return OtoEntry{}, false
    }

    filename := strings.TrimSpace(parts[0])
    fields := strings.Split(parts[1], ",")
    if len(fields) < 6 {
        return OtoEntry{}, false
    }

    alias := strings.TrimSpace(fields[0])
    offset, ok := parseFloat(fields[1])
    if !ok {
        return OtoEntry{}, false
    }
    fixed, ok := parseFloat(fields[2])
    if !ok {
        return OtoEntry{}, false
    }
    blank, ok := parseFloat(fields[3])
    if !ok {
        return OtoEntry{}, false
    }
    preutterance, ok := parseFloat(fields[4])
    if !ok {
        return OtoEntry{}, false
    }
    overlap, ok := parseFloat(fields[5])
    if !ok {
        return OtoEntry{}, false
    }

    fullPath := filename
    if !filepath.IsAbs(filename) {
        fullPath = filepath.Join(baseDir, filename)
    }

    return OtoEntry{
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
