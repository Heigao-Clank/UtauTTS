package evaluation

import (
	"errors"
	"math"

	"utautts/internal/acoustic"
	"utautts/internal/audio"
	"utautts/internal/plan"
)

const Version = 3

type Report struct {
	Version          int      `json:"version"`
	SampleRate       int      `json:"sample_rate"`
	ConnectedCount   int      `json:"connected_count"`
	MeanClick        float64  `json:"mean_click"`
	MaxClick         float64  `json:"max_click"`
	MeanPeakClick    float64  `json:"mean_peak_click"`
	MaxPeakClick     float64  `json:"max_peak_click"`
	MeanDeltaRMS     float64  `json:"mean_delta_rms"`
	MeanRMSDeltaDB   float64  `json:"mean_rms_delta_db"`
	MeanSpectrumDB   float64  `json:"mean_spectrum_delta_db"`
	MeanF0DeltaCents float64  `json:"mean_f0_delta_cents,omitempty"`
	Boundaries       []Metric `json:"boundaries"`
}

type Metric struct {
	UnitIndex       int     `json:"unit_index"`
	Position        int     `json:"position"`
	TimeMS          float64 `json:"time_ms"`
	HandoffStartMS  float64 `json:"handoff_start_ms"`
	HandoffEndMS    float64 `json:"handoff_end_ms"`
	Connected       bool    `json:"connected"`
	Click           float64 `json:"click"`
	PeakClick       float64 `json:"peak_click"`
	DeltaRMS        float64 `json:"delta_rms"`
	RMSDeltaDB      float64 `json:"rms_delta_db"`
	SpectrumDeltaDB float64 `json:"spectrum_delta_db"`
	F0DeltaCents    float64 `json:"f0_delta_cents,omitempty"`
}

// Analyzeは各ユニット境界周辺の波形の不連続性を測定する。サマリ値は隣接ユニットのみを
// 含み、フレーズ開始部はポーズ後のアタック診断用にBoundariesに残す。
func Analyze(pcm *audio.PCM, synthesisPlan *plan.Plan) (*Report, error) {
	if pcm == nil || pcm.SampleRate <= 0 || pcm.Channels <= 0 || len(pcm.Data) == 0 {
		return nil, errors.New("empty audio")
	}
	if synthesisPlan == nil || len(synthesisPlan.Units) < 2 {
		return nil, errors.New("plan needs at least two units")
	}
	wave := acoustic.Mono(pcm)
	window := max(32, int(math.Round(float64(pcm.SampleRate)*0.02)))
	report := &Report{Version: Version, SampleRate: pcm.SampleRate}
	var clickSum, peakClickSum, deltaRMSSum, rmsSum, spectrumSum, f0Sum float64
	var f0Count int
	for i := 1; i < len(synthesisPlan.Units); i++ {
		unit := synthesisPlan.Units[i]
		handoffStartMS, handoffEndMS := handoffRange(unit)
		timeMS := (handoffStartMS + handoffEndMS) / 2
		frame := int(math.Round(timeMS * float64(pcm.SampleRate) / 1000))
		if frame <= 0 || frame >= len(wave) {
			continue
		}
		left := wave[max(0, frame-window):frame]
		right := wave[frame:min(len(wave), frame+window)]
		leftFeatures := acoustic.AnalyzeFrame(left, pcm.SampleRate, 20, false)
		rightFeatures := acoustic.AnalyzeFrame(right, pcm.SampleRate, 20, false)
		transitionStart := max(1, int(math.Floor(handoffStartMS*float64(pcm.SampleRate)/1000))-window/10)
		transitionEnd := min(len(wave), int(math.Ceil(handoffEndMS*float64(pcm.SampleRate)/1000))+window/10)
		peakClick, deltaRMS := derivativeMetrics(wave[transitionStart-1 : transitionEnd])
		metric := Metric{
			UnitIndex: i, Position: unit.Position, TimeMS: timeMS,
			HandoffStartMS: handoffStartMS, HandoffEndMS: handoffEndMS,
			Connected:       unit.Position == synthesisPlan.Units[i-1].Position+1,
			Click:           math.Abs(wave[frame] - wave[frame-1]),
			PeakClick:       peakClick,
			DeltaRMS:        deltaRMS,
			RMSDeltaDB:      math.Abs(leftFeatures.RMSDB - rightFeatures.RMSDB),
			SpectrumDeltaDB: acoustic.MeanSpectrumDelta(leftFeatures.SpectrumDB, rightFeatures.SpectrumDB),
		}
		leftF0 := leftFeatures.F0Hz
		rightF0 := rightFeatures.F0Hz
		if leftF0 > 0 && rightF0 > 0 {
			metric.F0DeltaCents = math.Abs(1200 * math.Log2(rightF0/leftF0))
		}
		report.Boundaries = append(report.Boundaries, metric)
		if !metric.Connected {
			continue
		}
		report.ConnectedCount++
		clickSum += metric.Click
		peakClickSum += metric.PeakClick
		deltaRMSSum += metric.DeltaRMS
		rmsSum += metric.RMSDeltaDB
		spectrumSum += metric.SpectrumDeltaDB
		report.MaxClick = max(report.MaxClick, metric.Click)
		report.MaxPeakClick = max(report.MaxPeakClick, metric.PeakClick)
		if metric.F0DeltaCents > 0 {
			f0Sum += metric.F0DeltaCents
			f0Count++
		}
	}
	if report.ConnectedCount == 0 {
		return nil, errors.New("plan has no measurable connected boundaries")
	}
	n := float64(report.ConnectedCount)
	report.MeanClick = clickSum / n
	report.MeanPeakClick = peakClickSum / n
	report.MeanDeltaRMS = deltaRMSSum / n
	report.MeanRMSDeltaDB = rmsSum / n
	report.MeanSpectrumDB = spectrumSum / n
	if f0Count > 0 {
		report.MeanF0DeltaCents = f0Sum / float64(f0Count)
	}
	return report, nil
}

func derivativeMetrics(wave []float64) (peak, rms float64) {
	if len(wave) < 2 {
		return 0, 0
	}
	energy := 0.0
	for index := 1; index < len(wave); index++ {
		delta := wave[index] - wave[index-1]
		peak = max(peak, math.Abs(delta))
		energy += delta * delta
	}
	return peak, math.Sqrt(energy / float64(len(wave)-1))
}

func handoffRange(unit plan.Unit) (float64, float64) {
	preutterance := unit.PreutteranceMS
	overlap := unit.OverlapMS
	// Renderは正規化済みタイミングをこれらのフィールドに記録する。TimingScaleは、
	// レンダリングされたゼロ値と、監査フィールドのない古い未レンダリングのPlanを区別する。
	if unit.TimingScale > 0 {
		preutterance = unit.EffectivePreutteranceMS
		overlap = unit.EffectiveOverlapMS
	}
	preutterance = math.Max(0, preutterance)
	overlap = math.Min(overlap, preutterance)
	start := unit.NoteStartMS - preutterance
	end := unit.NoteStartMS - overlap
	if end < start {
		return end, start
	}
	return start, end
}
