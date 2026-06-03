package security

import (
	"testing"
)

func TestPIIGuardRedaction(t *testing.T) {
	pg := NewPIIGuard()

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "My OpenAI project key is sk-proj-12345abcde12345abcde12345abcde12345abcde12345 and email is test@domain.com",
			expected: "My OpenAI project key is [REDACTED_API_KEY] and email is [REDACTED_EMAIL]",
		},
		{
			input:    "Send payment details of card 4111 2222 3333 4444 to help@symaira.com",
			expected: "Send payment details of card [REDACTED_CARD_NUMBER] to [REDACTED_EMAIL]",
		},
		{
			input:    "GitHub auth token is gho_abcdefabcdefabcdefabcdefabcdefabcdef",
			expected: "GitHub auth token is [REDACTED_API_KEY]",
		},
	}

	for _, tt := range tests {
		result := pg.Redact(tt.input)
		if result != tt.expected {
			t.Errorf("expected redacted text:\n  '%s'\ngot:\n  '%s'", tt.expected, result)
		}
	}
}
