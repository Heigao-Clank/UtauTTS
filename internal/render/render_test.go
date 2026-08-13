package render

import (
	"math"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/pitch"
	"utautts/internal/plan"
)

func TestPackagedRuntimeCandidates(t *testing.T) {
	root := filepath.FromSlash("C:/package/UtauTTS")
	name := "worldline.dll"
	rootCandidates := packagedRuntimeCandidates(filepath.Join(root, "utautts.exe"), name)
	if want := filepath.Join(root, "runtime", name); !containsString(rootCandidates, want) {
		t.Fatalf("root candidates %v do not contain %q", rootCandidates, want)
	}
	toolCandidates := packagedRuntimeCandidates(filepath.Join(root, "tools", "utautts-cli.exe"), name)
	if want := filepath.Join(root, "runtime", name); !containsString(toolCandidates, want) {
		t.Fatalf("tool candidates %v do not contain shared runtime %q", toolCandidates, want)
	}
}

func TestFaithfulGPURendererIsRegistered(t *testing.T) {
	const backend = "openutau-classic-worldline-faithful-gpu"
	if !IsKnownRenderer(backend) {
		t.Fatalf("GPU faithful renderer %q is not registered", backend)
	}
}

func TestWorldlineR2RenderersAreRegisteredSeparately(t *testing.T) {
	for _, backend := range []string{"openutau-worldline-r2-cpu", "openutau-worldline-r2-directml"} {
		if !IsKnownRenderer(backend) {
			t.Fatalf("WORLDLINE-R2 renderer %q is not registered", backend)
		}
	}
}

func TestWorldlineR2RejectsNegativeDirectMLDevice(t *testing.T) {
	_, err := renderWorldlineR2(&plan.Plan{Units: []plan.Unit{{}}}, Config{OnnxDeviceID: -1}, true)
	if err == nil || !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("error = %v, want invalid DirectML device error", err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

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

func TestMinimumHandoffIsComplementaryWhenPreutteranceEqualsOverlap(t *testing.T) {
	timings := []effectiveTiming{{}, {preutteranceMS: 8, overlapMS: 8}}
	p := &plan.Plan{Units: []plan.Unit{{Position: 0}, {Position: 1, NoteStartMS: 100}}}
	const sampleRate = 1000
	start := 92
	for frame := start; frame <= start+6; frame++ {
		previous := handoffGain(frame, 0, p, timings, sampleRate)
		next := envelope(frame-start, 100, msToFrames(fadeInDurationMS(timings[1]), sampleRate), 0)
		if math.Abs(previous+next-1) > 1e-9 {
			t.Fatalf("non-complementary handoff at %d: previous=%f next=%f", frame, previous, next)
		}
	}
}

func TestFadeInDurationKeepsConfiguredLongCrossfade(t *testing.T) {
	if got := fadeInDurationMS(effectiveTiming{preutteranceMS: 60, overlapMS: 20}); got != 40 {
		t.Fatalf("fade-in duration=%f, want 40", got)
	}
}

func TestRenderRejectsUnknownBackend(t *testing.T) {
	_, err := Render(&plan.Plan{Units: []plan.Unit{{}}}, Config{Backend: "missing"})
	if err == nil {
		t.Fatal("unknown backend was accepted")
	}
}

func TestPitchCurveFactorInterpolatesInCents(t *testing.T) {
	curve := &PitchCurve{FrameMS: 10, Cents: []float64{0, 100}}
	want := math.Pow(2, 50.0/1200)
	if got := pitchCurveFactorAt(curve, 5); math.Abs(got-want) > 1e-12 {
		t.Fatalf("factor at midpoint = %.12f, want %.12f", got, want)
	}
	if got, end := pitchCurveFactorAt(curve, 100), math.Pow(2, 100.0/1200); math.Abs(got-end) > 1e-12 {
		t.Fatalf("factor after end = %.12f, want %.12f", got, end)
	}
}

func TestSmoothAndLimitPitchCurveDoesNotMutateAndLimitsSlope(t *testing.T) {
	source := &PitchCurve{FrameMS: 10, Cents: []float64{0, 80, -80, 80, 0}}
	result := smoothAndLimitPitchCurve(source, 20, 4)
	if source.Cents[1] != 80 || result == source {
		t.Fatal("pitch curve smoothing mutated its input")
	}
	for index := 1; index < len(result.Cents); index++ {
		if math.Abs(result.Cents[index]-result.Cents[index-1]) > 4.000001 {
			t.Fatalf("step %d is too steep: %f -> %f", index, result.Cents[index-1], result.Cents[index])
		}
	}
}

func TestRestoreRawHighBandIsComplementaryAndKeepsOnlyLowDifference(t *testing.T) {
	const sampleRate = 8000
	raw := &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: make([]int16, 800)}
	identical := &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: append([]int16(nil), raw.Data...)}
	if got := restoreRawHighBand(raw, identical, 500, 65); !reflect.DeepEqual(got.Data, raw.Data) {
		t.Fatal("identical raw/processed signals did not reconstruct exactly")
	}
	rmsFor := func(frequency float64) float64 {
		processed := &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: make([]int16, len(raw.Data))}
		for index := range processed.Data {
			processed.Data[index] = int16(math.Round(10000 * math.Sin(2*math.Pi*frequency*float64(index)/sampleRate)))
		}
		got := restoreRawHighBand(raw, processed, 500, 65)
		sum := 0.0
		for _, value := range got.Data[100:700] {
			sum += float64(value) * float64(value)
		}
		return math.Sqrt(sum / 600)
	}
	low, high := rmsFor(150), rmsFor(2000)
	if low < 5000 || high > low*0.1 {
		t.Fatalf("unexpected complementary filter response: low=%f high=%f", low, high)
	}
}

func TestOpenUtauEnvelopeUsesNextPhoneTailTiming(t *testing.T) {
	units := []plan.Unit{
		{NoteStartMS: 0, DurationMS: 100, PreutteranceMS: 30, OverlapMS: 5},
		{NoteStartMS: 100, DurationMS: 100, PreutteranceMS: 40, OverlapMS: 10},
	}
	timings, phraseStart := openUtauClassicTimings(units)
	if timings[0].tailIntrude != 40 || timings[0].tailOverlap != 10 || !timings[1].overlapped || phraseStart != -30 {
		t.Fatalf("timing = %+v %+v phraseStart=%.1f", timings[0], timings[1], phraseStart)
	}
	envelope := openUtauEnvelopeFromTiming(units[0], timings[0])
	wantX := []float64{-30, -25, 0, 60, 70}
	for index, want := range wantX {
		if envelope[index].XMS != want {
			t.Fatalf("envelope point %d x = %.1f, want %.1f", index, envelope[index].XMS, want)
		}
	}
}

func TestWaveformRendererRejectsFramePitchCurve(t *testing.T) {
	_, err := Render(&plan.Plan{Units: []plan.Unit{{}}}, Config{Backend: "waveform", PitchCurve: &PitchCurve{FrameMS: 5, Cents: []float64{0}}})
	if err == nil || !strings.Contains(err.Error(), "not supported by waveform") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderRejectsNonFinitePitchCurve(t *testing.T) {
	for _, curve := range []*PitchCurve{
		{FrameMS: math.NaN(), Cents: []float64{0}},
		{FrameMS: 5, Cents: []float64{math.Inf(1)}},
	} {
		_, err := Render(&plan.Plan{Units: []plan.Unit{{}}}, Config{Backend: "worldline", PitchCurve: curve})
		if err == nil {
			t.Fatalf("accepted non-finite curve: %+v", curve)
		}
	}
}

func TestRenderRejectsUnsafePitchCurveRangeAndFrame(t *testing.T) {
	for _, curve := range []*PitchCurve{
		{FrameMS: 0.01, Cents: []float64{0}},
		{FrameMS: 5, Cents: []float64{4801}},
		{FrameMS: 5, Cents: []float64{-4801}},
	} {
		_, err := Render(&plan.Plan{Units: []plan.Unit{{}}}, Config{Backend: "waveform-openutau-pitch", PitchCurve: curve})
		if err == nil {
			t.Fatalf("accepted unsafe curve: %+v", curve)
		}
	}
}

func TestPitchCurveHasShift(t *testing.T) {
	if pitchCurveHasShift(&PitchCurve{FrameMS: 5, Cents: []float64{0, 0}}) {
		t.Fatal("flat curve was treated as shifted")
	}
	if !pitchCurveHasShift(&PitchCurve{FrameMS: 5, Cents: []float64{0, 25}}) {
		t.Fatal("non-flat curve was not treated as shifted")
	}
}

func TestWaveformOpenUtauPitchFlatCurveIsExactWaveformBypass(t *testing.T) {
	path := t.TempDir() + "/unit.wav"
	data := make([]int16, 400)
	for i := range data {
		data[i] = int16(9000 * math.Sin(2*math.Pi*0.02*float64(i)))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	makePlan := func() *plan.Plan {
		return &plan.Plan{DurationMS: 180, Units: []plan.Unit{{
			Alias: "a", Source: path, NoteStartMS: 0, DurationMS: 180,
			ConsonantMS: 40, PreutteranceMS: 20, PitchFactor: 1, EnergyFactor: 1,
		}}}
	}
	waveform, err := Render(makePlan(), Config{Backend: "waveform", ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	hybrid, err := Render(makePlan(), Config{
		Backend: "waveform-openutau-pitch", ReleaseMS: 20,
		PitchCurve: &PitchCurve{FrameMS: 5, Cents: []float64{0, 0, 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(hybrid, waveform) {
		t.Fatal("flat hybrid output differs from waveform reference")
	}
}

func TestStableVowelFramesExcludeFixedConsonantAndNoteTail(t *testing.T) {
	unit := plan.Unit{NoteStartMS: 100, DurationMS: 140, PreutteranceMS: 60, ConsonantMS: 100}
	start, end, ok := stableVowelFrames([]plan.Unit{unit}, 0, 20, 1000, 400, 400)
	if !ok || start != 152 || end != 228 {
		t.Fatalf("stable vowel = %d..%d ok=%v, want 152..228", start, end, ok)
	}
}

func TestStableVowelFramesStopBeforeNextHandoff(t *testing.T) {
	units := []plan.Unit{
		{NoteStartMS: 100, DurationMS: 240, PreutteranceMS: 35, ConsonantMS: 70},
		{NoteStartMS: 340, DurationMS: 240, PreutteranceMS: 35, ConsonantMS: 70},
	}
	start, end, ok := stableVowelFrames(units, 0, 20, 1000, 700, 700)
	if !ok || start != 147 || end != 305 {
		t.Fatalf("stable vowel = %d..%d ok=%v, want 147..305", start, end, ok)
	}
}

func TestBestBranchLagFindsLocalAlignment(t *testing.T) {
	baseline := make([]float64, 300)
	processed := make([]float64, 300)
	for i := 0; i < 300; i++ {
		baseline[i] = math.Sin(2 * math.Pi * float64(i) / 23)
	}
	copy(processed[5:], baseline[:295])
	lag, correlation := bestBranchLag(baseline, processed, 80, 220, 1000, false)
	if lag != 5 || correlation < 0.99 {
		t.Fatalf("lag=%d correlation=%f, want lag 5 with high correlation", lag, correlation)
	}
}

func TestBestConstantBranchLagDoesNotWarpVowelInterior(t *testing.T) {
	baseline := make([]float64, 300)
	processed := make([]float64, 300)
	for i := range baseline {
		baseline[i] = math.Sin(2 * math.Pi * float64(i) / 23)
	}
	copy(processed[5:], baseline[:295])
	lag, correlation := bestConstantBranchLag(baseline, processed, 80, 220, 1000)
	if lag != 5 || correlation < 0.99 {
		t.Fatalf("lag=%d correlation=%f, want one constant lag 5", lag, correlation)
	}
	for frame := 80; frame < 220; frame++ {
		if processed[frame+lag] != baseline[frame] {
			t.Fatalf("constant alignment warped frame %d", frame)
		}
	}
}

func TestOpenUtauPitchRegionsKeepPhoneBoundariesRawAndReleaseProcessed(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{
		{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50},
		{NoteStartMS: 240, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50},
	}
	baseline := make([]float64, msToFrames(500, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		processed[index] = 0.2 * math.Sin(2*math.Pi*230*float64(index)/sampleRate)
	}
	regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed)
	if len(regions) != 2 || regions[0].end >= regions[1].start || regions[1].end != len(baseline) || regions[1].fadeOut {
		t.Fatalf("regions=%+v, want raw phone boundary and final processed release", regions)
	}
}

func TestOpenUtauPitchRegionsKeepUnvoicedGapRaw(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{
		{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50},
		{NoteStartMS: 240, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50},
	}
	baseline := make([]float64, msToFrames(500, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		value := 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		baseline[index], processed[index] = value, value
	}
	for index := msToFrames(230, sampleRate); index < msToFrames(270, sampleRate); index++ {
		baseline[index], processed[index] = 0, 0
	}
	regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed)
	if len(regions) != 2 {
		t.Fatalf("regions=%+v, want unvoiced gap to split regions", regions)
	}
}

func TestOpenUtauPitchRegionsDoNotHideBriefMissingConsonant(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{
		{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50},
		{NoteStartMS: 240, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50},
	}
	baseline := make([]float64, msToFrames(500, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		value := 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		baseline[index], processed[index] = value, value
	}
	// Simulate a short consonant that the processed branch has dropped. A
	// whole-gap RMS check would be dominated by the vowels on either side.
	for index := msToFrames(238, sampleRate); index < msToFrames(250, sampleRate); index++ {
		baseline[index] = 0.12 * math.Sin(2*math.Pi*3000*float64(index)/sampleRate)
		processed[index] = 0
	}
	regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed)
	if len(regions) != 2 {
		t.Fatalf("regions=%+v, want brief dropped consonant to split regions", regions)
	}
}

func TestOpenUtauPitchRegionsRejectCandidateWithBriefProcessedDropout(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50}}
	baseline := make([]float64, msToFrames(260, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		processed[index] = 0.18 * math.Sin(2*math.Pi*230*float64(index)/sampleRate)
	}
	// The processed vowel loses only 12 ms. A whole-vowel RMS still looks
	// healthy, but exposing it would produce the reported missing-phone sound.
	for index := msToFrames(130, sampleRate); index < msToFrames(142, sampleRate); index++ {
		processed[index] = 0
	}
	if regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed); len(regions) != 0 {
		t.Fatalf("regions=%+v, want dropped vowel candidate to remain raw", regions)
	}
}

func TestOpenUtauPitchRegionsAcceptCandidateWithModerateLevelDifference(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50}}
	baseline := make([]float64, msToFrames(260, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		processed[index] = 0.15 * math.Sin(2*math.Pi*230*float64(index)/sampleRate)
	}
	if regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed); len(regions) != 1 {
		t.Fatalf("regions=%+v, want intact lower-level vowel candidate accepted", regions)
	}
}

func TestOpenUtauPitchRegionsRejectCandidateWithExcessProcessedEnergy(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50}}
	baseline := make([]float64, msToFrames(260, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		processed[index] = 0.18 * math.Sin(2*math.Pi*230*float64(index)/sampleRate)
	}
	for index := msToFrames(130, sampleRate); index < msToFrames(142, sampleRate); index++ {
		processed[index] *= 2.5
	}
	if regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed); len(regions) != 0 {
		t.Fatalf("regions=%+v, want locally excessive processed candidate to remain raw", regions)
	}
}

func TestOpenUtauPitchRegionsDoNotMergeAcrossRejectedCandidate(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{
		{NoteStartMS: 0, DurationMS: 200, PreutteranceMS: 20, ConsonantMS: 40},
		{NoteStartMS: 200, DurationMS: 200, PreutteranceMS: 20, ConsonantMS: 40},
		{NoteStartMS: 400, DurationMS: 200, PreutteranceMS: 20, ConsonantMS: 40},
	}
	baseline := make([]float64, msToFrames(620, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		processed[index] = 0.18 * math.Sin(2*math.Pi*230*float64(index)/sampleRate)
	}
	for index := msToFrames(290, sampleRate); index < msToFrames(302, sampleRate); index++ {
		processed[index] = 0
	}
	regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed)
	if len(regions) != 2 {
		t.Fatalf("regions=%+v, want good vowels on either side of rejected candidate kept separate", regions)
	}
}

func TestOpenUtauPitchRegionsDoNotExtendIntoDroppedRelease(t *testing.T) {
	const sampleRate = 16000
	units := []plan.Unit{{NoteStartMS: 0, DurationMS: 240, PreutteranceMS: 20, ConsonantMS: 50}}
	baseline := make([]float64, msToFrames(280, sampleRate))
	processed := make([]float64, len(baseline))
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*220*float64(index)/sampleRate)
		processed[index] = 0.18 * math.Sin(2*math.Pi*230*float64(index)/sampleRate)
	}
	for index := msToFrames(245, sampleRate); index < len(processed); index++ {
		processed[index] = 0
	}
	regions := openUtauPitchRegions(units, 20, sampleRate, baseline, processed)
	if len(regions) != 1 || regions[0].end == len(baseline) || !regions[0].fadeOut {
		t.Fatalf("regions=%+v, want dropped release left as raw waveform", regions)
	}
}

func TestBoundaryBridgeRequiresWaveformRenderer(t *testing.T) {
	_, err := Render(&plan.Plan{Units: []plan.Unit{{}}}, Config{Backend: "worldline", BoundaryBridgeMS: 20})
	if err == nil {
		t.Fatal("boundary bridge was accepted by non-waveform renderer")
	}
}

func TestWaveformOpenUtauPitchAcceptsBoundaryBridge(t *testing.T) {
	path := t.TempDir() + "/unit.wav"
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: make([]int16, 300)}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 100, Units: []plan.Unit{{Source: path, DurationMS: 100, PitchFactor: 1}}}
	if _, err := Render(p, Config{Backend: "waveform-openutau-pitch", BoundaryBridgeMS: 20}); err != nil {
		t.Fatal(err)
	}
}

func TestRenderAllowsSilentClosureUnit(t *testing.T) {
	path := t.TempDir() + "/unit.wav"
	data := make([]int16, 200)
	for index := range data {
		data[index] = 8000
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 265, Units: []plan.Unit{
		{Position: 0, Alias: "あ", Source: path, NoteStartMS: 0, DurationMS: 100},
		{Position: 1, Alias: "<closure>", Silent: true, NoteStartMS: 100, DurationMS: 65},
		{Position: 2, Alias: "か", Source: path, NoteStartMS: 165, DurationMS: 100},
	}}
	pcm, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	if pcm.Data[140] != 0 {
		t.Fatalf("closure midpoint=%d, want silence", pcm.Data[140])
	}
}

func TestWaveformLongCollapsesConsecutiveSourceEntries(t *testing.T) {
	path := t.TempDir() + "/continuous.wav"
	data := make([]int16, 1000)
	for index := range data {
		data[index] = int16(7000 * math.Sin(2*math.Pi*0.02*float64(index)))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 280, Units: []plan.Unit{
		{Position: 0, Alias: "a か", Source: path, OtoPath: "oto.ini", OtoLine: 10, NoteStartMS: 0, DurationMS: 140, OffsetMS: 0, ConsonantMS: 80, PreutteranceMS: 40, PitchFactor: 1, EnergyFactor: 1},
		{Position: 1, Alias: "a き", Source: path, OtoPath: "oto.ini", OtoLine: 11, NoteStartMS: 140, DurationMS: 140, OffsetMS: 300, ConsonantMS: 80, PreutteranceMS: 40, PitchFactor: 1, EnergyFactor: 1},
	}}
	pcm, err := Render(p, Config{Backend: "waveform-long", ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(pcm.Data) < 280 || p.Units[0].LongUnitGroup != 1 || p.Units[1].LongUnitGroup != 1 || p.Units[0].LongUnitSize != 2 {
		t.Fatalf("long-unit audit missing: units=%+v frames=%d", p.Units, len(pcm.Data))
	}
}

func TestWorldlineF0CurveInterpolatesInLogFrequency(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{NoteStartMS: 0}, {NoteStartMS: 100}}}
	curve := worldlineF0Curve(p, []float64{200, 400}, []float64{1, 1}, 220, 11)
	if math.Abs(curve[0]-200) > 0.01 || math.Abs(curve[10]-400) > 0.01 {
		t.Fatalf("curve endpoints = %.2f..%.2f", curve[0], curve[10])
	}
	if math.Abs(curve[5]-math.Sqrt(200*400)) > 0.1 {
		t.Fatalf("log midpoint = %.2f", curve[5])
	}
}

func TestWorldlineF0CurveAppliesLearnedPitchFactors(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{NoteStartMS: 0}, {NoteStartMS: 100}}}
	curve := worldlineF0Curve(p, []float64{200, 200}, []float64{1.03, 0.97}, 200, 11)
	if math.Abs(curve[0]-206) > 0.01 || math.Abs(curve[10]-194) > 0.01 {
		t.Fatalf("factored curve endpoints = %.2f..%.2f", curve[0], curve[10])
	}
}

func TestWorldlineLocalF0CurveDoesNotGlideBetweenRecordings(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{NoteStartMS: 0}, {NoteStartMS: 100}}}
	curve := worldlineLocalF0Curve(p, []float64{200, 400}, []float64{1.03, 0.97}, 220, 11)
	for frame := 0; frame < 10; frame++ {
		if math.Abs(curve[frame]-206) > 0.01 {
			t.Fatalf("frame %d = %.2f, want local 206Hz baseline", frame, curve[frame])
		}
	}
	if math.Abs(curve[10]-388) > 0.01 {
		t.Fatalf("next unit = %.2f, want local 388Hz baseline", curve[10])
	}
}

func TestDirectConsonantWeightsRestoreOnlyAperiodicFixedRegion(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{
		Position: 0, NoteStartMS: 100, PreutteranceMS: 40, ConsonantMS: 80, DurationMS: 140,
	}}}
	baseline := make([]float64, 300)
	state := uint32(1)
	for i := range baseline {
		state = state*1664525 + 1013904223
		baseline[i] = (float64(state>>8)/float64(1<<24) - 0.5) * 0.4
	}
	weights := directConsonantWeights(p, 20, 1000, len(baseline), baseline, make([]float64, len(baseline)), cvRestoreNone, false)
	if weights[20] != 0 || weights[200] != 0 {
		t.Fatalf("direct audio leaked outside fixed region: %.2f %.2f", weights[20], weights[200])
	}
	if weights[100] == 0 {
		t.Fatal("aperiodic consonant was not restored")
	}
}

func TestDirectConsonantWeightsCanForceCVFixedRegion(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{
		Position: 0, Alias: "か", NoteStartMS: 100, PreutteranceMS: 40, ConsonantMS: 80, DurationMS: 140,
	}}}
	const sampleRate = 8000
	baseline := make([]float64, sampleRate/2)
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*200*float64(index)/sampleRate)
	}
	standard := directConsonantWeights(p, 20, sampleRate, len(baseline), baseline, baseline, cvRestoreNone, false)
	forced := directConsonantWeights(p, 20, sampleRate, len(baseline), baseline, baseline, cvRestoreFull, false)
	center := msToFrames(100, sampleRate)
	if standard[center] != 0 || forced[center] != 1 {
		t.Fatalf("standard=%f forced=%f", standard[center], forced[center])
	}
}

func TestDirectConsonantWeightsBalancedCVStopsBeforeVowelTail(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{
		Position: 0, Alias: "あ", NoteStartMS: 100, PreutteranceMS: 40, ConsonantMS: 100, DurationMS: 140,
	}}}
	const sampleRate = 8000
	baseline := make([]float64, sampleRate/2)
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*200*float64(index)/sampleRate)
	}
	balanced := directConsonantWeights(p, 20, sampleRate, len(baseline), baseline, baseline, cvRestoreBalanced, false)
	attack := msToFrames(90, sampleRate)
	vowelTail := msToFrames(145, sampleRate)
	if math.Abs(balanced[attack]-0.85) > 0.001 {
		t.Fatalf("attack weight=%f, want 0.85", balanced[attack])
	}
	if balanced[vowelTail] != 0 {
		t.Fatalf("vowel tail weight=%f, want 0", balanced[vowelTail])
	}
}

func TestDirectConsonantWeightsGentleCVUsesIntermediateAttackWeight(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{
		Position: 0, Alias: "あ", NoteStartMS: 100, PreutteranceMS: 40, ConsonantMS: 100, DurationMS: 140,
	}}}
	const sampleRate = 8000
	baseline := make([]float64, sampleRate/2)
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*200*float64(index)/sampleRate)
	}
	gentle := directConsonantWeights(p, 20, sampleRate, len(baseline), baseline, baseline, cvRestoreGentle, false)
	attack := msToFrames(90, sampleRate)
	if math.Abs(gentle[attack]-0.55) > 0.001 {
		t.Fatalf("gentle attack weight=%f, want 0.55", gentle[attack])
	}
}

func TestDirectConsonantWeightsBalancedAvoidsShiftedPeriodicVowel(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{
		Position: 0, Alias: "か", NoteStartMS: 100, PreutteranceMS: 40, ConsonantMS: 100, DurationMS: 140,
		PitchFactor: 1.03, IntonationFactor: 1.02,
	}}}
	const sampleRate = 8000
	baseline := make([]float64, sampleRate/2)
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*200*float64(index)/sampleRate)
	}
	balanced := directConsonantWeights(p, 20, sampleRate, len(baseline), baseline, baseline, cvRestoreBalanced, false)
	beforeBoundary := msToFrames(98, sampleRate)
	afterTransition := msToFrames(105, sampleRate)
	if balanced[beforeBoundary] == 0 {
		t.Fatal("shifted CV consonant attack was not protected")
	}
	if balanced[afterTransition] != 0 {
		t.Fatalf("shifted raw vowel leaked after transition: %f", balanced[afterTransition])
	}
}

func TestDirectConsonantWeightsBalancedRespectsFramePitchShift(t *testing.T) {
	p := &plan.Plan{Units: []plan.Unit{{
		Position: 0, Alias: "か", NoteStartMS: 100, PreutteranceMS: 40, ConsonantMS: 100, DurationMS: 140,
		PitchFactor: 1, IntonationFactor: 1,
	}}}
	const sampleRate = 8000
	baseline := make([]float64, sampleRate/2)
	for index := range baseline {
		baseline[index] = 0.2 * math.Sin(2*math.Pi*200*float64(index)/sampleRate)
	}
	balanced := directConsonantWeights(p, 20, sampleRate, len(baseline), baseline, baseline, cvRestoreBalanced, true)
	if got := balanced[msToFrames(105, sampleRate)]; got != 0 {
		t.Fatalf("frame-shifted raw vowel leaked after transition: %f", got)
	}
}

func TestNormalizeTimingCompressesLongVCVAndKeepsVowelTail(t *testing.T) {
	unit := plan.Unit{DurationMS: 140, PreutteranceMS: 360, OverlapMS: 120, ConsonantMS: 439}
	got := normalizeTiming(unit, 20)
	if math.Abs(got.preutteranceMS-105) > 0.001 {
		t.Fatalf("preutterance = %.3f, want 105", got.preutteranceMS)
	}
	if math.Abs(got.overlapMS-35) > 0.001 {
		t.Fatalf("overlap = %.3f, want 35", got.overlapMS)
	}
	if got.consonantMS >= got.preutteranceMS+unit.DurationMS+20-(20+49) {
		t.Fatalf("consonant %.3f leaves no guaranteed vowel tail", got.consonantMS)
	}
}

func TestNormalizeTimingLeavesOrdinaryBankAlone(t *testing.T) {
	unit := plan.Unit{DurationMS: 140, PreutteranceMS: 60, OverlapMS: 20, ConsonantMS: 100}
	got := normalizeTiming(unit, 20)
	if got.preutteranceMS != 60 || got.overlapMS != 20 || got.consonantMS != 100 || got.scale != 1 {
		t.Fatalf("ordinary timing changed: %#v", got)
	}
}

func TestRetimeCompressedPrefixRetainsVowelTail(t *testing.T) {
	source := make([]float64, 600)
	for i := 439; i < len(source); i++ {
		source[i] = 1
	}
	got := retimeWithCompressedPrefix(source, 265, 439, 128, 1000)
	if len(got) != 265 {
		t.Fatalf("length = %d", len(got))
	}
	voiced := 0
	for _, value := range got[128:] {
		if value > 0.5 {
			voiced++
		}
	}
	if voiced < 100 {
		t.Fatalf("vowel tail only has %d frames", voiced)
	}
	maxDelta := 0.0
	maxDeltaAt := 0
	for i := 1; i < len(got); i++ {
		if delta := math.Abs(got[i] - got[i-1]); delta > maxDelta {
			maxDelta, maxDeltaAt = delta, i
		}
	}
	if maxDelta > 0.25 {
		t.Fatalf("compressed-prefix join clicks: max delta %.3f at %d", maxDelta, maxDeltaAt)
	}
}

func TestRenderLongVCVUsesWeightedCrossfade(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/vcv.wav"
	data := make([]int16, 800)
	for i := range data {
		data[i] = 10000
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 280, Units: []plan.Unit{
		{Alias: "a", Source: path, NoteStartMS: 0, DurationMS: 140, PreutteranceMS: 360, OverlapMS: 120, ConsonantMS: 439},
		{Alias: "b", Source: path, NoteStartMS: 140, DurationMS: 140, PreutteranceMS: 360, OverlapMS: 120, ConsonantMS: 439},
	}}
	pcm, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	peak := int16(0)
	for _, value := range pcm.Data {
		if value > peak {
			peak = value
		}
	}
	if peak > 10010 {
		t.Fatalf("overlap was additively mixed: peak=%d", peak)
	}
	if p.Units[1].EffectivePreutteranceMS != 105 || p.Units[1].EffectiveOverlapMS != 35 {
		t.Fatalf("effective timing not recorded: %#v", p.Units[1])
	}
}

func TestConnectedUnitsUseComplementaryHandoff(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/unit.wav"
	data := make([]int16, 400)
	for i := range data {
		data[i] = 10000
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 1000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 280, Units: []plan.Unit{
		{Position: 0, Alias: "a", Source: path, NoteStartMS: 0, DurationMS: 140, PreutteranceMS: 60, OverlapMS: 20, ConsonantMS: 100},
		{Position: 1, Alias: "b", Source: path, NoteStartMS: 140, DurationMS: 140, PreutteranceMS: 60, OverlapMS: 20, ConsonantMS: 100},
	}}
	pcm, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	peak := int16(0)
	for _, value := range pcm.Data {
		if value > peak {
			peak = value
		}
	}
	if peak > 10010 {
		t.Fatalf("connected units were layered: peak=%d", peak)
	}
	for frame := 85; frame < 120; frame++ {
		if pcm.Data[frame] < 9500 {
			t.Fatalf("handoff dipped at frame %d: %d", frame, pcm.Data[frame])
		}
	}
}

func TestAnalyzeIntonationMeasuresAndLimitsCorrection(t *testing.T) {
	dir := t.TempDir()
	paths := make([]string, 3)
	for index, hz := range []float64{200, 220, 240} {
		paths[index] = dir + "/tone" + string(rune('0'+index)) + ".wav"
		data := make([]int16, 8000)
		for i := range data {
			data[i] = int16(6000 * math.Sin(2*math.Pi*hz*float64(i)/16000))
		}
		if err := audio.WriteWav(paths[index], &audio.PCM{SampleRate: 16000, Channels: 1, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	p := &plan.Plan{Units: []plan.Unit{
		{Position: 0, Source: paths[0]}, {Position: 1, Source: paths[1]}, {Position: 2, Source: paths[2]},
	}}
	timings := []effectiveTiming{{scale: 1}, {scale: 1}, {scale: 1}}
	factors := analyzeIntonation(p, timings, &sourceCache{}, 1)
	if len(factors) != 3 {
		t.Fatalf("factor count = %d", len(factors))
	}
	for i, factor := range factors {
		if factor < 0.92 || factor > 1.08 {
			t.Fatalf("factor %d out of bounds: %f", i, factor)
		}
		if p.Units[i].SourceF0Hz == 0 || p.Units[i].TargetF0Hz == 0 {
			t.Fatalf("missing F0 audit at %d: %#v", i, p.Units[i])
		}
	}
	if p.Units[2].TargetF0Hz >= p.Units[1].TargetF0Hz {
		t.Fatalf("phrase does not fall: %#v", p.Units)
	}
}

func TestSourceCacheReusesMonoAndNormalizedAudio(t *testing.T) {
	path := t.TempDir() + "/stereo.wav"
	data := []int16{1000, -1000, 2000, -2000, 3000, -3000, 4000, -4000}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 16000, Channels: 2, Data: data}); err != nil {
		t.Fatal(err)
	}

	cache := sourceCache{}
	first, err := cache.loadMono(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cache.loadMono(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("mono source was converted more than once")
	}
	if first.Channels != 1 || len(first.Data) != len(data)/2 {
		t.Fatalf("unexpected mono source: %#v", first)
	}

	native, err := cache.loadNormalized(path, 16000)
	if err != nil {
		t.Fatal(err)
	}
	resampled, err := cache.loadNormalized(path, 8000)
	if err != nil {
		t.Fatal(err)
	}
	resampledAgain, err := cache.loadNormalized(path, 8000)
	if err != nil {
		t.Fatal(err)
	}
	if native != first || resampled != resampledAgain {
		t.Fatal("normalized source cache did not reuse its entries")
	}
	if resampled.SampleRate != 8000 || len(resampled.Data) != len(first.Data)/2 {
		t.Fatalf("unexpected resampled source: %#v", resampled)
	}
}

func TestAnalyzeIntonationAuditIncludesLearnedPitchFactor(t *testing.T) {
	path := t.TempDir() + "/tone.wav"
	data := make([]int16, 8000)
	for i := range data {
		data[i] = int16(6000 * math.Sin(2*math.Pi*200*float64(i)/16000))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 16000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{Units: []plan.Unit{{Position: 0, Source: path, PitchFactor: 1.03}}}
	factors := analyzeIntonation(p, []effectiveTiming{{scale: 1}}, &sourceCache{}, 1)
	if math.Abs(p.Units[0].TargetF0Hz-p.Units[0].SourceF0Hz*factors[0]*1.03) > 0.1 {
		t.Fatalf("target F0=%f source=%f", p.Units[0].TargetF0Hz, p.Units[0].SourceF0Hz)
	}
}

func TestRenderPitchFactorRequiresExplicitMode(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/tone.wav"
	data := make([]int16, 8000)
	for i := range data {
		data[i] = int16(6000 * math.Sin(2*math.Pi*200*float64(i)/16000))
	}
	if err := audio.WriteWav(path, &audio.PCM{SampleRate: 16000, Channels: 1, Data: data}); err != nil {
		t.Fatal(err)
	}
	p := &plan.Plan{DurationMS: 300, Units: []plan.Unit{{Position: 0, Source: path, DurationMS: 300, PitchFactor: 1.05, EnergyFactor: 1}}}
	pcm, err := Render(p, Config{ReleaseMS: 20})
	if err != nil {
		t.Fatal(err)
	}
	baseline := pitch.EstimateMedian(pcmFloats(pcm.Data[1600:4000]), pcm.SampleRate)
	if math.Abs(baseline-200) > 5 {
		t.Fatalf("default F0 = %.2f, want about 200", baseline)
	}
	pcm, err = Render(p, Config{ReleaseMS: 20, ApplyPitch: true})
	if err != nil {
		t.Fatal(err)
	}
	shifted := pitch.EstimateMedian(pcmFloats(pcm.Data[1600:4000]), pcm.SampleRate)
	if math.Abs(shifted-210) > 5 {
		t.Fatalf("explicitly shifted F0 = %.2f, want about 210", shifted)
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

func TestBridgeEnvelopeIsBoundedAndFadesAtEdges(t *testing.T) {
	if got := bridgeEnvelope(0, 9); got != 0 {
		t.Fatalf("start envelope = %f, want 0", got)
	}
	if got := bridgeEnvelope(8, 9); got != 0 {
		t.Fatalf("end envelope = %f, want 0", got)
	}
	peak := 0.0
	for frame := 0; frame < 9; frame++ {
		value := bridgeEnvelope(frame, 9)
		if value < 0 || value > 1 {
			t.Fatalf("envelope[%d] = %f, want [0, 1]", frame, value)
		}
		peak = math.Max(peak, value)
	}
	if peak < 0.99 {
		t.Fatalf("envelope peak = %f, want near 1", peak)
	}
}

func TestBestAlignedVowelSegmentFindsPhaseShift(t *testing.T) {
	unit := renderedUnit{
		unit:   plan.Unit{DurationMS: 80},
		timing: effectiveTiming{preutteranceMS: 20, consonantMS: 40},
		wave:   make([]float64, 120),
	}
	for index := range unit.wave {
		unit.wave[index] = math.Sin(0.013 * float64(index*index))
	}
	target := append([]float64(nil), unit.wave[75:95]...)
	got, lag, correlation := bestAlignedVowelSegment(unit, target, 20, 1000)
	if len(got) != 20 || lag != -5 || correlation < 0.999 {
		t.Fatalf("aligned segment len=%d lag=%d correlation=%f", len(got), lag, correlation)
	}
}

func TestStabilizeWorldlinePitchesCorrectsHarmonicJump(t *testing.T) {
	got := stabilizeWorldlinePitches([]float64{296, 446, 298})
	if math.Abs(got[1]-297.333333) > 2 {
		t.Fatalf("stabilized harmonic pitch = %.2f, want near 297", got[1])
	}
}

func TestStabilizeWorldlinePitchesKeepsLowerShortPhraseAnchor(t *testing.T) {
	got := stabilizeWorldlinePitches([]float64{296, 446})
	if math.Abs(got[0]-296) > 0.01 || math.Abs(got[1]-297.333333) > 2 {
		t.Fatalf("short phrase pitches = %#v, want near [296, 297]", got)
	}
}

func TestStabilizeWorldlinePitchesKeepsOrdinaryMovement(t *testing.T) {
	input := []float64{280, 296, 315}
	got := stabilizeWorldlinePitches(input)
	for index := range input {
		if got[index] != input[index] {
			t.Fatalf("ordinary pitch[%d] changed from %.2f to %.2f", index, input[index], got[index])
		}
	}
}

func TestChooseBoundaryRepairKeepsNormalOrImprovesPeak(t *testing.T) {
	const sampleRate = 1000
	mix := make([]float64, 220)
	weights := make([]float64, len(mix))
	previousWave := make([]float64, 120)
	for index := range mix {
		mix[index] = 0.2 * math.Sin(2*math.Pi*float64(index)/20)
		weights[index] = 1
	}
	for index := range previousWave {
		previousWave[index] = 0.2 * math.Sin(2*math.Pi*float64(index)/20)
	}
	// A local impulse represents a boundary transient. The aligned source tail
	// should reduce it; otherwise normal connection remains the fallback.
	mix[110] += 0.8
	previous := renderedUnit{
		unit: plan.Unit{DurationMS: 80}, timing: effectiveTiming{preutteranceMS: 20}, wave: previousWave,
	}
	current := renderedUnit{startFrame: 100, fadeInFrames: 20}
	choice := chooseBoundaryRepair(mix, weights, previous, current, 20, sampleRate)
	if !choice.applied {
		t.Fatal("clear transient did not select an improving repair")
	}
	if choice.selected.peak >= choice.baseline.peak {
		t.Fatalf("selected peak=%f baseline=%f", choice.selected.peak, choice.baseline.peak)
	}
}

func TestWaveformBoundaryBridgeIsOptionalAndAudited(t *testing.T) {
	dir := t.TempDir()
	paths := []string{dir + "/left.wav", dir + "/right.wav"}
	for fileIndex, frequency := range []float64{180, 260} {
		data := make([]int16, 6400)
		for frame := range data {
			data[frame] = int16(7000 * math.Sin(2*math.Pi*frequency*float64(frame)/16000))
		}
		if err := audio.WriteWav(paths[fileIndex], &audio.PCM{SampleRate: 16000, Channels: 1, Data: data}); err != nil {
			t.Fatal(err)
		}
	}
	base := &plan.Plan{DurationMS: 200, Units: []plan.Unit{
		{Position: 0, Mora: "a", Source: paths[0], NoteStartMS: 0, DurationMS: 100, ConsonantMS: 10, PreutteranceMS: 40},
		{Position: 1, Mora: "i", Source: paths[1], NoteStartMS: 100, DurationMS: 100, ConsonantMS: 10, PreutteranceMS: 40},
	}}
	if _, err := Render(base, Config{ReleaseMS: 20}); err != nil {
		t.Fatal(err)
	}
	if len(base.BoundaryBridges) != 0 || base.BoundaryBridgeMS != 0 {
		t.Fatalf("disabled bridge changed plan: %#v", base)
	}

	experiment := &plan.Plan{DurationMS: 200, Units: append([]plan.Unit(nil), base.Units...)}
	if _, err := Render(experiment, Config{ReleaseMS: 20, BoundaryBridgeMS: 20, BoundaryBridgeThreshold: 100}); err != nil {
		t.Fatal(err)
	}
	if len(experiment.BoundaryRepairDecisions) != 1 {
		t.Fatalf("repair decision count = %d, want 1", len(experiment.BoundaryRepairDecisions))
	}
	decision := experiment.BoundaryRepairDecisions[0]
	if decision.SelectedKind != "normal" && decision.SelectedKind != "phase-aligned-vowel-tail" {
		t.Fatalf("repair decision = %#v", decision)
	}
	if decision.Applied && len(experiment.BoundaryBridges) != 1 {
		t.Fatalf("applied decision has %d bridge records", len(experiment.BoundaryBridges))
	}
}
