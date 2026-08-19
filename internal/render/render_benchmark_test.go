package render

import (
	"fmt"
	"math"
	"path/filepath"
	"testing"

	"utautts/internal/audio"
	"utautts/internal/plan"
)

// BenchmarkWaveformRendererはデスクトップGUIが生成するのと同じ独立ユニットの
// ワークロードを実行する。CPUとGPUのbackendを開発している間はこれを維持し、
// 最適化の判断に同等の入力を使えるようにする。
func BenchmarkWaveformRenderer(b *testing.B) {
	benchmarkWaveformRenderer(b, "waveform")
}

func benchmarkWaveformRenderer(b *testing.B, backend string) {
	const (
		sampleRate = 44100
		unitCount  = 48
	)
	directory := b.TempDir()
	units := make([]plan.Unit, unitCount)
	for index := range units {
		path := filepath.Join(directory, fmt.Sprintf("unit-%02d.wav", index))
		data := make([]int16, sampleRate/2)
		frequency := 170.0 + float64(index%9)*13
		for frame := range data {
			envelope := math.Min(1, float64(frame)/400) * math.Min(1, float64(len(data)-frame)/800)
			data[frame] = int16(math.Round(9000 * envelope * math.Sin(2*math.Pi*frequency*float64(frame)/sampleRate)))
		}
		if err := audio.WriteWav(path, &audio.PCM{SampleRate: sampleRate, Channels: 1, Data: data}); err != nil {
			b.Fatal(err)
		}
		units[index] = plan.Unit{
			Position: index, Alias: fmt.Sprintf("u%d", index), Source: path,
			NoteStartMS: float64(index) * 120, DurationMS: 120,
			ConsonantMS: 70, PreutteranceMS: 45, OverlapMS: 12,
			PitchFactor: 1, EnergyFactor: 1,
		}
	}
	template := plan.Plan{DurationMS: unitCount * 120, Units: units}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		input := template
		input.Units = append([]plan.Unit(nil), template.Units...)
		if _, err := Render(&input, Config{Backend: backend, ReleaseMS: 20}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWSOLA(b *testing.B) {
	const sampleRate = 44100
	source := make([]float64, sampleRate/2)
	for frame := range source {
		source[frame] = 0.3 * math.Sin(2*math.Pi*220*float64(frame)/sampleRate)
	}
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = wsola(source, 8820, sampleRate)
	}
}

func BenchmarkWSOLAGPU(b *testing.B) {
	if err := gpuWaveformAvailable(); err != nil {
		b.Skip(err)
	}
	const sampleRate = 44100
	source := make([]float64, sampleRate/2)
	for frame := range source {
		source[frame] = 0.3 * math.Sin(2*math.Pi*220*float64(frame)/sampleRate)
	}
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, err := gpuWSOLA(source, 8820, sampleRate); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWSOLALong(b *testing.B) {
	benchmarkWSOLASize(b, false, 4*44100, 2*44100)
}

func BenchmarkWSOLALongGPU(b *testing.B) {
	if err := gpuWaveformAvailable(); err != nil {
		b.Skip(err)
	}
	benchmarkWSOLASize(b, true, 4*44100, 2*44100)
}

func benchmarkWSOLASize(b *testing.B, gpu bool, sourceFrames, targetFrames int) {
	const sampleRate = 44100
	source := make([]float64, sourceFrames)
	for frame := range source {
		source[frame] = 0.3 * math.Sin(2*math.Pi*220*float64(frame)/sampleRate)
	}
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		if gpu {
			if _, err := gpuWSOLA(source, targetFrames, sampleRate); err != nil {
				b.Fatal(err)
			}
		} else {
			_ = wsola(source, targetFrames, sampleRate)
		}
	}
}

func TestGPUWSOLAStaysCloseToCPU(t *testing.T) {
	if err := gpuWaveformAvailable(); err != nil {
		t.Skip(err)
	}
	for _, sampleRate := range []int{44100, 96000} {
		t.Run(fmt.Sprintf("%dHz", sampleRate), func(t *testing.T) {
			source := make([]float64, sampleRate/2)
			for frame := range source {
				time := float64(frame) / float64(sampleRate)
				source[frame] = 0.25*math.Sin(2*math.Pi*220*time) + 0.08*math.Sin(2*math.Pi*443*time)
			}
			cpu := wsola(source, sampleRate/5, sampleRate)
			gpu, err := gpuWSOLA(source, len(cpu), sampleRate)
			if err != nil {
				t.Fatal(err)
			}
			if len(gpu) != len(cpu) {
				t.Fatalf("GPU length=%d, want %d", len(gpu), len(cpu))
			}
			errorEnergy, signalEnergy := 0.0, 0.0
			for index := range cpu {
				delta := cpu[index] - gpu[index]
				errorEnergy += delta * delta
				signalEnergy += cpu[index] * cpu[index]
			}
			if relative := math.Sqrt(errorEnergy / math.Max(signalEnergy, 1e-12)); relative > 0.02 {
				t.Fatalf("GPU relative RMS error=%f, want <=0.02", relative)
			}
		})
	}
}
