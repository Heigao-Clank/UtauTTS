package render

import (
	"math"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
)

// writeSineWav writes a mono 16-bit sine wave at the given frequency.
func writeSineWav(t *testing.T, dir, name string, freq float64, ms float64, sampleRate int) string {
	t.Helper()
	frames := int(ms / 1000 * float64(sampleRate))
	data := make([]int16, frames)
	for i := range data {
		value := 0.6 * math.Sin(2*math.Pi*freq*float64(i)/float64(sampleRate))
		data[i] = int16(math.Round(value * 32767))
	}
	path := filepath.Join(dir, name)
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	return path
}

func singleUnitPlan(t *testing.T, sourcePath, tone string) *plan.Plan {
	t.Helper()
	return &plan.Plan{
		Version:    1,
		Tone:       tone,
		DurationMS: 320,
		Units: []plan.Unit{{
			Position: 0, Mora: "あ", Alias: "あ", Source: sourcePath,
			NoteStartMS: 0, DurationMS: 300,
			OffsetMS: 0, CutoffMS: 0, ConsonantMS: 0,
			PreutteranceMS: 0, OverlapMS: 0,
			PitchFactor: 1, EnergyFactor: 1,
		}},
	}
}

// TestRenderWaveformAppliesBaseTone pins the GUI's 基準音高 (tone) contract:
// the rendered output register must follow the requested tone instead of the
// raw recording pitch, matching what the USTX export promises OpenUtau
// (notes at the requested tone). G3 (196.0Hz) requested as C4 (261.6Hz) is
// within the waveform renderer's resample clamp (+5 semitones).
func TestRenderWaveformAppliesBaseTone(t *testing.T) {
	dir := t.TempDir()
	const sampleRate = 44100
	source := writeSineWav(t, dir, "g3.wav", 196.0, 400, sampleRate)

	cases := []struct {
		name string
		tone string
		want float64
	}{
		{"tone C4 shifts register up", "C4", 261.63},
		{"tone matching source is identity", "G3", 196.0},
		{"empty tone keeps legacy behavior", "", 196.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := renderWaveform(singleUnitPlan(t, source, tc.tone), Config{
				ReleaseMS:  20,
				ApplyPitch: false,
			})
			if err != nil {
				t.Fatal(err)
			}
			got := pitch.EstimateMedian(pcmFloats(output.Data), output.SampleRate)
			if math.Abs(got-tc.want) > tc.want*0.05 {
				t.Fatalf("output F0 = %.1fHz, want %.1fHz (tone %q)", got, tc.want, tc.tone)
			}
		})
	}
}

// TestBasePitchFactor covers the register-shift math directly, including the
// full octave case that the worldline renderers handle via the frq-based
// native engine.
func TestBasePitchFactor(t *testing.T) {
	cases := []struct {
		reference float64
		tone      string
		want      float64
	}{
		{130.81, "C4", 2.0}, // C3 source asked as C4: +12 semitones
		{261.63, "C4", 1.0}, // already at C4
		{261.63, "C3", 0.5}, // C4 source asked as C3: -12 semitones
		{196, "G3", 1.0},    // identity
		{196, "", 1.0},      // no tone requested: legacy behavior
		{196, "高さ", 1.0},    // unparsable tone: legacy behavior
	}
	for _, tc := range cases {
		got := basePitchFactor(tc.reference, tc.tone)
		if math.Abs(got-tc.want) > 0.01 {
			t.Errorf("basePitchFactor(%v, %q) = %v, want %v", tc.reference, tc.tone, got, tc.want)
		}
	}
}
