//go:build windows

package main

import (
	"testing"

	"utautts/internal/audio"
	"utautts/internal/prosody"
)

func TestEditorStateStartsWithOneUtterance(t *testing.T) {
	editor := newEditorState("  こんにちは  ")
	if len(editor.Utterances) != 1 || editor.Selected != 0 {
		t.Fatalf("initial editor state = %#v", editor)
	}
	selected := editor.selected()
	if selected == nil || selected.Text != "  こんにちは  " || selected.Renderer != defaultRendererBackend {
		t.Fatalf("initial utterance = %#v", selected)
	}
}

func TestEditorStateAddSelectAndRemove(t *testing.T) {
	editor := newEditorState("one")
	if got := editor.add("two"); got != 1 || editor.selected().Text != "two" {
		t.Fatalf("add selected index = %d, state = %#v", got, editor)
	}
	if !editor.selectIndex(0) || editor.selected().Text != "one" {
		t.Fatalf("select did not move to first utterance: %#v", editor)
	}
	if !editor.remove(0) || editor.Selected != 0 || editor.selected().Text != "two" {
		t.Fatalf("remove did not preserve selection: %#v", editor)
	}
	if editor.remove(0) {
		t.Fatal("last utterance was removed")
	}
}

func TestUtteranceTextChangeClearsReadingBoundManualPitch(t *testing.T) {
	editor := newEditorState("old")
	selected := editor.selected()
	selected.Reading = "おーるど"
	selected.ManualPitch = &prosody.ManualPitchFile{Version: prosody.ManualPitchVersion, Reading: "おーるど"}
	selected.setText("new")
	if selected.Reading != "" || selected.ManualPitch != nil {
		t.Fatalf("text change retained stale pitch state: %#v", selected)
	}
}

func TestReorderUtterancePreservesSelectedItem(t *testing.T) {
	editor = &editorState{Utterances: []utteranceState{
		{Text: "one"}, {Text: "two"}, {Text: "three"},
	}, Selected: 1}
	t.Cleanup(func() { editor = nil })
	reorderUtterance(1, 0)
	if editor.Utterances[0].Text != "two" || editor.Utterances[1].Text != "one" || editor.Selected != 0 {
		t.Fatalf("reordered state = %#v", editor)
	}
	reorderUtterance(0, 2)
	if editor.Utterances[2].Text != "two" || editor.Selected != 2 {
		t.Fatalf("second reorder state = %#v", editor)
	}
}

func TestAppendPCMConcatenatesCompatibleAudio(t *testing.T) {
	left := &audio.PCM{SampleRate: 44100, Channels: 1, Data: []int16{1, 2}}
	right := &audio.PCM{SampleRate: 44100, Channels: 1, Data: []int16{3, 4}}
	got, err := appendPCM(left, right)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Data) != 4 || got.Data[2] != 3 || got.Data[3] != 4 {
		t.Fatalf("concatenated data = %#v", got.Data)
	}
}

func TestAppendPCMRejectsIncompatibleAudio(t *testing.T) {
	_, err := appendPCM(&audio.PCM{SampleRate: 44100, Channels: 1}, &audio.PCM{SampleRate: 48000, Channels: 1})
	if err == nil {
		t.Fatal("incompatible sample rates were accepted")
	}
}
