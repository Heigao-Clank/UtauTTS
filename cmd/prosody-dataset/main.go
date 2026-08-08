package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"utautts/internal/prosody"
)

type failure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}
type report struct {
	Attempted int       `json:"attempted"`
	Written   int       `json:"written"`
	Failed    []failure `json:"failed"`
}

func main() {
	var root, output, reportPath string
	var limit int
	var workers int
	flag.StringVar(&root, "jsut", "data/jsut/basic5000", "JSUT basic5000 directory")
	flag.StringVar(&output, "out", "out/prosody/jsut.jsonl", "output dataset JSONL")
	flag.StringVar(&reportPath, "report", "", "failure report JSON (default: <out>.report.json)")
	flag.IntVar(&limit, "limit", 0, "maximum utterances (0 means all)")
	flag.IntVar(&workers, "workers", runtime.NumCPU(), "parallel extraction workers")
	flag.Parse()
	if reportPath == "" {
		reportPath = output + ".report.json"
	}

	entries, err := readTranscript(filepath.Join(root, "transcript_utf8.txt"))
	if err != nil {
		log.Fatal(err)
	}
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		log.Fatal(err)
	}
	file, err := os.Create(output)
	if err != nil {
		log.Fatal(err)
	}
	writer := bufio.NewWriter(file)
	summary := report{Attempted: len(entries)}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan extractionJob)
	results := make(chan extractionResult, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			for job := range jobs {
				wav := filepath.Join(root, "wav", job.entry.id+".wav")
				record, err := prosody.ExtractRecord(job.entry.id, job.entry.text, wav, prosody.ExtractConfig{})
				results <- extractionResult{job.index, job.entry.id, record, err}
			}
		}()
	}
	go func() {
		for index, entry := range entries {
			jobs <- extractionJob{index, entry}
		}
		close(jobs)
		group.Wait()
		close(results)
	}()
	pending := map[int]extractionResult{}
	next := 0
	for item := range results {
		pending[item.index] = item
		for {
			current, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			if current.err != nil {
				summary.Failed = append(summary.Failed, failure{current.id, current.err.Error()})
			} else if err := prosody.WriteJSONLine(writer, current.record); err != nil {
				log.Fatal(err)
			} else {
				summary.Written++
			}
			next++
			if next%100 == 0 {
				fmt.Printf("%d/%d (written %d)\n", next, len(entries), summary.Written)
			}
		}
	}
	if err := writer.Flush(); err != nil {
		log.Fatal(err)
	}
	if err := file.Close(); err != nil {
		log.Fatal(err)
	}
	data, _ := json.MarshalIndent(summary, "", "  ")
	data = append(data, '\n')
	if err := os.WriteFile(reportPath, data, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s: %d/%d utterances (%d failed)\n", output, summary.Written, summary.Attempted, len(summary.Failed))
}

type transcriptEntry struct{ id, text string }
type extractionJob struct {
	index int
	entry transcriptEntry
}
type extractionResult struct {
	index  int
	id     string
	record prosody.Record
	err    error
}

func readTranscript(path string) ([]transcriptEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var result []transcriptEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid transcript line %q", line)
		}
		result = append(result, transcriptEntry{parts[0], parts[1]})
	}
	return result, scanner.Err()
}
