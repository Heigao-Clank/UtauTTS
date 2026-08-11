package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"utautts/internal/openutau"
)

type pathList []string

func (values *pathList) String() string         { return strings.Join(*values, ", ") }
func (values *pathList) Set(value string) error { *values = append(*values, value); return nil }

type report struct {
	Version  int                      `json:"version"`
	Projects []*openutau.ProjectAudit `json:"projects"`
}

func main() {
	var projects pathList
	var output string
	flag.Var(&projects, "project", "OpenUtau .ustx file to inspect (repeatable)")
	flag.StringVar(&output, "out", "", "optional JSON output path (default: stdout)")
	flag.Parse()
	if len(projects) == 0 {
		flag.Usage()
		log.Fatal("at least one --project is required")
	}

	result := report{Version: 1}
	for _, path := range projects {
		audit, err := openutau.InspectProject(path)
		if err != nil {
			log.Fatal(err)
		}
		result.Projects = append(result.Projects, audit)
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	data = append(data, '\n')
	if output == "" {
		fmt.Print(string(data))
		return
	}
	if err := os.WriteFile(output, data, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote OpenUtau audit to %s\n", output)
}
