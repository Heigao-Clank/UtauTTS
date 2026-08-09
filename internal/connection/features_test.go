package connection

import (
	"math"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/oto"
)

func TestBoundaryClampsFrameAtStartOfWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "start.wav")
	const sampleRate = 16000
	data := make([]int16, sampleRate/4)
	for index := range data {
		data[index] = int16(8000 * math.Sin(2*math.Pi*220*float64(index)/sampleRate))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	boundary := NewExtractor().Boundary(oto.Entry{Filename: path, Offset: 0, Preutterance: 10})
	if !boundary.Incoming.Valid {
		t.Fatal("incoming boundary at WAV start was not clamped to a valid frame")
	}
}

func TestPairMeasuresPitchSynchronousWaveformCorrelation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tone.wav")
	const sampleRate = 16000
	data := make([]int16, sampleRate/2)
	for index := range data {
		data[index] = int16(8000 * math.Sin(2*math.Pi*220*float64(index)/sampleRate))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	extractor := NewExtractor()
	left := oto.Entry{Filename: path, Offset: 20, Fixed: 60, Preutterance: 30, Overlap: 10}
	right := oto.Entry{Filename: path, Offset: 180, Fixed: 60, Preutterance: 30, Overlap: 10}
	features := extractor.Pair(left, right)
	if features.WaveformCorrelation < 0.95 {
		t.Fatalf("correlation=%f", features.WaveformCorrelation)
	}
}
