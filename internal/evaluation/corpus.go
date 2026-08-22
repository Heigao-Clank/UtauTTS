package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
)

// Corpusは客観評価と聴取評価に使う再現可能な発話集合。
type Corpus struct {
	Version int          `json:"version"`
	Name    string       `json:"name"`
	Cases   []CorpusCase `json:"cases"`
}

type CorpusCase struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Reading string   `json:"reading,omitempty"`
	Tags    []string `json:"tags,omitempty"`
}

func LoadCorpus(path string) (*Corpus, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var corpus Corpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	if corpus.Version != 1 || corpus.Name == "" || len(corpus.Cases) == 0 {
		return nil, fmt.Errorf("invalid evaluation corpus")
	}
	seen := map[string]bool{}
	for index, item := range corpus.Cases {
		if item.ID == "" || item.Text == "" {
			return nil, fmt.Errorf("case %d needs id and text", index)
		}
		if seen[item.ID] {
			return nil, fmt.Errorf("duplicate case id %q", item.ID)
		}
		seen[item.ID] = true
	}
	return &corpus, nil
}
