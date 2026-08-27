package render

import "math"

// toneMIDI parses scientific pitch notation ("C4", "F#3", "Bb2") into a MIDI
// note number. ok is false for empty or unparsable tones.
func toneMIDI(tone string) (float64, bool) {
	if len(tone) < 2 {
		return 0, false
	}
	semitones := map[byte]float64{'C': 0, 'D': 2, 'E': 4, 'F': 5, 'G': 7, 'A': 9, 'B': 11}
	upper := tone
	if upper[0] >= 'a' && upper[0] <= 'g' {
		upper = string(upper[0]-'a'+'A') + upper[1:]
	}
	semitone, ok := semitones[upper[0]]
	if !ok {
		return 0, false
	}
	index := 1
	if index < len(upper) && (upper[index] == '#' || upper[index] == 'b') {
		if upper[index] == '#' {
			semitone++
		} else {
			semitone--
		}
		index++
	}
	octave := 0
	for ; index < len(upper); index++ {
		digit := int(upper[index] - '0')
		if digit < 0 || digit > 9 {
			return 0, false
		}
		octave = octave*10 + digit
	}
	return (float64(octave)+1)*12 + semitone, true
}

// basePitchFactor returns the multiplicative shift that moves the voicebank's
// natural register (the median F0 of the source recordings) onto the
// requested base tone, so the GUI's 基準音高 setting changes the output
// register the same way the USTX export promises OpenUtau (notes at the
// requested tone). Returns 1 when there is no usable tone or reference, so
// the legacy behavior is preserved.
func basePitchFactor(referenceHz float64, tone string) float64 {
	if referenceHz <= 0 {
		return 1
	}
	midi, ok := toneMIDI(tone)
	if !ok {
		return 1
	}
	return 440 * math.Pow(2, (midi-69)/12) / referenceHz
}
