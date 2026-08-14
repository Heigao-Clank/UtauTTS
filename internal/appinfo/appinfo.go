package appinfo

import (
	_ "embed"
	"encoding/json"
)

//go:embed appinfo.json
var raw []byte

type Metadata struct {
	Name         string `json:"name"`
	Version      string `json:"version"`
	Organization string `json:"organization"`
	Repository   string `json:"repository"`
	License      string `json:"license"`
}

var metadata Metadata

func init() {
	if err := json.Unmarshal(raw, &metadata); err != nil {
		panic("invalid embedded application metadata: " + err.Error())
	}
}

func Name() string         { return metadata.Name }
func Version() string      { return metadata.Version }
func Organization() string { return metadata.Organization }
func Repository() string   { return metadata.Repository }
func License() string      { return metadata.License }
