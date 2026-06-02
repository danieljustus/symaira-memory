package extractor

import (
	"testing"
)

func TestPatternExtraction(t *testing.T) {
	pe := NewPatternExtractor()

	tests := []struct {
		input            string
		expectedFact     string
		expectedCategory string
	}{
		{
			input:            "I like TypeScript.",
			expectedFact:     "User prefers TypeScript",
			expectedCategory: "preference",
		},
		{
			input:            "Ich mag golang.",
			expectedFact:     "User prefers golang",
			expectedCategory: "preference",
		},
		{
			input:            "I'm working on the symaira-memory project.",
			expectedFact:     "User is building the symaira-memory project",
			expectedCategory: "project",
		},
		{
			input:            "Ich bin ein Senior Entwickler.",
			expectedFact:     "User is a Senior Entwickler",
			expectedCategory: "identity",
		},
	}

	for _, tt := range tests {
		facts := pe.ExtractFacts(tt.input)
		if len(facts) == 0 {
			t.Errorf("input '%s' failed to extract any facts", tt.input)
			continue
		}

		matched := false
		for _, f := range facts {
			if f.Content == tt.expectedFact && f.Category == tt.expectedCategory {
				matched = true
				break
			}
		}

		if !matched {
			t.Errorf("expected fact '%s' (%s), got %+v", tt.expectedFact, tt.expectedCategory, facts)
		}
	}
}

func TestKeywordFilterFallback(t *testing.T) {
	pe := NewPatternExtractor()
	
	// Sentence with no explicit regex trigger, but high-value keywords
	input := "Wir planen das symaira projekt aufzubauen."
	facts := pe.ExtractFacts(input)
	
	if len(facts) == 0 {
		t.Fatalf("expected keyword fallback fact to be extracted")
	}
	
	if facts[0].Category != "general" {
		t.Errorf("expected category 'general', got '%s'", facts[0].Category)
	}
	
	if !stringsContains(facts[0].Content, "symaira projekt") {
		t.Errorf("expected content to contain 'symaira projekt', got '%s'", facts[0].Content)
	}
}

func stringsContains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
