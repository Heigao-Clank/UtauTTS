package plan

import (
	"testing"

	"utautts/internal/frontend"
	"utautts/internal/oto"
	"utautts/internal/voicebank"
)

func TestBuildPlacesMoraeAndPause(t *testing.T) {
	morae, err := frontend.ParseKana("あ、んっ")
	if err != nil {
		t.Fatal(err)
	}
	bank := &voicebank.Bank{Root: "bank"}
	selections := []voicebank.Selection{
		{Position: 0, Mora: morae[0], Alias: "あ", Entry: oto.Entry{Filename: "a.wav"}},
		{Position: 2, Mora: morae[2], Alias: "ん", Entry: oto.Entry{Filename: "n.wav"}},
		{Position: 3, Mora: morae[3], Alias: "っ", Entry: oto.Entry{Filename: "cl.wav"}},
	}
	got, err := Build(bank, "あ、んっ", morae, selections, Config{MoraDurationMS: 100, PauseDurationMS: 200})
	if err != nil {
		t.Fatal(err)
	}
	if got.Units[1].NoteStartMS != 300 || got.Units[1].DurationMS != 90 {
		t.Fatalf("nasal unit = %+v", got.Units[1])
	}
	if got.Units[2].NoteStartMS != 390 || got.Units[2].DurationMS != 65 {
		t.Fatalf("closure unit = %+v", got.Units[2])
	}
	if got.DurationMS != 455 {
		t.Fatalf("duration = %v", got.DurationMS)
	}
}
