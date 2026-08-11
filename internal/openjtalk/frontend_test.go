package openjtalk

import "testing"

func TestAnalyzeRejectsEmptyTextBeforeResolvingRuntime(t *testing.T) {
	if _, err := Analyze("  ", Config{}); err == nil {
		t.Fatal("empty text was accepted")
	}
}

func TestResolveHelperRejectsMissingExplicitPath(t *testing.T) {
	if _, err := resolveHelper(t.TempDir()); err == nil {
		t.Fatal("directory was accepted as helper executable")
	}
}
