package render

import (
	"reflect"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

func TestRenderIsDeterministicAndUsesAbsolutePlacement(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/unit.wav"
	data := make([]int16, 300)
	for i := range data {
		data[i] = 10000
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 200, Units: []plan.Unit{{
		Alias: "あ", Source: path, NoteStartMS: 100, DurationMS: 100,
		PreutteranceMS: 100, OverlapMS: 0,
	}}}
	first, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("render is not deterministic")
	}
	if len(first.Data) != 220 {
		t.Fatalf("frames = %d, want 220", len(first.Data))
	}
	if first.Data[0] != 0 || first.Data[100] == 0 || first.Data[len(first.Data)-1] != 0 {
		t.Fatalf("unexpected envelope: first=%d middle=%d last=%d", first.Data[0], first.Data[100], first.Data[len(first.Data)-1])
	}
}

func TestStretchPreservesPrefixAndLength(t *testing.T) {
	source := make([]float64, 200)
	for i := range source {
		source[i] = float64(i) / 200
	}
	got := stretchPreservingPrefix(source, 350, 50, 1000)
	if len(got) != 350 {
		t.Fatalf("length = %d", len(got))
	}
	if !reflect.DeepEqual(got[:50], source[:50]) {
		t.Fatal("protected prefix changed")
	}
}
