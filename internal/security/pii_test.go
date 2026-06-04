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
			input:    "Send payment details of card 4111 1111 1111 1111 to help@symaira.com",
			expected: "Send payment details of card [REDACTED_CARD_NUMBER] to [REDACTED_EMAIL]",
		},
		{
			input:    "GitHub auth token is gho_abcdefabcdefabcdefabcdefabcdefabcdef",
			expected: "GitHub auth token is [REDACTED_API_KEY]",
		},
		{
			input:    "AWS key is AKIA1234567890ABCDEF and Slack is xoxb-123456789012-abcdef",
			expected: "AWS key is [REDACTED_API_KEY] and Slack is [REDACTED_API_KEY]",
		},
		{
			input:    "Stripe live key sk_live_12345abcde12345abcde12345",
			expected: "Stripe live key [REDACTED_API_KEY]",
		},
		{
			input:    "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
			expected: "[REDACTED_API_KEY]\nsecret\n-----END PRIVATE KEY-----",
		},
	}

	for _, tt := range tests {
		result := pg.Redact(tt.input)
		if result != tt.expected {
			t.Errorf("expected redacted text:\n  '%s'\ngot:\n  '%s'", tt.expected, result)
		}
	}
}
