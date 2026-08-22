package evaluation

import (
	"math"

	"utautts/internal/frontend"
)

type PitchSweepCase struct {
	ID           string    `json:"id"`
	Description  string    `json:"description"`
	Cents        []float64 `json:"cents"`
	PitchFactors []float64 `json:"pitch_factors"`
}

type FramePitchSweepCase struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	FrameMS     float64   `json:"frame_ms"`
	Cents       []float64 `json:"cents"`
}

// DeterministicFramePitchSweepはユニット内の丘状変化と段差を持つフレーム輪郭を生成する。
func DeterministicFramePitchSweep(durationMS, frameMS float64) []FramePitchSweepCase {
	if durationMS <= 0 || frameMS <= 0 {
		return nil
	}
	count := max(2, int(math.Ceil(durationMS/frameMS))+1)
	constant := func(value float64) []float64 {
		result := make([]float64, count)
		for index := range result {
			result[index] = value
		}
		return result
	}
	cases := []FramePitchSweepCase{{ID: "flat", Description: "0 cents", FrameMS: frameMS, Cents: constant(0)}}
	for _, value := range []int{25, 50, 100, -25, -50, -100} {
		id := "plus"
		if value < 0 {
			id = "minus"
		}
		magnitude := value
		if magnitude < 0 {
			magnitude = -magnitude
		}
		cases = append(cases, FramePitchSweepCase{
			ID: id + itoaSmall(magnitude), Description: itoaSigned(value) + " cents constant shift",
			FrameMS: frameMS, Cents: constant(float64(value)),
		})
	}
	hill := make([]float64, count)
	step := make([]float64, count)
	for index := range hill {
		progress := float64(index) / float64(count-1)
		hill[index] = 100 * math.Sin(math.Pi*progress)
		if progress >= 0.5 {
			step[index] = 100
		}
	}
	return append(cases,
		FramePitchSweepCase{ID: "hill100", Description: "slow single hill peaking at +100 cents", FrameMS: frameMS, Cents: hill},
		FramePitchSweepCase{ID: "step100", Description: "abrupt step from 0 to +100 cents", FrameMS: frameMS, Cents: step},
	)
}

// DeterministicPitchSweepは一定値・丘状・段差のレンダラー診断輪郭を返す。
func DeterministicPitchSweep(morae []frontend.Mora) []PitchSweepCase {
	voiced := make([]int, 0, len(morae))
	for index, mora := range morae {
		if !mora.Pause {
			voiced = append(voiced, index)
		}
	}
	makeCase := func(id, description string, cents []float64) PitchSweepCase {
		factors := make([]float64, len(cents))
		for index, value := range cents {
			factors[index] = math.Pow(2, value/1200)
		}
		return PitchSweepCase{ID: id, Description: description, Cents: cents, PitchFactors: factors}
	}
	constant := func(value float64) []float64 {
		result := make([]float64, len(morae))
		for _, index := range voiced {
			result[index] = value
		}
		return result
	}

	cases := []PitchSweepCase{makeCase("flat", "0 cents", constant(0))}
	for _, value := range []int{25, 50, 100, -25, -50, -100} {
		id := "plus"
		if value < 0 {
			id = "minus"
		}
		magnitude := value
		if magnitude < 0 {
			magnitude = -magnitude
		}
		cases = append(cases, makeCase(
			id+itoaSmall(magnitude),
			itoaSigned(value)+" cents constant shift",
			constant(float64(value)),
		))
	}

	hill := make([]float64, len(morae))
	step := make([]float64, len(morae))
	for position, index := range voiced {
		if len(voiced) == 1 {
			hill[index] = 100
		} else {
			hill[index] = 100 * math.Sin(math.Pi*float64(position)/float64(len(voiced)-1))
		}
		if position >= len(voiced)/2 {
			step[index] = 100
		}
	}
	cases = append(cases,
		makeCase("hill100", "slow single hill peaking at +100 cents", hill),
		makeCase("step100", "abrupt step from 0 to +100 cents", step),
	)
	return cases
}

func itoaSmall(value int) string {
	switch value {
	case 25:
		return "25"
	case 50:
		return "50"
	case 100:
		return "100"
	default:
		return "0"
	}
}

func itoaSigned(value int) string {
	if value > 0 {
		return "+" + itoaSmall(value)
	}
	return "-" + itoaSmall(-value)
}
