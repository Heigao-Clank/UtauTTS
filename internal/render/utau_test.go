package render

import "testing"

func TestMIDIToneName(t *testing.T) {
	if got := midiToneName(60); got != "C4" {
		t.Fatalf("midiToneName(60) = %q", got)
	}
	if got := midiToneName(69); got != "A4" {
		t.Fatalf("midiToneName(69) = %q", got)
	}
}

func TestEncodeInt12UsesUTAUDuplicateCompression(t *testing.T) {
	if got := encodeInt12([]int{0, 0, 0, 1, -1}); got != "AA#2#AB//" {
		t.Fatalf("encodeInt12 = %q", got)
	}
}
