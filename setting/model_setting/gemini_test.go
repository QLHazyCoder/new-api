package model_setting

import "testing"

func TestIsGeminiModelSupportImagineAcceptsResolutionSuffixedModels(t *testing.T) {
	for _, modelName := range []string{
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-1k",
		"gemini-3.1-flash-image-2K",
		"gemini-3.1-flash-image-4k",
	} {
		if !IsGeminiModelSupportImagine(modelName) {
			t.Fatalf("expected %q to be recognized as a Gemini image model", modelName)
		}
	}
}

func TestIsGeminiModelSupportImagineRejectsUnknownSuffix(t *testing.T) {
	if IsGeminiModelSupportImagine("gemini-3.1-flash-image-8k") {
		t.Fatal("expected unsupported resolution suffix to be rejected")
	}
}
