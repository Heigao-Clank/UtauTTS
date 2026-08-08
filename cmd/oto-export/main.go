package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"utautts/internal/audio"
	"utautts/internal/voicebank"
)

type manifestEntry struct {
	Alias        string  `json:"alias"`
	Source       string  `json:"source"`
	Trimmed      string  `json:"trimmed"`
	Offset       float64 `json:"offset_ms"`
	Fixed        float64 `json:"fixed_ms"`
	Blank        float64 `json:"blank_ms"`
	Preutterance float64 `json:"preutterance_ms"`
	Overlap      float64 `json:"overlap_ms"`
	SampleRate   int     `json:"sample_rate"`
	Channels     int     `json:"channels"`
	Frames       int     `json:"frames"`
}

func main() {
	var (
		otoPath    string
		outDir     string
		skipErrors bool
	)

	flag.StringVar(&otoPath, "oto", "", "path to a voicebank directory or oto.ini")
	flag.StringVar(&outDir, "out", "", "output directory")
	flag.BoolVar(&skipErrors, "skip-errors", false, "skip entries with errors")
	flag.Parse()

	if otoPath == "" || outDir == "" {
		log.Fatal("-oto and -out are required")
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	bank, err := voicebank.Load(otoPath)
	if err != nil {
		log.Fatal(err)
	}

	manifestPath := filepath.Join(outDir, "manifest.jsonl")
	manifestFile, err := os.Create(manifestPath)
	if err != nil {
		log.Fatal(err)
	}
	defer manifestFile.Close()

	writer := bufio.NewWriter(manifestFile)
	defer writer.Flush()

	cache := map[string]*audio.PCM{}
	index := 0
	aliases := bank.Aliases()
	for _, alias := range aliases {
		entries := bank.Entries[alias]
		for _, entry := range entries {
			pcm, err := loadPCM(cache, entry.Filename)
			if err != nil {
				if skipErrors {
					continue
				}
				log.Fatal(err)
			}

			trimmed, err := audio.TrimPCM(pcm, entry.Offset, entry.Fixed, entry.Blank)
			if err != nil {
				if skipErrors {
					continue
				}
				log.Fatal(err)
			}

			index++
			fileName := fmt.Sprintf("%06d.wav", index)
			outPath := filepath.Join(outDir, fileName)
			if err := audio.WriteWav(outPath, trimmed); err != nil {
				if skipErrors {
					continue
				}
				log.Fatal(err)
			}

			record := manifestEntry{
				Alias:        entry.Alias,
				Source:       entry.Filename,
				Trimmed:      fileName,
				Offset:       entry.Offset,
				Fixed:        entry.Fixed,
				Blank:        entry.Blank,
				Preutterance: entry.Preutterance,
				Overlap:      entry.Overlap,
				SampleRate:   trimmed.SampleRate,
				Channels:     trimmed.Channels,
				Frames:       len(trimmed.Data) / trimmed.Channels,
			}
			if err := writeJSONLine(writer, record); err != nil {
				if skipErrors {
					continue
				}
				log.Fatal(err)
			}
		}
	}

	fmt.Printf("wrote %s\n", manifestPath)
}

func loadPCM(cache map[string]*audio.PCM, path string) (*audio.PCM, error) {
	if pcm, ok := cache[path]; ok {
		return pcm, nil
	}
	pcm, err := audio.ReadWav(path)
	if err != nil {
		return nil, err
	}
	cache[path] = pcm
	return pcm, nil
}

func writeJSONLine(writer *bufio.Writer, record manifestEntry) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	if _, err := writer.WriteString("\n"); err != nil {
		return err
	}
	return nil
}
