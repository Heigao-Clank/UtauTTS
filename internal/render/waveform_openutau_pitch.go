package render

import "math"

// smoothAndLimitPitchCurveは入力のピッチカーブを書き換えずに、ガウス平滑化と
// フレーム毎のスルーレート制限を適用する。
func smoothAndLimitPitchCurve(curve *PitchCurve, sigmaMS, maxCentsPer10MS float64) *PitchCurve {
	if curve == nil || curve.FrameMS <= 0 || len(curve.Cents) == 0 {
		return curve
	}
	result := &PitchCurve{FrameMS: curve.FrameMS, Cents: append([]float64(nil), curve.Cents...)}
	if sigmaMS > 0 {
		sigma := sigmaMS / curve.FrameMS
		radius := max(1, int(math.Ceil(3*sigma)))
		weights := make([]float64, 2*radius+1)
		for offset := -radius; offset <= radius; offset++ {
			weights[offset+radius] = math.Exp(-0.5 * math.Pow(float64(offset)/sigma, 2))
		}
		smoothed := make([]float64, len(result.Cents))
		for position := range result.Cents {
			sum, weightSum := 0.0, 0.0
			for offset := -radius; offset <= radius; offset++ {
				source := max(0, min(len(result.Cents)-1, position+offset))
				weight := weights[offset+radius]
				sum += result.Cents[source] * weight
				weightSum += weight
			}
			smoothed[position] = sum / weightSum
		}
		result.Cents = smoothed
	}
	maximumStep := maxCentsPer10MS * curve.FrameMS / 10
	if maximumStep > 0 {
		for position := 1; position < len(result.Cents); position++ {
			result.Cents[position] = math.Max(result.Cents[position-1]-maximumStep, math.Min(result.Cents[position-1]+maximumStep, result.Cents[position]))
		}
		for position := len(result.Cents) - 2; position >= 0; position-- {
			result.Cents[position] = math.Max(result.Cents[position+1]-maximumStep, math.Min(result.Cents[position+1]+maximumStep, result.Cents[position]))
		}
	}
	return result
}

// ConstrainPitchCurveは、学習済み輪郭レンダラーが使うのと同じ保守的な平滑化と
// スルーレート制限を、外部で編集された輪郭に適用する。renderに置くことで、手動の
// GUI/CLI編集も学習済み輪郭と同じアーティファクト対策に従う。
func ConstrainPitchCurve(curve *PitchCurve, sigmaMS, maxCentsPer10MS float64) *PitchCurve {
	return smoothAndLimitPitchCurve(curve, sigmaMS, maxCentsPer10MS)
}
