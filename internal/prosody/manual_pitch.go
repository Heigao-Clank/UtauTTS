package prosody

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"utautts/internal/frontend"
)

const ManualPitchVersion = 1

// ManualPitchFile stores edits made to the pitch of individual morae.
// Cents are relative offsets from the renderer/model baseline. Keeping the
// edits relative means a phrase can still use a learned contour and the user
// only needs to correct the parts that sound wrong.
type ManualPitchFile struct {
	Version int                `json:"version"`
	Reading string             `json:"reading,omitempty"`
	Mode    string             `json:"mode,omitempty"`
	Points  []ManualPitchPoint `json:"points"`
}

type ManualPitchPoint struct {
	Position int     `json:"position"`
	Mora     string  `json:"mora,omitempty"`
	Cents    float64 `json:"cents"`
}

// LoadManualPitch reads and validates a hand-edited mora pitch file.
func LoadManualPitch(path string) (*ManualPitchFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file ManualPitchFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("decode manual pitch file: %w", err)
	}
	if err := file.Validate(); err != nil {
		return nil, err
	}
	return &file, nil
}

func (file *ManualPitchFile) Validate() error {
	if file == nil {
		return nil
	}
	if file.Version == 0 {
		return fmt.Errorf("manual pitch version is required")
	}
	if file.Version != ManualPitchVersion {
		return fmt.Errorf("unsupported manual pitch version %d", file.Version)
	}
	if file.Mode == "" {
		file.Mode = "offset"
	}
	if file.Mode != "offset" && file.Mode != "replace" {
		return fmt.Errorf("unsupported manual pitch mode %q", file.Mode)
	}
	for index, point := range file.Points {
		if point.Position < 0 {
			return fmt.Errorf("manual pitch point %d has negative position", index)
		}
		if math.IsNaN(point.Cents) || math.IsInf(point.Cents, 0) || math.Abs(point.Cents) > 1200 {
			return fmt.Errorf("manual pitch point %d is outside +/-1200 cents", index)
		}
	}
	return nil
}

// Curve converts mora-centered edits into a smooth 10 ms frame contour.
// Missing positions are zero, so a sparse file can correct only selected
// morae. Pauses are represented by their timing but do not receive a point.
func (file *ManualPitchFile) Curve(morae []frontend.Mora, timings []MoraTiming, durationMS float64) (*PitchContour, error) {
	if file == nil {
		return nil, nil
	}
	if len(timings) != len(morae) {
		return nil, fmt.Errorf("manual pitch timing count %d does not match mora count %d", len(timings), len(morae))
	}
	if durationMS <= 0 {
		return nil, fmt.Errorf("manual pitch duration must be positive")
	}
	values := make([]float64, len(morae))
	set := make([]bool, len(morae))
	for _, point := range file.Points {
		if point.Position < 0 || point.Position >= len(morae) {
			return nil, fmt.Errorf("manual pitch point position %d is outside mora count %d", point.Position, len(morae))
		}
		if morae[point.Position].Pause {
			return nil, fmt.Errorf("manual pitch point position %d refers to a pause", point.Position)
		}
		if set[point.Position] {
			return nil, fmt.Errorf("manual pitch point position %d is duplicated", point.Position)
		}
		if point.Mora != "" && point.Mora != morae[point.Position].Text {
			return nil, fmt.Errorf("manual pitch point %d mora %q does not match %q", point.Position, point.Mora, morae[point.Position].Text)
		}
		values[point.Position] = point.Cents
		set[point.Position] = true
	}
	centers := make([]float64, 0, len(morae))
	centerValues := make([]float64, 0, len(morae))
	for index, mora := range morae {
		if mora.Pause {
			continue
		}
		centers = append(centers, timings[index].StartMS+timings[index].DurationMS/2)
		centerValues = append(centerValues, values[index])
	}
	frameMS := 10.0
	length := max(2, int(math.Ceil(durationMS/frameMS))+1)
	curve := make([]float64, length)
	if len(centers) == 0 {
		return &PitchContour{FrameMS: frameMS, Cents: curve}, nil
	}
	for frame := range curve {
		timeMS := float64(frame) * frameMS
		curve[frame] = interpolateManualPoint(centers, centerValues, timeMS)
	}
	return &PitchContour{FrameMS: frameMS, Cents: curve}, nil
}

func interpolateManualPoint(times, values []float64, timeMS float64) float64 {
	if timeMS <= times[0] {
		return values[0]
	}
	for index := 1; index < len(times); index++ {
		if timeMS <= times[index] {
			span := times[index] - times[index-1]
			if span <= 0 {
				return values[index]
			}
			progress := (timeMS - times[index-1]) / span
			return values[index-1]*(1-progress) + values[index]*progress
		}
	}
	return values[len(values)-1]
}
